<script setup lang="ts">
import { ref, onMounted } from 'vue'
import {
  listCampaigns,
  createCampaign,
  listAccounts,
  listApprovedTemplates,
  addRecipients,
  startCampaign,
  pauseCampaign,
  cancelCampaign,
  deleteCampaign,
  type Campaign,
  type Account,
  type Template
} from '@/services/campaigns'
import { listContacts } from '@/services/contacts'

const campaigns = ref<Campaign[]>([])
const accounts = ref<Account[]>([])
const templates = ref<Template[]>([])
const loading = ref(false)

// Create form
const showCreate = ref(false)
const form = ref({ name: '', whatsapp_account: '', template_id: '' })
const creating = ref(false)
const createError = ref('')

// Recipients editor (per campaign, inline)
const recipientsFor = ref<string | null>(null)
const recipientText = ref('')
const savingRecipients = ref(false)

async function loadAll() {
  loading.value = true
  try {
    const [c, a, t] = await Promise.all([listCampaigns(), listAccounts(), listApprovedTemplates()])
    campaigns.value = c
    accounts.value = a
    templates.value = t
    if (!form.value.whatsapp_account && a.length) form.value.whatsapp_account = a[0].name
  } finally {
    loading.value = false
  }
}

async function submitCreate() {
  createError.value = ''
  if (!form.value.name.trim() || !form.value.whatsapp_account || !form.value.template_id) {
    createError.value = 'İsim, hesap ve şablon zorunlu.'
    return
  }
  creating.value = true
  try {
    await createCampaign({
      name: form.value.name.trim(),
      whatsapp_account: form.value.whatsapp_account,
      template_id: form.value.template_id
    })
    form.value = { name: '', whatsapp_account: form.value.whatsapp_account, template_id: '' }
    showCreate.value = false
    await loadAll()
  } catch (e: any) {
    createError.value = e?.response?.data?.message || 'Kampanya oluşturulamadı.'
  } finally {
    creating.value = false
  }
}

// Contact picker
const pickerContacts = ref<{ id: string; name: string; phone_number: string }[]>([])
const picked = ref<Set<string>>(new Set())
const showPicker = ref(false)

async function openRecipients(c: Campaign) {
  const opening = recipientsFor.value !== c.id
  recipientsFor.value = opening ? c.id : null
  recipientText.value = ''
  showPicker.value = false
  picked.value = new Set()
  if (opening && !pickerContacts.value.length) {
    try {
      const list = await listContacts('')
      pickerContacts.value = list.map((x) => ({
        id: x.id,
        name: x.name || x.profile_name || x.phone_number,
        phone_number: x.phone_number
      }))
    } catch {
      /* sessiz */
    }
  }
}

function togglePick(id: string) {
  const s = new Set(picked.value)
  s.has(id) ? s.delete(id) : s.add(id)
  picked.value = s
}

function addPickedToText() {
  const phones = pickerContacts.value
    .filter((c) => picked.value.has(c.id))
    .map((c) => c.phone_number)
  const existing = recipientText.value.trim()
  recipientText.value = (existing ? existing + '\n' : '') + phones.join('\n')
  showPicker.value = false
  picked.value = new Set()
}

async function saveRecipients(c: Campaign) {
  const phones = recipientText.value
    .split(/[\n,;]+/)
    .map((p) => p.trim().replace(/^\+/, ''))
    .filter(Boolean)
  if (!phones.length) return
  savingRecipients.value = true
  try {
    await addRecipients(c.id, phones.map((phone_number) => ({ phone_number })))
    recipientsFor.value = null
    recipientText.value = ''
    await loadAll()
  } catch (e: any) {
    alert(e?.response?.data?.message || 'Alıcılar eklenemedi.')
  } finally {
    savingRecipients.value = false
  }
}

async function act(fn: (id: string) => Promise<void>, c: Campaign) {
  try {
    await fn(c.id)
    await loadAll()
  } catch (e: any) {
    alert(e?.response?.data?.message || 'İşlem başarısız.')
  }
}

async function removeCampaign(c: Campaign) {
  if (!confirm(`"${c.name}" kampanyası silinsin mi?`)) return
  await act(deleteCampaign, c)
}

const STATUS_LABEL: Record<string, string> = {
  draft: 'Taslak',
  queued: 'Kuyrukta',
  processing: 'Gönderiliyor',
  paused: 'Duraklatıldı',
  completed: 'Tamamlandı',
  failed: 'Başarısız'
}

onMounted(loadAll)
</script>

