import { createRouter, createWebHistory } from 'vue-router'
import { api } from './api'

const routes = [
  { path: '/login', component: () => import('./views/Login.vue') },
  {
    path: '/',
    component: () => import('./layout/Main.vue'),
    children: [
      { path: '', component: () => import('./views/Dashboard.vue'), meta: { title: '仪表盘' } },
      { path: 'sources', component: () => import('./views/Sources.vue'), meta: { title: '入站通道' } },
      { path: 'targets', component: () => import('./views/Targets.vue'), meta: { title: '出站通道' } },
      { path: 'routes', component: () => import('./views/Routes.vue'), meta: { title: '路由规则' } },
      { path: 'messages', component: () => import('./views/Messages.vue'), meta: { title: '消息中心' } },
      { path: 'settings', component: () => import('./views/Settings.vue'), meta: { title: '系统设置' } },
    ],
  },
]

const router = createRouter({ history: createWebHistory(), routes })

router.beforeEach(async (to) => {
  if (to.path === '/login') return true
  try {
    const data = await api.get('/api/meta')
    if (!data.authed) return '/login'
    return true
  } catch {
    return '/login'
  }
})

export default router
