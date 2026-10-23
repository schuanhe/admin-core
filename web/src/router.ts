import { createRouter, createWebHistory } from 'vue-router'
import { clearSession, restoreSession } from './state'
import ShellLayout from './components/ShellLayout.vue'

export const router = createRouter({
  history: createWebHistory(),
  routes: [
    {
      path: '/login',
      component: () => import('./views/LoginView.vue'),
      meta: { public: true, title: '管理员登录' },
    },
    {
      path: '/',
      component: ShellLayout,
      children: [
        {
          path: '',
          name: 'dashboard',
          component: () => import('./views/DashboardView.vue'),
          meta: { title: '概览' },
        },
        {
          path: 'services',
          name: 'services',
          component: () => import('./views/ServicesView.vue'),
          meta: { title: '服务管理' },
        },
        {
          path: 'services/:id',
          component: () => import('./views/ServiceDetailView.vue'),
          meta: { title: '服务详情' },
        },
        {
          path: 'services/:id/pages/:pageId',
          component: () => import('./views/DynamicPageView.vue'),
          meta: { title: '服务工作台' },
        },
        {
          path: 'settings',
          component: () => import('./views/SettingsView.vue'),
          meta: { title: '系统设置' },
        },
        {
          path: 'audit',
          component: () => import('./views/AuditView.vue'),
          meta: { title: '操作审计' },
        },
        {
          path: ':pathMatch(.*)*',
          component: () => import('./views/NotFoundView.vue'),
          meta: { title: '页面未找到' },
        },
      ],
    },
  ],
})

router.beforeEach(async (to) => {
  const authenticated = await restoreSession()
  if (!to.meta.public && !authenticated) return { path: '/login', query: { redirect: to.fullPath } }
  if (to.path === '/login' && authenticated) return '/'
})
router.afterEach((to) => {
  document.title = `${String(to.meta.title ?? '工作站')} · 控制中心`
})
window.addEventListener('session-expired', () => {
  clearSession()
  if (router.currentRoute.value.path !== '/login')
    void router.replace({ path: '/login', query: { expired: '1' } })
})
