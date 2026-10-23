import { afterEach, describe, expect, it, vi } from 'vitest'
import { requestServiceJSON, RESPONSE_MAX_BYTES, serviceURL, validateMutationResult, validateRecordResult, validateTableResult } from './transport'
import { setCSRFToken } from '../api'

const base = 'demo'
const json = (value: unknown) => new Response(JSON.stringify(value), { headers: { 'Content-Type': 'application/json' } })
afterEach(() => { vi.unstubAllGlobals(); vi.useRealTimers() })

describe('中心受限业务网关', () => {
  it('仅按服务和页面标识生成同源网关路径', () => {
    expect(serviceURL(base, 'items')).toBe('/api/v1/services/demo/pages/items/invoke')
    for (const origin of ['file:///tmp/data', 'https://user:pass@example.com', 'https://example.com/path', 'https://example.com?token=secret', 'javascript:alert(1)']) expect(() => serviceURL(origin, 'api')).toThrow()
    expect(() => serviceURL(base, '//example.com/x')).toThrow()
  })
  it('携带中心同源会话与CSRF，不携带业务密钥且拒绝重定向', async () => {
    const fetcher = vi.fn().mockResolvedValue(json({ ok: true }))
    vi.stubGlobal('fetch', fetcher)
    setCSRFToken('csrf-token')
    await expect(requestServiceJSON(base, 'settings', { operation: 'submit', body: { enabled: false } })).resolves.toEqual({ ok: true })
    const [url, init] = fetcher.mock.calls[0]!
    expect(String(url)).toBe('/api/v1/services/demo/pages/settings/invoke')
    expect(init).toMatchObject({ credentials: 'same-origin', redirect: 'error', referrerPolicy: 'no-referrer', method: 'POST', body: '{"operation":"submit","body":{"enabled":false}}', cache: 'no-store' })
    expect(init.headers).toEqual({ Accept: 'application/json', 'Content-Type': 'application/json', 'X-CSRF-Token': 'csrf-token' })
  })
  it('拒绝非JSON、无效JSON和不符合响应结构的数据', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(new Response('<html>错误</html>', { headers: { 'Content-Type': 'text/html' } })))
    await expect(requestServiceJSON(base, 'api')).rejects.toThrow('application/json')
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(new Response('{bad', { headers: { 'Content-Type': 'application/json' } })))
    await expect(requestServiceJSON(base, 'api')).rejects.toThrow('有效 JSON')
    expect(() => validateTableResult({ items: [{}], total: 0 }, 20)).toThrow('表格响应')
    expect(() => validateTableResult({ items: [{ nested: {} }], total: 1 }, 20)).toThrow('业务字段')
    expect(() => validateTableResult({ items: [{}, {}], total: 2 }, 1)).toThrow('表格响应')
    expect(() => validateTableResult({ items: [], total: 1.5 }, 20)).toThrow('表格响应')
    expect(() => validateRecordResult([])).toThrow('JSON 对象')
    expect(() => validateMutationResult(null)).toThrow('JSON 对象')
    expect(validateTableResult({ items: [{ id: 1, enabled: false }], total: 1 }, 20)).toEqual({ items: [{ id: 1, enabled: false }], total: 1 })
  })
  it('使用响应头和实际流字节双重限制大小', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(new Response('{}', { headers: { 'Content-Type': 'application/json', 'Content-Length': String(RESPONSE_MAX_BYTES + 1) } })))
    await expect(requestServiceJSON(base, 'api')).rejects.toThrow('1 MiB')
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(new Response('x'.repeat(RESPONSE_MAX_BYTES + 1), { headers: { 'Content-Type': 'application/json' } })))
    await expect(requestServiceJSON(base, 'api')).rejects.toThrow('1 MiB')
  })
  it('保护响应原型属性与嵌套深度', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(new Response('{"__proto__":{"polluted":true}}', { headers: { 'Content-Type': 'application/json' } })))
    await expect(requestServiceJSON(base, 'api')).rejects.toThrow('属性名称')
    let value: unknown = null; for (let i = 0; i < 13; i++) value = { next: value }
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(json(value)))
    await expect(requestServiceJSON(base, 'api')).rejects.toThrow('12 层')
  })
  it('网络、认证和服务故障给出可操作错误且不泄露响应内容', async () => {
    vi.stubGlobal('fetch', vi.fn().mockRejectedValue(new TypeError('包含内部地址')))
    await expect(requestServiceJSON(base, 'api')).rejects.toThrow('网关')
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(new Response(JSON.stringify({ error: { code: 'unauthorized', message: '认证已失效' } }), { status: 401, headers: { 'Content-Type': 'application/json' } })))
    await expect(requestServiceJSON(base, 'api')).rejects.toThrow('认证')
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(new Response('{}', { status: 500, headers: { 'Content-Type': 'application/json' } })))
    await expect(requestServiceJSON(base, 'api')).rejects.toThrow('HTTP 500')
  })
  it('8秒超时中止请求', async () => {
    vi.useFakeTimers()
    vi.stubGlobal('fetch', vi.fn((_url: unknown, init: RequestInit) => new Promise((_resolve, reject) => init.signal?.addEventListener('abort', () => reject(new DOMException('aborted', 'AbortError'))))))
    const result = expect(requestServiceJSON(base, 'api')).rejects.toMatchObject({ code: 'timeout' })
    await vi.advanceTimersByTimeAsync(8000)
    await result
  })
  it('页面切换支持外部取消且不会继续发出已取消请求', async () => {
    const fetcher = vi.fn((_url: unknown, init: RequestInit) => new Promise((_resolve, reject) => init.signal?.addEventListener('abort', () => reject(new DOMException('aborted', 'AbortError')))))
    vi.stubGlobal('fetch', fetcher)
    const controller = new AbortController()
    const result = expect(requestServiceJSON(base, 'api', { signal: controller.signal })).rejects.toMatchObject({ code: 'cancelled' })
    controller.abort()
    await result
    await expect(requestServiceJSON(base, 'api', { signal: controller.signal })).rejects.toMatchObject({ code: 'cancelled' })
    expect(fetcher).toHaveBeenCalledTimes(1)
  })
})
