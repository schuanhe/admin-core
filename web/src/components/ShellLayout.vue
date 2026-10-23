<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { NButton, NTooltip, useMessage } from 'naive-ui'
import AppIcon from './AppIcon.vue'
import ThemeToggle from './ThemeToggle.vue'
import { api } from '../api'
import { session, workspace, clearSession, refreshMenus, loadSiteName, compareNavigation, saveNavigation, navigationState } from '../state'
import { dynamicPath, errorMessage, servicePath } from '../utils'
import { validateDefinition } from '../dynamic/schema'
import type { Service } from '../models'

const route = useRoute(),
  router = useRouter(),
  message = useMessage()
const mobileOpen = ref(false),
  loggingOut = ref(false)
const collapsed = ref<Record<string, boolean>>(Object.create(null))
const dragging = ref(''), dropTarget = ref('')
async function persistNavigation(items: Service[]) {
  try { await saveNavigation(items) }
  catch (error) { message.error(`导航未保存：${errorMessage(error)}`); void refreshMenus() }
}
async function pinService(service: Service) {
  const items = [...workspace.services].sort(compareNavigation).map(item => item.id === service.id ? { ...item, pinned: !item.pinned } : item)
  // 固定状态先分组，分组内部保留原有相对顺序。
  items.sort((a, b) => Number(Boolean(b.pinned)) - Number(Boolean(a.pinned)))
  await persistNavigation(items)
}
function dragStart(event: DragEvent, id: string) {
  if (navigationState.saving || !event.dataTransfer) { event.preventDefault(); return }
  dragging.value = id
  event.dataTransfer.effectAllowed = 'move'
  event.dataTransfer.setData('text/plain', id)
}
function canDrop(id: string) {
  const source = workspace.services.find(item => item.id === dragging.value)
  const target = workspace.services.find(item => item.id === id)
  return !navigationState.saving && source && target && source.id !== target.id && Boolean(source.pinned) === Boolean(target.pinned)
}
function dragOver(event: DragEvent, id: string) {
  if (!canDrop(id)) { dropTarget.value = ''; return }
  event.preventDefault()
  if (event.dataTransfer) event.dataTransfer.dropEffect = 'move'
  dropTarget.value = id
}
function dropAfter(id: string) {
  const items = [...workspace.services].sort(compareNavigation)
  return items.findIndex(item => item.id === dragging.value) < items.findIndex(item => item.id === id)
}
async function dropService(event: DragEvent, id: string) {
  event.preventDefault()
  if (!canDrop(id)) return
  const items = [...workspace.services].sort(compareNavigation)
  const from = items.findIndex(item => item.id === dragging.value), to = items.findIndex(item => item.id === id)
  const [item] = items.splice(from, 1)
  if (item) items.splice(to, 0, item)
  dragging.value = ''; dropTarget.value = ''
  await persistNavigation(items)
}
async function moveService(id: string, direction: number) {
  if (navigationState.saving) return
  const items = [...workspace.services].sort(compareNavigation)
  const index = items.findIndex(item => item.id === id), target = index + direction
  if (index < 0 || !items[target] || Boolean(items[index]!.pinned) !== Boolean(items[target]!.pinned)) return
  ;[items[index], items[target]] = [items[target]!, items[index]!]
  await persistNavigation(items)
}
try {
  const saved: unknown = JSON.parse(localStorage.getItem('workstation.collapsed-services') ?? '{}')
  if (saved && typeof saved === 'object' && !Array.isArray(saved)) {
    for (const [id, value] of Object.entries(saved)) {
      if (/^[a-z0-9][a-z0-9-]{0,63}$/.test(id) && value === true) collapsed.value[id] = true
    }
  }
} catch { /* 浏览器禁止存储时仍可使用菜单。 */ }
function toggleService(id: string) {
  collapsed.value[id] = !collapsed.value[id]
  try { localStorage.setItem('workstation.collapsed-services', JSON.stringify(collapsed.value)) } catch { /* 只影响偏好保存。 */ }
}
watch(() => [route.params.id, route.params.pageId], () => {
  if (route.params.pageId && typeof route.params.id === 'string') collapsed.value[route.params.id] = false
}, { immediate: true })
const nav = [
  { path: '/', label: '工作站概览', icon: 'overview' },
  { path: '/services', label: '服务管理', icon: 'services' },
  { path: '/audit', label: '操作审计', icon: 'audit' },
  { path: '/settings', label: '系统设置', icon: 'settings' },
]
const serviceMenus = computed(() =>
  workspace.services
    .filter((service) => service.activation === 'enabled')
    .sort(compareNavigation)
    .map((service) => {
      try {
        const definition = validateDefinition(service.definition)
        return {
          ...service,
          icon: definition.icon ?? 'services',
          menus: definition.menus.map(menu => {
            const type = definition.pages.find(page => page.id === menu.page_id)?.type
            return { ...menu, icon: menu.icon ?? (type === 'compose' ? 'overview' : type ?? 'page') }
          }),
          invalidDefinition: false,
        }
      } catch {
        return { ...service, icon: 'services', menus: [], invalidDefinition: true }
      }
    }),
)
const loadedService = ref<Service | null>(null)
let contextGeneration = 0
watch(() => route.params.id, async (id) => {
  const generation = ++contextGeneration
  loadedService.value = null
  if (typeof id !== 'string') return
  try {
    const result = await api<{ service: Service }>(servicePath(id))
    if (generation === contextGeneration) loadedService.value = result.service
  } catch { /* 页面自身负责显示加载错误。 */ }
}, { immediate: true })
const currentService = computed(() => workspace.services.find(service => service.id === route.params.id) ?? loadedService.value)
const breadcrumbs = computed(() => {
  const items: { label: string; to?: string }[] = [{ label: workspace.siteName, to: '/' }]
  const service = currentService.value
  if (route.params.id) {
    if (!route.params.pageId) items.push({ label: '服务管理', to: '/services' })
    items.push({ label: service?.name ?? '服务详情', to: route.params.pageId ? servicePath(String(route.params.id)) : undefined })
    if (route.params.pageId) {
      try {
        const definition = validateDefinition(service?.definition)
        const page = definition.pages.find(page => page.id === route.params.pageId)
        for (const crumb of page?.breadcrumbs ?? []) {
          items.push({ label: crumb.label, to: crumb.page_id ? dynamicPath(service!.id, crumb.page_id) : undefined })
        }
        items.push({ label: page?.title ?? '页面不存在' })
      } catch { items.push({ label: '服务页面' }) }
    }
  } else items.push({ label: String(route.meta.title ?? '工作站') })
  return items
})
watch(breadcrumbs, (items) => {
  document.title = `${items[items.length - 1]?.label ?? '工作站'} · ${workspace.siteName}`
}, { immediate: true, flush: 'post' })
let timer: ReturnType<typeof setInterval> | undefined
onMounted(() => {
  void refreshMenus()
  void loadSiteName()
  timer = setInterval(() => {
    if (!document.hidden) void refreshMenus()
  }, 30000)
})
onUnmounted(() => { clearInterval(timer); contextGeneration++ })
async function logout() {
  loggingOut.value = true
  try {
    await api('/auth/logout', { method: 'POST' })
    clearSession()
    await router.replace('/login')
  } catch (error) {
    message.error(errorMessage(error))
  } finally {
    loggingOut.value = false
  }
}
function active(path: string) {
  return path === '/'
    ? route.path === '/'
    : path === '/services'
      ? route.path.startsWith('/services') && !route.path.includes('/pages/')
      : route.path.startsWith(path)
}
</script>
<template>
  <div class="app-shell">
    <a class="skip-link" href="#main-content">跳至主要内容</a>
    <button
      v-if="mobileOpen"
      class="nav-overlay"
      aria-label="关闭导航"
      @click="mobileOpen = false"
    />
    <aside class="sidebar" :class="{ open: mobileOpen }">
      <RouterLink to="/" class="brand" @click="mobileOpen = false"
        ><span class="brand-icon"><AppIcon name="workstation" :size="24" /></span
        ><span><strong>工作站</strong><small>个人控制中心</small></span
        ><span class="brand-version">v1</span></RouterLink
      >
      <div class="nav-section-label">控制中心</div>
      <nav class="main-nav" aria-label="主导航">
        <RouterLink
          v-for="item in nav"
          :key="item.path"
          :to="item.path"
          :class="{ active: active(item.path) }"
          :aria-current="active(item.path) ? 'page' : undefined"
          @click="mobileOpen = false"
          ><AppIcon :name="item.icon" /><span>{{ item.label }}</span
          ><span v-if="item.path === '/services' && workspace.services.length" class="nav-count">{{
            workspace.services.length
          }}</span></RouterLink
        >
      </nav>
      <div class="service-navigation">
        <div class="nav-section-label">
          服务工作台
          <button title="刷新服务菜单" aria-label="刷新服务菜单" @click="refreshMenus">
            <AppIcon name="refresh" :size="14" />
          </button>
        </div>
        <p v-if="workspace.menuError" class="nav-hint warning">菜单同步失败，可点击刷新重试。</p>
        <p v-else-if="!serviceMenus.length" class="nav-hint">启用服务后，服务菜单会显示在这里。</p>
        <p v-if="navigationState.saving" class="nav-hint" role="status">正在保存导航…</p>
        <div v-for="service in serviceMenus" :key="service.id" class="service-nav-group"
          :class="{ 'drag-target': dropTarget === service.id, 'drop-after': dropTarget === service.id && dropAfter(service.id), dragging: dragging === service.id }"
          @dragover="dragOver($event, service.id)" @drop="dropService($event, service.id)">
          <div class="service-nav-header">
          <button type="button" class="service-nav-tool drag-handle" :disabled="navigationState.saving" draggable="true"
            :aria-label="`调整${service.name}顺序，按Alt加上下方向键移动`" title="拖动排序；Alt+↑ / ↓ 移动（固定与普通服务分组排序）"
            @dragstart="dragStart($event, service.id)" @dragend="dragging = ''; dropTarget = ''"
            @keydown.alt.up.prevent="moveService(service.id, -1)" @keydown.alt.down.prevent="moveService(service.id, 1)">
            <AppIcon name="grip" :size="16" />
          </button>
          <button type="button" class="service-nav-title service-nav-toggle"
            :aria-expanded="!collapsed[service.id]" :aria-controls="`service-menu-${service.id}`"
            @click="toggleService(service.id)">
            <AppIcon :name="service.icon" :size="17" />
            <span class="service-nav-name" :title="service.name">{{ service.name }}</span>
            <span class="status-dot" :class="service.online" :title="service.online === 'online' ? '在线' : service.online === 'offline' ? '离线' : '尚未检测'" />
            <svg class="service-nav-chevron" :class="{ collapsed: collapsed[service.id] }" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7" aria-hidden="true"><path d="m6 9 6 6 6-6" /></svg>
          </button>
          <button type="button" class="service-nav-tool" :class="{ pinned: service.pinned }" :disabled="navigationState.saving"
            :aria-pressed="Boolean(service.pinned)" :aria-label="`${service.pinned ? '取消固定' : '固定'}${service.name}`" :title="service.pinned ? '取消固定' : '固定到顶部'"
            @click="pinService(service)"><AppIcon name="pin" :size="15" /></button>
          </div>
          <div :id="`service-menu-${service.id}`" v-show="!collapsed[service.id]" class="service-nav-items">
          <RouterLink
            v-for="menu in service.menus"
            :key="menu.id"
            :to="dynamicPath(service.id, menu.page_id)"
            :class="{ active: route.path === dynamicPath(service.id, menu.page_id) }"
            :aria-current="route.path === dynamicPath(service.id, menu.page_id) ? 'page' : undefined"
            @click="mobileOpen = false"
            ><AppIcon :name="menu.icon" :size="16" /><span>{{ menu.label }}</span></RouterLink
          >
          <p v-if="!service.menus.length" class="nav-hint compact">
            {{ service.invalidDefinition ? '定义无效，请到服务管理查看' : '该服务未提供菜单' }}
          </p>
          </div>
        </div>
      </div>
    </aside>
    <div class="workspace">
      <header class="topbar">
        <div class="topbar-left">
          <NButton
            class="mobile-toggle"
            quaternary
            circle
            aria-label="打开导航"
            @click="mobileOpen = !mobileOpen"
            ><AppIcon name="menu" /></NButton
          >
          <nav class="breadcrumbs" aria-label="面包屑">
            <ol>
              <li v-for="(item, index) in breadcrumbs" :key="index">
                <span v-if="index" class="breadcrumb-divider" aria-hidden="true">/</span>
                <RouterLink v-if="item.to && index < breadcrumbs.length - 1" :to="item.to" :title="item.label">{{ item.label }}</RouterLink>
                <span v-else :class="{ 'breadcrumb-current': index === breadcrumbs.length - 1 }" :aria-current="index === breadcrumbs.length - 1 ? 'page' : undefined" :title="item.label">{{ item.label }}</span>
              </li>
            </ol>
          </nav>
        </div>
        <div class="topbar-right">
          <ThemeToggle />
          <span class="toolbar-divider" aria-hidden="true" />
          <span class="account-avatar">{{ session.username.slice(0, 1).toUpperCase() }}</span
          ><span class="account-name">{{ session.username }}<small>管理员</small></span
          ><NTooltip
            ><template #trigger
              ><NButton
                quaternary
                circle
                :loading="loggingOut"
                aria-label="退出登录"
                @click="logout"
                ><AppIcon name="logout" :size="19" /></NButton></template
            >退出登录</NTooltip
          >
        </div>
      </header>
      <main id="main-content" class="main-content" tabindex="-1">
        <RouterView v-slot="{ Component }"
          ><component :is="Component" :key="route.fullPath"
        /></RouterView>
      </main>
    </div>
  </div>
