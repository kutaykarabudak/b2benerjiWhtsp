<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { getDashboardStats, type DashboardStats } from '@/services/analytics'

const stats = ref<DashboardStats | null>(null)
const loading = ref(false)
const error = ref('')

async function load() {
  loading.value = true
  error.value = ''
  try {
    stats.value = await getDashboardStats()
  } catch {
    error.value = 'İstatistikler yüklenemedi.'
  } finally {
    loading.value = false
  }
}

function changeClass(v: number) {
  return v > 0 ? 'up' : v < 0 ? 'down' : ''
}
function fmtChange(v: number) {
  const s = v > 0 ? '+' : ''
  return s + Math.round(v) + '%'
}

onMounted(load)
</script>

<template>
  <div class="page">
    <header class="page-head">
      <div>
        <h1>Analitik</h1>
        <p class="muted">Son 30 günün özeti (önceki döneme göre değişim).</p>
      </div>
      <button @click="load">↻ Yenile</button>
    </header>

    <div v-if="loading" class="muted center">Yükleniyor…</div>
    <div v-else-if="error" class="card center error">{{ error }}</div>
    <div v-else-if="stats" class="tiles">
      <div class="tile card">
        <div class="t-label">Mesaj</div>
        <div class="t-val">{{ stats.total_messages.toLocaleString('tr-TR') }}</div>
        <div class="t-change" :class="changeClass(stats.messages_change)">{{ fmtChange(stats.messages_change) }}</div>
      </div>
      <div class="tile card">
        <div class="t-label">Kişi</div>
        <div class="t-val">{{ stats.total_contacts.toLocaleString('tr-TR') }}</div>
        <div class="t-change" :class="changeClass(stats.contacts_change)">{{ fmtChange(stats.contacts_change) }}</div>
      </div>
      <div class="tile card">
        <div class="t-label">Chatbot Oturumu</div>
        <div class="t-val">{{ stats.chatbot_sessions.toLocaleString('tr-TR') }}</div>
        <div class="t-change" :class="changeClass(stats.chatbot_change)">{{ fmtChange(stats.chatbot_change) }}</div>
      </div>
      <div class="tile card">
        <div class="t-label">Kampanya</div>
        <div class="t-val">{{ stats.campaigns_sent.toLocaleString('tr-TR') }}</div>
        <div class="t-change" :class="changeClass(stats.campaigns_change)">{{ fmtChange(stats.campaigns_change) }}</div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.page { padding: 24px; max-width: 900px; }
.page-head { display: flex; justify-content: space-between; align-items: flex-start; margin-bottom: 16px; gap: 12px; }
.page-head h1 { margin: 0 0 4px; font-size: 20px; }
.page-head p { margin: 0; }
.center { text-align: center; padding: 24px; }
.error { color: var(--danger); }

.tiles { display: grid; grid-template-columns: repeat(auto-fit, minmax(180px, 1fr)); gap: 12px; }
.tile { text-align: center; }
.t-label { font-size: 13px; color: var(--muted); }
.t-val { font-size: 30px; font-weight: 700; margin: 6px 0; }
.t-change { font-size: 13px; color: var(--muted); }
.t-change.up { color: var(--brand); }
.t-change.down { color: var(--danger); }
</style>
