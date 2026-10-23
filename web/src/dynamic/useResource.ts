import { onBeforeUnmount, ref, shallowRef } from 'vue'
import { errorMessage, isCancelled } from './transport'

/** 路由切换和连续刷新会取消旧请求，并拒绝旧响应覆盖新状态。 */
export function useResource<T>(fetcher: (signal: AbortSignal) => Promise<T>) {
  const data = shallowRef<T | null>(null)
  const error = ref('')
  const loading = ref(false)
  let controller: AbortController | null = null
  let sequence = 0
  let disposed = false
  async function load(): Promise<boolean> {
    if (disposed) return false
    controller?.abort()
    const request = new AbortController()
    controller = request
    const version = ++sequence
    loading.value = true
    error.value = ''
    try {
      const result = await fetcher(request.signal)
      if (disposed || version !== sequence) return false
      data.value = result
      return true
    } catch (cause) {
      if (!disposed && version === sequence && !isCancelled(cause)) error.value = errorMessage(cause)
      return false
    } finally {
      if (!disposed && version === sequence) loading.value = false
    }
  }
  onBeforeUnmount(() => { disposed = true; sequence++; controller?.abort() })
  return { data, error, loading, load }
}
