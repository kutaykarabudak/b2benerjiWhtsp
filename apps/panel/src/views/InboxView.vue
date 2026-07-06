<script setup lang="ts">
import { ref, computed, onMounted, onBeforeUnmount, watch, nextTick } from 'vue'
import { useRoute } from 'vue-router'
import {
  listConversations,
  getMessages,
  sendText,
  sendButtons,
  sendMedia,
  sendLocation,
  messageBody,
  type Conversation,
  type Message,
  type ChannelType
} from '@/services/inbox'
import { createRealtime } from '@/services/ws'

// Display metadata for all channel types (used for conversation icons).
const CHANNELS: { type: ChannelType; label: string; icon: string }[] = [
  { type: 'whatsapp', label: 'WhatsApp', icon: '🟢' },
  { type: 'whatsapp_qr', label: 'WhatsApp Web', icon: '🟢' },
  { type: 'instagram', label: 'Instagram', icon: '📸' },
  { type: 'messenger', label: 'Messenger', icon: '💬' },
  { type: 'telegram', label: 'Telegram', icon: '✈️' }
]

// Only WhatsApp is offered as a filter chip in this deployment.
const CHIP_CHANNELS = CHANNELS.filter((c) => c.type === 'whatsapp')

const conversations = ref<Conversation[]>([])
const loadingList = ref(false)
const listError = ref('')

const activeChannel = ref<ChannelType | 'all'>('all')
const unreadOnly = ref(false)
const assignedToMe = ref(false)
const search = ref('')

const selected = ref<Conversation | null>(null)
const messages = ref<Message[]>([])
const loadingMessages = ref(false)
const draft = ref('')
const sending = ref(false)
const messagesEl = ref<HTMLElement | null>(null)

// Interactive (button) composer — Cloud API channels only.
const buttonMode = ref(false)
const btnBody = ref('')
const btnTitles = ref<string[]>([''])
const canUseButtons = computed(() => selected.value?.channel_type === 'whatsapp')

function shareLocation() {
  if (!selected.value) return
  if (!navigator.geolocation) {
    alert('Cihaz konum desteklemiyor.')
    return
  }
  sending.value = true
  navigator.geolocation.getCurrentPosition(
    async (pos) => {
      try {
        await sendLocation(selected.value!.id, pos.coords.latitude, pos.coords.longitude)
        await loadMessages()
      } catch (err: any) {
        alert(err?.response?.data?.message || 'Konum gönderilemedi.')
      } finally {
        sending.value = false
      }
    },
    (err) => {
      sending.value = false
      alert('Konum alınamadı: ' + err.message)
    },
    { enableHighAccuracy: true, timeout: 10000 }
  )
}

const fileInput = ref<HTMLInputElement | null>(null)
async function onImageChosen(e: Event) {
  const input = e.target as HTMLInputElement
  const file = input.files?.[0]
  input.value = ''
  if (!file || !selected.value) return
  sending.value = true
  try {
    await sendMedia(selected.value.id, file, draft.value.trim())
    draft.value = ''
    await loadMessages()
  } catch (err: any) {
    alert(err?.response?.data?.message || 'Görsel gönderilemedi.')
  } finally {
    sending.value = false
  }
}

let realtime: { close(): void } | null = null
let fallbackTimer: number | undefined
let searchTimer: number | undefined

function channelMeta(t: ChannelType) {
  return CHANNELS.find((c) => c.type === t) ?? { type: t, label: t, icon: '•' }
}

async function loadConversations() {
  loadingList.value = true
  listError.value = ''
  try {
    conversations.value = await listConversations({
      channel: activeChannel.value === 'all' ? undefined : [activeChannel.value],
      unread: unreadOnly.value || undefined,
      assigned: assignedToMe.value ? 'me' : undefined,
      search: search.value.trim() || undefined
    })
    // Keep the selected conversation's row reference in sync (preview/unread).
    if (selected.value) {
      const fresh = conversations.value.find((c) => c.id === selected.value!.id)
      if (fresh) selected.value = fresh
    }
  } catch {
    listError.value = 'Konuşmalar yüklenemedi.'
  } finally {
    loadingList.value = false
  }
}

async function openConversation(c: Conversation) {
  selected.value = c
  await loadMessages()
  c.unread_count = 0
}

function backToList() {
  selected.value = null
}

async function loadMessages(scroll = true) {
  if (!selected.value) return
  loadingMessages.value = true
  try {
    messages.value = await getMessages(selected.value.id)
    if (scroll) scrollToBottom()
  } finally {
    loadingMessages.value = false
  }
}

function scrollToBottom() {
  nextTick(() => {
    const el = messagesEl.value
    if (el) el.scrollTop = el.scrollHeight
  })
}

