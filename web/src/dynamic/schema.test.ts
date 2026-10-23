import { describe, expect, it } from 'vitest'
import { assertJSONLimits, assertSafePath, displayValue, ownValue, validateDefinition, validateFormValues, type Definition, type Field } from './schema'

function definition(): Definition {
  return {
    schema_version: 1,
    menus: [{ id: 'inventory', label: '清单', page_id: 'items' }],
    pages: [
      { id: 'items', title: '资产清单', type: 'table', source: { path: '/api/items' }, columns: [{ key: 'name', label: '名称' }], page_size: 20, actions: [{ id: 'remove', label: '删除', method: 'DELETE', path: '/api/items', confirm: '确认删除？' }] },
      { id: 'settings', title: '设置', type: 'form', fields: [{ key: 'name', label: '名称', type: 'text', required: true }, { key: 'count', label: '数量', type: 'number' }, { key: 'enabled', label: '启用', type: 'boolean' }, { key: 'mode', label: '模式', type: 'select', options: [{ label: '个人', value: 'personal' }] }], submit: { method: 'PUT', path: '/api/settings' } },
      { id: 'about', title: '详情', type: 'detail', source: { path: '/api/about' }, fields: [{ key: 'version', label: '版本' }] },
    ],
  }
}

describe('受限页面协议', () => {
  it('接受三种内置页面和没有页面的服务', () => {
    expect(validateDefinition(definition()).pages).toHaveLength(3)
    expect(validateDefinition({ schema_version: 1, menus: [], pages: [] }).pages).toEqual([])
  })
  it.each(['script', 'html', 'iframe', 'remote'])('拒绝未知组件 %s', type => {
    const input = definition() as unknown as { pages: Array<Record<string, unknown>> }
    input.pages[0]!.type = type
    expect(() => validateDefinition(input)).toThrow('页面类型')
  })
  it('拒绝模板、事件处理器、自定义请求头与不适用字段', () => {
    for (const key of ['template', 'onClick', 'headers', 'fields']) {
      const input = definition() as unknown as { pages: Array<Record<string, unknown>> }
      input.pages[0]![key] = 'alert(1)'
      expect(() => validateDefinition(input)).toThrow('不支持的字段')
    }
  })
  it('拒绝不兼容版本、缺失数据源与不存在的菜单关联', () => {
    expect(() => validateDefinition({ ...definition(), schema_version: 2 })).toThrow('版本')
    const missing = definition() as unknown as { pages: Array<Record<string, unknown>> }
    delete missing.pages[0]!.source
    expect(() => validateDefinition(missing)).toThrow('source')
    const linked = definition()
    linked.menus[0]!.page_id = 'missing'
    expect(() => validateDefinition(linked)).toThrow('不存在')
  })
  it('拒绝重复页面、菜单、列、操作与选项', () => {
    const page = definition(); page.pages.push(page.pages[0]!)
    expect(() => validateDefinition(page)).toThrow('不能重复')
    const menu = definition(); menu.menus.push(menu.menus[0]!)
    expect(() => validateDefinition(menu)).toThrow('不能重复')
    const column = definition(); if (column.pages[0]!.type === 'table') column.pages[0]!.columns.push(column.pages[0]!.columns[0]!)
    expect(() => validateDefinition(column)).toThrow('不能重复')
    const action = definition(); if (action.pages[0]!.type === 'table') action.pages[0]!.actions!.push(action.pages[0]!.actions![0]!)
    expect(() => validateDefinition(action)).toThrow('不能重复')
    const option = definition(); if (option.pages[1]!.type === 'form') option.pages[1]!.fields[3]!.options!.push({ label: '其他', value: 'personal' })
    expect(() => validateDefinition(option)).toThrow('不能重复')
  })
  it('拒绝原型污染属性和危险字段标识', () => {
    expect(() => validateDefinition(JSON.parse('{"schema_version":1,"menus":[],"pages":[],"__proto__":{"polluted":true}}'))).toThrow('属性名称')
    for (const key of ['constructor', 'prototype', '__proto__', 'nested.value']) {
      const input = definition()
      if (input.pages[0]!.type === 'table') input.pages[0]!.columns[0]!.key = key
      expect(() => validateDefinition(input)).toThrow()
    }
    expect(({} as Record<string, unknown>).polluted).toBeUndefined()
  })
  it('限制字符串、字段数、列数量、层级和总体积', () => {
    const label = definition(); label.pages[0]!.title = 'a'.repeat(121)
    expect(() => validateDefinition(label)).toThrow('120')
    const columns = definition(); if (columns.pages[0]!.type === 'table') columns.pages[0]!.columns = Array.from({ length: 31 }, (_, i) => ({ key: `key${i}`, label: '字段' }))
    expect(() => validateDefinition(columns)).toThrow('30')
    let deep: unknown = null
    for (let i = 0; i < 13; i++) deep = { value: deep }
    expect(() => assertJSONLimits(deep)).toThrow('12 层')
    expect(() => assertJSONLimits(Array.from({ length: 3001 }, () => 0))).toThrow('3000')
    expect(() => assertJSONLimits('x'.repeat(2001))).toThrow('2000')
    expect(() => assertJSONLimits(Array.from({ length: 100 }, () => '中'.repeat(2000)))).toThrow('大小')
    const cyclic: Record<string, unknown> = {}; cyclic.self = cyclic
    expect(() => assertJSONLimits(cyclic)).toThrow('循环')
  })
  it('只允许受限接口路径', () => {
    for (const path of ['/', '/api/items', '/v1/some-resource.json']) expect(() => assertSafePath(path)).not.toThrow()
    for (const path of ['https://evil.test/x', '//evil.test', '/a//b', '/a/../x', '/a/./b', '/%2f%2fevil.test', '/%252f', '/api?token=x', '/api#x', '/api\\x', '/api\n', '/x:443']) expect(() => assertSafePath(path)).toThrow()
  })
  it('标签中的HTML仅作为普通文本数据保留', () => {
    const input = definition(); input.pages[0]!.title = '<img src=x onerror=alert(1)>'
    expect(validateDefinition(input).pages[0]!.title).toBe(input.pages[0]!.title)
  })
})

describe('表单校验与只读展示', () => {
  const fields: Field[] = [
    { key: 'name', label: '名称', type: 'text', required: true },
    { key: 'count', label: '数量', type: 'number', required: true },
    { key: 'enabled', label: '启用', type: 'boolean', required: true },
    { key: 'mode', label: '模式', type: 'select', required: true, options: [{ label: '自动', value: 0 }] },
  ]
  it('数字0、布尔false和数值选项0满足必填', () => {
    expect(validateFormValues(fields, { name: '工作站', count: 0, enabled: false, mode: 0 })).toEqual({})
  })
  it('拒绝空文本、错误类型与不存在的选项', () => {
    expect(Object.keys(validateFormValues(fields, { name: '  ', count: '1', enabled: 'false', mode: 1 }))).toHaveLength(4)
    expect(Object.keys(validateFormValues(fields, {}))).toHaveLength(4)
  })
  it('空值、布尔值和字段自身属性有稳定展示', () => {
    expect(displayValue(null)).toBe('—'); expect(displayValue('')).toBe('—')
    expect(displayValue(false)).toBe('否'); expect(displayValue(0)).toBe('0')
    expect(displayValue({ unexpected: true })).toBe('—')
    expect(ownValue({}, 'constructor')).toBeUndefined()
  })
})
