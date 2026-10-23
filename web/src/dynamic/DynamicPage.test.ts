// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import DynamicPage from './DynamicPage.vue'
import { type Definition, type DynamicService } from './schema'

let wrappers: VueWrapper[] = []
const base = 'http://127.0.0.1:8091'
function service(definition: Definition): DynamicService {
  return { id: 'demo', name: '测试服务', base_url: base, activation: 'enabled', online: 'online', definition }
}
function render(definition: Definition, pageId: string, extra = {}) {
  const wrapper = mount(DynamicPage, { props: { service: { ...service(definition), ...extra }, pageId }, attachTo: document.body })
  wrappers.push(wrapper)
  return wrapper
}
function json(value: unknown) { return new Response(JSON.stringify(value), { headers: { 'Content-Type': 'application/json' } }) }
beforeEach(() => {
  vi.stubGlobal('ResizeObserver', class { observe() {} unobserve() {} disconnect() {} })
  Object.defineProperty(window, 'matchMedia', { configurable: true, value: vi.fn().mockImplementation(query => ({ matches: false, media: query, addEventListener() {}, removeEventListener() {}, addListener() {}, removeListener() {}, dispatchEvent() { return false } })) })
})
afterEach(() => { wrappers.forEach(wrapper => wrapper.unmount()); wrappers = []; document.body.innerHTML = ''; vi.unstubAllGlobals() })

