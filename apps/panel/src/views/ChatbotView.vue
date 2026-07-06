<script setup lang="ts">
import { ref, onMounted } from 'vue'
import {
  listRules,
  createRule,
  updateRule,
  toggleRule,
  deleteRule,
  type KeywordRule,
  type RuleInput,
  type MatchType
} from '@/services/chatbot'

const rules = ref<KeywordRule[]>([])
const loading = ref(false)

const MATCH_LABEL: Record<MatchType, string> = {
  contains: 'İçeriyor',
  exact: 'Tam eşleşme',
  starts_with: 'İle başlıyor',
  regex: 'Regex'
}

// Editor state (create or edit)
const editing = ref(false)
const editId = ref<string | null>(null)
const form = ref<RuleInput & { keywordsText: string }>(blankForm())
const saving = ref(false)
const formError = ref('')

function blankForm() {
  return {
    name: '',
    keywords: [] as string[],
    keywordsText: '',
    match_type: 'contains' as MatchType,
    reply: '',
    buttons: [] as { id: string; title: string }[],
    priority: 10,
    enabled: true
  }
}

function addButton() {
  if (form.value.buttons.length < 3) form.value.buttons.push({ id: '', title: '' })
}
function removeButton(i: number) {
  form.value.buttons.splice(i, 1)
}

async function load() {
  loading.value = true
  try {
    rules.value = await listRules()
  } finally {
    loading.value = false
  }
}

function startCreate() {
  form.value = blankForm()
  editId.value = null
  editing.value = true
}

function startEdit(r: KeywordRule) {
  form.value = {
    name: r.name,
    keywords: r.keywords,
    keywordsText: (r.keywords || []).join(', '),
    match_type: r.match_type,
    reply: r.response_content?.body ?? '',
    buttons: Array.isArray((r.response_content as any)?.buttons)
      ? (r.response_content as any).buttons.map((b: any) => ({ id: b.id ?? b.title, title: b.title ?? '' }))
      : [],
    priority: r.priority,
    enabled: r.enabled
  }
  editId.value = r.id
  editing.value = true
}

async function save() {
  formError.value = ''
  const keywords = form.value.keywordsText
    .split(/[\n,;]+/)
    .map((k) => k.trim())
    .filter(Boolean)
  if (!keywords.length) {
    formError.value = 'En az bir anahtar kelime girin.'
    return
  }
  if (!form.value.reply.trim()) {
    formError.value = 'Yanıt metni zorunlu.'
    return
  }
  const payload: RuleInput = {
    name: form.value.name.trim(),
    keywords,
    match_type: form.value.match_type,
    reply: form.value.reply.trim(),
    buttons: form.value.buttons.filter((b) => b.title.trim()),
    priority: Number(form.value.priority) || 10,
    enabled: form.value.enabled
  }
  saving.value = true
  try {
    if (editId.value) await updateRule(editId.value, payload)
    else await createRule(payload)
    editing.value = false
    await load()
  } catch (e: any) {
    formError.value = e?.response?.data?.message || 'Kaydedilemedi.'
  } finally {
    saving.value = false
  }
}

async function onToggle(r: KeywordRule) {
  try {
    await toggleRule(r.id, !r.enabled)
    r.enabled = !r.enabled
  } catch {
    alert('Durum değiştirilemedi.')
  }
}

async function remove(r: KeywordRule) {
  if (!confirm(`"${r.name}" kuralı silinsin mi?`)) return
  await deleteRule(r.id)
  await load()
}

onMounted(load)
</script>

