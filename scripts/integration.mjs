// 真正启动两个独立进程，并使用临时 SQLite 数据库完成 HTTP 验收。
import assert from 'node:assert/strict'
import { spawn, spawnSync } from 'node:child_process'
import { randomBytes } from 'node:crypto'
import { mkdirSync, mkdtempSync, writeFileSync } from 'node:fs'
import { createServer } from 'node:net'
import { dirname, join, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

const root = resolve(dirname(fileURLToPath(import.meta.url)), '..')
mkdirSync(join(root, '.runtime'), { recursive: true })
const workspace = mkdtempSync(join(root, '.runtime', 'integration-'))
const suffix = process.platform === 'win32' ? '.exe' : ''
const controllerBin = join(workspace, 'controller' + suffix)
const demoBin = join(workspace, 'demo-service' + suffix)
const outputs = []
const children = new Set()
const results = []
const password = randomBytes(24).toString('base64url')
const registrationKey = randomBytes(32).toString('hex')
const bootstrapToken = randomBytes(32).toString('hex')
const businessToken = randomBytes(48).toString('base64url')
let cookie = ''
let csrf = ''

async function freePort() {
  const server = createServer()
  await new Promise((resolve, reject) => { server.once('error', reject); server.listen(0, '127.0.0.1', resolve) })
  const port = server.address().port
  await new Promise(resolve => server.close(resolve))
  return port
}
const controllerPort = await freePort()
let demoPort = await freePort()
while (demoPort === controllerPort) demoPort = await freePort()
const origin = `http://127.0.0.1:${controllerPort}`
const demoOrigin = `http://127.0.0.1:${demoPort}`
const configPath = join(workspace, 'controller.json')
const accessPath = join(workspace, 'service-access.json')
writeFileSync(accessPath, JSON.stringify({ 'workstation-demo': {
  origin: demoOrigin, bearer_token: businessToken,
  endpoints: [{ path: '/api/tasks', methods: ['GET'] }, { path: '/api/tasks/archive', methods: ['POST'] }, { path: '/api/preferences', methods: ['GET', 'PUT'] }, { path: '/api/overview', methods: ['GET'] }],
} }), { mode: 0o600 })
writeFileSync(configPath, JSON.stringify({ listen: `127.0.0.1:${controllerPort}`, database_path: join(workspace, 'controller.db'), static_dir: join(root, 'web', 'dist'), public_origin: origin, bootstrap_token: bootstrapToken, registration_key: registrationKey, allowed_service_origins: [demoOrigin], session_hours: 1, secure_cookie: false, check_interval_seconds: 30, service_access_file: accessPath }), { mode: 0o600 })

function build(target, output) {
  const child = spawnSync('go', ['build', '-o', output, target], { cwd: root, encoding: 'utf8', windowsHide: true })
  if (child.status !== 0) throw new Error(`编译失败 ${target}\n${child.stderr || child.error}`)
}
function start(binary, args, env = {}) {
  const cleanEnv = Object.fromEntries(Object.entries(process.env).filter(([key]) => !key.startsWith('CONTROLLER_') && !key.startsWith('DEMO_')))
  const child = spawn(binary, args, { cwd: root, env: { ...cleanEnv, ...env }, stdio: ['ignore', 'pipe', 'pipe'], windowsHide: true })
  children.add(child)
  child.stdout.on('data', data => outputs.push(data.toString()))
  child.stderr.on('data', data => outputs.push(data.toString()))
  child.on('error', err => outputs.push(`进程错误：${err.message}`))
  child.on('exit', () => children.delete(child))
  return child
}
async function stop(child) {
  if (!child || child.exitCode !== null || !children.has(child)) return
  await new Promise(resolve => { child.once('exit', resolve); child.kill(); setTimeout(resolve, 3000).unref() })
}
async function waitFor(predicate, label) {
  for (let i = 0; i < 100; i++) {
    try { if (await predicate()) return } catch { /* 启动或状态更新过程中短暂失败可重试。 */ }
    await new Promise(resolve => setTimeout(resolve, 150))
  }
  throw new Error(`等待超时：${label}`)
}
async function request(path, { method = 'GET', body, auth = true, token, csrfHeader = true, raw } = {}) {
  const headers = { Origin: origin }
  if (auth && cookie) headers.Cookie = cookie
  if (auth && csrfHeader && csrf && method !== 'GET') headers['X-CSRF-Token'] = csrf
  if (token) headers.Authorization = `Bearer ${token}`
  if (body !== undefined || raw !== undefined) headers['Content-Type'] = 'application/json'
  const res = await fetch(origin + '/api/v1' + path, { method, headers, body: raw ?? (body === undefined ? undefined : JSON.stringify(body)), signal: AbortSignal.timeout(10000), redirect: 'error' })
  const text = await res.text()
  let data
  try { data = text ? JSON.parse(text) : null } catch { throw new Error(`响应不是JSON：${path} (${res.status})`) }
  return { status: res.status, data, headers: res.headers }
}
async function pass(name, fn) { await fn(); results.push(name); console.log(`通过：${name}`) }
function ok(response, status = 200) { assert.equal(response.status, status, JSON.stringify(response.data)); return response.data }
const servicePath = '/services/workstation-demo'
let controller
let demo
let payload
try {
  console.log('编译中心控制器和独立测试服务……')
  build('./cmd/controller', controllerBin)
  build('./cmd/demo-service', demoBin)
  controller = start(controllerBin, ['-config', configPath])
  await waitFor(async () => (await fetch(origin + '/healthz')).ok, '中心启动')
  await pass('管理接口拒绝未登录访问', async () => { assert.equal((await request('/services', { auth: false })).status, 401) })
  await pass('初始化受密钥保护且只能进行一次', async () => {
    assert.equal(ok(await request('/auth/status', { auth: false })).initialized, false)
    assert.equal((await request('/auth/setup', { method: 'POST', auth: false, body: { username: 'integration-admin', password, bootstrap_token: 'invalid-token' } })).status, 403)
    ok(await request('/auth/setup', { method: 'POST', auth: false, body: { username: 'integration-admin', password, bootstrap_token: bootstrapToken } }), 201)
    assert.equal(ok(await request('/auth/status', { auth: false })).initialized, true)
    assert.equal((await request('/auth/setup', { method: 'POST', auth: false, body: { username: 'other-admin', password, bootstrap_token: bootstrapToken } })).status, 409)
  })
  await pass('登录失败处理与安全会话', async () => {
    assert.equal((await request('/auth/login', { method: 'POST', auth: false, body: { username: 'integration-admin', password: 'wrong-password' } })).status, 401)
    const res = await request('/auth/login', { method: 'POST', auth: false, body: { username: 'integration-admin', password } })
    csrf = ok(res).csrf_token
    assert.ok(csrf)
    const header = res.headers.get('set-cookie')
    assert.match(header, /HttpOnly/i); assert.match(header, /SameSite=Strict/i)
    cookie = header.split(';')[0]
    assert.equal(ok(await request('/auth/me')).user.username, 'integration-admin')
  })
  await pass('写操作校验CSRF', async () => { assert.equal((await request('/settings', { method: 'PATCH', csrfHeader: false, body: { site_name: '不应写入', check_interval_seconds: 30 } })).status, 403) })
  demo = start(demoBin, ['-listen', `127.0.0.1:${demoPort}`, '-base-url', demoOrigin, '-controller', origin], { DEMO_REGISTRATION_KEY: registrationKey, DEMO_API_TOKEN: businessToken })
  await waitFor(async () => (await request(servicePath)).status === 200, '演示服务主动注册')
  await pass('主动发现与默认待启用、三类定义持久化', async () => {
    const service = ok(await request(servicePath)).service
    assert.equal(service.activation, 'pending')
    assert.deepEqual(service.definition.pages.map(page => page.type).sort(), ['detail', 'form', 'table'])
    payload = Object.fromEntries(['protocol_version', 'id', 'name', 'version', 'description', 'base_url', 'health_path', 'definition'].map(key => [key, service[key]]))
    assert.equal(ok(await request('/dashboard')).pending, 1)
  })
  await pass('启用、健康检查与重复登记保持管理员选择', async () => {
    assert.equal(ok(await request(servicePath, { method: 'PATCH', body: { activation: 'enabled' } })).service.activation, 'enabled')
    assert.equal(ok(await request(servicePath + '/check', { method: 'POST' })).service.online, 'online')
    payload.version = '1.0.1'
    const updated = ok(await request('/registry/register', { method: 'POST', auth: false, token: registrationKey, body: payload })).service
    assert.equal(updated.activation, 'enabled'); assert.equal(updated.version, '1.0.1')
    assert.equal(ok(await request('/services')).items.length, 1)
    assert.equal(ok(await request('/services?q=workstation&activation=enabled&online=online')).items.length, 1)
  })
  await pass('注册认证、协议版本、地址白名单和页面注入校验', async () => {
    assert.equal((await request('/registry/register', { method: 'POST', auth: false, token: 'wrong', body: payload })).status, 401)
    for (const change of [{ protocol_version: 99 }, { base_url: 'http://169.254.169.254' }, { base_url: 'http://127.0.0.1:1' }, { health_path: '//evil.example' }, { definition: { ...payload.definition, pages: [{ id: 'bad', title: '非法模板', type: 'script', template: '<script>alert(1)</script>' }] } }]) {
      const res = await request('/registry/register', { method: 'POST', auth: false, token: registrationKey, body: { ...payload, ...change } })
      assert.ok(res.status >= 400 && res.status < 500, `应拒绝非法注册：${res.status}`)
    }
    const invalid = await request('/registry/register', { method: 'POST', auth: false, token: registrationKey, raw: '{"id":"workstation-demo",' })
    assert.ok(invalid.status >= 400)
    const oversized = await request('/registry/register', { method: 'POST', auth: false, token: registrationKey, body: { ...payload, description: 'x'.repeat(300000) } })
    assert.ok(oversized.status >= 400 && oversized.status < 500)
    const old = ok(await request(servicePath))
    assert.equal(old.service.definition.pages.length, 3)
    assert.ok(old.service.last_registration_error)
    assert.ok(old.events.some(event => !event.success))
    ok(await request('/registry/register', { method: 'POST', auth: false, token: registrationKey, body: payload }))
  })
  await pass('受限网关业务调用与独立服务鉴权', async () => {
    const invoke = (page, input) => request(servicePath + '/pages/' + page + '/invoke', { method: 'POST', body: input })
    assert.equal((await fetch(demoOrigin + '/api/tasks')).status, 401)
    const list = ok(await invoke('tasks', { operation: 'source', query: { page: 1, page_size: 2 } }))
    assert.equal(list.items.length, 2); assert.equal(list.total, 3)
    ok(await invoke('tasks', { operation: 'action', action_id: 'archive', body: { id: 1 } }))
    ok(await invoke('preferences', { operation: 'submit', body: { owner: '集成测试', refresh_seconds: 60, theme: 'dark', notifications: false } }))
    assert.equal(ok(await invoke('preferences', { operation: 'source' })).notifications, false)
    assert.equal(ok(await invoke('overview', { operation: 'source' })).task_count, 3)
    assert.equal((await request(servicePath + '/pages/tasks/invoke', { method: 'POST', csrfHeader: false, body: { operation: 'source' } })).status, 403)
    assert.equal((await invoke('tasks', { operation: 'source', url: 'http://169.254.169.254/' })).status, 400)
    assert.equal((await invoke('tasks', { operation: 'action', action_id: 'unknown', body: { id: 1 } })).status, 400)
    assert.equal((await request('/proxy?url=' + encodeURIComponent(demoOrigin))).status, 404)
  })
  await stop(demo)
  await pass('服务停止后中心仍可用且启用状态不变', async () => {
    const service = ok(await request(servicePath + '/check', { method: 'POST' })).service
    assert.equal(service.online, 'offline'); assert.equal(service.activation, 'enabled')
    assert.equal(ok(await request('/dashboard')).offline, 1)
    assert.ok(ok(await request(servicePath)).service.last_error)
  })
  demo = start(demoBin, ['-listen', `127.0.0.1:${demoPort}`, '-base-url', demoOrigin, '-controller', origin], { DEMO_REGISTRATION_KEY: registrationKey, DEMO_API_TOKEN: businessToken })
  await waitFor(async () => (await fetch(demoOrigin + '/healthz')).ok, '演示服务恢复')
  await pass('服务恢复后检测上线且无需重新启用', async () => {
    const service = ok(await request(servicePath + '/check', { method: 'POST' })).service
    assert.equal(service.online, 'online'); assert.equal(service.activation, 'enabled')
  })
  await pass('已禁用服务重新注册不得自动启用', async () => {
    ok(await request(servicePath, { method: 'PATCH', body: { activation: 'disabled' } }))
    assert.equal(ok(await request('/registry/register', { method: 'POST', auth: false, token: registrationKey, body: payload })).service.activation, 'disabled')
  })
  await stop(demo)
  await pass('离线服务可禁用和删除，删除只影响中心记录', async () => {
    ok(await request(servicePath + '/check', { method: 'POST' }))
    assert.equal(ok(await request(servicePath, { method: 'PATCH', body: { activation: 'disabled' } })).service.activation, 'disabled')
    ok(await request(servicePath, { method: 'DELETE' }))
    assert.equal((await request(servicePath)).status, 404)
    assert.equal(ok(await request('/services')).items.length, 0)
    assert.equal(ok(await request('/registry/register', { method: 'POST', auth: false, token: registrationKey, body: payload })).service.activation, 'pending')
  })
  await pass('系统设置和审计来自真实数据库且不回传秘密', async () => {
    const settings = ok(await request('/settings', { method: 'PATCH', body: { site_name: '端到端验收工作站', check_interval_seconds: 15 } }))
    assert.equal(settings.site_name, '端到端验收工作站')
    assert.ok(!JSON.stringify(settings).includes(registrationKey)); assert.ok(!JSON.stringify(settings).includes(bootstrapToken))
    const audit = ok(await request('/audit?page=1&page_size=100'))
    assert.ok(audit.total >= 8); assert.ok(audit.items.some(item => item.action.includes('delete')))
    assert.ok(!JSON.stringify(audit).includes(password))
  })
  await stop(controller)
  controller = start(controllerBin, ['-config', configPath])
  await waitFor(async () => (await fetch(origin + '/healthz')).ok, '中心重启')
  await pass('中心重启后SQLite登记、管理员、会话和设置保留', async () => {
    assert.equal(ok(await request('/auth/status')).initialized, true)
    assert.equal(ok(await request('/services')).items.length, 1)
    assert.equal(ok(await request('/settings')).site_name, '端到端验收工作站')
    assert.equal(ok(await request(servicePath)).service.activation, 'pending')
  })
  await pass('退出后会话失效', async () => { ok(await request('/auth/logout', { method: 'POST' })); assert.equal((await request('/auth/me')).status, 401) })
  writeFileSync(join(workspace, 'result.json'), JSON.stringify({ passed: results.length, cases: results, completed_at: new Date().toISOString() }, null, 2))
  console.log(`\n全部 ${results.length} 组集成验收通过。结果：${join(workspace, 'result.json')}`)
} catch (error) {
  console.error(`\n验收失败：${error.stack || error}`)
  console.error(outputs.join('').slice(-15000).replaceAll(password, '[已隐藏]').replaceAll(registrationKey, '[已隐藏]').replaceAll(bootstrapToken, '[已隐藏]').replaceAll(businessToken, '[已隐藏]'))
  process.exitCode = 1
} finally {
  for (const child of [...children]) await stop(child)
}
