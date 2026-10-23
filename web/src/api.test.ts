import { afterEach, describe, expect, it, vi } from 'vitest'
import { api, ApiError, setCSRFToken } from './api'

afterEach(() => {
  vi.unstubAllGlobals()
  setCSRFToken('')
})
describe('管理 API 客户端', () => {
  it('只向同源管理接口携带会话和 CSRF', async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response('{"ok":true}', { status: 200 }))
    vi.stubGlobal('fetch', fetchMock)
    setCSRFToken('test-csrf')
    await api('/services/demo', { method: 'PATCH', body: { activation: 'enabled' } })
    expect(fetchMock).toHaveBeenCalledWith(
      '/api/v1/services/demo',
      expect.objectContaining({
        method: 'PATCH',
        credentials: 'same-origin',
        redirect: 'error',
        headers: expect.objectContaining({ 'X-CSRF-Token': 'test-csrf' }),
      }),
    )
  })
  it('拒绝外部目标 URL', async () => {
    await expect(api('https://example.com')).rejects.toThrow('无效的管理接口路径')
    await expect(api('//example.com')).rejects.toThrow('无效的管理接口路径')
  })
  it('未认证响应通知会话失效，不显示原始响应', async () => {
    vi.stubGlobal(
      'fetch',
      vi
        .fn()
        .mockResolvedValue(
          new Response('{"error":{"code":"unauthorized","message":"请先登录"}}', { status: 401 }),
        ),
    )
    const listener = vi.fn()
    window.addEventListener('session-expired', listener, { once: true })
    await expect(api('/dashboard')).rejects.toThrow('请先登录')
    expect(listener).toHaveBeenCalledOnce()
  })
  it('登录失败不会触发全局重定向', async () => {
    vi.stubGlobal(
      'fetch',
      vi
        .fn()
        .mockResolvedValue(
          new Response('{"error":{"message":"用户名或密码错误"}}', { status: 401 }),
        ),
    )
    const listener = vi.fn()
    window.addEventListener('session-expired', listener)
    await expect(
      api('/auth/login', { method: 'POST', silentUnauthorized: true }),
    ).rejects.toBeInstanceOf(ApiError)
    expect(listener).not.toHaveBeenCalled()
    window.removeEventListener('session-expired', listener)
  })
  it('无效 JSON 显示统一错误', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue(new Response('<html>bad gateway</html>', { status: 200 })),
    )
    await expect(api('/dashboard')).rejects.toThrow('无效的 JSON')
  })
})
