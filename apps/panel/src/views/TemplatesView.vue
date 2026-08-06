<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import {
  listTemplates,
  syncTemplates,
  createTemplate,
  updateTemplate,
  submitTemplate,
  deleteTemplate,
  setTemplateFirstMessage,
  uploadTemplateHeaderMedia,
  type Template,
  type TemplateButton
} from '@/services/templates'
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

// Literal WhatsApp variable placeholder, kept out of the template to avoid
// nested-mustache parsing issues.
const VAR1 = '{{1}}'

// ---- Editor ----
const showEditor = ref(false)
const editId = ref<string | null>(null)
const saving = ref(false)
const editError = ref('')
const form = ref(blankForm())

type HeaderType = '' | 'TEXT' | 'IMAGE' | 'VIDEO' | 'DOCUMENT'

function blankForm() {
  return {
    whatsapp_account: '',
    name: '',
    language: 'tr',
    category: 'MARKETING',
    header_type: '' as HeaderType,
    header_content: '',
    header_media_filename: '',
    body_content: '',
    footer_content: '',
    buttons: [] as TemplateButton[]
  }
}

const uploadingHeaderMedia = ref(false)
const headerUploadError = ref('')

const headerFileAccept = computed(() => {
  if (form.value.header_type === 'IMAGE') return 'image/*'
  if (form.value.header_type === 'VIDEO') return 'video/*'
  if (form.value.header_type === 'DOCUMENT') return '.pdf'
  return '*/*'
})

// Switching header type invalidates whatever text/handle was in header_content.
function onHeaderTypeChange() {
  form.value.header_content = ''
  form.value.header_media_filename = ''
  headerUploadError.value = ''
}

async function onHeaderFileChosen(e: Event) {
  const input = e.target as HTMLInputElement
  const file = input.files?.[0]
  input.value = ''
  if (!file || !form.value.whatsapp_account) return
  uploadingHeaderMedia.value = true
  headerUploadError.value = ''
  try {
    const result = await uploadTemplateHeaderMedia(file, form.value.whatsapp_account)
    form.value.header_content = result.handle
    form.value.header_media_filename = result.filename
  } catch (e: any) {
    headerUploadError.value = e?.response?.data?.message || 'Dosya yüklenemedi.'
  } finally {
    uploadingHeaderMedia.value = false
  }
}

function openEditor() {
  form.value = blankForm()
  editId.value = null
  if (accounts.value.length) form.value.whatsapp_account = accounts.value[0].name
  editError.value = ''
  showEditor.value = true
}

function startEditTpl(t: Template) {
  editId.value = t.id
  const headerType = (t.header_type || (t.header_content ? 'TEXT' : '')) as HeaderType
  form.value = {
    whatsapp_account: t.whatsapp_account,
    name: t.name,
    language: t.language,
    category: t.category,
    header_type: headerType,
    header_content: t.header_content || '',
    header_media_filename: ['IMAGE', 'VIDEO', 'DOCUMENT'].includes(headerType) ? 'Yüklenen dosya' : '',
    body_content: t.body_content,
    footer_content: t.footer_content || '',
    buttons: Array.isArray(t.buttons)
      ? (t.buttons as any[]).map((b) => ({
          type: (b.type || 'QUICK_REPLY') as TemplateButton['type'],
          text: b.text || b.title || '',
          url: b.url,
          phone_number: b.phone_number
        }))
      : []
  }
  editError.value = ''
  showEditor.value = true
}

function addTplButton() {
  if (form.value.buttons.length < 3) form.value.buttons.push({ type: 'QUICK_REPLY', text: '' })
}
function removeTplButton(i: number) {
  form.value.buttons.splice(i, 1)
}

function normalizeName(v: string) {
  // WhatsApp requires lowercase snake_case names.
  form.value.name = v.toLowerCase().replace(/[^a-z0-9_]/g, '_')
}

async function saveTemplate(submitAfter: boolean) {
  editError.value = ''
  const f = form.value
  if (!f.whatsapp_account) {
    editError.value = 'Kanal seçin (önce Yönetim → Kanallar’dan numara ekleyin).'
    return
  }
  if (!f.name.trim() || !f.body_content.trim()) {
    editError.value = 'Ad ve gövde zorunlu.'
    return
  }
  if (['IMAGE', 'VIDEO', 'DOCUMENT'].includes(f.header_type) && !f.header_content.trim()) {
    editError.value = 'Seçilen başlık türü için bir dosya yükleyin.'
    return
  }
  saving.value = true
  try {
    const payload = {
      whatsapp_account: f.whatsapp_account,
      name: f.name.trim(),
      language: f.language.trim() || 'tr',
      category: f.category,
      header_type: f.header_type,
      header_content: f.header_type === 'TEXT' ? f.header_content.trim() : f.header_content,
      body_content: f.body_content.trim(),
      footer_content: f.footer_content.trim(),
      buttons: f.buttons.filter((b) => b.text.trim())
    }
    let id = editId.value
    if (id) {
      await updateTemplate(id, payload)
    } else {
      id = await createTemplate(payload)
    }
    if (submitAfter && id) {
      await submitTemplate(id)
      msg.value = '✓ Oluşturuldu ve Meta onayına gönderildi.'
    } else {
      msg.value = '✓ Şablon kaydedildi (taslak).'
    }
    showEditor.value = false
    await load()
  } catch (e: any) {
    editError.value = e?.response?.data?.message || 'Kaydedilemedi.'
  } finally {
    saving.value = false
  }
}

