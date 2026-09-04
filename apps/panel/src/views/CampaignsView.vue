<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { useRoute } from 'vue-router'
import {
  listCampaigns,
  createCampaign,
  listAccounts,
  listApprovedTemplates,
  addRecipients,
  uploadCampaignMedia,
  startCampaign,
  pauseCampaign,
  cancelCampaign,
  deleteCampaign,
  type Campaign,
  type Account,
  type Template
} from '@/services/campaigns'
import { listContacts } from '@/services/contacts'

const MEDIA_HEADER_TYPES = new Set(['IMAGE', 'VIDEO', 'DOCUMENT'])

const campaigns = ref<Campaign[]>([])
const accounts = ref<Account[]>([])
const templates = ref<Template[]>([])
const loading = ref(false)
const route = useRoute()
const crmFilterActive = !!(route.query.tags || route.query.has_purchased || route.query.min_purchase_score || route.query.city || route.query.district)

// Create form
const showCreate = ref(false)
const form = ref({ name: '', whatsapp_account: '', template_id: '' })
const creating = ref(false)
const createError = ref('')
const createMediaFile = ref<File | null>(null)

// A template with an IMAGE/VIDEO/DOCUMENT header needs its actual media
// uploaded per-campaign — Meta's approval only stores an example asset, not a
// reusable send-time media ID. Without it every send in the campaign is
// rejected by Meta with the header component missing.
function templateNeedsMedia(templateId: string): boolean {
  const tmpl = templates.value.find((t) => t.id === templateId)
  return !!tmpl?.header_type && MEDIA_HEADER_TYPES.has(tmpl.header_type.toUpperCase())
}

const createNeedsMedia = computed(() => templateNeedsMedia(form.value.template_id))

function campaignNeedsMedia(c: Campaign): boolean {
  return templateNeedsMedia(c.template_id) && !c.header_media_id
}

function onCreateMediaChange(e: Event) {
  createMediaFile.value = (e.target as HTMLInputElement).files?.[0] || null
}

// Recipients editor (per campaign, inline)
const recipientsFor = ref<string | null>(null)
const recipientText = ref('')
const savingRecipients = ref(false)

// Media uploader (per campaign, inline) — for media-header templates whose
// campaign wasn't given its media at creation time (or the upload failed).
const mediaUploadFor = ref<string | null>(null)
const mediaUploadFile = ref<File | null>(null)
const uploadingMedia = ref(false)
const mediaUploadError = ref('')

function openMediaUpload(c: Campaign) {
  mediaUploadFor.value = mediaUploadFor.value === c.id ? null : c.id
  mediaUploadFile.value = null
  mediaUploadError.value = ''
}

function onMediaUploadChange(e: Event) {
  mediaUploadFile.value = (e.target as HTMLInputElement).files?.[0] || null
}

async function submitMediaUpload(c: Campaign) {
  if (!mediaUploadFile.value) return
  uploadingMedia.value = true
  mediaUploadError.value = ''
  try {
    await uploadCampaignMedia(c.id, mediaUploadFile.value)
    mediaUploadFor.value = null
    mediaUploadFile.value = null
    await loadAll()
  } catch (e: any) {
    mediaUploadError.value = e?.response?.data?.message || 'Medya yüklenemedi.'
  } finally {
    uploadingMedia.value = false
  }
}

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
  if (createNeedsMedia.value && !createMediaFile.value) {
    createError.value = 'Bu şablonun görsel/video/doküman başlığı var — göndermeden önce medya dosyası seçin.'
    return
  }
  creating.value = true
  try {
    const created = await createCampaign({
      name: form.value.name.trim(),
      whatsapp_account: form.value.whatsapp_account,
      template_id: form.value.template_id
    })
    if (createNeedsMedia.value && createMediaFile.value) {
      try {
        await uploadCampaignMedia(created.id, createMediaFile.value)
      } catch (e: any) {
        // Campaign exists as a draft; surface the failure but don't lose it —
        // the inline uploader on the campaign row lets them retry.
        createError.value = e?.response?.data?.message || 'Kampanya oluşturuldu ama medya yüklenemedi. Kampanya kartından tekrar deneyin.'
      }
    }
    form.value = { name: '', whatsapp_account: form.value.whatsapp_account, template_id: '' }
    createMediaFile.value = null
    showCreate.value = false
    await loadAll()
  } catch (e: any) {
    createError.value = e?.response?.data?.message || 'Kampanya oluşturulamadı.'
  } finally {
    creating.value = false
  }
}

