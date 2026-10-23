import type { Activation, Online } from './models'

export function formatDate(value: string | null | undefined): string {
  if (!value) return '尚未记录'
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return '时间不可用'
  return new Intl.DateTimeFormat('zh-CN', {
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
    hour12: false,
  }).format(date)
}
export const activationLabels: Record<Activation, string> = {
  pending: '待启用',
  enabled: '已启用',
  disabled: '已禁用',
}
export const onlineLabels: Record<Online, string> = {
  unknown: '未检测',
  online: '在线',
  offline: '离线',
}
export const errorMessage = (error: unknown) =>
  error instanceof Error ? error.message : '操作失败，请稍后重试'
export function servicePath(id: string) {
  return `/services/${encodeURIComponent(id)}`
}
export function dynamicPath(id: string, page: string) {
  return `${servicePath(id)}/pages/${encodeURIComponent(page)}`
}
