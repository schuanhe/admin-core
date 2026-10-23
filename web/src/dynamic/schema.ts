import { isIconName, type IconName } from '../components/icons'
import catalog from './catalog.json'

export type Scalar = string | number | boolean | null
export type DataRecord = Record<string, Scalar>

export interface Source { path: string }
export interface Column { key: string; label: string }
export interface Menu { id: string; label: string; page_id: string; icon?: IconName }
export interface Breadcrumb { label: string; page_id?: string }
export interface Action { id: string; label: string; path: string; method: 'POST' | 'DELETE'; confirm?: string }
export interface Option { label: string; value: string | number }
export interface Field extends Column {
  type: 'text' | 'textarea' | 'date' | 'radio' | 'number' | 'select' | 'boolean'
  required?: boolean
  options?: Option[]
  placeholder?: string; help?: string; min?: number; max?: number; max_length?: number; rows?: number
}
export interface Presentation { class_name?: string; style?: Record<string, string> }
export interface Block extends Presentation {
  id: string
  type: 'stack' | 'grid' | 'card' | 'tabs' | 'tab' | 'text' | 'alert' | 'statistic' | 'progress' | 'descriptions' | 'divider' | 'list' | 'page' | 'page-link'
  title?: string; text?: string; value?: string | number; value_key?: string; unit?: string
  tone?: 'info' | 'success' | 'warning' | 'error'; columns?: number; gap?: number
  children?: Block[]; fields?: Column[]; items?: string[]; page_id?: string
}
interface BasePage extends Presentation { id: string; title: string; description?: string; source?: Source; breadcrumbs?: Breadcrumb[]; stylesheet?: string }
export interface TablePageDefinition extends BasePage {
  type: 'table'; source: Source; columns: Column[]; actions?: Action[]; page_size?: number
}
export interface FormPageDefinition extends BasePage {
  type: 'form'; fields: Field[]; submit: { path: string; method: 'POST' | 'PUT' }; submit_label?: string
}
export interface DetailPageDefinition extends BasePage { type: 'detail'; source: Source; fields: Column[] }
export interface ComposePageDefinition extends BasePage { type: 'compose'; blocks: Block[] }
export type BasicPageDefinition = TablePageDefinition | FormPageDefinition | DetailPageDefinition
export type PageDefinition = BasicPageDefinition | ComposePageDefinition
export interface Definition { schema_version: 1; icon?: IconName; menus: Menu[]; pages: PageDefinition[] }
export interface DynamicService {
  id: string; name?: string; base_url: string; activation: string; online: string; definition: unknown
}

export const DEFINITION_MAX_BYTES = catalog.limits.registration_bytes
const dangerous = new Set(['__proto__', 'constructor', 'prototype'])
const identifierPattern = /^[A-Za-z][A-Za-z0-9_-]{0,63}$/

export class DefinitionError extends Error {
  constructor(message: string) { super(message); this.name = 'DefinitionError' }
}

function fail(message: string): never { throw new DefinitionError(message) }

export function isRecord(value: unknown): value is Record<string, unknown> {
  return value !== null && typeof value === 'object' && !Array.isArray(value)
}

/** JSON 只能作为数据使用：先限制资源消耗，再检查每种组件的精确字段。 */
export function assertJSONLimits(value: unknown, maxBytes = DEFINITION_MAX_BYTES, maxFields = catalog.limits.json_fields): void {
  let fields = 0
  const seen = new WeakSet<object>()
  function visit(item: unknown, depth: number, key = ''): void {
    if (depth > catalog.limits.json_depth) fail('JSON 嵌套不能超过 12 层。')
    if (typeof item === 'string' && Array.from(item).length > (key === 'stylesheet' ? 16384 : 2000)) fail('JSON 字符串超过字段长度限制。')
    if (typeof item === 'number' && !Number.isFinite(item)) fail('JSON 包含无效数字。')
    if (item === undefined || typeof item === 'function' || typeof item === 'symbol' || typeof item === 'bigint') fail('包含非 JSON 数据。')
    if (item !== null && typeof item === 'object') {
      if (seen.has(item)) fail('JSON 不得包含循环引用。')
      seen.add(item)
      if (!Array.isArray(item) && Object.getPrototypeOf(item) !== Object.prototype && Object.getPrototypeOf(item) !== null) fail('JSON 对象类型无效。')
      const entries = Object.entries(item)
      fields += entries.length
      if (fields > maxFields) fail(`JSON 字段总数不能超过 ${maxFields}。`)
      for (const [key, child] of entries) {
        if (dangerous.has(key)) fail('JSON 包含不允许的属性名称。')
        if (key.length > 120) fail('JSON 属性名称过长。')
        visit(child, depth + 1, key)
      }
      seen.delete(item)
    }
  }
  visit(value, 1)
  if (new TextEncoder().encode(JSON.stringify(value)).byteLength > maxBytes) fail('JSON 超过允许的大小。')
}