// Contact picker
const pickerContacts = ref<{ id: string; name: string; phone_number: string; city: string; district: string; purchase_score: number; has_purchased: boolean; tags: string[] }[]>([])
const picked = ref<Set<string>>(new Set())
const showPicker = ref(false)
const pickerLoading = ref(false)
const pickerError = ref('')
const pickerFilters = ref({
  category: String(route.query.tags || ''),
  purchased: String(route.query.has_purchased || ''),
  minScore: String(route.query.min_purchase_score || ''),
  city: String(route.query.city || ''),
  district: String(route.query.district || '')
})

async function loadPickerContacts() {
  pickerLoading.value = true
  pickerError.value = ''
  picked.value = new Set()
  try {
    const list = await listContacts('', {
      tags: pickerFilters.value.category.trim() || undefined,
      has_purchased: pickerFilters.value.purchased ? pickerFilters.value.purchased === 'yes' : undefined,
      min_purchase_score: pickerFilters.value.minScore ? Number(pickerFilters.value.minScore) : undefined,
      city: pickerFilters.value.city.trim() || undefined,
      district: pickerFilters.value.district.trim() || undefined,
      b2b_registered: true
    })
    pickerContacts.value = list.map((x) => ({
      id: x.id,
      name: x.name || x.profile_name || x.phone_number,
      phone_number: x.phone_number,
      city: x.city || '', district: x.district || '', purchase_score: x.purchase_score || 0,
      has_purchased: !!x.has_purchased, tags: x.tags || []
    }))
  } catch (e: any) {
    pickerContacts.value = []
    pickerError.value = e?.response?.data?.message || 'Kişiler filtrelenemedi.'
  } finally {
    pickerLoading.value = false
  }
}

function clearPickerFilters() {
  pickerFilters.value = { category: '', purchased: '', minScore: '', city: '', district: '' }
  loadPickerContacts()
}

function toggleAllFiltered() {
  picked.value = picked.value.size === pickerContacts.value.length
    ? new Set()
    : new Set(pickerContacts.value.map((contact) => contact.id))
}

