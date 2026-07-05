<script setup lang="ts">
import { ref, computed, onMounted, onBeforeUnmount, watch } from 'vue'
import {
  listConversations,
  getMessages,
  sendText,
  messageBody,
  type Conversation,
  type Message,
  type ChannelType
} from '@/services/inbox'

// Channel presentation. Only whatsapp is live today; the rest are shown so the
// filter bar reflects the omnichannel model as adapters come online.
const CHANNELS: { type: ChannelType; label: string; icon: string }[] = [
  { type: 'whatsapp', label: 'WhatsApp', icon: '🟢' },
  { type: 'instagram', label: 'Instagram', icon: '📸' },
  { type: 'messenger', label: 'Messenger', icon: '💬' },
  { type: 'telegram', label: 'Telegram', icon: '✈️' }
]

const conversations = ref<Conversation[]>([])
const loadingList = ref(false)
const listError = ref('')

// Filters
const activeChannel = ref<ChannelType | 'all'>('all')
const unreadOnly = ref(false)
const assignedToMe = ref(false)
const search = ref('')

const selected = ref<Conversation | null>(null)
const messages = ref<Message[]>([])
const loadingMessages = ref(false)
const draft = ref('')
const sending = ref(false)

let messagesTimer: number | undefined
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
  } catch {
    listError.value = 'Konuşmalar yüklenemedi.'
  } finally {
    loadingList.value = false
  }
}

async function openConversation(c: Conversation) {
  selected.value = c
  await loadMessages()
  // Opening the thread marks it read on the server; reflect that locally.
  c.unread_count = 0
}

