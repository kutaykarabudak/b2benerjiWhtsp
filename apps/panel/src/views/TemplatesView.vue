<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { listTemplates, syncTemplates, type Template } from '@/services/templates'
import { listAccounts, type Account } from '@/services/campaigns'

const templates = ref<Template[]>([])
const accounts = ref<Account[]>([])
const loading = ref(false)
const syncAccount = ref('')
const syncing = ref(false)
const msg = ref('')

async function load() {
  loading.value = true
  try {
    const [t, a] = await Promise.all([listTemplates(), listAccounts()])
    templates.value = t
    accounts.value = a
    if (!syncAccount.value && a.length) syncAccount.value = a[0].name
  } finally {
    loading.value = false
  }
}

async function doSync() {
  if (!syncAccount.value) {
    msg.value = 'Önce bir kanal (WhatsApp numarası) ekleyin.'
    return
  }
  syncing.value = true
  msg.value = ''
  try {
    await syncTemplates(syncAccount.value)
    msg.value = '✓ Meta’dan senkronlandı.'
    await load()
  } catch (e: any) {
    msg.value = '✗ ' + (e?.response?.data?.message || 'Senkronlanamadı.')
  } finally {
    syncing.value = false
  }
}

function statusClass(s: string) {
  return s === 'APPROVED' ? 'ok' : s === 'REJECTED' ? 'bad' : 'pending'
}

onMounted(load)
</script>

<template>
  <div class="page">
    <header class="page-head">
      <div>
        <h1>Şablonlar</h1>
        <p class="muted">WhatsApp mesaj şablonları. Toplu mesaj bunları kullanır — Meta’dan senkronlayın.</p>
      </div>
    </header>

    <div class="card explainer">
      Şablonlar Meta tarafında oluşturulup onaylanır. Burada onları <b>görüntüler</b> ve <b>Meta’dan senkronlarsınız</b>.
      Sadece <b>APPROVED (onaylı)</b> şablonlar toplu mesajda kullanılabilir.
      <div class="sync-row">
        <select v-model="syncAccount">
          <option value="" disabled>Kanal seçin</option>
          <option v-for="a in accounts" :key="a.name" :value="a.name">{{ a.name }}</option>
        </select>
        <button class="primary" :disabled="syncing" @click="doSync">
          {{ syncing ? 'Senkronlanıyor…' : '↻ Meta’dan Senkronla' }}
        </button>
        <span v-if="msg" class="muted small">{{ msg }}</span>
      </div>
    </div>

    <div v-if="loading" class="muted center">Yükleniyor…</div>
    <div v-else-if="!templates.length" class="card center muted">
      Şablon yok. Meta’da şablon oluşturup yukarıdan senkronlayın.
    </div>

    <div v-for="t in templates" :key="t.id" class="card tpl">
      <div class="tpl-head">
        <div>
          <span class="tpl-name">{{ t.display_name || t.name }}</span>
          <span class="muted small">· {{ t.language }} · {{ t.category }} · {{ t.whatsapp_account }}</span>
        </div>
        <span :class="['status', statusClass(t.status)]">{{ t.status }}</span>
      </div>
      <div v-if="t.header_content" class="tpl-header">{{ t.header_content }}</div>
      <div class="tpl-body">{{ t.body_content }}</div>
      <div v-if="t.footer_content" class="tpl-footer muted small">{{ t.footer_content }}</div>
      <div v-if="t.buttons?.length" class="tpl-buttons">
        <span v-for="(b, i) in t.buttons" :key="i" class="tpl-btn">{{ (b as any).text || (b as any).title || 'Buton' }}</span>
      </div>
    </div>
  </div>
</template>

<style scoped>
.page { padding: 24px; max-width: 800px; }
.page-head { margin-bottom: 16px; }
.page-head h1 { margin: 0 0 4px; font-size: 20px; }
.page-head p { margin: 0; }
.center { text-align: center; padding: 24px; }
.explainer { margin-bottom: 16px; font-size: 14px; line-height: 1.6; }
.sync-row { display: flex; gap: 8px; align-items: center; margin-top: 12px; flex-wrap: wrap; }
.sync-row select { max-width: 220px; }

.tpl { margin-bottom: 10px; }
.tpl-head { display: flex; justify-content: space-between; align-items: flex-start; gap: 12px; margin-bottom: 8px; }
.tpl-name { font-weight: 600; }
.status { font-size: 11px; padding: 2px 10px; border-radius: 999px; border: 1px solid var(--border); white-space: nowrap; }
.status.ok { color: #0a6d4e; border-color: #0a6d4e; }
.status.bad { color: var(--danger); border-color: var(--danger); }
.status.pending { color: #b26a00; border-color: #b26a00; }
.tpl-header { font-weight: 600; margin-bottom: 4px; }
.tpl-body { white-space: pre-wrap; word-break: break-word; }
.tpl-footer { margin-top: 4px; }
.tpl-buttons { display: flex; gap: 6px; flex-wrap: wrap; margin-top: 8px; }
.tpl-btn { font-size: 12px; padding: 3px 10px; border-radius: 6px; border: 1px solid var(--brand); color: var(--brand); }
</style>