function object(value: unknown, allowed: string[], label: string): Record<string, unknown> {
  if (!isRecord(value)) fail(`${label}必须是对象。`)
  if (Object.keys(value).some(key => !allowed.includes(key))) fail(`${label}包含不支持的字段。`)
  return value
}
function text(value: unknown, label: string, max = 120, allowEmpty = false): asserts value is string {
  if (typeof value !== 'string' || (!allowEmpty && !value.trim()) || Array.from(value).length > max) fail(`${label}必须是${allowEmpty ? '长度不超过' : '非空且不超过'} ${max} 字符的文本。`)
}
function identifier(value: unknown, label: string): asserts value is string {
  if (typeof value !== 'string' || !identifierPattern.test(value) || dangerous.has(value)) fail(`${label}必须以字母开头，仅含字母、数字、下划线或连字符，最多 64 字符。`)
}
function list(value: unknown, label: string, min: number, max: number): unknown[] {
  if (!Array.isArray(value) || value.length < min || value.length > max) fail(`${label}数量必须在 ${min} 至 ${max} 之间。`)
  return value
}
function unique(values: unknown[], key: string, label: string): void {
  const seen = new Set<unknown>()
  for (const value of values) {
    const id = (value as Record<string, unknown>)[key]
    if (seen.has(id)) fail(`${label}不能重复。`)
    seen.add(id)
  }
}

/** 仅允许根相对路径；拒绝编码歧义、路径穿越和 URL 变形。 */
export function assertSafePath(value: unknown): asserts value is string {
  if (typeof value !== 'string' || value.length > 512 || !/^\/[A-Za-z0-9_~./-]*$/.test(value) || value.includes('//') || value.split('/').some(part => part === '.' || part === '..')) {
    fail('接口路径必须为安全的根相对路径，例如 /api/items，不得包含编码、查询、片段、路径穿越或外部地址。')
  }
}

function source(value: unknown): void {
  const obj = object(value, ['path'], '数据源')
  assertSafePath(obj.path)
}
function column(value: unknown): void {
  const obj = object(value, ['key', 'label'], '字段')
  identifier(obj.key, '字段标识')
  text(obj.label, '字段名称')
}
function field(value: unknown): void {
  const obj = object(value, Object.keys(catalog.form_fields), '表单字段')
  identifier(obj.key, '字段标识')
  text(obj.label, '字段名称')
  if (!Object.prototype.hasOwnProperty.call(catalog.form_types, String(obj.type))) fail('不支持此表单字段类型。')
  if (obj.required !== undefined && typeof obj.required !== 'boolean') fail('required 必须为布尔值。')
  if (obj.placeholder !== undefined) text(obj.placeholder, '输入提示', 120, true)
  if (obj.help !== undefined) text(obj.help, '帮助说明', 300, true)
  for (const key of ['min', 'max']) {
    if (obj[key] !== undefined && (obj.type !== 'number' || typeof obj[key] !== 'number' || !Number.isFinite(obj[key]))) fail('min/max 只用于数字字段且必须为有限数字。')
  }
  if (typeof obj.min === 'number' && typeof obj.max === 'number' && obj.min > obj.max) fail('min 不能大于 max。')
  if (obj.max_length !== undefined && (!['text', 'textarea'].includes(String(obj.type)) || typeof obj.max_length !== 'number' || !Number.isInteger(obj.max_length) || obj.max_length < 1 || obj.max_length > 2000)) fail('max_length 只用于文本字段，范围为 1 至 2000。')
  if (obj.rows !== undefined && (obj.type !== 'textarea' || typeof obj.rows !== 'number' || !Number.isInteger(obj.rows) || obj.rows < 2 || obj.rows > 8)) fail('rows 只用于多行文本，范围为 2 至 8。')
  if (obj.type === 'select' || obj.type === 'radio') {
    const options = list(obj.options, '选项', 1, obj.type === 'radio' ? 10 : 100)
    for (const entry of options) {
      const option = object(entry, ['label', 'value'], '下拉选项')
      text(option.label, '选项名称')
      if (typeof option.value !== 'string' && (typeof option.value !== 'number' || !Number.isFinite(option.value))) fail('下拉选项值必须为文本或有限数字。')
    }
    unique(options, 'value', '下拉选项值')
  } else if (obj.options !== undefined) fail('只有下拉或单选字段支持 options。')
}

