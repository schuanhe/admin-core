let csrfToken = ''
export function setCSRFToken(value: string) {
  csrfToken = value
}
export function getCSRFToken() { return csrfToken }

export class ApiError extends Error {
  constructor(
    message: string,
    public status: number,
    public code = 'request_failed',
  ) {
    super(message)
    this.name = 'ApiError'
  }
}

interface RequestOptions {
  method?: 'GET' | 'POST' | 'PATCH' | 'DELETE' | 'PUT'
  body?: unknown
  signal?: AbortSignal
  silentUnauthorized?: boolean
}

export async function api<T>(path: string, options: RequestOptions = {}): Promise<T> {
  if (!path.startsWith('/') || path.startsWith('//')) throw new Error('无效的管理接口路径')
  const controller = new AbortController()
  const timeout = window.setTimeout(() => controller.abort(), 15000)
  const onAbort = () => controller.abort()
  options.signal?.addEventListener('abort', onAbort, { once: true })
  if (options.signal?.aborted) controller.abort()
  try {
    const headers: Record<string, string> = { Accept: 'application/json' }
    if (options.body !== undefined) headers['Content-Type'] = 'application/json'
    if (options.method && options.method !== 'GET' && csrfToken) headers['X-CSRF-Token'] = csrfToken
    const response = await fetch(`/api/v1${path}`, {
      method: options.method ?? 'GET',
      credentials: 'same-origin',
      redirect: 'error',
      headers,
      signal: controller.signal,
      body: options.body === undefined ? undefined : JSON.stringify(options.body),
    })
    const payload = await response.json().catch(() => null)
    if (!response.ok) {
      if (response.status === 401 && !options.silentUnauthorized) {
        window.dispatchEvent(new CustomEvent('session-expired'))
      }
      throw new ApiError(
        payload?.error?.message ?? `请求失败（${response.status}）`,
        response.status,
        payload?.error?.code,
      )
    }
    if (payload === null) throw new ApiError('服务器返回了无效的 JSON 数据', response.status)
    return payload as T
  } catch (error) {
    if (error instanceof ApiError) throw error
    if (controller.signal.aborted)
      throw new ApiError(
        options.signal?.aborted ? '请求已取消' : '请求超时，请稍后重试',
        0,
        'timeout',
      )
    throw new ApiError('暂时无法连接控制中心，请检查网络后重试', 0, 'network_error')
  } finally {
    window.clearTimeout(timeout)
    options.signal?.removeEventListener('abort', onAbort)
  }
}