async function openRecipients(c: Campaign) {
  const opening = recipientsFor.value !== c.id
  recipientsFor.value = opening ? c.id : null
  recipientText.value = ''
  showPicker.value = false
  picked.value = new Set()
  if (opening) await loadPickerContacts()
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
    <div v-if="crmFilterActive" class="card crm-notice">Kişiler ekranındaki CRM filtreleri alıcı seçimine uygulandı. Kampanyada “Alıcı Ekle → Kişilerden Seç” dediğinizde yalnızca hedef kitle görüntülenir.</div>

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
      <div v-if="createNeedsMedia" class="field media-field">
        <label>Şablon görseli/videosu/dokümanı *</label>
        <p class="muted small">
          Şablonun onayı sırasında eklenen görsel sadece örnektir; Meta her gönderim için medyanın
          tekrar yüklenmesini ister. Bu dosya seçilmeden kampanya başlatılamaz.
        </p>
        <input type="file" accept="image/jpeg,image/png,image/webp,video/mp4,video/3gpp,.pdf,.doc,.docx,.xls,.xlsx,.ppt,.pptx" @change="onCreateMediaChange" />
        <span v-if="createMediaFile" class="muted small">Seçildi: {{ createMediaFile.name }}</span>
      </div>
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

      <p v-if="(c.status === 'draft' || c.status === 'paused') && campaignNeedsMedia(c)" class="error small">
        Bu şablon görsel/video/doküman başlığı gerektiriyor — başlatmadan önce medya yükleyin.
      </p>

      <div class="campaign-actions">
        <button v-if="c.status === 'draft'" @click="openRecipients(c)">Alıcı Ekle</button>
        <button v-if="(c.status === 'draft' || c.status === 'paused') && campaignNeedsMedia(c)" @click="openMediaUpload(c)">
          Medya Yükle
        </button>
        <button
          v-if="(c.status === 'draft' || c.status === 'paused') && !campaignNeedsMedia(c)"
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
        <div v-if="showPicker" class="picker-panel">
          <div class="picker-filter-head"><div><b>CRM hedef kitlesi</b><div class="muted small">Filtreler birlikte uygulanır.</div></div><span class="target-count">{{ pickerLoading ? 'Aranıyor…' : pickerContacts.length + ' kişi bulundu' }}</span></div>
          <div class="picker-filters">
            <div class="field"><label>Şehir</label><input v-model="pickerFilters.city" placeholder="Ör. Antalya" @keyup.enter="loadPickerContacts" /></div>
            <div class="field"><label>İlçe</label><input v-model="pickerFilters.district" placeholder="Ör. Muratpaşa" @keyup.enter="loadPickerContacts" /></div>
            <div class="field"><label>Alışveriş durumu</label><select v-model="pickerFilters.purchased"><option value="">Tümü</option><option value="yes">Daha önce alanlar</option><option value="no">Daha önce almayanlar</option></select></div>
            <div class="field"><label>Minimum alım puanı</label><input v-model="pickerFilters.minScore" type="number" min="0" max="100" placeholder="Ör. 70" @keyup.enter="loadPickerContacts" /></div>
            <div class="field"><label>Kategori</label><input v-model="pickerFilters.category" placeholder="Ör. VIP" @keyup.enter="loadPickerContacts" /></div>
          </div>
          <div class="picker-filter-actions"><button type="button" @click="clearPickerFilters">Temizle</button><button type="button" class="primary" :disabled="pickerLoading" @click="loadPickerContacts">{{ pickerLoading ? 'Filtreleniyor…' : 'Filtreleri Uygula' }}</button></div>
          <p v-if="pickerError" class="error">{{ pickerError }}</p>
          <div class="select-all-row"><label><input type="checkbox" :checked="pickerContacts.length > 0 && picked.size === pickerContacts.length" @change="toggleAllFiltered" /> Filtrelenenlerin tümünü seç</label><b>{{ picked.size }} seçili</b></div>
          <div class="picker-list">
          <label v-for="pc in pickerContacts" :key="pc.id" class="picker-item">
            <input type="checkbox" :checked="picked.has(pc.id)" @change="togglePick(pc.id)" />
            <span class="picker-person"><b>{{ pc.name }}</b><span class="muted small">{{ pc.phone_number }} · {{ [pc.city, pc.district].filter(Boolean).join(' / ') || 'Konum yok' }} · {{ pc.purchase_score }} puan<span v-if="pc.has_purchased"> · Alım yaptı</span></span></span>
          </label>
          <div v-if="!pickerLoading && !pickerContacts.length" class="muted small">Bu filtrelere uygun kişi bulunamadı.</div>
          </div>
        </div>

        <div class="form-actions">
          <button type="button" @click="recipientsFor = null">Kapat</button>
          <button class="primary" :disabled="savingRecipients" @click="saveRecipients(c)">Ekle</button>
        </div>
      </div>

      <!-- Inline media uploader (media-header templates only) -->
      <div v-if="mediaUploadFor === c.id" class="recipients">
        <label class="muted small">
          Şablonun görseli/videosu/dokümanı — Meta her gönderim için bunu ayrıca ister, onay
          sırasındaki örnek görsel otomatik kullanılmaz.
        </label>
        <input type="file" accept="image/jpeg,image/png,image/webp,video/mp4,video/3gpp,.pdf,.doc,.docx,.xls,.xlsx,.ppt,.pptx" @change="onMediaUploadChange" />
        <span v-if="mediaUploadFile" class="muted small">Seçildi: {{ mediaUploadFile.name }}</span>
        <p v-if="mediaUploadError" class="error small">{{ mediaUploadError }}</p>
        <div class="form-actions">
          <button type="button" @click="openMediaUpload(c)">Kapat</button>
          <button class="primary" :disabled="uploadingMedia || !mediaUploadFile" @click="submitMediaUpload(c)">
            {{ uploadingMedia ? 'Yükleniyor…' : 'Yükle' }}
          </button>
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
.crm-notice { margin-bottom: 12px; color: #0a6d4e; font-size: 13px; }
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
.picker-panel { border: 1px solid var(--border); border-radius: var(--radius); padding: 12px; margin-top: 8px; }
.picker-filter-head, .select-all-row { display: flex; justify-content: space-between; align-items: center; gap: 12px; }
.target-count { background: var(--bg); border: 1px solid var(--border); border-radius: 999px; padding: 4px 10px; font-size: 12px; }
.picker-filters { display: grid; grid-template-columns: repeat(auto-fit, minmax(135px, 1fr)); gap: 8px; margin-top: 10px; }
.picker-filters .field { min-width: 0; }
.picker-filter-actions { display: flex; justify-content: flex-end; gap: 8px; margin-top: 8px; }
.select-all-row { border-top: 1px solid var(--border); margin-top: 10px; padding-top: 10px; font-size: 13px; }
.select-all-row label { display: flex; align-items: center; gap: 7px; }
.select-all-row input { width: auto; }
.picker-list { max-height: 260px; overflow-y: auto; padding-top: 8px; display: flex; flex-direction: column; gap: 4px; }
.picker-item { display: flex; align-items: center; gap: 8px; font-size: 14px; }
.picker-item input { width: auto; }
.picker-person { display: flex; flex-direction: column; gap: 2px; }
</style>
