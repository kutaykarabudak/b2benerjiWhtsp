<script setup lang="ts">
import { ref, onMounted, watch } from 'vue'
import { useRouter } from 'vue-router'
import {
  listContacts,
  createContact,
  deleteContact,
  importContactsCSV,
  type Contact,
  type ImportResult
} from '@/services/contacts'

const contacts = ref<Contact[]>([])
const loading = ref(false)
const search = ref('')
let searchTimer: number | undefined

// Add-contact form
const showAdd = ref(false)
const router = useRouter()
const form = ref({ phone_number: '', profile_name: '', company: '', email: '', notes: '', tags: '' })

function startChat(c: Contact) {
  router.push({ path: '/inbox', query: { contact: c.id } })
}
const saving = ref(false)
const formError = ref('')

// CSV import
const fileInput = ref<HTMLInputElement | null>(null)
const importing = ref(false)
const updateOnDup = ref(true)
const importResult = ref<ImportResult | null>(null)

async function load() {
  loading.value = true
  try {
    contacts.value = await listContacts(search.value)
  } finally {
    loading.value = false
  }
}

async function submitAdd() {
  formError.value = ''
  if (!form.value.phone_number.trim()) {
    formError.value = 'Telefon numarası zorunlu.'
    return
  }
  saving.value = true
  try {
    const metadata: Record<string, unknown> = {}
    if (form.value.company.trim()) metadata.company = form.value.company.trim()
    if (form.value.email.trim()) metadata.email = form.value.email.trim()
    if (form.value.notes.trim()) metadata.notes = form.value.notes.trim()
    await createContact({
      phone_number: form.value.phone_number.trim(),
      profile_name: form.value.profile_name.trim() || undefined,
      tags: form.value.tags
        ? form.value.tags.split(',').map((t) => t.trim()).filter(Boolean)
        : undefined,
      metadata: Object.keys(metadata).length ? metadata : undefined
    })
    form.value = { phone_number: '', profile_name: '', company: '', email: '', notes: '', tags: '' }
    showAdd.value = false
    await load()
  } catch (e: any) {
    formError.value = e?.response?.data?.message || 'Kişi eklenemedi.'
  } finally {
    saving.value = false
  }
}

async function remove(c: Contact) {
  if (!confirm(`${c.name || c.phone_number} silinsin mi?`)) return
  await deleteContact(c.id)
  await load()
}

async function onFileChosen(e: Event) {
  const input = e.target as HTMLInputElement
  const file = input.files?.[0]
  if (!file) return
  importing.value = true
  importResult.value = null
  try {
    importResult.value = await importContactsCSV(file, updateOnDup.value)
    await load()
  } catch (err: any) {
    alert(err?.response?.data?.message || 'CSV içe aktarılamadı.')
  } finally {
    importing.value = false
    input.value = '' // allow re-selecting the same file
  }
}

watch(search, () => {
  window.clearTimeout(searchTimer)
  searchTimer = window.setTimeout(load, 350)
})
onMounted(load)
</script>

