// @vitest-environment jsdom
import { defineComponent } from 'vue'
import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import { useResource } from './useResource'

describe('异步页面资源生命周期', () => {
  it('连续刷新取消上次请求，晚到结果不能覆盖新结果', async () => {
    const pending: Array<{ signal: AbortSignal; resolve: (value: string) => void }> = []
    let resource!: ReturnType<typeof useResource<string>>
    const wrapper = mount(defineComponent({
      setup() {
        resource = useResource(signal => new Promise<string>(resolve => pending.push({ signal, resolve })))
        return () => null
      },
    }))
    const first = resource.load()
    const second = resource.load()
    expect(pending[0]!.signal.aborted).toBe(true)
    pending[1]!.resolve('新数据')
    await second
    pending[0]!.resolve('旧数据')
    await first
    expect(resource.data.value).toBe('新数据')
    expect(resource.loading.value).toBe(false)
    wrapper.unmount()
  })
  it('卸载中止请求且忽略后续响应，不留下页面错误', async () => {
    let resolve!: (value: string) => void
    let signal!: AbortSignal
    let resource!: ReturnType<typeof useResource<string>>
    const wrapper = mount(defineComponent({
      setup() {
        resource = useResource(receivedSignal => new Promise<string>(receivedResolve => { signal = receivedSignal; resolve = receivedResolve }))
        return () => null
      },
    }))
    const request = resource.load()
    wrapper.unmount()
    expect(signal.aborted).toBe(true)
    resolve('不应显示的数据')
    expect(await request).toBe(false)
    expect(resource.data.value).toBeNull()
    expect(resource.error.value).toBe('')
    expect(await resource.load()).toBe(false)
  })
})