async function submitTpl(t: Template) {
  if (!confirm(`"${t.name}" Meta onayına gönderilsin mi?`)) return
  try {
    await submitTemplate(t.id)
    msg.value = '✓ Meta onayına gönderildi.'
    await load()
  } catch (e: any) {
    alert(e?.response?.data?.message || 'Gönderilemedi.')
  }
}

async function removeTpl(t: Template) {
  if (!confirm(`"${t.name}" silinsin mi?`)) return
  try {
    await deleteTemplate(t.id)
    await load()
  } catch (e: any) {
    alert(e?.response?.data?.message || 'Silinemedi.')
  }
}

async function toggleFirstMessage(t: Template) {
  const enabled = !t.is_first_message
  try {
    await setTemplateFirstMessage(t.id, enabled)
    t.is_first_message = enabled
    msg.value = enabled
      ? `✓ ${t.display_name || t.name} kapalı sohbetlerde ilk mesaj olarak gösterilecek.`
      : `✓ ${t.display_name || t.name} ilk mesaj listesinden kaldırıldı.`
  } catch (e: any) {
    alert(e?.response?.data?.message || 'İlk mesaj ayarı kaydedilemedi.')
  }
}

onMounted(load)
</script>

<template>
  <div class="page">
    <header class="page-head">
      <div>
        <h1>Şablonlar</h1>
        <p class="muted">WhatsApp mesaj şablonları. Toplu mesaj bunları kullanır.</p>
      </div>
      <button class="primary" @click="openEditor">＋ Yeni Şablon</button>
    </header>

    <!-- Editor -->
    <form v-if="showEditor" class="card editor" @submit.prevent="saveTemplate(true)">
      <div class="row">
        <div class="field grow">
          <label>Kanal *</label>
          <select v-model="form.whatsapp_account">
            <option value="" disabled>Seçin</option>
            <option v-for="a in accounts" :key="a.name" :value="a.name">{{ a.name }}</option>
          </select>
        </div>
        <div class="field grow">
          <label>Ad * <span class="muted small">(küçük harf, alt çizgi)</span></label>
          <input :value="form.name" @input="normalizeName(($event.target as HTMLInputElement).value)" placeholder="ornek_sablon" />
        </div>
      </div>
      <div class="row">
        <div class="field">
          <label>Dil</label>
          <input v-model="form.language" placeholder="tr" />
        </div>
        <div class="field grow">
          <label>Kategori</label>
          <select v-model="form.category">
            <option value="MARKETING">MARKETING</option>
            <option value="UTILITY">UTILITY</option>
            <option value="AUTHENTICATION">AUTHENTICATION</option>
          </select>
        </div>
      </div>
      <div class="field">
        <label>Başlık türü (opsiyonel)</label>
        <select v-model="form.header_type" @change="onHeaderTypeChange">
          <option value="">Yok</option>
          <option value="TEXT">Metin</option>
          <option value="IMAGE">Görsel</option>
          <option value="VIDEO">Video</option>
          <option value="DOCUMENT">PDF / Belge</option>
        </select>
      </div>
      <div v-if="form.header_type === 'TEXT'" class="field">
        <label>Başlık metni</label>
        <input v-model="form.header_content" placeholder="Ör. Kampanya!" />
      </div>
      <div v-else-if="form.header_type === 'IMAGE' || form.header_type === 'VIDEO' || form.header_type === 'DOCUMENT'" class="field">
        <label>Başlık dosyası *</label>
        <input
          type="file"
          :accept="headerFileAccept"
          :disabled="!form.whatsapp_account || uploadingHeaderMedia"
          @change="onHeaderFileChosen"
        />
        <span v-if="!form.whatsapp_account" class="muted small">Önce kanal seçin.</span>
        <span v-else-if="uploadingHeaderMedia" class="muted small">Yükleniyor…</span>
        <span v-else-if="form.header_media_filename" class="muted small">✓ {{ form.header_media_filename }}</span>
        <p v-if="headerUploadError" class="error small">{{ headerUploadError }}</p>
      </div>
      <div class="field">
        <label>Gövde * <span class="muted small">(değişken için {{ VAR1 }} kullanın)</span></label>
        <textarea v-model="form.body_content" rows="3" :placeholder="'Merhaba ' + VAR1 + ', size özel fırsatımız var.'"></textarea>
      </div>
      <div class="field">
        <label>Altbilgi (opsiyonel)</label>
        <input v-model="form.footer_content" placeholder="Ör. B2B Enerji" />
      </div>
      <div class="field">
        <label>Butonlar (opsiyonel · en fazla 3)</label>
        <div v-for="(b, i) in form.buttons" :key="i" class="btn-row">
          <select v-model="b.type" class="btn-type">
            <option value="QUICK_REPLY">Hızlı Yanıt</option>
            <option value="URL">URL</option>
            <option value="PHONE_NUMBER">Telefon</option>
          </select>
          <input v-model="b.text" placeholder="Buton yazısı" />
          <input v-if="b.type === 'URL'" v-model="b.url" placeholder="https://..." />
          <input v-if="b.type === 'PHONE_NUMBER'" v-model="b.phone_number" placeholder="+90..." />
          <button type="button" class="danger-btn" @click="removeTplButton(i)">✕</button>
        </div>
        <button v-if="form.buttons.length < 3" type="button" @click="addTplButton">＋ Buton</button>
      </div>
      <p v-if="editError" class="error">{{ editError }}</p>
      <div class="form-actions">
        <button type="button" @click="showEditor = false">İptal</button>
        <button type="button" :disabled="saving" @click="saveTemplate(false)">Taslak Kaydet</button>
        <button class="primary" type="submit" :disabled="saving">Kaydet ve Meta’ya Gönder</button>
      </div>
    </form>

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
      <div v-if="t.header_content" class="tpl-header">
        <template v-if="t.header_type === 'IMAGE'">🖼️ Görsel başlık</template>
        <template v-else-if="t.header_type === 'VIDEO'">🎥 Video başlık</template>
        <template v-else-if="t.header_type === 'DOCUMENT'">📄 Belge başlık</template>
        <template v-else>{{ t.header_content }}</template>
      </div>
      <div class="tpl-body">{{ t.body_content }}</div>
      <div v-if="t.footer_content" class="tpl-footer muted small">{{ t.footer_content }}</div>
      <div v-if="t.buttons?.length" class="tpl-buttons">
        <span v-for="(b, i) in t.buttons" :key="i" class="tpl-btn">{{ (b as any).text || (b as any).title || 'Buton' }}</span>
      </div>
      <div class="tpl-actions">
        <button
          v-if="t.status === 'APPROVED'"
          :class="{ 'first-message-on': t.is_first_message }"
          @click="toggleFirstMessage(t)"
        >{{ t.is_first_message ? '✓ İlk mesaj olarak açık' : 'İlk mesaj olarak kullan' }}</button>
        <button v-if="t.status !== 'PENDING'" @click="startEditTpl(t)">Düzenle</button>
        <button v-if="t.status !== 'APPROVED' && t.status !== 'PENDING'" @click="submitTpl(t)">Meta’ya Gönder</button>
        <button class="danger-btn" @click="removeTpl(t)">Sil</button>
      </div>
    </div>
  </div>
