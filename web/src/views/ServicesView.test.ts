import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import { h } from 'vue'
import { NDialogProvider, NMessageProvider } from 'naive-ui'
import ServicesView from './ServicesView.vue'
import type { Service } from '../models'

const { request, refreshMenus, push } = vi.hoisted(() => ({
  request: vi.fn(),
  refreshMenus: vi.fn().mockResolvedValue(undefined),
  push: vi.fn(),
}))
vi.mock('../api', () => ({ api: request }))
vi.mock('../state', () => ({ refreshMenus }))
vi.mock('vue-router', () => ({ useRouter: () => ({ push }) }))

let wrapper: VueWrapper | undefined
const service: Service = {
  id: 'demo-service',
  name: '独立测试服务',
  version: '1.0.0',
  description: '',
  base_url: 'http://127.0.0.1:8091',
  protocol_version: 1,
  health_path: '/healthz',
  activation: 'enabled',
  online: 'offline',
  registered_at: '2026-10-22T08:00:00Z',
  checked_at: null,
  created_at: '2026-10-22T08:00:00Z',
  updated_at: '2026-10-22T08:00:00Z',
  last_error: '',
  last_registration_error: '',
  definition: { schema_version: 1, menus: [], pages: [] },
}

function render() {
  wrapper = mount(
    {
      render: () =>
        h(NMessageProvider, null, {
          default: () => h(NDialogProvider, null, { default: () => h(ServicesView) }),
        }),
    },
    { attachTo: document.body },
  )
  return wrapper
}

beforeEach(() => {
  request.mockReset()
  refreshMenus.mockClear()
  vi.stubGlobal('ResizeObserver', class { observe() {} unobserve() {} disconnect() {} })
  Object.defineProperty(window, 'matchMedia', {
    configurable: true,
    value: vi.fn().mockImplementation((query) => ({
      matches: false, media: query,
      addEventListener() {}, removeEventListener() {}, addListener() {}, removeListener() {},
      dispatchEvent() { return false },
    })),
  })
})
afterEach(() => {
  wrapper?.unmount()
  document.body.innerHTML = ''
  vi.unstubAllGlobals()
})

describe('真实管理组件行为', () => {
  it('启用状态和离线状态可同时显示，空检查时间不崩溃', async () => {
    request.mockResolvedValue({ items: [service] })
    const view = render()
    await flushPromises()
    expect(view.text()).toContain('独立测试服务')
    expect(view.text()).toContain('已启用')
    expect(view.text()).toContain('离线')
    expect(view.text()).toContain('尚未记录')
    expect(view.text()).toContain('2026')
  })

  it('离线服务删除必须先确认，之后删除登记并刷新菜单', async () => {
    request
      .mockResolvedValueOnce({ items: [service] })
      .mockResolvedValueOnce({ ok: true })
      .mockResolvedValueOnce({ items: [] })
    const view = render()
    await flushPromises()
    await view.findAll('button').find((button) => button.text() === '删除')!.trigger('click')
    await flushPromises()
    expect(request).toHaveBeenCalledTimes(1)
    expect(document.body.textContent).toContain('不会删除服务自身的文件、数据库或容器')
    const confirm = [...document.querySelectorAll<HTMLButtonElement>('.n-dialog button')]
      .find((button) => button.textContent?.trim() === '确认删除登记')
    expect(confirm).toBeDefined()
    confirm!.click()
    await flushPromises()
    expect(request).toHaveBeenNthCalledWith(2, '/services/demo-service', { method: 'DELETE' })
    expect(refreshMenus).toHaveBeenCalledOnce()
    expect(view.text()).toContain('等待第一个服务登记')
  })

  it('管理员可禁用离线服务，并刷新已启用服务菜单', async () => {
    request
      .mockResolvedValueOnce({ items: [service] })
      .mockResolvedValueOnce({ service: { ...service, activation: 'disabled' } })
      .mockResolvedValueOnce({ items: [{ ...service, activation: 'disabled' }] })
    const view = render()
    await flushPromises()
    await view.findAll('button').find((button) => button.text() === '禁用')!.trigger('click')
    await flushPromises()
    expect(request).toHaveBeenNthCalledWith(2, '/services/demo-service', {
      method: 'PATCH', body: { activation: 'disabled' },
    })
    expect(refreshMenus).toHaveBeenCalledOnce()
    expect(view.text()).toContain('已禁用')
    expect(view.text()).toContain('离线')
  })

  it('搜索提交给真实管理 API，不在客户端伪造筛选结果', async () => {
    request.mockResolvedValue({ items: [] })
    const view = render()
    await flushPromises()
    await view.find('input').setValue(' demo ')
    await new Promise((resolve) => setTimeout(resolve, 350))
    await flushPromises()
    expect(request).toHaveBeenLastCalledWith('/services?q=demo')
    expect(view.text()).toContain('没有符合条件的服务')
  })
})