function presentation(obj: Record<string, unknown>): void {
  if (obj.class_name !== undefined) {
    text(obj.class_name, 'CSS 类名', 128, true)
    const names = obj.class_name.trim().split(/\s+/).filter(Boolean)
    if (names.length > 4 || names.some(name => !/^[A-Za-z][A-Za-z0-9_-]{0,31}$/.test(name))) fail('最多 4 个 CSS 类名，每个最多 32 个 ASCII 字符。')
  }
  if (obj.style !== undefined) {
    if (!isRecord(obj.style) || Object.keys(obj.style).length > catalog.limits.style_properties) fail('style 必须为最多 30 个属性的对象。')
    for (const [key, value] of Object.entries(obj.style)) {
      if (!/^(?:[a-z][a-z-]{0,47}|--service-[a-z][a-z0-9-]{0,31})$/.test(key)) fail('style 使用小写连字符 CSS 属性名或 --service-* 局部变量。')
      text(value, 'CSS 属性值', 256)
    }
  }
}
const blockFields: Record<string, string[]> = Object.fromEntries(Object.entries(catalog.components).map(([type, meta]) => [type, Object.keys(meta.fields)]))
function blocks(input: unknown): void {
  let count = 0
  const ids = new Set<string>()
  function visit(input: unknown, depth: number, parent: string): void {
    if (depth > catalog.limits.block_depth) fail('组件最多嵌套 4 层。')
    for (const value of list(input, '组件', 1, catalog.limits.block_children)) {
      if (!isRecord(value) || typeof value.type !== 'string' || !Object.prototype.hasOwnProperty.call(blockFields, value.type)) fail('组件不在内部白名单中。')
      const obj = object(value, [...Object.keys(catalog.block_common), ...blockFields[value.type]!], '组件')
      identifier(obj.id, '组件标识')
      if (++count > catalog.limits.blocks || ids.has(obj.id)) fail('每页最多 80 个组件，组件标识不能重复。')
      ids.add(obj.id)
      presentation(obj)
      if (obj.title !== undefined) text(obj.title, '组件标题')
      if (obj.text !== undefined) text(obj.text, '组件文本', 2000, true)
      if (obj.unit !== undefined) text(obj.unit, '统计单位', 20, true)
      if (obj.value_key !== undefined) identifier(obj.value_key, '绑定字段')
      if (obj.columns !== undefined && (typeof obj.columns !== 'number' || !Number.isInteger(obj.columns) || obj.columns < 1 || obj.columns > catalog.limits.columns)) fail('组件列数范围为 1 至 4。')
      if (obj.gap !== undefined && (typeof obj.gap !== 'number' || !Number.isInteger(obj.gap) || obj.gap < 0 || obj.gap > catalog.limits.gap)) fail('组件间距范围为 0 至 48 像素。')
      if (['stack', 'grid', 'card', 'tabs', 'tab'].includes(String(obj.type))) {
        if (obj.type === 'tab' && (parent !== 'tabs' || !obj.title)) fail('tab 必须放在 tabs 内并提供标题。')
        if (obj.type === 'tabs') {
          if (!Array.isArray(obj.children) || obj.children.length > catalog.limits.tabs || obj.children.some(child => !isRecord(child) || child.type !== 'tab')) fail('tabs 需要 1 至 8 个 tab 子项。')
        }
        visit(obj.children, depth + 1, String(obj.type))
      } else if (obj.type === 'text' && !obj.text && !obj.value_key) fail('文本组件需要 text 或 value_key。')
      else if (obj.type === 'alert') {
        text(obj.text, '提示内容', 2000)
        if (obj.tone !== undefined && !['info', 'success', 'warning', 'error'].includes(String(obj.tone))) fail('提示 tone 无效。')
      } else if (obj.type === 'statistic' || obj.type === 'progress') {
        if (obj.type === 'statistic') text(obj.title, '统计标题')
        if (obj.value === undefined && !obj.value_key) fail('统计或进度需要 value 或 value_key。')
        if (obj.value !== undefined) {
          if (typeof obj.value === 'string' && obj.type === 'statistic') text(obj.value, '统计值', 120, true)
          else if (typeof obj.value !== 'number' || !Number.isFinite(obj.value) || (obj.type === 'progress' && (obj.value < 0 || obj.value > 100))) fail('统计值必须为文本/数字，进度必须为 0 至 100 的数字。')
        }
      } else if (obj.type === 'descriptions') {
        const fields = list(obj.fields, '描述字段', 1, 30)
        fields.forEach(column); unique(fields, 'key', '描述字段')
      } else if (obj.type === 'list') list(obj.items, '列表项', 0, 30).forEach(item => text(item, '列表文本', 2000))
      else if (obj.type === 'page' || obj.type === 'page-link') {
        identifier(obj.page_id, '本服务页面')
        if (obj.type === 'page-link') text(obj.title, '链接名称')
      }
    }
  }
  visit(input, 1, '')
}
function action(value: unknown): void {
  const obj = object(value, ['id', 'label', 'path', 'method', 'confirm'], '表格操作')
  identifier(obj.id, '操作标识')
  text(obj.label, '操作名称')
  assertSafePath(obj.path)
  if (!['POST', 'DELETE'].includes(String(obj.method))) fail('表格操作仅支持 POST 或 DELETE。')
  if (obj.confirm !== undefined) text(obj.confirm, '确认提示', 2000)
}