</template>

<style scoped>
.page { padding: 24px; max-width: 800px; }
.page-head { margin-bottom: 16px; display: flex; justify-content: space-between; align-items: flex-start; gap: 12px; }
.page-head h1 { margin: 0 0 4px; font-size: 20px; }
.page-head p { margin: 0; }
.center { text-align: center; padding: 24px; }
.explainer { margin-bottom: 16px; font-size: 14px; line-height: 1.6; }
.sync-row { display: flex; gap: 8px; align-items: center; margin-top: 12px; flex-wrap: wrap; }
.sync-row select { max-width: 220px; }

.editor { margin-bottom: 16px; display: flex; flex-direction: column; gap: 12px; }
.editor .row { display: flex; gap: 12px; flex-wrap: wrap; }
.editor .field { display: flex; flex-direction: column; gap: 4px; }
.editor .field.grow { flex: 1; min-width: 160px; }
.editor label { font-size: 12px; color: var(--muted); }
.editor textarea { width: 100%; font-family: inherit; }
.btn-row { display: flex; gap: 6px; margin-bottom: 6px; }
.btn-row .btn-type { max-width: 130px; }
.btn-row input { flex: 1; }
.form-actions { display: flex; justify-content: flex-end; gap: 8px; flex-wrap: wrap; }
.error { color: var(--danger); margin: 0; }
.tpl-actions { display: flex; gap: 8px; margin-top: 10px; padding-top: 10px; border-top: 1px solid var(--border); }
.danger-btn { color: var(--danger); }
.first-message-on { color: #087a55; border-color: #87d7b7; background: #edfbf5; }

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
