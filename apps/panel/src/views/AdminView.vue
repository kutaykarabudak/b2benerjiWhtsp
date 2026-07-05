<script setup lang="ts">
import { ref, onMounted } from 'vue'
import {
  listUsers,
  createUser,
  setUserActive,
  deleteUser,
  listRoles,
  listAccounts,
  createAccount,
  deleteAccount,
  testAccount,
  getMetaAppSettings,
  saveMetaAppSettings,
  type User,
  type Role,
  type WhatsAppAccount,
  type AccountInput
} from '@/services/admin'
import {
  getEmbeddedSignupConfig,
  loadFacebookSDK,
  launchWhatsAppSignup,
  exchangeToken,
  type EmbeddedSignupConfig
} from '@/services/embeddedSignup'
import { getQRStatus, qrConnect, qrLogout, type QRStatus } from '@/services/qr'
import QRCode from 'qrcode'
import { onBeforeUnmount } from 'vue'

type Tab = 'users' | 'channels'
const tab = ref<Tab>('users')

// ---- Users ----
const users = ref<User[]>([])
const roles = ref<Role[]>([])
const loadingUsers = ref(false)
const showUserForm = ref(false)
const userForm = ref({ email: '', full_name: '', password: '', role_id: '' })
const savingUser = ref(false)
const userError = ref('')

async function loadUsers() {
  loadingUsers.value = true
  try {
    const [u, r] = await Promise.all([listUsers(), listRoles()])
    users.value = u
    roles.value = r
    if (!userForm.value.role_id && r.length) userForm.value.role_id = r[0].id
  } finally {
    loadingUsers.value = false
  }
}

async function submitUser() {
  userError.value = ''
  const f = userForm.value
  if (!f.email.trim() || !f.full_name.trim() || !f.password) {
    userError.value = 'E-posta, ad ve şifre zorunlu.'
    return
  }
  savingUser.value = true
  try {
    await createUser({
      email: f.email.trim(),
      full_name: f.full_name.trim(),
      password: f.password,
      role_id: f.role_id || undefined
    })
    userForm.value = { email: '', full_name: '', password: '', role_id: f.role_id }
    showUserForm.value = false
    await loadUsers()
  } catch (e: any) {
    userError.value = e?.response?.data?.message || 'Kullanıcı eklenemedi.'
  } finally {
    savingUser.value = false
  }
}

async function toggleUser(u: User) {
  try {
    await setUserActive(u.id, !u.is_active)
    u.is_active = !u.is_active
  } catch {
    alert('Durum değiştirilemedi.')
  }
}

async function removeUser(u: User) {
  if (!confirm(`${u.email} silinsin mi?`)) return
  await deleteUser(u.id)
  await loadUsers()
}

// ---- Channels ----
const accounts = ref<WhatsAppAccount[]>([])
const loadingAccounts = ref(false)
const showAccountForm = ref(false)
const accountForm = ref<AccountInput>(blankAccount())
const savingAccount = ref(false)
const accountError = ref('')
const testResult = ref<Record<string, string>>({})

function blankAccount(): AccountInput {
  return {
    name: '',
    phone_id: '',
    business_id: '',
    access_token: '',
    app_id: '',
    app_secret: '',
    webhook_verify_token: ''
  }
}

async function loadAccounts() {
  loadingAccounts.value = true
  try {
    accounts.value = await listAccounts()
  } finally {
    loadingAccounts.value = false
  }
}

// ---- Meta App integration (entered from the UI, no backend config needed) ----
const metaForm = ref({ meta_app_id: '', meta_config_id: '', meta_app_secret: '' })
const metaHasSecret = ref(false)
const savingMeta = ref(false)
const metaMsg = ref('')

async function loadMetaSettings() {
  try {
    const s = await getMetaAppSettings()
    metaForm.value.meta_app_id = s.meta_app_id
    metaForm.value.meta_config_id = s.meta_config_id
    metaHasSecret.value = s.has_meta_app_secret
  } catch {
    /* yetki yoksa sessiz geç */
  }
}

async function saveMeta() {
  metaMsg.value = ''
  savingMeta.value = true
  try {
    await saveMetaAppSettings({
      meta_app_id: metaForm.value.meta_app_id.trim(),
      meta_config_id: metaForm.value.meta_config_id.trim(),
      meta_app_secret: metaForm.value.meta_app_secret.trim() || undefined
    })
    metaForm.value.meta_app_secret = ''
    metaMsg.value = '✓ Kaydedildi.'
    await loadMetaSettings()
    await initEmbeddedSignup() // "Bağla" butonu yeni değerlerle güncellensin
  } catch (e: any) {
    metaMsg.value = '✗ ' + (e?.response?.data?.message || 'Kaydedilemedi.')
  } finally {
    savingMeta.value = false
  }
}

