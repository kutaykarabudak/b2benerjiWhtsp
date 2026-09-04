<script setup lang="ts">
import { ref, onMounted, watch } from 'vue'
import { useRouter } from 'vue-router'
import {
  listContacts,
  createContact,
  updateContact,
  deleteContact,
  downloadContactsCSV,
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
const emptyForm = () => ({ phone_number: '', profile_name: '', company_name: '', email: '', tax_office: '', tax_number: '', address: '', city: '', district: '', postal_code: '', purchase_score: 0, has_purchased: false, notes: '', tags: '' })
const form = ref(emptyForm())
const editingId = ref<string | null>(null)
const filters = ref({ registry: 'b2b', category: '', purchased: '', minScore: '', city: '', district: '' })

function startChat(c: Contact) {
  router.push({ path: '/inbox', query: { contact: c.id } })
}
const saving = ref(false)
const formError = ref('')

// CSV import
const fileInput = ref<HTMLInputElement | null>(null)
const importing = ref(false)
const downloading = ref(false)
const importResult = ref<ImportResult | null>(null)

async function downloadCSVTemplate() {
  downloading.value = true
  try {
    const blob = await downloadContactsCSV()
    const url = URL.createObjectURL(blob)
    const link = document.createElement('a')
    link.href = url
    link.download = `kisiler_tam_liste_${new Date().toISOString().slice(0, 10)}.xlsx`
    document.body.appendChild(link)
    link.click()
    link.remove()
    URL.revokeObjectURL(url)
  } catch (err: any) {
    alert(err?.response?.data?.message || 'Mevcut kisi listesi indirilemedi.')
  } finally {
    downloading.value = false
  }
}

async function load() {
  loading.value = true
  try {
    contacts.value = await listContacts(search.value, {
      tags: filters.value.category || undefined,
      has_purchased: filters.value.purchased ? filters.value.purchased === 'yes' : undefined,
      min_purchase_score: filters.value.minScore ? Number(filters.value.minScore) : undefined,
      city: filters.value.city || undefined,
      district: filters.value.district || undefined,
      b2b_registered: filters.value.registry === 'all' ? undefined : filters.value.registry === 'b2b'
    })
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
    if (form.value.notes.trim()) metadata.notes = form.value.notes.trim()
    const payload = {
      phone_number: form.value.phone_number.trim(),
      profile_name: form.value.profile_name.trim() || undefined,
      company_name: form.value.company_name.trim(), email: form.value.email.trim(),
      tax_office: form.value.tax_office.trim(), tax_number: form.value.tax_number.trim(),
      address: form.value.address.trim(), city: form.value.city.trim(), district: form.value.district.trim(), postal_code: form.value.postal_code.trim(),
      purchase_score: Number(form.value.purchase_score), has_purchased: form.value.has_purchased,
      tags: form.value.tags
        ? form.value.tags.split(',').map((t) => t.trim()).filter(Boolean)
        : undefined,
      metadata: Object.keys(metadata).length ? metadata : undefined
    }
    if (editingId.value) await updateContact(editingId.value, payload)
    else await createContact(payload)
    form.value = emptyForm()
    editingId.value = null
    showAdd.value = false
    await load()
  } catch (e: any) {
    formError.value = e?.response?.data?.message || 'Kişi eklenemedi.'
  } finally {
    saving.value = false
  }
}

function edit(c: Contact) {
  editingId.value = c.id
  form.value = { phone_number: c.phone_number, profile_name: c.profile_name || c.name || '', company_name: c.company_name || '', email: c.email || '', tax_office: c.tax_office || '', tax_number: c.tax_number || '', address: c.address || '', city: c.city || '', district: c.district || '', postal_code: c.postal_code || '', purchase_score: c.purchase_score || 0, has_purchased: !!c.has_purchased, notes: String(c.metadata?.notes || ''), tags: (c.tags || []).join(', ') }
  showAdd.value = true
  window.scrollTo({ top: 0, behavior: 'smooth' })
}

function cancelForm() { showAdd.value = false; editingId.value = null; form.value = emptyForm() }
function clearFilters() { filters.value = { registry: 'b2b', category: '', purchased: '', minScore: '', city: '', district: '' }; load() }
function openCampaigns() { router.push({ path: '/campaigns', query: { tags: filters.value.category || undefined, has_purchased: filters.value.purchased || undefined, min_purchase_score: filters.value.minScore || undefined, city: filters.value.city || undefined, district: filters.value.district || undefined } }) }

async function remove(c: Contact) {
  if (!confirm(`${c.name || c.phone_number} silinsin mi?`)) return
  await deleteContact(c.id)
  await load()
}

async function onFileChosen(e: Event) {
  const input = e.target as HTMLInputElement
  const file = input.files?.[0]
  if (!file) return
  if (!confirm('Yuklenen dosya yeni tam kisi listesi olacak. Dosyada bulunmayan mevcut kisiler silinecek. Devam edilsin mi?')) {
    input.value = ''
    return
  }
  importing.value = true
  importResult.value = null
  try {
    importResult.value = await importContactsCSV(file)
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
        <button @click="showAdd ? cancelForm() : showAdd = true">＋ Kişi Ekle</button>
        <button type="button" :disabled="downloading" @click="downloadCSVTemplate">
          {{ downloading ? 'Liste hazırlanıyor…' : '⬇ Mevcut Liste / Şablon İndir' }}
        </button>
        <button class="primary" :disabled="importing" @click="fileInput?.click()">
          {{ importing ? 'İçe aktarılıyor…' : '⬆ Excel İçe Aktar' }}
        </button>
        <input ref="fileInput" type="file" accept=".xlsx,.csv,application/vnd.openxmlformats-officedocument.spreadsheetml.sheet,text/csv" hidden @change="onFileChosen" />
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
          <label>Firma / Ünvan</label>
          <input v-model="form.company_name" placeholder="Firma adı" />
        </div>
      </div>
      <div class="row">
        <div class="field"><label>Vergi dairesi</label><input v-model="form.tax_office" /></div>
        <div class="field"><label>Vergi / T.C. kimlik no</label><input v-model="form.tax_number" /></div>
        <div class="field"><label>Posta kodu</label><input v-model="form.postal_code" /></div>
      </div>
      <div class="row">
        <div class="field"><label>Şehir</label><input v-model="form.city" /></div>
        <div class="field"><label>İlçe</label><input v-model="form.district" /></div>
        <div class="field grow"><label>Açık adres</label><input v-model="form.address" /></div>
      </div>
      <div class="row">
        <div class="field"><label>Satın alma puanı (0–100)</label><input v-model.number="form.purchase_score" type="number" min="0" max="100" /></div>
        <label class="check-field"><input v-model="form.has_purchased" type="checkbox" /> Daha önce satın alım yaptı</label>
      </div>
      <div class="row">
        <div class="field">
          <label>E-posta</label>
          <input v-model="form.email" placeholder="ornek@firma.com" />
        </div>
        <div class="field">
          <label>Kategoriler (virgülle, birden fazla seçilebilir)</label>
          <input v-model="form.tags" placeholder="müşteri, vip, bayi" />
        </div>
        <div class="field grow">
          <label>Not</label>
          <input v-model="form.notes" placeholder="Müşteri notu…" />
        </div>
      </div>
      <p v-if="formError" class="error">{{ formError }}</p>
      <div class="form-actions">
        <button type="button" @click="cancelForm">İptal</button>
        <button class="primary" type="submit" :disabled="saving">{{ editingId ? 'Güncelle' : 'Kaydet' }}</button>
      </div>
    </form>

    <!-- Spreadsheet import hint / result -->
    <div class="csv-hint">
      <div>
        <b>Excel yükleme kuralı</b>
        <ul class="muted">
          <li>Önce mevcut listeyi indirin; A sütununda sistem <code>ID</code>, B sütununda dış sistem eşleştirmesi için <code>B2B Panel ID</code> bulunur.</li>
          <li>Mevcut kişiyi güncellemek için ID değerini değiştirmeyin. ID boşsa satır yeni kişi olarak oluşturulur.</li>
          <li><code>B2B Panel ID</code> yalnızca Excel eşleştirmesi içindir; kişi ekranlarında gösterilmez.</li>
          <li>Yüklenen dosya tam listedir; dosyada olmayan eski kişiler silinir. Hatalı satır varsa hiçbir değişiklik uygulanmaz.</li>
          <li>Dosya gerçek <code>.xlsx</code> biçimindedir; Türkçe karakterler ve uzun telefon numaraları Excel tarafından bozulmaz.</li>
        </ul>
      </div>
      <strong class="replace-warning">Dosya mevcut kişi listesinin tamamının yerine geçer.</strong>
    </div>
    <div v-if="importResult" class="card import-result">
      İçe aktarma tamam: <b>{{ importResult.created }}</b> eklendi,
      <b>{{ importResult.updated }}</b> güncellendi,
      <b>{{ importResult.deleted }}</b> silindi,
      <b>{{ importResult.skipped }}</b> atlandı,
      <b>{{ importResult.errors }}</b> hata.
      <ul v-if="importResult.messages?.length" class="err-list">
        <li v-for="(m, i) in importResult.messages.slice(0, 10)" :key="i">{{ m }}</li>
      </ul>
    </div>

    <div class="card crm-filters">
      <div class="filter-head"><div><b>CRM hedef kitle filtreleri</b><div class="muted small">Filtreleri birleştirerek toplu mesaj kitlesi oluşturun.</div></div><button class="primary" @click="openCampaigns">📣 Toplu Mesaj</button></div>
      <div class="row">
        <div class="field"><label>Kayıt kaynağı</label><select v-model="filters.registry"><option value="b2b">Yalnızca B2B müşterileri</option><option value="all">Tüm kişiler</option><option value="whatsapp">B2B kaydı olmayanlar</option></select></div>
        <div class="field"><label>Kategori</label><input v-model="filters.category" placeholder="Ör. vip" /></div>
        <div class="field"><label>Satın alım</label><select v-model="filters.purchased"><option value="">Tümü</option><option value="yes">Alım yapanlar</option><option value="no">Alım yapmayanlar</option></select></div>
        <div class="field"><label>Minimum puan</label><input v-model="filters.minScore" type="number" min="0" max="100" /></div>
        <div class="field"><label>Şehir</label><input v-model="filters.city" /></div>
        <div class="field"><label>İlçe</label><input v-model="filters.district" /></div>
      </div>
      <div class="form-actions"><button @click="clearFilters">Temizle</button><button @click="load">Filtrele</button></div>
    </div>
    <input class="search" v-model="search" placeholder="Ara: isim / telefon" />

    <div class="card table-card">
      <table>
        <thead>
          <tr><th>İsim</th><th>Telefon</th><th>Konum</th><th>Kategoriler</th><th>Satın alma</th><th></th></tr>
        </thead>
        <tbody>
          <tr v-if="loading"><td colspan="6" class="muted center">Yükleniyor…</td></tr>
          <tr v-else-if="!contacts.length"><td colspan="6" class="muted center">Kişi yok.</td></tr>
          <tr v-for="c in contacts" :key="c.id">
            <td>{{ c.name || c.profile_name || '—' }}</td>
            <td>{{ c.phone_number }}</td>
            <td>{{ [c.city, c.district].filter(Boolean).join(' / ') || '—' }}</td>
            <td class="tags-cell">{{ (c.tags || []).join(', ') || '—' }}</td>
            <td><span :class="['score', { bought: c.has_purchased }]">{{ c.purchase_score || 0 }} puan</span></td>
            <td class="right">
              <button @click="startChat(c)">💬 Sohbet</button>
              <button @click="edit(c)">Düzenle</button>
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
.crm-filters { margin-bottom: 14px; }
.filter-head { display: flex; justify-content: space-between; gap: 12px; margin-bottom: 12px; }
.check-field { display: flex; align-items: center; gap: 8px; min-width: 220px; }
.check-field input { width: auto; }
.score { font-size: 12px; border: 1px solid var(--border); padding: 3px 8px; border-radius: 999px; white-space: nowrap; }
.score.bought { color: #0a6d4e; border-color: #0a6d4e; }
.csv-hint { display: flex; justify-content: space-between; align-items: flex-start; gap: 20px; margin: 14px 0; padding: 14px 16px; border: 1px solid var(--border); border-radius: var(--radius); background: var(--bg-2); }
.csv-hint ul { margin: 7px 0 0; padding-left: 18px; }
.csv-hint li + li { margin-top: 3px; }
.replace-warning { color: var(--danger); max-width: 280px; }
.row { display: flex; gap: 12px; flex-wrap: wrap; }
.field { flex: 1; min-width: 180px; display: flex; flex-direction: column; gap: 4px; }
.field label { font-size: 12px; color: var(--muted); }
.form-actions { display: flex; justify-content: flex-end; gap: 8px; margin-top: 12px; }
.error { color: var(--danger); margin: 8px 0 0; }

.csv-hint code { background: var(--bg); padding: 1px 5px; border-radius: 4px; }
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
@media (max-width: 760px) {
  .csv-hint { flex-direction: column; }
}
</style>
