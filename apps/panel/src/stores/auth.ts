import { defineStore } from 'pinia'
import { ref } from 'vue'
import { api } from '@/services/api'

export interface User {
  id: string
  email: string
  first_name?: string
  last_name?: string
  is_active?: boolean
  role?: { name?: string } | null
}

export const useAuthStore = defineStore('auth', () => {
  const user = ref<User | null>(null)
  const ready = ref(false)

  async function login(email: string, password: string) {
    const res = await api.post('/auth/login', { email, password })
    user.value = res.data?.user ?? null
    return user.value
  }

  // Cookies are HttpOnly, so we can't read them; ask the server who we are.
  async function fetchMe() {
    try {
      const res = await api.get('/me')
      user.value = res.data ?? null
    } catch {
      user.value = null
    } finally {
      ready.value = true
    }
    return user.value
  }

  async function logout() {
    try {
      await api.post('/auth/logout')
    } finally {
      user.value = null
    }
  }

  return { user, ready, login, fetchMe, logout }
})