// ---- Embedded Signup ----
const esConfig = ref<EmbeddedSignupConfig | null>(null)
const connecting = ref(false)
const connectMsg = ref('')
// Coexistence (number stays live on the WhatsApp Business app) requires Tech
// Provider status; off by default so the standard onboarding works today.
const coexistenceMode = ref(false)

async function initEmbeddedSignup() {
  esConfig.value = await getEmbeddedSignupConfig()
  if (esConfig.value) {
    try {
      await loadFacebookSDK(esConfig.value)
    } catch {
      /* SDK yüklenemedi; buton tıklanınca tekrar denenir */
    }
  }
}

async function connectWhatsApp() {
  if (!esConfig.value) return
  connectMsg.value = ''
  connecting.value = true
  try {
    await loadFacebookSDK(esConfig.value)
    const signup = await launchWhatsAppSignup(esConfig.value, coexistenceMode.value)
    const res = await exchangeToken(signup)
    if (res.status === 'active') {
      connectMsg.value = '✓ WhatsApp hesabı bağlandı!' + (res.pin ? ` (2FA PIN: ${res.pin} — güvenli bir yere kaydedin)` : '')
    } else if (res.status === 'pending_registration') {
      connectMsg.value = '✓ Hesap oluşturuldu, telefon kaydı gerekiyor.'
    } else {
      connectMsg.value = '✓ Bağlantı tamamlandı.'
    }
    await loadAccounts()
  } catch (e: any) {
    connectMsg.value = '✗ ' + (e?.message || e?.response?.data?.message || 'Bağlantı başarısız.')
  } finally {
    connecting.value = false
  }
}

async function submitAccount() {
  accountError.value = ''
  const f = accountForm.value
  if (!f.name.trim() || !f.phone_id.trim() || !f.business_id.trim() || !f.access_token.trim()) {
    accountError.value = 'Ad, Phone ID, Business ID ve Access Token zorunlu.'
    return
  }
  savingAccount.value = true
  try {
    await createAccount(f)
    accountForm.value = blankAccount()
    showAccountForm.value = false
    await loadAccounts()
  } catch (e: any) {
    accountError.value = e?.response?.data?.message || 'Hesap eklenemedi.'
  } finally {
    savingAccount.value = false
  }
}

async function removeAccount(a: WhatsAppAccount) {
  if (!confirm(`"${a.name}" kanalı silinsin mi?`)) return
  await deleteAccount(a.id)
  await loadAccounts()
}

async function runTest(a: WhatsAppAccount) {
  testResult.value[a.id] = '…'
  const res = await testAccount(a.id)
  testResult.value[a.id] = res.ok ? '✓ Bağlantı başarılı' : '✗ ' + (res.message || 'Hata')
}

// ---- WhatsApp Web (QR) connector ----
const qr = ref<QRStatus>({ state: 'disconnected' })
const qrImage = ref('')
const qrBusy = ref(false)
let qrTimer: number | undefined

async function refreshQR() {
  try {
    qr.value = await getQRStatus()
    if (qr.value.state === 'qr' && qr.value.qr) {
      qrImage.value = await QRCode.toDataURL(qr.value.qr, { width: 260, margin: 1 })
    } else {
      qrImage.value = ''
    }
  } catch {
    /* sessiz */
  }
}

async function startQR() {
  qrBusy.value = true
  try {
    qr.value = await qrConnect()
    if (qr.value.state === 'qr' && qr.value.qr) {
      qrImage.value = await QRCode.toDataURL(qr.value.qr, { width: 260, margin: 1 })
    }
  } catch (e: any) {
    alert(e?.response?.data?.message || 'QR başlatılamadı.')
  } finally {
    qrBusy.value = false
  }
}

async function logoutQR() {
  if (!confirm('WhatsApp Web bağlantısı kesilsin mi? Yeniden QR taramak gerekir.')) return
  await qrLogout()
  qrImage.value = ''
  await refreshQR()
}

onMounted(() => {
  loadUsers()
  loadAccounts()
  loadMetaSettings()
  initEmbeddedSignup()
  refreshQR()
  // Poll so a new QR code / successful pairing shows without a manual refresh.
  qrTimer = window.setInterval(refreshQR, 3000)
})
onBeforeUnmount(() => window.clearInterval(qrTimer))
</script>

