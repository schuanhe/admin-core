import { reactive } from 'vue'
import { api, ApiError, setCSRFToken } from './api'
import type { Service, Session, Settings } from './models'

export const session = reactive({ checked: false, username: '' })
export const workspace = reactive({
  siteName: '个人工作站',
  services: [] as Service[],
  menuError: '',
})
let restorePromise: Promise<boolean> | null = null
let menuGeneration = 0
export const navigationState = reactive({ saving: false })

export function acceptSession(value: Session) {
  session.username = value.user.username
  session.checked = true
  setCSRFToken(value.csrf_token)
}

export function clearSession() {
  session.username = ''
  session.checked = true
  workspace.services = []
  workspace.menuError = ''
  menuGeneration++
  setCSRFToken('')
}

export async function restoreSession(): Promise<boolean> {
  if (session.checked) return Boolean(session.username)
  if (restorePromise) return restorePromise
  restorePromise = (async () => {
    try {
      acceptSession(await api<Session>('/auth/me', { silentUnauthorized: true }))
      return true
    } catch (error) {
      if (error instanceof ApiError && error.status === 401) clearSession()
      return false
    } finally {
      restorePromise = null
    }
  })()
  return restorePromise
}

export async function refreshMenus() {
  if (!session.username || navigationState.saving) return
  const generation = ++menuGeneration
  try {
    const result = await api<{ items: Service[] }>('/services?activation=enabled')
    if (generation !== menuGeneration || !session.username) return
    workspace.services = Array.isArray(result.items) ? result.items : []
    workspace.menuError = ''
  } catch (error) {
    if (generation === menuGeneration)
      workspace.menuError = error instanceof Error ? error.message : '服务菜单加载失败'
  }
}

export function compareNavigation(a: Service, b: Service): number {
  return Number(Boolean(b.pinned)) - Number(Boolean(a.pinned))
    || (a.navigation_order ?? 2147483647) - (b.navigation_order ?? 2147483647)
    || a.created_at.localeCompare(b.created_at) || a.id.localeCompare(b.id)
}

export async function saveNavigation(items: Service[]) {
  if (navigationState.saving) return
  navigationState.saving = true
  menuGeneration++ // 丢弃排序前已经发出的菜单读取，避免旧快照覆盖本次操作。
  try {
    await api('/service-navigation', { method: 'PUT', body: { items: items.map(item => ({ service_id: item.id, pinned: Boolean(item.pinned) })) } })
    if (!session.username) return
    const preferences = new Map(items.map((item, index) => [item.id, { pinned: Boolean(item.pinned), navigation_order: index }]))
    workspace.services = workspace.services.map(item => ({ ...item, ...preferences.get(item.id) }))
  } finally { navigationState.saving = false }
}

export async function loadSiteName() {
  try {
    workspace.siteName = (await api<Settings>('/settings')).site_name
  } catch {
    /* 页面会单独显示相应错误。 */
  }
}
