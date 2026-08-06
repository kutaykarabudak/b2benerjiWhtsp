<script setup lang="ts">
import { ref, watch, onMounted, onBeforeUnmount } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useAuthStore } from '@/stores/auth'
import { useRealtimeStore } from '@/stores/realtime'

const auth = useAuthStore()
const router = useRouter()
const route = useRoute()
const realtime = useRealtimeStore()

const nav = [
  { to: '/inbox', label: 'Gelen Kutusu', icon: 'inbox' },
  { to: '/contacts', label: 'Kişiler', icon: 'contacts' },
  { to: '/campaigns', label: 'Toplu Mesaj', icon: 'campaign' },
  { to: '/templates', label: 'Şablonlar', icon: 'template' },
  { to: '/catalog', label: 'Katalog', icon: 'catalog' },
  { to: '/chatbot', label: 'Chatbot', icon: 'bot' },
  { to: '/analytics', label: 'Analitik', icon: 'analytics' },
  { to: '/admin', label: 'Yönetim', icon: 'settings' }
]

const icons: Record<string, string> = {
  inbox: '<path d="M4 5h16v14H4z"/><path d="M4 14h4l2 2h4l2-2h4"/>',
  contacts: '<path d="M16 21v-2a4 4 0 0 0-4-4H6a4 4 0 0 0-4 4v2"/><circle cx="9" cy="7" r="4"/><path d="M22 21v-2a4 4 0 0 0-3-3.87M16 3.13a4 4 0 0 1 0 7.75"/>',
  campaign: '<path d="m3 11 18-5v12L3 14z"/><path d="M11.6 16.4 13 21H8l-1.5-6"/>',
  template: '<path d="M6 2h9l5 5v15H6z"/><path d="M14 2v6h6M9 13h8M9 17h6"/>',
  catalog: '<path d="M6 8h12l1 13H5z"/><path d="M9 10V6a3 3 0 0 1 6 0v4"/>',
  bot: '<rect x="4" y="7" width="16" height="13" rx="3"/><path d="M12 3v4M8 12h.01M16 12h.01M8 16h8"/>',
  analytics: '<path d="M4 20V10M10 20V4M16 20v-7M22 20H2"/>',
  settings: '<circle cx="12" cy="12" r="3"/><path d="M19.4 15a1.7 1.7 0 0 0 .34 1.88l.06.06-2.83 2.83-.06-.06A1.7 1.7 0 0 0 15 19.4a1.7 1.7 0 0 0-1 .6 1.7 1.7 0 0 0-.4 1.1V21H9.6v-.1A1.7 1.7 0 0 0 8.5 19.4a1.7 1.7 0 0 0-1.88.34l-.06.06-2.83-2.83.06-.06A1.7 1.7 0 0 0 4.6 15a1.7 1.7 0 0 0-1.5-1H3v-4h.1A1.7 1.7 0 0 0 4.6 9a1.7 1.7 0 0 0-.34-1.88l-.06-.06 2.83-2.83.06.06A1.7 1.7 0 0 0 9 4.6a1.7 1.7 0 0 0 1-1.5V3h4v.1A1.7 1.7 0 0 0 15 4.6a1.7 1.7 0 0 0 1.88-.34l.06-.06 2.83 2.83-.06.06A1.7 1.7 0 0 0 19.4 9a1.7 1.7 0 0 0 1.5 1h.1v4h-.1a1.7 1.7 0 0 0-1.5 1Z"/>'
}

const drawerOpen = ref(false)

// Close the mobile drawer whenever the route changes.
watch(() => route.fullPath, () => (drawerOpen.value = false))
watch(() => realtime.unreadCount, (count) => {
  document.title = count > 0 ? `(${count}) B2B WhatsApp` : 'B2B WhatsApp Panel'
}, { immediate: true })

const currentLabel = () => nav.find((n) => route.path.startsWith(n.to))?.label ?? 'B2B Panel'

async function logout() {
  await auth.logout()
  router.push('/login')
}

function openToast(contactId?: string) {
  router.push(contactId ? { path: '/inbox', query: { contact: contactId } } : '/inbox')
}

onMounted(() => realtime.start())
onBeforeUnmount(() => realtime.stop())
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
      <div class="brand">
        <span class="brand-mark">B</span>
        <span><strong>B2B</strong><small>WhatsApp Panel</small></span>
      </div>
      <div class="connection-pill" :class="realtime.status">
        <span class="live-dot"></span>{{ realtime.statusLabel }}
      </div>
      <nav>
        <router-link
          v-for="item in nav"
          :key="item.to"
          :to="item.to"
          class="nav-item"
          @click="drawerOpen = false"
        >
          <svg class="icon" viewBox="0 0 24 24" aria-hidden="true" v-html="icons[item.icon]"></svg>
          <span>{{ item.label }}</span>
          <span v-if="item.to === '/inbox' && realtime.unreadCount" class="nav-badge">{{ realtime.unreadCount > 99 ? '99+' : realtime.unreadCount }}</span>
        </router-link>
      </nav>
      <div class="sidebar-footer">
        <button
          v-if="realtime.notificationPermission === 'default'"
          class="notification-cta"
          @click="realtime.requestNotifications"
        >
          <span>🔔</span><span><b>Bildirimleri aç</b><small>Yeni mesajları kaçırma</small></span>
        </button>
        <div class="who muted">{{ auth.user?.email }}</div>
        <button class="logout-btn" @click="logout">Çıkış Yap</button>
      </div>
    </aside>

    <main class="content">
      <router-view />
    </main>

    <div class="toast-stack" aria-live="polite">
      <button
        v-for="toast in realtime.toasts"
        :key="toast.id"
        class="message-toast"
        @click="openToast(toast.contactId); realtime.dismissToast(toast.id)"
      >
        <span class="toast-icon">W</span>
        <span class="toast-copy"><b>{{ toast.title }}</b><small>{{ toast.body }}</small></span>
        <span class="toast-close" @click.stop="realtime.dismissToast(toast.id)">×</span>
      </button>
    </div>
  </div>
