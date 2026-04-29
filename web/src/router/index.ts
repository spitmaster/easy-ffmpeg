import { createRouter, createWebHashHistory, type RouteRecordRaw } from 'vue-router'

// Hash mode keeps Go's http.FileServer happy: every URL still resolves to
// `/` server-side, so SPA fallback is unnecessary.
const routes: RouteRecordRaw[] = [
  { path: '/', redirect: '/convert' },
  { path: '/convert', name: 'convert', component: () => import('@/views/ConvertView.vue') },
  { path: '/audio', name: 'audio', component: () => import('@/views/AudioView.vue') },
  { path: '/editor', name: 'editor', component: () => import('@/views/EditorView.vue') },
]

export const router = createRouter({
  history: createWebHashHistory(),
  routes,
})
