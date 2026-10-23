import { assertJSONLimits, isRecord, type DataRecord } from './schema'
import { getCSRFToken } from '../api'

export const RESPONSE_MAX_BYTES = 1024 * 1024
export const REQUEST_TIMEOUT_MS = 8000

export class ServiceRequestError extends Error {
  constructor(public code: string, message: string) { super(message); this.name = 'ServiceRequestError' }
}

export function serviceURL(serviceId: string, pageId: string): string {
  if (!/^[a-z0-9][a-z0-9-]{0,63}$/.test(serviceId) || !/^[A-Za-z][A-Za-z0-9_-]{0,63}$/.test(pageId)) {
    throw new ServiceRequestError('address', '服务或页面标识无效。')
  }
  return `/api/v1/services/${encodeURIComponent(serviceId)}/pages/${encodeURIComponent(pageId)}/invoke`
}

async function readJSON(response: Response): Promise<unknown> {
  const declaredLength = Number(response.headers.get('content-length'))
  if (declaredLength > RESPONSE_MAX_BYTES) {
    await response.body?.cancel()
    throw new ServiceRequestError('size', '服务响应超过 1 MiB 限制。')
  }
  if (!/^application\/(?:[\w.+-]+\+)?json(?:\s*;|$)/i.test(response.headers.get('content-type') ?? '')) {
    await response.body?.cancel()
    throw new ServiceRequestError('json', '服务必须返回 application/json 数据。')
  }
  if (!response.body) throw new ServiceRequestError('json', '服务返回了空响应。')
  const reader = response.body.getReader()
  const decoder = new TextDecoder('utf-8', { fatal: true })
  let bytes = 0
  let body = ''
  try {
    while (true) {
      const next = await reader.read()
      if (next.done) break
      bytes += next.value.byteLength
      if (bytes > RESPONSE_MAX_BYTES) {
        await reader.cancel()
        throw new ServiceRequestError('size', '服务响应超过 1 MiB 限制。')
      }
      body += decoder.decode(next.value, { stream: true })
    }
    body += decoder.decode()
    const value: unknown = JSON.parse(body)
    assertJSONLimits(value, RESPONSE_MAX_BYTES, 20000)
    return value
  } catch (error) {
    if (error instanceof ServiceRequestError) throw error
    throw new ServiceRequestError('json', error instanceof SyntaxError || error instanceof TypeError ? '服务返回的不是有效 JSON。' : error instanceof Error ? error.message : '服务响应无效。')
  } finally { reader.releaseLock() }
}

/** 浏览器只调用同源受限网关；独立业务令牌仅保留在后端。 */
export async function requestServiceJSON(serviceId: string, pageId: string, options: {
  operation?: 'source' | 'submit' | 'action'
  actionId?: string
  body?: DataRecord
  query?: Record<string, number>
  signal?: AbortSignal
} = {}): Promise<unknown> {
  const url = serviceURL(serviceId, pageId)
  const controller = new AbortController()
  let timedOut = false
  const abort = () => controller.abort()
  if (options.signal?.aborted) throw new ServiceRequestError('cancelled', '请求已取消。')
  options.signal?.addEventListener('abort', abort, { once: true })
  const timeout = setTimeout(() => { timedOut = true; controller.abort() }, REQUEST_TIMEOUT_MS)
  try {
    const response = await fetch(url, {
      method: 'POST',
      headers: { Accept: 'application/json', 'Content-Type': 'application/json', 'X-CSRF-Token': getCSRFToken() },
      body: JSON.stringify({ operation: options.operation ?? 'source', action_id: options.actionId, query: options.query, body: options.body }),
      credentials: 'same-origin',
      redirect: 'error',
      referrerPolicy: 'no-referrer',
      cache: 'no-store',
      signal: controller.signal,
    })
    if (!response.ok) {
      const payload = await readJSON(response)
      if (response.status === 401) window.dispatchEvent(new CustomEvent('session-expired'))
      if (isRecord(payload) && isRecord(payload.error) && typeof payload.error.message === 'string') {
        throw new ServiceRequestError(typeof payload.error.code === 'string' ? payload.error.code : 'http', payload.error.message)
      }
      throw new ServiceRequestError('http', `服务返回 HTTP ${response.status}，请检查服务运行状态。`)
    }
    return await readJSON(response)
  } catch (error) {
    if (timedOut) throw new ServiceRequestError('timeout', '请求超过 8 秒，请检查服务是否在线。')
    if (options.signal?.aborted) throw new ServiceRequestError('cancelled', '请求已取消。')
    if (error instanceof ServiceRequestError) throw error
    throw new ServiceRequestError('network', '无法连接中心业务网关，请检查网络状态。中心管理功能不受影响。')
  } finally {
    clearTimeout(timeout)
    options.signal?.removeEventListener('abort', abort)
  }
}

export function validateRecordResult(value: unknown): DataRecord {
  if (!isRecord(value) || Object.keys(value).length > 100) throw new ServiceRequestError('shape', '业务数据必须为最多 100 个字段的 JSON 对象。')
  for (const [key, item] of Object.entries(value)) {
    if (['__proto__', 'constructor', 'prototype'].includes(key) || !(item === null || typeof item === 'string' || typeof item === 'boolean' || (typeof item === 'number' && Number.isFinite(item)))) {
      throw new ServiceRequestError('shape', '业务字段只允许文本、数字、布尔值或 null。')
    }
  }
  return value as DataRecord
}

export function validateTableResult(value: unknown, pageSize: number): { items: DataRecord[]; total: number } {
  if (!isRecord(value) || Object.keys(value).some(key => key !== 'items' && key !== 'total') || !Array.isArray(value.items) || value.items.length > pageSize || typeof value.total !== 'number' || !Number.isSafeInteger(value.total) || value.total < 0 || value.total < value.items.length) {
    throw new ServiceRequestError('shape', '表格响应必须包含 items 数组和有效的 total 总数，且不得超过每页条数。')
  }
  return { items: value.items.map(validateRecordResult), total: value.total }
}

export function validateMutationResult(value: unknown): void {
  if (!isRecord(value)) throw new ServiceRequestError('shape', '提交接口必须返回 JSON 对象。')
}

export function errorMessage(error: unknown): string {
  return error instanceof Error ? error.message : '请求失败，请稍后重试。'
}
export function isCancelled(error: unknown): boolean {
  return error instanceof ServiceRequestError && error.code === 'cancelled'
}