export function validateDefinition(input: unknown): Definition {
  assertJSONLimits(input)
  const definition = object(input, ['schema_version', 'icon', 'menus', 'pages'], '页面定义')
  if (definition.icon !== undefined && !isIconName(definition.icon)) fail('服务图标不在内置白名单中。')
  if (definition.schema_version !== 1) fail('不支持此页面定义版本，目前只支持 schema_version=1。')
  const menus = list(definition.menus, '菜单', 0, catalog.limits.menus)
  const pages = list(definition.pages, '页面', 0, catalog.limits.pages)
  for (const value of pages) {
    if (!isRecord(value)) fail('页面必须是对象。')
    const meta = typeof value.type === 'string' && Object.prototype.hasOwnProperty.call(catalog.page_types, value.type) ? catalog.page_types[value.type as keyof typeof catalog.page_types] : null
    const extra = meta ? Object.keys(meta.fields) : null
    if (!extra) fail('不支持的页面类型，只允许 table、form、detail 或 compose。')
    const page = object(value, [...Object.keys(catalog.page_common), ...extra], '页面')
    presentation(page)
    if (page.stylesheet !== undefined) {
      text(page.stylesheet, '页面 CSS', catalog.limits.stylesheet_bytes, true)
      if (new TextEncoder().encode(page.stylesheet).byteLength > catalog.limits.stylesheet_bytes) fail('页面 CSS 不能超过 16 KiB。')
    }
    identifier(page.id, '页面标识')
    text(page.title, '页面标题')
    if (page.description !== undefined) text(page.description, '页面说明', 2000, true)
    if (page.source !== undefined) source(page.source)
    else if (page.type !== 'form' && page.type !== 'compose') fail('表格和详情页面必须提供 source。')
    if (page.type === 'table') {
      const columns = list(page.columns, '表格列', 1, 30)
      columns.forEach(column)
      unique(columns, 'key', '列标识')
      if (page.page_size !== undefined && (typeof page.page_size !== 'number' || !Number.isInteger(page.page_size) || page.page_size < 1 || page.page_size > 100)) fail('表格每页条数必须为 1 至 100 的整数。')
      if (page.actions !== undefined) {
        const actions = list(page.actions, '表格操作', 0, 8)
        actions.forEach(action)
        unique(actions, 'id', '操作标识')
      }
    } else if (page.type === 'form') {
      const fields = list(page.fields, '表单字段', 1, 40)
      fields.forEach(field)
      unique(fields, 'key', '字段标识')
      const submit = object(page.submit, ['path', 'method'], '提交设置')
      assertSafePath(submit.path)
      if (!['POST', 'PUT'].includes(String(submit.method))) fail('表单提交仅支持 POST 或 PUT。')
      if (page.submit_label !== undefined) text(page.submit_label, '提交按钮文本')
    } else if (page.type === 'compose') {
      blocks(page.blocks)
      const needsSource = (items: Block[]): boolean => items.some(block => Boolean(block.value_key) || block.type === 'descriptions' || needsSource(block.children ?? []))
      if (needsSource(page.blocks as Block[]) && !page.source) fail('包含数据绑定或描述列表的组合页面需要 source。')
    }
    else {
      const fields = list(page.fields, '详情字段', 1, 40)
      fields.forEach(column)
      unique(fields, 'key', '字段标识')
    }
  }
  unique(pages, 'id', '页面标识')
  function references(items: unknown): void {
    for (const block of (items ?? []) as Block[]) {
      if (block.type === 'page' || block.type === 'page-link') {
        const target = pages.find(value => (value as Record<string, unknown>).id === block.page_id) as Record<string, unknown> | undefined
        if (!target || (block.type === 'page' && target.type === 'compose')) fail('组件必须引用已有本服务页面；嵌入仅支持 table/form/detail。')
      }
      if (block.children) references(block.children)
    }
  }
  for (const value of pages) references((value as Record<string, unknown>).blocks)
  for (const value of pages) {
    const page = value as Record<string, unknown>
    if (page.breadcrumbs === undefined) continue
    const crumbs = list(page.breadcrumbs, '面包屑', 0, 4)
    const linkedPages = new Set<string>()
    for (const value of crumbs) {
      const crumb = object(value, ['label', 'page_id'], '面包屑')
      text(crumb.label, '面包屑名称')
      if (crumb.page_id !== undefined) {
        identifier(crumb.page_id, '面包屑页面')
        if (crumb.page_id === page.id || linkedPages.has(crumb.page_id) || !pages.some(item => (item as Record<string, unknown>).id === crumb.page_id)) fail('面包屑引用的本服务页面不存在、重复或指向当前页。')
        linkedPages.add(crumb.page_id)
      }
    }
  }
  for (const value of menus) {
    const menu = object(value, ['id', 'label', 'page_id', 'icon'], '菜单')
    if (menu.icon !== undefined && !isIconName(menu.icon)) fail('菜单图标不在内置白名单中。')
    identifier(menu.id, '菜单标识')
    text(menu.label, '菜单名称')
    identifier(menu.page_id, '菜单关联页面')
    if (!pages.some(page => (page as Record<string, unknown>).id === menu.page_id)) fail('菜单关联的页面不存在。')
  }
  unique(menus, 'id', '菜单标识')
  return input as Definition
}