</template>

<style scoped>
.service-nav-header { display: flex; align-items: center; min-width: 0; }
.service-nav-tool { display: inline-flex; align-items: center; justify-content: center; flex-shrink: 0; width: 26px; min-height: 44px; padding: 0; border: 0; border-radius: 6px; background: transparent; color: var(--text-tertiary); cursor: pointer; }
.service-nav-tool:hover, .service-nav-tool:focus-visible { color: var(--text-primary); background: var(--surface-hover); }
.service-nav-tool:focus-visible { outline: 2px solid var(--accent); outline-offset: -2px; }
.service-nav-tool.pinned { color: var(--accent-text); }
.service-nav-tool:disabled { cursor: wait; opacity: .5; }
.drag-handle { cursor: grab; }
.drag-handle:active { cursor: grabbing; }
.service-nav-group.drag-target { box-shadow: 0 -2px 0 var(--accent); }
.service-nav-group.drag-target.drop-after { box-shadow: 0 2px 0 var(--accent); }
.service-nav-group.dragging { opacity: .55; }
.service-nav-header .service-nav-toggle { width: auto; flex: 1; min-width: 0; padding-left: 5px; padding-right: 5px; }
.service-nav-toggle { width: 100%; min-height: 44px; border: 0; background: transparent; border-radius: var(--radius-nav); text-align: left; cursor: pointer; font: inherit; font-size: 12px; color: var(--text-secondary); gap: 9px; }
.service-nav-toggle:hover { background: var(--surface-hover); color: var(--text-primary); }
.service-nav-toggle:focus-visible { outline: 2px solid var(--accent); outline-offset: 2px; }
.service-nav-toggle > svg, .service-nav-toggle .status-dot { flex-shrink: 0; }
.service-nav-name { flex: 1; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.service-nav-chevron.collapsed { transform: rotate(-90deg); }
.service-nav-items { margin-left: 12px; border-left: 1px solid var(--border-subtle); padding-left: 5px; }
.service-nav-items a { padding-left: 13px; }
.topbar-left { min-width: 0; flex: 1; }
.breadcrumbs { min-width: 0; overflow-x: auto; }
.breadcrumbs ol { display: flex; align-items: center; gap: 10px; padding: 4px 0; margin: 0; list-style: none; width: max-content; }
.breadcrumbs li { display: flex; align-items: center; gap: 10px; min-width: 0; font-size: 13px; color: var(--text-secondary); }
.breadcrumbs li > a, .breadcrumbs li > span:not(.breadcrumb-divider) { max-width: 180px; overflow: hidden; white-space: nowrap; text-overflow: ellipsis; }
.breadcrumbs a { color: var(--text-secondary); border-radius: 3px; }
.breadcrumbs a:hover { color: var(--accent-text); text-decoration: underline; }
.breadcrumbs a:focus-visible { outline: 2px solid var(--accent); outline-offset: 2px; }
.breadcrumbs .breadcrumb-divider { display: inline; color: var(--text-tertiary); }
@media (max-width: 680px) { .breadcrumbs li > a, .breadcrumbs li > span:not(.breadcrumb-divider) { max-width: 120px; } }
</style>
