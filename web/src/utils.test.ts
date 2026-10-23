import { describe, expect, it } from 'vitest'
import {
  activationLabels,
  dynamicPath,
  errorMessage,
  formatDate,
  onlineLabels,
  servicePath,
} from './utils'

describe('管理界面格式化与路由边界', () => {
  it('启用与在线状态独立显示', () => {
    expect(activationLabels.enabled).toBe('已启用')
    expect(onlineLabels.offline).toBe('离线')
  })
  it('时间空值和无效值不会崩溃', () => {
    expect(formatDate(null)).toBe('尚未记录')
    expect(formatDate('not-a-date')).toBe('时间不可用')
    expect(formatDate('2026-10-22T00:00:00Z')).toContain('2026')
  })
  it('服务参数不会变成额外路径片段', () => {
    expect(servicePath('test/service')).toBe('/services/test%2Fservice')
    expect(dynamicPath('demo', 'x?y=1')).toBe('/services/demo/pages/x%3Fy%3D1')
  })
  it('未知错误使用可读提示', () => {
    expect(errorMessage({})).toBe('操作失败，请稍后重试')
    expect(errorMessage(new Error('服务离线'))).toBe('服务离线')
  })
})
