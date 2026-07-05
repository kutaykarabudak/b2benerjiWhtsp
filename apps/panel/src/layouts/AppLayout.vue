<script setup lang="ts">
import { useRouter } from 'vue-router'
import { useAuthStore } from '@/stores/auth'

const auth = useAuthStore()
const router = useRouter()

const nav = [
  { to: '/inbox', label: 'Gelen Kutusu', icon: '💬' },
  { to: '/contacts', label: 'Kişiler', icon: '👥' },
  { to: '/campaigns', label: 'Toplu Mesaj', icon: '📣' },
  { to: '/chatbot', label: 'Chatbot', icon: '🤖' },
  { to: '/admin', label: 'Yönetim', icon: '⚙️' }
]

async function logout() {
  await auth.logout()
  router.push('/login')
}
</script>

<template>
  <div class="shell">
    <aside class="sidebar">
      <div class="brand">B2B Panel</div>
      <nav>
        <router-link v-for="item in nav" :key="item.to" :to="item.to" class="nav-item">
          <span class="icon">{{ item.icon }}</span>{{ item.label }}
        </router-link>
      </nav>
      <div class="sidebar-footer">
        <div class="who muted">{{ auth.user?.email }}</div>
        <button @click="logout">Çıkış Yap</button>
      </div>
    </aside>
    <main class="content">
      <router-view />
    </main>
  </div>
</template>

<style scoped>
.shell { display: flex; height: 100%; }
.sidebar {
  width: 220px;
  flex-shrink: 0;
  background: var(--panel);
  border-right: 1px solid var(--border);
  display: flex;
  flex-direction: column;
  padding: 16px 12px;
}
.brand { font-weight: 700; font-size: 16px; padding: 4px 8px 16px; }
nav { display: flex; flex-direction: column; gap: 2px; flex: 1; }
.nav-item {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 9px 10px;
  border-radius: var(--radius);
  color: var(--muted);
}
.nav-item:hover { background: var(--bg); color: var(--text); }
.nav-item.router-link-active { background: var(--bg); color: var(--brand); font-weight: 600; }
.icon { width: 18px; text-align: center; }
.sidebar-footer { display: flex; flex-direction: column; gap: 8px; padding-top: 12px; border-top: 1px solid var(--border); }
.who { font-size: 12px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.content { flex: 1; overflow: auto; }
</style>