<template>
  <div class="page">
    <header class="page-head">
      <h1>Yönetim</h1>
      <p class="muted">Giriş kullanıcıları ve kanal/API entegrasyon ayarları.</p>
    </header>

    <div class="tabs">
      <button :class="['tab', { on: tab === 'users' }]" @click="tab = 'users'">Kullanıcılar</button>
      <button :class="['tab', { on: tab === 'channels' }]" @click="tab = 'channels'">Kanallar</button>
    </div>

    <!-- Users tab -->
    <section v-if="tab === 'users'">
      <div class="section-head">
        <h2>Giriş Kullanıcıları</h2>
        <button class="primary" @click="showUserForm = !showUserForm">＋ Kullanıcı Ekle</button>
      </div>

      <form v-if="showUserForm" class="card form" @submit.prevent="submitUser">
        <div class="row">
          <div class="field grow">
            <label>E-posta / kullanıcı adı</label>
            <input v-model="userForm.email" />
          </div>
          <div class="field grow">
            <label>Ad Soyad</label>
            <input v-model="userForm.full_name" />
          </div>
        </div>
        <div class="row">
          <div class="field grow">
            <label>Şifre</label>
            <input v-model="userForm.password" type="password" />
          </div>
          <div class="field grow">
            <label>Rol</label>
            <select v-model="userForm.role_id">
              <option value="">(rolsüz)</option>
              <option v-for="r in roles" :key="r.id" :value="r.id">{{ r.name }}</option>
            </select>
          </div>
        </div>
        <p v-if="userError" class="error">{{ userError }}</p>
        <div class="form-actions">
          <button type="button" @click="showUserForm = false">İptal</button>
          <button class="primary" type="submit" :disabled="savingUser">Kaydet</button>
        </div>
      </form>

      <div class="card table-card">
        <table>
          <thead>
            <tr><th>Ad</th><th>E-posta</th><th>Rol</th><th>Durum</th><th></th></tr>
          </thead>
          <tbody>
            <tr v-if="loadingUsers"><td colspan="5" class="center muted">Yükleniyor…</td></tr>
            <tr v-else-if="!users.length"><td colspan="5" class="center muted">Kullanıcı yok.</td></tr>
            <tr v-for="u in users" :key="u.id">
              <td>{{ u.full_name }} <span v-if="u.is_super_admin" class="tag">süper admin</span></td>
              <td>{{ u.email }}</td>
              <td>{{ u.role?.name || '—' }}</td>
              <td>
                <span :class="['dot', u.is_active ? 'ok' : 'off']"></span>
                {{ u.is_active ? 'Aktif' : 'Pasif' }}
              </td>
              <td class="right">
                <button @click="toggleUser(u)">{{ u.is_active ? 'Pasifleştir' : 'Aktifleştir' }}</button>
                <button class="danger-btn" @click="removeUser(u)">Sil</button>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </section>

    <!-- Channels tab -->
    <section v-if="tab === 'channels'">
      <!-- Meta App integration (portable: entered here, not in backend config) -->
      <div class="card meta-card">
        <div class="meta-head">
          <h2>Meta App Entegrasyonu</h2>
          <span class="muted small">Bu değerler panelden yönetilir — sunucu/config dosyası düzenlemeye gerek yok.</span>
        </div>
        <div class="row">
          <div class="field grow">
            <label>App ID</label>
            <input v-model="metaForm.meta_app_id" placeholder="Meta App ID" />
          </div>
          <div class="field grow">
            <label>Configuration ID</label>
            <input v-model="metaForm.meta_config_id" placeholder="Embedded Signup Config ID" />
          </div>
        </div>
        <div class="field">
          <label>
            App Secret
            <span v-if="metaHasSecret" class="muted small">(kayıtlı — değiştirmek için yeni değer gir)</span>
          </label>
          <input v-model="metaForm.meta_app_secret" type="password" :placeholder="metaHasSecret ? '••••••••' : 'Meta App Secret'" />
        </div>
        <div class="meta-actions">
          <span v-if="metaMsg" class="meta-msg small">{{ metaMsg }}</span>
          <button class="primary" :disabled="savingMeta" @click="saveMeta">Kaydet</button>
        </div>
      </div>

      <!-- WhatsApp Web (QR) — chat + chatbot on a number that stays on the phone -->
      <div class="card qr-card">
        <div class="meta-head">
          <h2>WhatsApp Web (QR) — Sohbet + Chatbot</h2>
          <span class="muted small">
            Numaranı telefondaki WhatsApp'tan çıkarmadan bağlar (WhatsApp Web gibi). Sadece sohbet ve chatbot içindir —
            toplu mesaj için Cloud API kullan.
          </span>
        </div>

        <div v-if="qr.state === 'connected'" class="qr-connected">
          🟢 Bağlı{{ qr.phone ? ' · +' + qr.phone : '' }}
          <button class="danger-btn" @click="logoutQR">Bağlantıyı Kes</button>
        </div>

        <div v-else class="qr-connect">
          <div v-if="qrImage" class="qr-box">
            <img :src="qrImage" alt="QR" width="220" height="220" />
            <p class="muted small">
              Telefonda <b>WhatsApp Business → Ayarlar → Bağlı cihazlar → Cihaz bağla</b> ile bu kodu tara.
            </p>
          </div>
          <button v-else class="primary" :disabled="qrBusy" @click="startQR">
            {{ qrBusy ? 'Başlatılıyor…' : '📱 QR ile Bağla' }}
          </button>
        </div>
      </div>

      <div class="section-head">
        <h2>Kanallar (WhatsApp Business API)</h2>
        <div class="head-actions">
          <button
            v-if="esConfig"
            class="primary"
            :disabled="connecting"
            @click="connectWhatsApp"
          >
            {{ connecting ? 'Bağlanıyor…' : '🟢 WhatsApp Business Hesabını Bağla' }}
          </button>
          <button @click="showAccountForm = !showAccountForm">Manuel Ekle</button>
        </div>
      </div>

      <div v-if="esConfig" class="coexistence-note muted small">
        <label class="cox-toggle">
          <input type="checkbox" v-model="coexistenceMode" />
          Coexistence modu (numara telefonda + panelde birlikte)
        </label>
        <div v-if="coexistenceMode" class="cox-warn">
          ⚠️ Coexistence, Meta <b>Tech Provider</b> statüsü gerektirir. Uygulaman Tech Provider değilse bu mod hata verir.
        </div>
        <div v-else>
          Standart bağlama: numara panele taşınır (telefondaki WhatsApp'tan çıkar). Config'in Meta'da
          <b>"WhatsApp Embedded Signup Configuration"</b> template'inden oluşturulmuş olması gerekir.
        </div>
      </div>
      <div v-else class="coexistence-note muted small">
        Embedded Signup yapılandırılmamış (config.toml → <code>[whatsapp] app_id, config_id</code>).
        Bağla butonu için Meta App ID ve Configuration ID gerekir. Şimdilik aşağıdan manuel ekleyebilirsin.
      </div>
      <div v-if="connectMsg" class="card connect-msg">{{ connectMsg }}</div>

      <form v-if="showAccountForm" class="card form" @submit.prevent="submitAccount">
        <div class="row">
          <div class="field grow">
            <label>Kanal adı *</label>
            <input v-model="accountForm.name" placeholder="Ör. Ana Hat" />
          </div>
          <div class="field grow">
            <label>Phone ID *</label>
            <input v-model="accountForm.phone_id" />
          </div>
        </div>
        <div class="row">
          <div class="field grow">
            <label>Business ID *</label>
            <input v-model="accountForm.business_id" />
          </div>
          <div class="field grow">
            <label>App ID</label>
            <input v-model="accountForm.app_id" />
          </div>
        </div>
        <div class="field">
          <label>Access Token *</label>
          <input v-model="accountForm.access_token" type="password" />
        </div>
        <div class="row">
          <div class="field grow">
            <label>App Secret (webhook imzası)</label>
            <input v-model="accountForm.app_secret" type="password" />
          </div>
          <div class="field grow">
            <label>Webhook Verify Token (boşsa otomatik)</label>
            <input v-model="accountForm.webhook_verify_token" />
          </div>
        </div>
        <p v-if="accountError" class="error">{{ accountError }}</p>
        <div class="form-actions">
          <button type="button" @click="showAccountForm = false">İptal</button>
          <button class="primary" type="submit" :disabled="savingAccount">Kaydet</button>
        </div>
      </form>

      <div v-if="loadingAccounts" class="muted center">Yükleniyor…</div>
      <div v-else-if="!accounts.length" class="card center muted">Kanal yok. WhatsApp entegrasyonu için bir kanal ekleyin.</div>

      <div v-for="a in accounts" :key="a.id" class="card account">
        <div class="account-head">
          <div>
            <div class="account-name">{{ a.name }} <span class="tag">{{ a.status || 'active' }}</span></div>
            <div class="muted small">Phone ID: {{ a.phone_id }} · Business ID: {{ a.business_id }}</div>
            <div class="muted small">
              Token: {{ a.has_access_token ? '✓' : '—' }} ·
              App Secret: {{ a.has_app_secret ? '✓' : '—' }} ·
              Verify Token: {{ a.webhook_verify_token || '—' }}
            </div>
          </div>
          <div class="account-actions">
            <button @click="runTest(a)">Bağlantıyı Test Et</button>
            <button class="danger-btn" @click="removeAccount(a)">Sil</button>
          </div>
        </div>
        <div v-if="testResult[a.id]" class="test-result muted small">{{ testResult[a.id] }}</div>
      </div>
    </section>
  </div>
</template>

<style scoped>
.page { padding: 24px; max-width: 1000px; }
.page-head { margin-bottom: 16px; }
.page-head h1 { margin: 0 0 4px; font-size: 20px; }
.page-head p { margin: 0; }
.small { font-size: 12px; }
.center { text-align: center; padding: 24px; }

.tabs { display: flex; gap: 6px; border-bottom: 1px solid var(--border); margin-bottom: 16px; }
.tab { border: none; border-radius: 0; background: transparent; padding: 10px 14px; color: var(--muted); border-bottom: 2px solid transparent; }
.tab.on { color: var(--brand); border-bottom-color: var(--brand); font-weight: 600; }

.section-head { display: flex; justify-content: space-between; align-items: center; margin-bottom: 12px; gap: 12px; }
.section-head h2 { font-size: 16px; margin: 0; }
.head-actions { display: flex; gap: 8px; flex-shrink: 0; }
.meta-card { margin-bottom: 20px; display: flex; flex-direction: column; gap: 12px; }
.qr-card { margin-bottom: 20px; display: flex; flex-direction: column; gap: 12px; }
.qr-connected { display: flex; align-items: center; gap: 12px; font-weight: 600; }
.qr-connect { display: flex; flex-direction: column; align-items: flex-start; gap: 8px; }
.qr-box { display: flex; flex-direction: column; align-items: center; gap: 8px; }
.qr-box img { border: 1px solid var(--border); border-radius: 8px; }
.meta-head { display: flex; flex-direction: column; gap: 2px; }
.meta-head h2 { font-size: 16px; margin: 0; }
.meta-actions { display: flex; justify-content: flex-end; align-items: center; gap: 12px; }
.meta-msg { color: var(--muted); }
.coexistence-note { margin-bottom: 12px; line-height: 1.5; }
.coexistence-note code { background: var(--bg); padding: 1px 5px; border-radius: 4px; }
.cox-toggle { display: flex; align-items: center; gap: 6px; margin-bottom: 4px; }
.cox-toggle input { width: auto; }
.cox-warn { color: var(--danger); }
.connect-msg { margin-bottom: 12px; }

.form { margin-bottom: 16px; display: flex; flex-direction: column; gap: 12px; }
.row { display: flex; gap: 12px; flex-wrap: wrap; }
.field { display: flex; flex-direction: column; gap: 4px; }
.field.grow { flex: 1; min-width: 180px; }
.field label { font-size: 12px; color: var(--muted); }
.form-actions { display: flex; justify-content: flex-end; gap: 8px; }
.error { color: var(--danger); margin: 0; }

.table-card { padding: 0; overflow: hidden; }
table { width: 100%; border-collapse: collapse; }
th, td { text-align: left; padding: 10px 14px; border-bottom: 1px solid var(--border); }
th { font-size: 12px; color: var(--muted); font-weight: 600; }
tr:last-child td { border-bottom: none; }
.right { text-align: right; display: flex; gap: 6px; justify-content: flex-end; }
.tag { font-size: 11px; padding: 1px 8px; border-radius: 999px; background: var(--bg); border: 1px solid var(--border); }
.dot { display: inline-block; width: 8px; height: 8px; border-radius: 50%; margin-right: 4px; }
.dot.ok { background: var(--brand); }
.dot.off { background: #bbb; }
.danger-btn { color: var(--danger); }

.account { margin-bottom: 10px; }
.account-head { display: flex; justify-content: space-between; align-items: flex-start; gap: 12px; }
.account-name { font-weight: 600; }
.account-actions { display: flex; gap: 6px; flex-shrink: 0; }
.test-result { margin-top: 10px; padding-top: 10px; border-top: 1px solid var(--border); }
</style>
