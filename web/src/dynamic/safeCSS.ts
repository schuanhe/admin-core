import { parse, generate, walk, type CssNode, type Declaration } from 'css-tree'
import type { CSSProperties } from 'vue'
import catalog from './catalog.json'
const limits = catalog.limits

export class StyleError extends Error {
  constructor(message: string) { super(message); this.name = 'StyleError' }
}
const reject = (message: string): never => { throw new StyleError(message) }
const properties = new Set(catalog.css.properties)
const functions = new Set(catalog.css.functions)
const variables = new Set(catalog.css.variables.map(name => `--${name}`))
const states = new Set(catalog.css.states)
const safeName = /^[A-Za-z][A-Za-z0-9_-]{0,63}$/
const localVariable = /^--service-[a-z][a-z0-9-]{0,31}$/
const media = /^(?:(?:screen|all)\s+and\s+)?\((?:(?:min|max)-width:\s*[1-9]\d{1,3}px|prefers-color-scheme:\s*(?:dark|light)|prefers-reduced-motion:\s*reduce)\)$/i

function parseStrict(value: string, context: 'stylesheet' | 'value'): CssNode {
  // 不接受CSS转义，杜绝混淆后的URL、全局指令或属性名；普通中文文本不受影响。
  if (/[\\<\u0000-\u0008\u000b\u000c\u000e-\u001f\u007f]/.test(value)) reject('CSS 不支持转义、HTML 或控制字符。')
  try {
    const ast = parse(value, { context, parseCustomProperty: true, onParseError: () => reject('CSS 语法无效。') })
    let count = 0
    walk(ast, node => {
      if (++count > limits.css_ast_nodes || node.type === 'Raw' || node.type === 'Url') reject('CSS 包含无效、超限或外部资源规则。')
    })
    return ast
  } catch (error) {
    if (error instanceof StyleError) throw error
    return reject('CSS 语法无效，请检查选择器与声明。')
  }
}

function declaration(property: string, value: CssNode): string {
  if (!properties.has(property) && !localVariable.test(property)) reject(`CSS 属性 ${property} 暂不支持。`)
  const rendered = generate(value)
  if (rendered.length > limits.css_value_length) reject('单个 CSS 属性值不能超过 256 字符。')
  if (property === 'position' && !catalog.css.positions.includes(rendered)) reject('页面 CSS 只允许 static / relative 定位。')
  let count = 0
  walk(value, node => {
    if (++count > limits.css_value_nodes || node.type === 'Raw' || node.type === 'Url') reject('CSS 属性值无效或过于复杂。')
    if (node.type === 'Function') {
      if (!functions.has(node.name.toLowerCase())) reject(`CSS 函数 ${node.name} 不受支持。`)
      if (node.name.toLowerCase() === 'var') {
        const match = /^var\((--[a-z0-9-]+)\)$/.exec(generate(node))
        if (!match || (!variables.has(match[1]!) && !localVariable.test(match[1]!))) reject('var() 只能引用公开主题变量或 --service-* 局部变量，不能提供回退值。')
      }
    }
    if (node.type === 'Dimension' || node.type === 'Number' || node.type === 'Percentage') {
      if (!Number.isFinite(Number(node.value)) || Math.abs(Number(node.value)) > limits.css_number) reject('CSS 数值绝对值不能超过 2000。')
      if (node.type === 'Dimension' && !catalog.css.units.includes(node.unit.toLowerCase())) reject('CSS 长度单位不受支持。')
    }
  })
  // 用浏览器自身做最终属性值语法检查；不把语法错误静默当成成功配置。
  if (typeof CSS !== 'undefined' && !CSS.supports(property, rendered)) reject(`CSS 属性 ${property} 的值无效或当前浏览器不支持。`)
  return rendered
}

export function safeInlineStyle(input?: Record<string, string>): CSSProperties {
  const result: Record<string, string> = Object.create(null)
  if (!input) return result
  if (Object.keys(input).length > limits.style_properties) reject('声明式样式最多 30 个属性。')
  for (const [property, value] of Object.entries(input)) {
    if (typeof value !== 'string' || !value.trim() || value.length > limits.css_value_length) reject('样式值必须是非空 CSS 字符串，最多 256 字符。')
    result[property] = declaration(property, parseStrict(value, 'value'))
  }
  return result as CSSProperties
}

/** 每个选择器都强制增加外层边界；不允许能定义全局名称的at-rule或逃逸选择器。 */
export function compileScopedCSS(input: string, scope: string): string {
  if (!input.trim()) return ''
  if (new TextEncoder().encode(input).byteLength > limits.stylesheet_bytes || !/^s[a-f0-9-]+$/.test(scope)) reject('页面 CSS 超过 16 KiB 或作用域无效。')
  const root = parseStrict(input, 'stylesheet')
  if (root.type !== 'StyleSheet') return reject('需要完整 CSS 样式表。')
  const prefix = `[data-service-scope="${scope}"]`
  let rules = 0
  let declarations = 0
  function compile(nodes: Iterable<CssNode>, depth: number): string {
    if (depth > limits.css_media_depth) return reject('@media 最多嵌套两层。')
    return Array.from(nodes, rule => {
      if (++rules > limits.css_rules) return reject('CSS 规则最多 100 项。')
      if (rule.type === 'Atrule') {
        if (rule.name.toLowerCase() !== 'media' || !rule.prelude || !rule.block) return reject('仅支持 @media，不允许 @import、字体、动画、全局变量或其他指令。')
        const condition = generate(rule.prelude)
        if (!media.test(condition)) return reject('@media 仅支持宽度、明暗主题或减少动态效果的单条件查询。')
        return `@media ${condition}{${compile(rule.block.children, depth + 1)}}`
      }
      if (rule.type !== 'Rule' || rule.prelude.type !== 'SelectorList') return reject('CSS 只支持普通样式规则和 @media。')
      if (rule.prelude.children.size > limits.css_selectors) return reject('单条规则最多 10 个选择器。')
      const selectors = rule.prelude.children.toArray().map(selector => {
        if (selector.type !== 'Selector' || selector.children.first?.type === 'Combinator') return reject('CSS 不能使用相对根选择器。')
        walk(selector, node => {
          if (node.type === 'Selector') return
          if (node.type === 'ClassSelector' && safeName.test(node.name)) return
          if (node.type === 'TypeSelector' && (node.name === '*' || (safeName.test(node.name) && !['html', 'body'].includes(node.name.toLowerCase())))) return
          if (node.type === 'Combinator' && [' ', '>', '+', '~'].includes(node.name)) return
          if (node.type === 'PseudoClassSelector' && states.has(node.name.toLowerCase()) && node.children === null) return
          if (node.type === 'PseudoElementSelector' && catalog.css.pseudo_elements.includes(node.name.toLowerCase()) && node.children === null) return
          return reject('选择器仅支持类名、标签、通配符、组合符及常用状态；不支持全局、属性、ID或函数选择器。')
        })
        return `${prefix} ${generate(selector)}`
      })
      const body = rule.block.children.toArray().map(node => {
        if (++declarations > limits.css_declarations || node.type !== 'Declaration') return reject('CSS 最多 300 个声明，不支持 CSS 嵌套。')
        const item: Declaration = node
        if (item.value.type !== 'Value') return reject('CSS 属性值无效。')
        return `${item.property}:${declaration(item.property, item.value)}${item.important ? '!important' : ''}`
      }).join(';')
      return `${selectors.join(',')}{${body}}`
    }).join('')
  }
  return compile(root.children, 0)
}