async function send() {
  const body = draft.value.trim()
  if (!body || !selected.value || sending.value) return
  sending.value = true
  try {
    await sendText(selected.value.id, body)
    draft.value = ''
    await loadMessages()
  } catch (e: any) {
    alert(e?.response?.data?.message || 'Mesaj gönderilemedi.')
  } finally {
    sending.value = false
  }
}

function addBtnTitle() {
  if (btnTitles.value.length < 3) btnTitles.value.push('')
}

async function sendButtonMsg() {
  if (!selected.value || sending.value) return
  const body = btnBody.value.trim()
  const titles = btnTitles.value.map((t) => t.trim()).filter(Boolean)
  if (!body) {
    alert('Mesaj metni gerekli.')
    return
  }
  if (!titles.length) {
    alert('En az bir buton gerekli.')
    return
  }
  sending.value = true
  try {
    await sendButtons(
      selected.value.id,
      body,
      titles.map((t) => ({ id: t, title: t }))
    )
    btnBody.value = ''
    btnTitles.value = ['']
    buttonMode.value = false
    await loadMessages()
  } catch (e: any) {
    alert(e?.response?.data?.message || 'Butonlu mesaj gönderilemedi.')
  } finally {
    sending.value = false
  }
}

const conversationTitle = computed(() => {
  const c = selected.value
  if (!c) return ''
  return c.name || c.profile_name || c.phone_number
})

// Realtime: react to server push. Reload is cheap and always correct.
function onRealtime(ev: { type: string; payload: any }) {
  if (ev.type === 'new_message') {
    loadConversations()
    const cid = ev.payload?.contact_id
    if (selected.value && cid && String(cid) === String(selected.value.id)) {
      loadMessages()
    }
  } else if (ev.type === 'status_update') {
    if (selected.value) loadMessages(false)
  } else if (ev.type === 'contact_update') {
    loadConversations()
  }
}

watch([activeChannel, unreadOnly, assignedToMe], loadConversations)
watch(search, () => {
  window.clearTimeout(searchTimer)
  searchTimer = window.setTimeout(loadConversations, 350)
})

const route = useRoute()

onMounted(async () => {
  await loadConversations()
  // Opened from Contacts → "Sohbet": select that conversation.
  const cid = route.query.contact as string | undefined
  if (cid) {
    const conv = conversations.value.find((c) => c.id === cid)
    if (conv) openConversation(conv)
  }
  realtime = createRealtime(onRealtime)
  // Safety net if WebSocket can't get through (e.g. some proxies): periodic sync.
  fallbackTimer = window.setInterval(() => {
    loadConversations()
    if (selected.value && !loadingMessages.value) loadMessages(false)
  }, 15000)
})
onBeforeUnmount(() => {
  realtime?.close()
  window.clearInterval(fallbackTimer)
  window.clearTimeout(searchTimer)
})

function fmtTime(iso: string | null): string {
  if (!iso) return ''
  const d = new Date(iso)
  return d.toLocaleString('tr-TR', { hour: '2-digit', minute: '2-digit', day: '2-digit', month: '2-digit' })
}
</script>