<template>
  <div class="page">
    <header class="page-head">
      <div>
        <h1>Toplu Mesaj</h1>
        <p class="muted">WhatsApp onaylı şablonlarla toplu kampanya gönderimi.</p>
      </div>
      <button class="primary" @click="showCreate = !showCreate">＋ Yeni Kampanya</button>
    </header>

    <!-- Create form -->
    <form v-if="showCreate" class="card create-form" @submit.prevent="submitCreate">
      <div class="row">
        <div class="field">
          <label>Kampanya adı *</label>
          <input v-model="form.name" placeholder="Ör. Temmuz kampanyası" />
        </div>
        <div class="field">
          <label>WhatsApp hesabı *</label>
          <select v-model="form.whatsapp_account">
            <option value="" disabled>Seçin</option>
            <option v-for="a in accounts" :key="a.name" :value="a.name">{{ a.name }}</option>
          </select>
        </div>
        <div class="field">
          <label>Şablon (onaylı) *</label>
          <select v-model="form.template_id">
            <option value="" disabled>Seçin</option>
            <option v-for="t in templates" :key="t.id" :value="t.id">
              {{ t.display_name || t.name }} ({{ t.language }})
            </option>
          </select>
        </div>
      </div>
      <p v-if="!templates.length" class="muted small">
        Onaylı şablon yok. Kampanya için Meta tarafından onaylanmış bir şablon gerekir.
      </p>
      <p v-if="createError" class="error">{{ createError }}</p>
      <div class="form-actions">
        <button type="button" @click="showCreate = false">İptal</button>
        <button class="primary" type="submit" :disabled="creating">Oluştur</button>
      </div>
    </form>

    <!-- Campaign list -->
    <div v-if="loading" class="muted center">Yükleniyor…</div>
    <div v-else-if="!campaigns.length" class="card center muted">Henüz kampanya yok.</div>

    <div v-for="c in campaigns" :key="c.id" class="card campaign">
      <div class="campaign-head">
        <div>
          <div class="campaign-name">{{ c.name }}</div>
          <div class="muted small">{{ c.template_name || c.template_id }} · {{ c.whatsapp_account }}</div>
        </div>
        <span :class="['status', c.status]">{{ STATUS_LABEL[c.status] || c.status }}</span>
      </div>

      <div class="stats">
        <span>Alıcı: <b>{{ c.total_recipients }}</b></span>
        <span>Gönderildi: <b>{{ c.sent_count }}</b></span>
        <span>Ulaştı: <b>{{ c.delivered_count }}</b></span>
        <span>Okundu: <b>{{ c.read_count }}</b></span>
        <span class="fail">Başarısız: <b>{{ c.failed_count }}</b></span>
      </div>

      <div class="campaign-actions">
        <button v-if="c.status === 'draft'" @click="openRecipients(c)">Alıcı Ekle</button>
        <button
          v-if="c.status === 'draft' || c.status === 'paused'"
          class="primary"
          :disabled="!c.total_recipients"
          @click="act(startCampaign, c)"
        >
          Başlat
        </button>
        <button v-if="c.status === 'processing'" @click="act(pauseCampaign, c)">Duraklat</button>
        <button v-if="['queued', 'processing', 'paused'].includes(c.status)" @click="act(cancelCampaign, c)">
          İptal
        </button>
        <button class="danger-btn" @click="removeCampaign(c)">Sil</button>
      </div>

      <!-- Inline recipients editor -->
      <div v-if="recipientsFor === c.id" class="recipients">
        <label class="muted small">Telefon numaraları (her satıra bir tane veya virgülle)</label>
        <textarea v-model="recipientText" rows="4" placeholder="905551112233&#10;905552223344"></textarea>

        <div class="picker-bar">
          <button type="button" @click="showPicker = !showPicker">
            👥 Kişilerden Seç{{ pickerContacts.length ? ' (' + pickerContacts.length + ')' : '' }}
          </button>
          <button v-if="showPicker && picked.size" type="button" class="primary" @click="addPickedToText">
            {{ picked.size }} kişiyi ekle
          </button>
        </div>
        <div v-if="showPicker" class="picker-list">
          <label v-for="pc in pickerContacts" :key="pc.id" class="picker-item">
            <input type="checkbox" :checked="picked.has(pc.id)" @change="togglePick(pc.id)" />
            {{ pc.name }} <span class="muted small">· {{ pc.phone_number }}</span>
          </label>
          <div v-if="!pickerContacts.length" class="muted small">Kayıtlı kişi yok.</div>
        </div>

        <div class="form-actions">
          <button type="button" @click="recipientsFor = null">Kapat</button>
          <button class="primary" :disabled="savingRecipients" @click="saveRecipients(c)">Ekle</button>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.page { padding: 24px; max-width: 900px; }
.page-head { display: flex; justify-content: space-between; align-items: flex-start; margin-bottom: 16px; gap: 12px; }
.page-head h1 { margin: 0 0 4px; font-size: 20px; }
.page-head p { margin: 0; }
.small { font-size: 12px; }
.center { text-align: center; padding: 24px; }

.create-form { margin-bottom: 16px; }
.row { display: flex; gap: 12px; flex-wrap: wrap; }
.field { flex: 1; min-width: 180px; display: flex; flex-direction: column; gap: 4px; }
.field label { font-size: 12px; color: var(--muted); }
.form-actions { display: flex; justify-content: flex-end; gap: 8px; margin-top: 12px; }
.error { color: var(--danger); margin: 8px 0 0; }

.campaign { margin-bottom: 12px; }
.campaign-head { display: flex; justify-content: space-between; align-items: flex-start; gap: 12px; }
.campaign-name { font-weight: 600; }
.status { font-size: 12px; padding: 2px 10px; border-radius: 999px; background: var(--bg); border: 1px solid var(--border); white-space: nowrap; }
.status.processing { color: #0a6d4e; }
.status.completed { color: #0a6d4e; }
.status.failed { color: var(--danger); }
.status.paused { color: #b26a00; }

.stats { display: flex; gap: 16px; flex-wrap: wrap; margin: 12px 0; font-size: 13px; color: var(--muted); }
.stats .fail b { color: var(--danger); }

.campaign-actions { display: flex; gap: 8px; flex-wrap: wrap; }
.danger-btn { color: var(--danger); }

.recipients { margin-top: 12px; padding-top: 12px; border-top: 1px solid var(--border); display: flex; flex-direction: column; gap: 6px; }
.recipients textarea { width: 100%; font-family: inherit; }
.picker-bar { display: flex; gap: 8px; margin-top: 8px; }
.picker-list { max-height: 180px; overflow-y: auto; border: 1px solid var(--border); border-radius: var(--radius); padding: 8px; margin-top: 8px; display: flex; flex-direction: column; gap: 4px; }
.picker-item { display: flex; align-items: center; gap: 8px; font-size: 14px; }
.picker-item input { width: auto; }
</style>
