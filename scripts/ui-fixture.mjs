// 仅本机浏览器验收用：独立临时数据库和测试凭据，绝不能作为生产启动脚本。
import { spawn, spawnSync } from 'node:child_process'
import { mkdirSync, mkdtempSync, writeFileSync } from 'node:fs'
import { dirname, join, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'
import { randomBytes } from 'node:crypto'

const root = resolve(dirname(fileURLToPath(import.meta.url)), '..')
mkdirSync(join(root, '.runtime'), { recursive: true })
const folder = mkdtempSync(join(root, '.runtime', 'browser-'))
const suffix = process.platform === 'win32' ? '.exe' : ''
const key = randomBytes(32).toString('hex')
const businessToken = randomBytes(48).toString('base64url')
const accessPath = join(folder, 'service-access.json')
writeFileSync(accessPath, JSON.stringify({ 'workstation-demo': {
  origin: 'http://127.0.0.1:18091', bearer_token: businessToken,
  endpoints: [{ path: '/api/tasks', methods: ['GET'] }, { path: '/api/tasks/archive', methods: ['POST'] }, { path: '/api/preferences', methods: ['GET', 'PUT'] }, { path: '/api/overview', methods: ['GET'] }],
} }), { mode: 0o600 })
const config = {
  listen: '127.0.0.1:18080', database_path: join(folder, 'controller.db'),
  static_dir: join(root, 'web', 'dist'), public_origin: 'http://127.0.0.1:18080',
  bootstrap_token: 'local-browser-test-bootstrap-2026-do-not-deploy',
  registration_key: key, allowed_service_origins: ['http://127.0.0.1:18091'],
  session_hours: 1, secure_cookie: false, check_interval_seconds: 10, service_access_file: accessPath,
}
const configFile = join(folder, 'config.json')
writeFileSync(configFile, JSON.stringify(config), { mode: 0o600 })
const cleanEnv = Object.fromEntries(Object.entries(process.env).filter(([key]) => !key.startsWith('CONTROLLER_') && !key.startsWith('DEMO_')))
const processes = []
let demoProcess
let demoBinary
let demoArgs
function startDemo() {
  if (demoProcess && demoProcess.exitCode === null) return
  demoProcess = spawn(demoBinary, demoArgs, { cwd: root, env: { ...cleanEnv, DEMO_REGISTRATION_KEY: key, DEMO_API_TOKEN: businessToken }, windowsHide: true, stdio: ['ignore', 'pipe', 'pipe'] })
  demoProcess.stdout.on('data', data => process.stdout.write(data))
  demoProcess.stderr.on('data', data => process.stderr.write(data))
  demoProcess.once('error', error => { console.error('演示进程失败：', error.message); cleanup(); process.exitCode = 1 })
  processes.push(demoProcess)
}
for (const target of ['controller', 'demo-service']) {
  const binary = join(folder, target + suffix)
  const result = spawnSync('go', ['build', '-o', binary, `./cmd/${target}`], { cwd: root, encoding: 'utf8', windowsHide: true })
  if (result.status !== 0) throw new Error(result.stderr || '编译失败')
  const args = target === 'controller' ? ['-config', configFile] : ['-listen', '127.0.0.1:18091', '-base-url', 'http://127.0.0.1:18091', '-controller', config.public_origin]
  if (target === 'demo-service') { demoBinary = binary; demoArgs = args; startDemo(); continue }
  const child = spawn(binary, args, { cwd: root, env: { ...cleanEnv, DEMO_REGISTRATION_KEY: key, DEMO_API_TOKEN: businessToken }, windowsHide: true, stdio: ['ignore', 'pipe', 'pipe'] })
  child.stdout.on('data', data => process.stdout.write(data))
  child.stderr.on('data', data => process.stderr.write(data))
  child.once('error', error => { console.error('测试进程失败：', error.message); cleanup(); process.exitCode = 1 })
  processes.push(child)
}
function cleanup() { for (const child of processes) if (child.exitCode === null) child.kill() }
process.once('SIGINT', () => { cleanup(); process.exit() })
process.once('SIGTERM', () => { cleanup(); process.exit() })
process.once('exit', cleanup)
process.stdin.setEncoding('utf8')
process.stdin.on('data', command => {
  if (command.trim() === 'stop-demo') demoProcess?.kill()
  if (command.trim() === 'start-demo') startDemo()
  if (command.trim() === 'quit') { cleanup(); process.exit() }
})
console.log('浏览器验收地址：http://127.0.0.1:18080（临时数据，非正式实例）。可输入 stop-demo / start-demo 检查离线恢复，quit 结束测试。')