describe('动态页面组件', () => {
  const table: Definition = { schema_version: 1, menus: [], pages: [{ id: 'items', title: '条目', type: 'table', source: { path: '/api/items' }, columns: [{ key: 'name', label: '名称' }] }] }
  it('从真实请求结果渲染表格，并将HTML内容作为文本', async () => {
    const dangerous = '<img src=x onerror=alert(1)>'
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(json({ items: [{ id: 1, name: dangerous }], total: 1 })))
    const wrapper = render(table, 'items')
    await flushPromises()
    expect(wrapper.text()).toContain(dangerous)
    expect(wrapper.find('img').exists()).toBe(false)
    expect(wrapper.text()).toContain('共 1 条记录')
  })
  it('无效定义或未启用时不会调用业务接口', async () => {
    const fetcher = vi.fn(); vi.stubGlobal('fetch', fetcher)
    const invalid = render({ ...table, schema_version: 2 } as unknown as Definition, 'items')
    expect(invalid.text()).toContain('不支持此页面定义版本')
    const disabled = render(table, 'items', { activation: 'disabled' })
    expect(disabled.text()).toContain('服务尚未启用')
    await flushPromises()
    expect(fetcher).not.toHaveBeenCalled()
  })
  it('业务接口失败仅影响当前页面并保留重试按钮', async () => {
    vi.stubGlobal('fetch', vi.fn().mockRejectedValue(new TypeError('offline')))
    const wrapper = render(table, 'items', { online: 'offline' })
    await flushPromises()
    expect(wrapper.text()).toContain('最近检测到服务离线')
    expect(wrapper.text()).toContain('表格加载失败')
    expect(wrapper.text()).toContain('刷新数据')
    expect(wrapper.text()).toContain('中心管理功能不受影响')
  })
  it('详情显示缺失值占位和布尔值', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(json({ enabled: false })))
    const wrapper = render({ schema_version: 1, menus: [], pages: [{ id: 'detail', title: '详情', type: 'detail', source: { path: '/api/about' }, fields: [{ key: 'enabled', label: '已启用' }, { key: 'missing', label: '空字段' }] }] }, 'detail')
    await flushPromises()
    expect(wrapper.text()).toContain('否')
    expect(wrapper.text()).toContain('—')
  })
  it('表单先进行必填校验，然后通过中心网关提交', async () => {
    const fetcher = vi.fn().mockResolvedValue(json({ ok: true })); vi.stubGlobal('fetch', fetcher)
    const wrapper = render({ schema_version: 1, menus: [], pages: [{ id: 'form', title: '设置', type: 'form', fields: [{ key: 'name', label: '名称', type: 'text', required: true }, { key: 'enabled', label: '启用', type: 'boolean', required: true }], submit: { path: '/api/settings', method: 'PUT' } }] }, 'form')
    await wrapper.find('form').trigger('submit')
    expect(wrapper.text()).toContain('请填写名称')
    expect(fetcher).not.toHaveBeenCalled()
    await wrapper.find('input').setValue('工作站')
    await wrapper.find('form').trigger('submit')
    await flushPromises()
    expect(fetcher).toHaveBeenCalledTimes(1)
    const [url, init] = fetcher.mock.calls[0]!
    expect(String(url)).toBe('/api/v1/services/demo/pages/form/invoke')
    expect(JSON.parse(init.body)).toEqual({ operation: 'submit', body: { name: '工作站', enabled: false } })
    expect(wrapper.text()).toContain('提交成功')
  })
  it('表格操作显示确认弹窗，确认后才提交行标识并刷新数据', async () => {
    const fetcher = vi.fn()
      .mockResolvedValueOnce(json({ items: [{ id: 7, name: '待处理记录' }], total: 1 }))
      .mockResolvedValueOnce(json({ ok: true }))
      .mockResolvedValueOnce(json({ items: [], total: 0 }))
    vi.stubGlobal('fetch', fetcher)
    const definition: Definition = { schema_version: 1, menus: [], pages: [{
      id: 'items', title: '清单', type: 'table', source: { path: '/api/items' }, columns: [{ key: 'name', label: '名称' }],
      actions: [{ id: 'remove', label: '删除记录', method: 'DELETE', path: '/api/items', confirm: '只删除这条业务记录？' }],
    }] }
    const wrapper = render(definition, 'items')
    await flushPromises()
    await wrapper.findAll('button').find(button => button.text() === '删除记录')!.trigger('click')
    await flushPromises()
    expect(document.body.textContent).toContain('只删除这条业务记录？')
    expect(fetcher).toHaveBeenCalledTimes(1)
    const confirm = [...document.querySelectorAll<HTMLButtonElement>('.n-popconfirm__action button')].find(button => button.textContent?.includes('确认执行'))
    expect(confirm).toBeDefined()
    confirm!.click()
    await flushPromises()
    expect(fetcher).toHaveBeenCalledTimes(3)
    const [url, init] = fetcher.mock.calls[1]!
    expect(String(url)).toBe('/api/v1/services/demo/pages/items/invoke')
    expect(init.method).toBe('POST')
    expect(JSON.parse(init.body)).toEqual({ operation: 'action', action_id: 'remove', body: { id: 7 } })
    expect(wrapper.text()).toContain('共 0 条记录')
    expect(wrapper.text()).toContain('操作已完成')
  })
  it('表格分页向服务请求对应页码', async () => {
    const fetcher = vi.fn()
      .mockResolvedValueOnce(json({ items: [{ id: 1, name: '第一页' }], total: 21 }))
      .mockResolvedValueOnce(json({ items: [{ id: 21, name: '第二页' }], total: 21 }))
    vi.stubGlobal('fetch', fetcher)
    const wrapper = render(table, 'items')
    await flushPromises()
    const second = wrapper.findAll('.n-pagination-item').find(item => item.text() === '2')
    expect(second).toBeDefined()
    await second!.trigger('click')
    await flushPromises()
    expect(String(fetcher.mock.calls[1]![0])).toBe('/api/v1/services/demo/pages/items/invoke')
    expect(JSON.parse(fetcher.mock.calls[1]![1].body).query).toEqual({ page: 2, page_size: 20 })
    expect(wrapper.text()).toContain('第二页')
    expect(wrapper.text()).not.toContain('第一页')
  })
  it('表单提交非法JSON时显示失败、保留输入并允许重试', async () => {
    const fetcher = vi.fn()
      .mockResolvedValueOnce(new Response('{broken', { headers: { 'Content-Type': 'application/json' } }))
      .mockResolvedValueOnce(json({ ok: true }))
    vi.stubGlobal('fetch', fetcher)
    const wrapper = render({ schema_version: 1, menus: [], pages: [{ id: 'form', title: '设置', type: 'form', fields: [{ key: 'name', label: '名称', type: 'text', required: true }], submit: { path: '/api/settings', method: 'PUT' } }] }, 'form')
    await wrapper.find('input').setValue('保留此输入')
    await wrapper.find('form').trigger('submit')
    await flushPromises()
    expect(wrapper.text()).toContain('提交失败')
    expect(wrapper.text()).toContain('有效 JSON')
    expect(wrapper.find('input').element.value).toBe('保留此输入')
    expect(wrapper.text()).not.toContain('提交成功')
    await wrapper.find('form').trigger('submit')
    await flushPromises()
    expect(wrapper.text()).toContain('提交成功')
    expect(wrapper.text()).not.toContain('提交失败')
  })
  it('某个业务接口返回非法JSON不影响切换到其他页面', async () => {
    vi.stubGlobal('fetch', vi.fn()
      .mockResolvedValueOnce(new Response('not-json', { headers: { 'Content-Type': 'application/json' } }))
      .mockResolvedValueOnce(json({ status: '其他页面正常' })))
    const definition: Definition = { ...table, pages: [...table.pages, { id: 'about', title: '独立详情', type: 'detail', source: { path: '/api/about' }, fields: [{ key: 'status', label: '状态' }] }] }
    const wrapper = render(definition, 'items')
    await flushPromises()
    expect(wrapper.text()).toContain('表格加载失败')
    await wrapper.setProps({ pageId: 'about' })
    await flushPromises()
    expect(wrapper.text()).toContain('其他页面正常')
    expect(wrapper.text()).not.toContain('表格加载失败')
  })
  it('跨页面切换中止旧请求，即使旧响应晚到也不会污染新页面', async () => {
    let oldSignal!: AbortSignal
    let resolveOld!: (value: Response) => void
    vi.stubGlobal('fetch', vi.fn()
      .mockImplementationOnce((_url: URL, init: RequestInit) => new Promise<Response>(resolve => { oldSignal = init.signal!; resolveOld = resolve }))
      .mockResolvedValueOnce(json({ status: '当前详情' })))
    const definition: Definition = { ...table, pages: [...table.pages, { id: 'about', title: '详情', type: 'detail', source: { path: '/api/about' }, fields: [{ key: 'status', label: '状态' }] }] }
    const wrapper = render(definition, 'items')
    await wrapper.setProps({ pageId: 'about' })
    await flushPromises()
    expect(oldSignal.aborted).toBe(true)
    expect(wrapper.text()).toContain('当前详情')
    resolveOld(json({ items: [{ id: 1, name: '旧表格数据' }], total: 1 }))
    await flushPromises()
    expect(wrapper.text()).not.toContain('旧表格数据')
    expect(wrapper.text()).toContain('当前详情')
  })
})