<template>
  <div class="page">
    <header class="page-head">
      <div>
        <h1>Chatbot</h1>
        <p class="muted">Anahtar kelime geldiğinde otomatik yanıt. Kurallar tüm kanallarda geçerlidir.</p>
      </div>
      <button class="primary" @click="startCreate">＋ Yeni Kural</button>
    </header>

    <!-- How it works -->
    <div class="card explainer">
      <b>Nasıl çalışır?</b>
      <ol>
        <li>Bir <b>anahtar kelime</b> belirlersin (ör. "merhaba", "fiyat").</li>
        <li>Müşteri o kelimeyi yazınca panel <b>otomatik yanıt</b> gönderir.</li>
        <li>İstersen yanıta <b>çoktan seçmeli butonlar</b> eklersin (ör. [Fiyat] [Sipariş]) — <span class="muted">sadece Cloud API kanalında</span>.</li>
        <li>Butona basınca gelen yazı (buton başlığı) yeni bir kuralı tetikler → böylece <b>menü akışı</b> kurulur.</li>
      </ol>
      <div class="example muted">
        Örnek: "merhaba" → "Nasıl yardımcı olalım?" + [Fiyat] [Destek] &nbsp;·&nbsp; sonra "Fiyat" → "Fiyat listemiz…"
      </div>
    </div>

    <!-- Editor -->
    <form v-if="editing" class="card editor" @submit.prevent="save">
      <div class="row">
        <div class="field grow">
          <label>Kural adı</label>
          <input v-model="form.name" placeholder="(boşsa ilk kelime kullanılır)" />
        </div>
        <div class="field">
          <label>Eşleşme</label>
          <select v-model="form.match_type">
            <option value="contains">İçeriyor</option>
            <option value="exact">Tam eşleşme</option>
            <option value="starts_with">İle başlıyor</option>
            <option value="regex">Regex</option>
          </select>
        </div>
        <div class="field small-field">
          <label>Öncelik</label>
          <input v-model="form.priority" type="number" />
        </div>
      </div>
      <div class="field">
        <label>Anahtar kelimeler (virgülle)</label>
        <input v-model="form.keywordsText" placeholder="merhaba, selam, fiyat" />
      </div>
      <div class="field">
        <label>Otomatik yanıt</label>
        <textarea v-model="form.reply" rows="3" placeholder="Merhaba! Size nasıl yardımcı olabiliriz?"></textarea>
      </div>

      <div class="field">
        <label>
          Butonlar (çoktan seçmeli · en fazla 3)
          <span class="muted small">— sadece Cloud API kanalında çalışır (QR'da yok)</span>
        </label>
        <div v-for="(btn, i) in form.buttons" :key="i" class="btn-row">
          <input v-model="btn.title" maxlength="20" placeholder="Buton yazısı (ör. Fiyat)" />
          <button type="button" class="danger-btn" @click="removeButton(i)">✕</button>
        </div>
        <button v-if="form.buttons.length < 3" type="button" class="add-btn" @click="addButton">＋ Buton ekle</button>
        <p class="muted small">
          İpucu: butona basınca gelen yazı (buton başlığı) bir sonraki kurala anahtar kelime olur.
          Ör. "Fiyat" butonu → anahtar kelimesi "Fiyat" olan başka bir kural yanıt verir.
        </p>
      </div>
      <label class="enable"><input type="checkbox" v-model="form.enabled" /> Aktif</label>
      <p v-if="formError" class="error">{{ formError }}</p>
      <div class="form-actions">
        <button type="button" @click="editing = false">İptal</button>
        <button class="primary" type="submit" :disabled="saving">Kaydet</button>
      </div>
    </form>

    <!-- Rules list -->
    <div v-if="loading" class="muted center">Yükleniyor…</div>
    <div v-else-if="!rules.length" class="card center muted">Henüz kural yok.</div>

    <div v-for="r in rules" :key="r.id" class="card rule" :class="{ off: !r.enabled }">
      <div class="rule-main">
        <div class="rule-top">
          <span class="rule-name">{{ r.name }}</span>
          <span class="tag">{{ MATCH_LABEL[r.match_type] || r.match_type }}</span>
          <span class="tag muted">öncelik {{ r.priority }}</span>
        </div>
        <div class="kw">
          <span v-for="k in r.keywords" :key="k" class="chip">{{ k }}</span>
        </div>
        <div class="reply muted">↳ {{ r.response_content?.body || '—' }}</div>
        <div v-if="(r.response_content as any)?.buttons?.length" class="rule-buttons">
          <span v-for="(b, i) in (r.response_content as any).buttons" :key="i" class="btn-chip">{{ b.title }}</span>
        </div>
      </div>
      <div class="rule-actions">
        <label class="switch">
          <input type="checkbox" :checked="r.enabled" @change="onToggle(r)" />
          <span>{{ r.enabled ? 'Aktif' : 'Kapalı' }}</span>
        </label>
        <button @click="startEdit(r)">Düzenle</button>
        <button class="danger-btn" @click="remove(r)">Sil</button>
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

.editor { margin-bottom: 16px; display: flex; flex-direction: column; gap: 12px; }
.row { display: flex; gap: 12px; flex-wrap: wrap; }
.field { display: flex; flex-direction: column; gap: 4px; }
.field.grow { flex: 1; min-width: 180px; }
.small-field { width: 100px; }
.field label { font-size: 12px; color: var(--muted); }
.editor textarea { width: 100%; font-family: inherit; }
.enable { display: flex; align-items: center; gap: 6px; font-size: 13px; }
.enable input { width: auto; }
.form-actions { display: flex; justify-content: flex-end; gap: 8px; }
.error { color: var(--danger); margin: 0; }

.rule { display: flex; justify-content: space-between; align-items: flex-start; gap: 16px; margin-bottom: 10px; }
.rule.off { opacity: 0.6; }
.rule-main { min-width: 0; flex: 1; }
.rule-top { display: flex; align-items: center; gap: 8px; flex-wrap: wrap; margin-bottom: 6px; }
.rule-name { font-weight: 600; }
.tag { font-size: 11px; padding: 1px 8px; border-radius: 999px; background: var(--bg); border: 1px solid var(--border); }
.kw { display: flex; gap: 6px; flex-wrap: wrap; margin-bottom: 6px; }
.chip { font-size: 12px; padding: 2px 9px; border-radius: 999px; background: var(--brand); color: #fff; }
.reply { font-size: 13px; white-space: pre-wrap; word-break: break-word; }
.btn-row { display: flex; gap: 6px; margin-bottom: 6px; }
.btn-row input { flex: 1; }
.add-btn { align-self: flex-start; }
.rule-buttons { display: flex; gap: 6px; flex-wrap: wrap; margin-top: 6px; }
.btn-chip { font-size: 12px; padding: 2px 10px; border-radius: 6px; border: 1px solid var(--brand); color: var(--brand); background: #fff; }
.explainer { margin-bottom: 16px; font-size: 14px; }
.explainer ol { margin: 8px 0 0; padding-left: 20px; line-height: 1.7; }
.example { margin-top: 10px; font-size: 13px; padding: 8px 10px; background: var(--bg); border-radius: var(--radius); }
.rule-actions { display: flex; flex-direction: column; align-items: flex-end; gap: 6px; flex-shrink: 0; }
.switch { display: flex; align-items: center; gap: 6px; font-size: 12px; color: var(--muted); }
.switch input { width: auto; }
.danger-btn { color: var(--danger); }
</style>