export function displayValue(value: unknown): string {
  if (value === null || value === undefined || value === '') return '—'
  if (typeof value === 'boolean') return value ? '是' : '否'
  return typeof value === 'string' || typeof value === 'number' ? String(value) : '—'
}

/** 只读自身属性，不解释点号路径或原型链。 */
export function ownValue(record: DataRecord, key: string): Scalar | undefined {
  return Object.prototype.hasOwnProperty.call(record, key) ? record[key] : undefined
}

export function validateFormValues(fields: Field[], values: DataRecord): Record<string, string> {
  const errors: Record<string, string> = Object.create(null) as Record<string, string>
  for (const field of fields) {
    const value = ownValue(values, field.key)
    const empty = value === null || value === undefined || value === '' || (typeof value === 'string' && !value.trim())
    if (empty) {
      if (field.required) errors[field.key] = `请填写${field.label}`
      continue
    }
    if (['text', 'textarea'].includes(field.type) && (typeof value !== 'string' || Array.from(value).length > (field.max_length ?? 2000))) errors[field.key] = `${field.label}必须为不超过 ${field.max_length ?? 2000} 字符的文本`
    if (field.type === 'date' && (typeof value !== 'string' || !/^\d{4}-\d{2}-\d{2}$/.test(value) || !Number.isFinite(Date.parse(`${value}T00:00:00Z`)) || new Date(`${value}T00:00:00Z`).toISOString().slice(0, 10) !== value)) errors[field.key] = `${field.label}必须为有效的 YYYY-MM-DD 日期`
    if (field.type === 'number' && (typeof value !== 'number' || !Number.isFinite(value))) errors[field.key] = `${field.label}必须是有效数字`
    if (field.type === 'number' && typeof value === 'number' && ((field.min !== undefined && value < field.min) || (field.max !== undefined && value > field.max))) errors[field.key] = `${field.label}超出允许范围`
    if (field.type === 'boolean' && typeof value !== 'boolean') errors[field.key] = `${field.label}必须是布尔值`
    if ((field.type === 'select' || field.type === 'radio') && !field.options?.some(option => option.value === value)) errors[field.key] = `请选择有效的${field.label}`
  }
  return errors
}