async function loadMessages() {
  if (!selected.value) return
  loadingMessages.value = true
  try {
    messages.value = await getMessages(selected.value.id)
  } finally {
    loadingMessages.value = false
  }
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

const conversationTitle = computed(() => {
  const c = selected.value
  if (!c) return ''
  return c.name || c.profile_name || c.phone_number
})

// Re-fetch the list when filters change (search is debounced).
watch([activeChannel, unreadOnly, assignedToMe], loadConversations)
watch(search, () => {
  window.clearTimeout(searchTimer)
  searchTimer = window.setTimeout(loadConversations, 350)
})

onMounted(() => {
  loadConversations()
  // Light polling keeps the open thread fresh without a WebSocket for now.
  messagesTimer = window.setInterval(() => {
    if (selected.value && !loadingMessages.value) loadMessages()
  }, 8000)
})
onBeforeUnmount(() => {
  window.clearInterval(messagesTimer)
  window.clearTimeout(searchTimer)
})

function fmtTime(iso: string | null): string {
  if (!iso) return ''
  const d = new Date(iso)
  return d.toLocaleString('tr-TR', { hour: '2-digit', minute: '2-digit', day: '2-digit', month: '2-digit' })
}
</script>

<template>
  <div class="inbox">
    <!-- Top filter bar -->
    <div class="filterbar">
      <div class="chips">
        <button :class="['chip', { on: activeChannel === 'all' }]" @click="activeChannel = 'all'">Tümü</button>
        <button
          v-for="ch in CHANNELS"
          :key="ch.type"
          :class="['chip', { on: activeChannel === ch.type }]"
          @click="activeChannel = ch.type"
        >
          {{ ch.icon }} {{ ch.label }}
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
        <div v-if="loadingList" class="hint muted">Yükleniyor…</div>
        <div v-else-if="listError" class="hint error">{{ listError }}</div>
        <div v-else-if="!conversations.length" class="hint muted">Konuşma yok.</div>
        <button
          v-for="c in conversations"
          :key="c.id"
          :class="['conv', { active: selected?.id === c.id }]"
          @click="openConversation(c)"
        >
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
        </button>
      </div>

      <!-- Chat pane -->
      <div class="chat">
        <template v-if="selected">
          <div class="chat-head">
            <div>
              <div class="chat-title">{{ conversationTitle }}</div>
              <div class="muted small">
                {{ channelMeta(selected.channel_type).icon }} {{ channelMeta(selected.channel_type).label }}
                · {{ selected.phone_number }}
              </div>
            </div>
            <span v-if="!selected.service_window_open && selected.channel_type === 'whatsapp'" class="window-closed">
              24s penceresi kapalı
            </span>
          </div>

          <div class="messages">
            <div v-if="loadingMessages && !messages.length" class="hint muted">Yükleniyor…</div>
            <div
              v-for="m in messages"
              :key="m.id"
              :class="['msg', m.direction === 'outgoing' ? 'out' : 'in']"
            >
              <div class="bubble">
                <div class="bubble-text">{{ messageBody(m) || '[' + m.message_type + ']' }}</div>
                <div class="bubble-meta">{{ fmtTime(m.created_at) }}</div>
              </div>
            </div>
          </div>

          <form class="composer" @submit.prevent="send">
            <input v-model="draft" placeholder="Mesaj yazın…" autocomplete="off" />
            <button class="primary" type="submit" :disabled="sending || !draft.trim()">Gönder</button>
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
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding: 12px 16px;
  background: var(--panel);
  border-bottom: 1px solid var(--border);
  flex-wrap: wrap;
}
.chips { display: flex; gap: 6px; flex-wrap: wrap; }
.chip { padding: 6px 12px; border-radius: 999px; font-size: 13px; }
.chip.on { background: var(--brand); border-color: var(--brand); color: #fff; }
.filters-right { display: flex; align-items: center; gap: 12px; }
.toggle { display: flex; align-items: center; gap: 6px; font-size: 13px; color: var(--muted); white-space: nowrap; }
.toggle input { width: auto; }
.search { width: 220px; }

.body { flex: 1; display: flex; min-height: 0; }

.list {
  width: 320px;
  flex-shrink: 0;
  border-right: 1px solid var(--border);
  background: var(--panel);
  overflow-y: auto;
}
.hint { padding: 20px; text-align: center; }
.conv {
  width: 100%;
  text-align: left;
  border: none;
  border-bottom: 1px solid var(--border);
  border-radius: 0;
  background: transparent;
  padding: 12px 14px;
  display: flex;
  flex-direction: column;
  gap: 4px;
}
.conv:hover { background: var(--bg); }
.conv.active { background: var(--bg); box-shadow: inset 3px 0 0 var(--brand); }
.conv-top, .conv-bottom { display: flex; justify-content: space-between; align-items: center; gap: 8px; }
.conv-name { font-weight: 600; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.conv-time { font-size: 11px; flex-shrink: 0; }
.conv-preview { font-size: 12px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.ch-icon { margin-right: 2px; }
.badge {
  background: var(--brand); color: #fff; font-size: 11px; font-weight: 600;
  border-radius: 999px; padding: 1px 7px; flex-shrink: 0;
}

.chat { flex: 1; display: flex; flex-direction: column; min-width: 0; }
.empty { margin: auto; }
.chat-head {
  display: flex; justify-content: space-between; align-items: center;
  padding: 12px 16px; border-bottom: 1px solid var(--border); background: var(--panel);
}
.chat-title { font-weight: 600; }
.small { font-size: 12px; }
.window-closed { font-size: 12px; color: var(--danger); }

.messages { flex: 1; overflow-y: auto; padding: 16px; display: flex; flex-direction: column; gap: 8px; }
.msg { display: flex; }
.msg.out { justify-content: flex-end; }
.bubble { max-width: 68%; padding: 8px 11px; border-radius: 10px; background: var(--panel); border: 1px solid var(--border); }
.msg.out .bubble { background: #d9fdd3; border-color: #cdeec7; }
.bubble-text { white-space: pre-wrap; word-break: break-word; }
.bubble-meta { font-size: 10px; color: var(--muted); text-align: right; margin-top: 2px; }

.composer { display: flex; gap: 8px; padding: 12px 16px; border-top: 1px solid var(--border); background: var(--panel); }
.composer input { flex: 1; }
</style>
