<script setup lang="ts">
import { ref, onMounted } from 'vue'
import {
  listTemplates,
  syncTemplates,
  createTemplate,
  submitTemplate,
  deleteTemplate,
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
const saving = ref(false)
const editError = ref('')
const form = ref(blankForm())

function blankForm() {
  return {
    whatsapp_account: '',
    name: '',
    language: 'tr',
    category: 'MARKETING',
    header_content: '',
    body_content: '',
    footer_content: '',
    buttons: [] as TemplateButton[]
  }
}

function openEditor() {
  form.value = blankForm()
  if (accounts.value.length) form.value.whatsapp_account = accounts.value[0].name
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
  saving.value = true
  try {
    const id = await createTemplate({
      whatsapp_account: f.whatsapp_account,
      name: f.name.trim(),
      language: f.language.trim() || 'tr',
      category: f.category,
      header_type: f.header_content.trim() ? 'TEXT' : '',
      header_content: f.header_content.trim(),
      body_content: f.body_content.trim(),
      footer_content: f.footer_content.trim(),
      buttons: f.buttons.filter((b) => b.text.trim())
    })
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
        <label>Başlık (opsiyonel, metin)</label>
        <input v-model="form.header_content" placeholder="Ör. Kampanya!" />
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
      <div v-if="t.header_content" class="tpl-header">{{ t.header_content }}</div>
      <div class="tpl-body">{{ t.body_content }}</div>
      <div v-if="t.footer_content" class="tpl-footer muted small">{{ t.footer_content }}</div>
      <div v-if="t.buttons?.length" class="tpl-buttons">
        <span v-for="(b, i) in t.buttons" :key="i" class="tpl-btn">{{ (b as any).text || (b as any).title || 'Buton' }}</span>
      </div>
      <div class="tpl-actions">
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