<template>
  <div class="inbox" :class="{ 'chat-open': !!selected }">
    <!-- Top filter bar -->
    <div class="filterbar">
      <div class="chips">
        <button :class="['chip', { on: activeChannel === 'all' }]" @click="activeChannel = 'all'">Tümü</button>
        <button
          v-for="ch in CHIP_CHANNELS"
          :key="ch.type"
          :class="['chip', { on: activeChannel === ch.type }]"
          @click="activeChannel = ch.type"
        >
          {{ ch.icon }} <span class="chip-label">{{ ch.label }}</span>
        </button>
      </div>
      <div class="filters-right">
        <label class="toggle"><input type="checkbox" v-model="unreadOnly" /> Okunmamış</label>
        <label class="toggle"><input type="checkbox" v-model="assignedToMe" /> Bana atanan</label>
        <input class="search" v-model="search" placeholder="Ara: isim / telefon" />
      </div>
    </div>

    <div class="body">
      <!-- Conversation list -->
      <div class="list">
        <div v-if="loadingList && !conversations.length" class="hint muted">Yükleniyor…</div>
        <div v-else-if="listError" class="hint error">{{ listError }}</div>
        <div v-else-if="!conversations.length" class="hint muted">Konuşma yok.</div>
        <button
          v-for="c in conversations"
          :key="c.id"
          :class="['conv', { active: selected?.id === c.id }]"
          @click="openConversation(c)"
        >
          <div class="avatar">{{ (c.name || c.profile_name || c.phone_number || '?').charAt(0).toUpperCase() }}</div>
          <div class="conv-main">
            <div class="conv-top">
              <span class="conv-name">{{ c.name || c.profile_name || c.phone_number }}</span>
              <span class="conv-time muted">{{ fmtTime(c.last_message_at) }}</span>
            </div>
            <div class="conv-bottom">
              <span class="conv-preview muted">
                <span class="ch-icon">{{ channelMeta(c.channel_type).icon }}</span>
                {{ c.last_message_preview || '—' }}
              </span>
              <span v-if="c.unread_count" class="badge">{{ c.unread_count }}</span>
            </div>
          </div>
        </button>
      </div>

      <!-- Chat pane -->
      <div class="chat">
        <template v-if="selected">
          <div class="chat-head">
            <button class="back-btn" @click="backToList" aria-label="Geri">←</button>
            <div class="avatar sm">{{ conversationTitle.charAt(0).toUpperCase() }}</div>
            <div class="chat-head-info">
              <div class="chat-title">{{ conversationTitle }}</div>
              <div class="muted small">
                {{ channelMeta(selected.channel_type).icon }} {{ channelMeta(selected.channel_type).label }}
                · {{ selected.phone_number }}
              </div>
            </div>
            <span v-if="!selected.service_window_open && selected.channel_type === 'whatsapp'" class="window-closed">
              24s kapalı
            </span>
          </div>

          <div ref="messagesEl" class="messages">
            <div v-if="loadingMessages && !messages.length" class="hint muted">Yükleniyor…</div>
            <div
              v-for="m in messages"
              :key="m.id"
              :class="['msg', m.direction === 'outgoing' ? 'out' : 'in']"
            >
              <div class="bubble">
                <img v-if="m.media_url && m.message_type === 'image'" :src="m.media_url" class="bubble-img" />
                <a
                  v-else-if="m.message_type === 'location'"
                  :href="messageBody(m)"
                  target="_blank"
                  rel="noopener"
                  class="bubble-loc"
                >📍 Konumu haritada aç</a>
                <div
                  v-else-if="messageBody(m) || m.interactive_data?.body || m.message_type !== 'image'"
                  class="bubble-text"
                >
                  {{ messageBody(m) || m.interactive_data?.body || (m.message_type === 'image' ? '' : '[' + m.message_type + ']') }}
                </div>
                <div v-if="m.interactive_data?.buttons?.length" class="bubble-buttons">
                  <span v-for="(b, i) in m.interactive_data.buttons" :key="i" class="bubble-btn">{{ b.title }}</span>
                </div>
                <div class="bubble-meta">{{ fmtTime(m.created_at) }}</div>
              </div>
            </div>
          </div>

          <!-- Interactive (button) composer -->
          <div v-if="buttonMode" class="composer btn-composer">
            <input v-model="btnBody" placeholder="Soru / mesaj metni…" />
            <div v-for="(_, i) in btnTitles" :key="i" class="btn-line">
              <input v-model="btnTitles[i]" maxlength="20" :placeholder="'Buton ' + (i + 1)" />
            </div>
            <div class="btn-composer-actions">
              <button type="button" v-if="btnTitles.length < 3" @click="addBtnTitle">＋ Buton</button>
              <button type="button" @click="buttonMode = false">İptal</button>
              <button class="primary" :disabled="sending" @click="sendButtonMsg">Butonlu Gönder</button>
            </div>
          </div>

          <form v-else class="composer" @submit.prevent="send">
            <button
              type="button"
              class="btn-toggle"
              title="Görsel gönder"
              @click="fileInput?.click()"
            >
              📎
            </button>
            <input ref="fileInput" type="file" accept="image/*" hidden @change="onImageChosen" />
            <button
              v-if="canUseButtons"
              type="button"
              class="btn-toggle"
              title="Anlık konum gönder"
              @click="shareLocation"
            >
              📍
            </button>
            <button
              v-if="canUseButtons"
              type="button"
              class="btn-toggle"
              title="Çoktan seçmeli buton gönder"
              @click="buttonMode = true"
            >
              ⊞
            </button>
            <input v-model="draft" placeholder="Mesaj yazın…" autocomplete="off" />
            <button class="primary send-btn" type="submit" :disabled="sending || !draft.trim()">Gönder</button>
          </form>
        </template>
        <div v-else class="empty muted">Görüntülemek için bir konuşma seçin.</div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.inbox { display: flex; flex-direction: column; height: 100%; }

