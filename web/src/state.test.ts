import { beforeEach, describe, expect, it, vi } from 'vitest'
import { api, ApiError, setCSRFToken } from './api'
import type { Service } from './models'
import {
  acceptSession,
  clearSession,
  refreshMenus,
  restoreSession,
  session,
  workspace,
} from './state'

vi.mock('./api', async (importOriginal) => ({
  ...(await importOriginal<typeof import('./api')>()),
  api: vi.fn(),
  setCSRFToken: vi.fn(),
}))

beforeEach(() => {
  vi.resetAllMocks()
  clearSession()
  session.checked = false
})

describe('管理员会话与动态菜单状态', () => {
  it('刷新时恢复服务端会话和 CSRF，仅进行一次并发查询', async () => {
    vi.mocked(api).mockResolvedValue({ user: { username: 'admin' }, csrf_token: 'csrf-demo' })
    expect(await Promise.all([restoreSession(), restoreSession()])).toEqual([true, true])
    expect(api).toHaveBeenCalledTimes(1)
    expect(session.username).toBe('admin')
    expect(setCSRFToken).toHaveBeenLastCalledWith('csrf-demo')
    expect(await restoreSession()).toBe(true)
    expect(api).toHaveBeenCalledTimes(1)
  })

  it('会话过期后清空用户和菜单，不保留旧密钥', async () => {
    vi.mocked(api).mockRejectedValue(new ApiError('会话过期', 401))
    expect(await restoreSession()).toBe(false)
    expect(session).toEqual({ checked: true, username: '' })
    expect(workspace.services).toEqual([])
    expect(setCSRFToken).toHaveBeenLastCalledWith('')
  })

  it('网络中断不会永久缓存为未登录，下次可重新恢复', async () => {
    vi.mocked(api).mockRejectedValueOnce(new ApiError('网络中断', 0))
    expect(await restoreSession()).toBe(false)
    expect(session.checked).toBe(false)
    vi.mocked(api).mockResolvedValueOnce({ user: { username: 'admin' }, csrf_token: 'new-csrf' })
    expect(await restoreSession()).toBe(true)
  })

  it('菜单只请求已启用服务，离线不会主动移除菜单', async () => {
    acceptSession({ user: { username: 'admin' }, csrf_token: 'csrf' })
    const enabledOffline = { id: 'demo', activation: 'enabled', online: 'offline' } as Service
    vi.mocked(api).mockResolvedValue({ items: [enabledOffline] })
    await refreshMenus()
    expect(api).toHaveBeenCalledWith('/services?activation=enabled')
    expect(workspace.services).toEqual([enabledOffline])
  })

  it('退出后忽略尚未完成的菜单响应，避免恢复旧用户菜单', async () => {
    acceptSession({ user: { username: 'admin' }, csrf_token: 'csrf' })
    let finish!: (value: unknown) => void
    vi.mocked(api).mockImplementation(() => new Promise((resolve) => { finish = resolve }))
    const pending = refreshMenus()
    clearSession()
    finish({ items: [{ id: 'old-service' }] })
    await pending
    expect(workspace.services).toEqual([])
    expect(session.username).toBe('')
  })

  it('同步失败保留菜单并显示错误，下一次成功自动恢复', async () => {
    acceptSession({ user: { username: 'admin' }, csrf_token: 'csrf' })
    workspace.services = [{ id: 'saved-service' } as Service]
    vi.mocked(api).mockRejectedValueOnce(new Error('暂时无法连接'))
    await refreshMenus()
    expect(workspace.services[0]?.id).toBe('saved-service')
    expect(workspace.menuError).toBe('暂时无法连接')
    vi.mocked(api).mockResolvedValueOnce({ items: [] })
    await refreshMenus()
    expect(workspace.menuError).toBe('')
    expect(workspace.services).toEqual([])
  })
})
