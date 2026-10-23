import catalog from '../dynamic/catalog.json'

// 图标路径与协议手册来自同一内置注册表，不接受服务提供的SVG或URL。
export const iconPaths = catalog.icons
export type IconName = keyof typeof iconPaths
export function isIconName(value: unknown): value is IconName {
  return typeof value === 'string' && Object.prototype.hasOwnProperty.call(iconPaths, value)
}