.filterbar {
  display: flex; align-items: center; justify-content: space-between; gap: 12px;
  padding: 10px 16px; background: var(--panel); border-bottom: 1px solid var(--border); flex-wrap: wrap;
}
.chips { display: flex; gap: 6px; flex-wrap: wrap; }
.chip { padding: 6px 12px; border-radius: 999px; font-size: 13px; }
.chip.on { background: var(--brand); border-color: var(--brand); color: #fff; }
.filters-right { display: flex; align-items: center; gap: 12px; }
.toggle { display: flex; align-items: center; gap: 6px; font-size: 13px; color: var(--muted); white-space: nowrap; }
.toggle input { width: auto; }
.search { width: 220px; }

.body { flex: 1; display: flex; min-height: 0; }

.list { width: 340px; flex-shrink: 0; border-right: 1px solid var(--border); background: var(--panel); overflow-y: auto; }
.hint { padding: 20px; text-align: center; }
.conv {
  width: 100%; text-align: left; border: none; border-bottom: 1px solid var(--border);
  border-radius: 0; background: transparent; padding: 10px 14px; display: flex; align-items: center; gap: 12px;
}
.conv:hover { background: var(--bg); }
.conv.active { background: var(--bg); box-shadow: inset 3px 0 0 var(--brand); }
.avatar {
  width: 42px; height: 42px; border-radius: 50%; background: var(--brand); color: #fff;
  display: grid; place-items: center; font-weight: 700; flex-shrink: 0; font-size: 17px;
}
.avatar.sm { width: 34px; height: 34px; font-size: 14px; }
.conv-main { flex: 1; min-width: 0; }
.conv-top, .conv-bottom { display: flex; justify-content: space-between; align-items: center; gap: 8px; }
.conv-name { font-weight: 600; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.conv-time { font-size: 11px; flex-shrink: 0; }
.conv-preview { font-size: 13px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.ch-icon { margin-right: 2px; }
.badge { background: var(--brand); color: #fff; font-size: 11px; font-weight: 600; border-radius: 999px; padding: 1px 7px; flex-shrink: 0; }

.chat { flex: 1; display: flex; flex-direction: column; min-width: 0; background: #efeae2; }
.empty { margin: auto; }
.chat-head {
  display: flex; align-items: center; gap: 10px;
  padding: 10px 16px; border-bottom: 1px solid var(--border); background: var(--panel);
}
.chat-head-info { flex: 1; min-width: 0; }
.chat-title { font-weight: 600; }
.small { font-size: 12px; }
.window-closed { font-size: 12px; color: var(--danger); }
.back-btn { display: none; border: none; background: transparent; font-size: 22px; padding: 0 4px; line-height: 1; }

.messages { flex: 1; overflow-y: auto; padding: 16px; display: flex; flex-direction: column; gap: 6px; }
.msg { display: flex; }
.msg.out { justify-content: flex-end; }
.bubble { max-width: 72%; padding: 7px 10px; border-radius: 8px; background: #fff; box-shadow: 0 1px 0.5px rgba(0,0,0,0.08); }
.msg.out .bubble { background: #d9fdd3; }
.bubble-img { max-width: 240px; max-height: 240px; border-radius: 6px; display: block; margin-bottom: 4px; }
.bubble-loc { color: #027eb5; text-decoration: none; font-weight: 600; }
.bubble-text { white-space: pre-wrap; word-break: break-word; }
.bubble-meta { font-size: 10px; color: var(--muted); text-align: right; margin-top: 2px; }
.bubble-buttons { display: flex; flex-direction: column; gap: 4px; margin-top: 6px; border-top: 1px solid rgba(0,0,0,0.08); padding-top: 6px; }
.bubble-btn { text-align: center; font-size: 13px; color: #027eb5; padding: 5px; border-radius: 6px; background: rgba(0,0,0,0.03); }

.btn-composer { flex-direction: column; align-items: stretch; gap: 6px; }
.btn-line { display: flex; }
.btn-composer-actions { display: flex; gap: 8px; justify-content: flex-end; }
.btn-toggle { border: 1px solid var(--border); background: var(--panel); padding: 0 12px; font-size: 18px; border-radius: var(--radius); }

.composer { display: flex; gap: 8px; padding: 10px 16px; border-top: 1px solid var(--border); background: var(--panel); }
.composer input { flex: 1; }

/* --- Mobile: WhatsApp-style single column --- */
@media (max-width: 768px) {
  .list { width: 100%; }
  .chat { display: none; }
  .inbox.chat-open .list { display: none; }
  .inbox.chat-open .filterbar { display: none; }
  .inbox.chat-open .chat { display: flex; }
  .back-btn { display: inline-flex; }
  .filters-right { width: 100%; }
  .search { flex: 1; width: auto; }
  .chip-label { display: none; }
}
</style>