</template>

<style scoped>
.shell { display: flex; height: 100%; background: var(--bg); }

.mobile-topbar { display: none; }

.sidebar {
  width: 252px;
  flex-shrink: 0;
  background: var(--panel);
  border-right: 1px solid var(--border);
  display: flex;
  flex-direction: column;
  padding: 18px 14px 14px;
}
.brand {
  display: flex;
  align-items: center;
  gap: 10px;
  font-weight: 700;
  font-size: 16px;
  padding: 4px 7px 16px;
}
.brand > span:last-child { display: flex; flex-direction: column; line-height: 1.15; }
.brand small { color: var(--muted); font-size: 10px; font-weight: 600; letter-spacing: .04em; text-transform: uppercase; margin-top: 3px; }
.brand-mark {
  display: grid;
  place-items: center;
  width: 30px;
  height: 30px;
  border-radius: 10px;
  background: linear-gradient(145deg, #25d366, #07875e);
  color: #fff;
  font-size: 15px;
  box-shadow: 0 2px 6px rgba(13, 150, 104, 0.35);
}
.connection-pill { display: flex; align-items: center; gap: 7px; margin: 0 6px 16px; padding: 7px 10px; border-radius: 10px; font-size: 11px; color: var(--muted); background: var(--bg-2); }
.live-dot { width: 7px; height: 7px; border-radius: 50%; background: #a8b0bd; }
.connection-pill.connected { color: #087a55; background: var(--brand-soft); }
.connection-pill.connected .live-dot { background: #20c77a; box-shadow: 0 0 0 4px rgba(32,199,122,.12); }
.connection-pill.connecting .live-dot { background: #f2a93b; animation: pulse 1.4s infinite; }
@keyframes pulse { 50% { opacity: .35; } }
nav { display: flex; flex-direction: column; gap: 4px; flex: 1; }
.nav-item {
  display: flex;
  align-items: center;
  gap: 11px;
  padding: 10px 11px;
  border-radius: 11px;
  color: var(--muted);
  font-weight: 500;
  transition: background 0.14s ease, color 0.14s ease;
}
.nav-item:hover { background: var(--bg-2); color: var(--text); text-decoration: none; transform: translateX(2px); }
.nav-item.router-link-active { background: linear-gradient(90deg, var(--brand-soft), rgba(13,150,104,.04)); color: var(--brand); font-weight: 650; }
.icon { width: 19px; height: 19px; fill: none; stroke: currentColor; stroke-width: 1.8; stroke-linecap: round; stroke-linejoin: round; flex-shrink: 0; }
.nav-badge { margin-left: auto; min-width: 21px; padding: 2px 6px; border-radius: 999px; color: #fff; background: var(--brand); font-size: 10px; text-align: center; box-shadow: 0 2px 6px rgba(13,150,104,.24); }
.sidebar-footer { display: flex; flex-direction: column; gap: 9px; padding-top: 14px; margin-top: 6px; border-top: 1px solid var(--border); }
.notification-cta { display: flex; align-items: center; gap: 9px; text-align: left; padding: 9px 10px; background: #fff9e9; border-color: #f1dfad; }
.notification-cta > span:last-child { display: flex; flex-direction: column; }
.notification-cta b { font-size: 12px; }
.notification-cta small { font-size: 10px; color: var(--muted); }
.logout-btn { background: var(--bg-2); box-shadow: none; }
.who { font-size: 12px; color: var(--muted); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; padding: 0 4px; }
.content { flex: 1; overflow: auto; min-width: 0; }
.toast-stack { position: fixed; right: 22px; top: 18px; z-index: 100; display: flex; flex-direction: column; gap: 10px; width: min(380px, calc(100vw - 32px)); }
.message-toast { width: 100%; display: flex; align-items: center; gap: 11px; padding: 13px; text-align: left; border: 1px solid rgba(13,150,104,.24); border-radius: 15px; background: rgba(255,255,255,.97); box-shadow: var(--shadow-lg); animation: toast-in .24s ease-out; }
.message-toast:hover { background: #fff; transform: translateY(-1px); }
.toast-icon { width: 36px; height: 36px; display: grid; place-items: center; border-radius: 50%; color: #fff; background: linear-gradient(145deg,#25d366,#0d9668); font-weight: 800; }
.toast-copy { min-width: 0; flex: 1; display: flex; flex-direction: column; }
.toast-copy b, .toast-copy small { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.toast-copy small { color: var(--muted); margin-top: 2px; }
.toast-close { color: var(--muted); font-size: 20px; }
@keyframes toast-in { from { opacity: 0; transform: translateY(-8px) scale(.98); } }

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
  .toast-stack { top: 64px; right: 12px; }
}
</style>