<template>
  <div class="page">
    <header class="page-head">
      <div>
        <h1>Kişiler</h1>
        <p class="muted">Kişi listesi ve CSV içe aktarma.</p>
      </div>
      <div class="actions">
        <button @click="showAdd = !showAdd">＋ Kişi Ekle</button>
        <button class="primary" :disabled="importing" @click="fileInput?.click()">
          {{ importing ? 'İçe aktarılıyor…' : '⬆ CSV İçe Aktar' }}
        </button>
        <input ref="fileInput" type="file" accept=".csv,text/csv" hidden @change="onFileChosen" />
      </div>
    </header>

    <!-- Add contact -->
    <form v-if="showAdd" class="card add-form" @submit.prevent="submitAdd">
      <div class="row">
        <div class="field">
          <label>Telefon *</label>
          <input v-model="form.phone_number" placeholder="905551112233" />
        </div>
        <div class="field">
          <label>İsim</label>
          <input v-model="form.profile_name" placeholder="Ad Soyad" />
        </div>
        <div class="field">
          <label>Şirket</label>
          <input v-model="form.company" placeholder="Firma adı" />
        </div>
      </div>
      <div class="row">
        <div class="field">
          <label>E-posta</label>
          <input v-model="form.email" placeholder="ornek@firma.com" />
        </div>
        <div class="field">
          <label>Etiketler (virgülle)</label>
          <input v-model="form.tags" placeholder="müşteri, vip" />
        </div>
        <div class="field grow">
          <label>Not</label>
          <input v-model="form.notes" placeholder="Müşteri notu…" />
        </div>
      </div>
      <p v-if="formError" class="error">{{ formError }}</p>
      <div class="form-actions">
        <button type="button" @click="showAdd = false">İptal</button>
        <button class="primary" type="submit" :disabled="saving">Kaydet</button>
      </div>
    </form>

    <!-- CSV import hint / result -->
    <div class="csv-hint muted">
      CSV kolonları: <code>phone_number</code> (zorunlu), <code>profile_name</code>, <code>tags</code>,
      <code>whats_app_account</code>.
      <label class="dup"><input type="checkbox" v-model="updateOnDup" /> Var olanları güncelle</label>
    </div>
    <div v-if="importResult" class="card import-result">
      İçe aktarma tamam: <b>{{ importResult.created }}</b> eklendi,
      <b>{{ importResult.updated }}</b> güncellendi,
      <b>{{ importResult.skipped }}</b> atlandı,
      <b>{{ importResult.errors }}</b> hata.
      <ul v-if="importResult.messages?.length" class="err-list">
        <li v-for="(m, i) in importResult.messages.slice(0, 10)" :key="i">{{ m }}</li>
      </ul>
    </div>

    <input class="search" v-model="search" placeholder="Ara: isim / telefon" />

    <div class="card table-card">
      <table>
        <thead>
          <tr><th>İsim</th><th>Telefon</th><th>Kanal</th><th>Etiketler</th><th></th></tr>
        </thead>
        <tbody>
          <tr v-if="loading"><td colspan="5" class="muted center">Yükleniyor…</td></tr>
          <tr v-else-if="!contacts.length"><td colspan="5" class="muted center">Kişi yok.</td></tr>
          <tr v-for="c in contacts" :key="c.id">
            <td>{{ c.name || c.profile_name || '—' }}</td>
            <td>{{ c.phone_number }}</td>
            <td>{{ c.channel_type }}</td>
            <td class="tags-cell muted">—</td>
            <td class="right">
              <button @click="startChat(c)">💬 Sohbet</button>
              <button class="danger-btn" @click="remove(c)">Sil</button>
            </td>
          </tr>
        </tbody>
      </table>
    </div>
  </div>
</template>

<style scoped>
.page { padding: 24px; max-width: 1100px; }
.page-head { display: flex; justify-content: space-between; align-items: flex-start; margin-bottom: 16px; gap: 12px; }
.page-head h1 { margin: 0 0 4px; font-size: 20px; }
.page-head p { margin: 0; }
.actions { display: flex; gap: 8px; flex-shrink: 0; }

.add-form { margin-bottom: 16px; }
.row { display: flex; gap: 12px; flex-wrap: wrap; }
.field { flex: 1; min-width: 180px; display: flex; flex-direction: column; gap: 4px; }
.field label { font-size: 12px; color: var(--muted); }
.form-actions { display: flex; justify-content: flex-end; gap: 8px; margin-top: 12px; }
.error { color: var(--danger); margin: 8px 0 0; }

.csv-hint { font-size: 13px; margin-bottom: 12px; display: flex; align-items: center; gap: 12px; flex-wrap: wrap; }
.csv-hint code { background: var(--bg); padding: 1px 5px; border-radius: 4px; }
.dup { display: flex; align-items: center; gap: 6px; }
.dup input { width: auto; }
.import-result { margin-bottom: 16px; }
.err-list { margin: 8px 0 0; padding-left: 18px; font-size: 12px; }

.search { max-width: 320px; margin-bottom: 12px; }

.table-card { padding: 0; overflow: hidden; }
table { width: 100%; border-collapse: collapse; }
th, td { text-align: left; padding: 10px 14px; border-bottom: 1px solid var(--border); }
th { font-size: 12px; color: var(--muted); font-weight: 600; }
tr:last-child td { border-bottom: none; }
.center { text-align: center; padding: 24px; }
.right { text-align: right; }
.danger-btn { color: var(--danger); border-color: var(--border); }
</style>
