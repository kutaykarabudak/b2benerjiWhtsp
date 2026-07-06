import { createRouter, createWebHistory } from 'vue-router'
import { useAuthStore } from '@/stores/auth'

const router = createRouter({
  history: createWebHistory(),
  routes: [
    {
      path: '/login',
      name: 'login',
      component: () => import('@/views/LoginView.vue'),
      meta: { public: true }
    },
    {
      path: '/',
      component: () => import('@/layouts/AppLayout.vue'),
      children: [
        { path: '', redirect: '/inbox' },
        { path: 'inbox', name: 'inbox', component: () => import('@/views/InboxView.vue') },
        { path: 'contacts', name: 'contacts', component: () => import('@/views/ContactsView.vue') },
        { path: 'campaigns', name: 'campaigns', component: () => import('@/views/CampaignsView.vue') },
        { path: 'templates', name: 'templates', component: () => import('@/views/TemplatesView.vue') },
        { path: 'catalog', name: 'catalog', component: () => import('@/views/CatalogsView.vue') },
        { path: 'chatbot', name: 'chatbot', component: () => import('@/views/ChatbotView.vue') },
        { path: 'admin', name: 'admin', component: () => import('@/views/AdminView.vue') }
      ]
    },
    { path: '/:pathMatch(.*)*', redirect: '/inbox' }
  ]
})

// Global guard: resolve the current user once, then gate protected routes.
router.beforeEach(async (to) => {
  const auth = useAuthStore()
  if (!auth.ready) await auth.fetchMe()

  if (to.meta.public) {
    if (auth.user && to.name === 'login') return { path: '/inbox' }
    return true
  }
  if (!auth.user) return { path: '/login', query: { redirect: to.fullPath } }
  return true
})

export default router
