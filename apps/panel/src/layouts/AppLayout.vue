<script setup lang="ts">
import { ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useAuthStore } from '@/stores/auth'

const auth = useAuthStore()
const router = useRouter()
const route = useRoute()

const nav = [
  { to: '/inbox', label: 'Gelen Kutusu', icon: '💬' },
  { to: '/contacts', label: 'Kişiler', icon: '👥' },
  { to: '/campaigns', label: 'Toplu Mesaj', icon: '📣' },
  { to: '/templates', label: 'Şablonlar', icon: '📄' },
  { to: '/chatbot', label: 'Chatbot', icon: '🤖' },
  { to: '/admin', label: 'Yönetim', icon: '⚙️' }
]

const drawerOpen = ref(false)

// Close the mobile drawer whenever the route changes.
watch(() => route.fullPath, () => (drawerOpen.value = false))

const currentLabel = () => nav.find((n) => route.path.startsWith(n.to))?.label ?? 'B2B Panel'

async function logout() {
  await auth.logout()
  router.push('/login')
}
</script>

<template>
  <div class="shell">
    <!-- Mobile top bar with hamburger -->
    <header class="mobile-topbar">
      <button class="hamburger" aria-label="Menü" @click="drawerOpen = true">☰</button>
      <span class="mt-title">{{ currentLabel() }}</span>
    </header>

    <!-- Backdrop for the mobile drawer -->
    <div v-if="drawerOpen" class="backdrop" @click="drawerOpen = false"></div>

    <aside class="sidebar" :class="{ open: drawerOpen }">
      <div class="brand">B2B Panel</div>
      <nav>
        <router-link
          v-for="item in nav"
          :key="item.to"
          :to="item.to"
          class="nav-item"
          @click="drawerOpen = false"
        >
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

.mobile-topbar { display: none; }

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
  padding: 11px 10px;
  border-radius: var(--radius);
  color: var(--muted);
}
.nav-item:hover { background: var(--bg); color: var(--text); }
.nav-item.router-link-active { background: var(--bg); color: var(--brand); font-weight: 600; }
.icon { width: 18px; text-align: center; }
.sidebar-footer { display: flex; flex-direction: column; gap: 8px; padding-top: 12px; border-top: 1px solid var(--border); }
.who { font-size: 12px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.content { flex: 1; overflow: auto; min-width: 0; }

/* --- Mobile: sidebar becomes a hamburger drawer --- */
@media (max-width: 768px) {
  .shell { flex-direction: column; }
  .mobile-topbar {
    display: flex;
    align-items: center;
    gap: 14px;
    height: 54px;
    flex-shrink: 0;
    padding: 0 14px;
    background: var(--brand);
    color: #fff;
  }
  .hamburger {
    background: transparent;
    border: none;
    color: #fff;
    font-size: 24px;
    line-height: 1;
    padding: 4px;
    cursor: pointer;
  }
  .mt-title { font-weight: 600; font-size: 17px; }

  .sidebar {
    position: fixed;
    top: 0;
    left: 0;
    bottom: 0;
    width: 264px;
    z-index: 50;
    transform: translateX(-100%);
    transition: transform 0.22s ease;
    box-shadow: 2px 0 12px rgba(0, 0, 0, 0.15);
  }
  .sidebar.open { transform: translateX(0); }
  .backdrop { position: fixed; inset: 0; background: rgba(0, 0, 0, 0.45); z-index: 40; }
  .content { flex: 1; min-height: 0; }
}
</style>
