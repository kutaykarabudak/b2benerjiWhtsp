import { computed, ref } from 'vue'
import { defineStore } from 'pinia'
import { createRealtime, type RealtimeStatus, type WSEvent } from '@/services/ws'
import { listConversations } from '@/services/inbox'

export interface RealtimeEvent extends WSEvent {
  sequence: number
}

export interface AppToast {
  id: number
  title: string
  body: string
  contactId?: string
}

export const useRealtimeStore = defineStore('realtime', () => {
  const status = ref<RealtimeStatus>('disconnected')
  const unreadCount = ref(0)
  const lastEvent = ref<RealtimeEvent | null>(null)
  const activeContactId = ref<string | null>(null)
  const toasts = ref<AppToast[]>([])
  const notificationPermission = ref<NotificationPermission>(
    typeof Notification === 'undefined' ? 'denied' : Notification.permission
  )

  let connection: { close(): void } | null = null
  let sequence = 0
  let toastId = 0
  let refreshTimer: number | undefined

  const statusLabel = computed(() => ({
    connected: 'Anlık bağlantı aktif',
    connecting: 'Bağlantı kuruluyor',
    disconnected: 'Yeniden bağlanıyor'
  })[status.value])

  function start() {
    if (connection) return
    registerNotificationWorker()
    connection = createRealtime(handleEvent, (next) => {
      status.value = next
      if (next === 'connected') syncUnread()
    })
    syncUnread()
  }

  function stop() {
    connection?.close()
    connection = null
  }

  async function syncUnread() {
    window.clearTimeout(refreshTimer)
    refreshTimer = window.setTimeout(async () => {
      try {
        const conversations = await listConversations()
        unreadCount.value = conversations.reduce((total, item) => total + (item.unread_count || 0), 0)
      } catch {
        // Periodic inbox refresh will reconcile the number later.
      }
    }, 250)
  }

  function setUnreadCount(value: number) {
    unreadCount.value = Math.max(0, value)
  }

  function setActiveContact(id: string | null) {
    activeContactId.value = id
  }

  async function requestNotifications() {
    if (typeof Notification === 'undefined') return
    notificationPermission.value = await Notification.requestPermission()
  }

  function handleEvent(event: WSEvent) {
    sequence += 1
    lastEvent.value = { ...event, sequence }

    if (event.type === 'new_message') {
      syncUnread()
      const payload = event.payload || {}
      const isIncoming = payload.direction === 'incoming'
      // Permission means "notify for every incoming message", including when
      // this conversation or the panel itself is currently visible.
      if (isIncoming) notifyIncoming(payload)
    }
  }

  async function notifyIncoming(payload: any) {
    const title = payload.profile_name || payload.name || payload.phone_number || 'Yeni WhatsApp mesajı'
    const rawBody = payload.content?.body || payload.interactive_data?.body || 'Yeni bir mesajınız var.'
    const body = String(rawBody).slice(0, 120)
    const contactId = payload.contact_id ? String(payload.contact_id) : undefined

    playTone()
    const id = ++toastId
    toasts.value.push({ id, title, body, contactId })
    window.setTimeout(() => dismissToast(id), 6000)

    if (notificationPermission.value === 'granted') {
      const url = contactId ? `/inbox?contact=${encodeURIComponent(contactId)}` : '/inbox'
      const messageKey = payload.id || payload.message_id || payload.whatsapp_message_id || `${Date.now()}-${sequence}`
      try {
        if ('serviceWorker' in navigator) {
          const registration = await navigator.serviceWorker.ready
          await registration.showNotification(title, {
            body,
            icon: '/favicon.svg',
            badge: '/favicon.svg',
            tag: `whatsapp-message-${messageKey}`,
            data: { url }
          })
        } else {
          const notification = new Notification(title, { body, tag: `whatsapp-message-${messageKey}` })
          notification.onclick = () => {
            window.focus()
            window.location.href = url
            notification.close()
          }
        }
      } catch {
        // The in-panel toast and sound still provide a fallback.
      }
    }
  }

  function registerNotificationWorker() {
    if (!('serviceWorker' in navigator)) return
    navigator.serviceWorker.register('/notification-sw.js').catch(() => {
      // Desktop browsers can still use the Notification constructor fallback.
    })
  }

  function dismissToast(id: number) {
    toasts.value = toasts.value.filter((toast) => toast.id !== id)
  }

  function playTone() {
    try {
      const AudioContextCtor = window.AudioContext || (window as any).webkitAudioContext
      if (!AudioContextCtor) return
      const ctx = new AudioContextCtor()
      const oscillator = ctx.createOscillator()
      const gain = ctx.createGain()
      oscillator.type = 'sine'
      oscillator.frequency.setValueAtTime(720, ctx.currentTime)
      oscillator.frequency.exponentialRampToValueAtTime(520, ctx.currentTime + 0.14)
      gain.gain.setValueAtTime(0.0001, ctx.currentTime)
      gain.gain.exponentialRampToValueAtTime(0.12, ctx.currentTime + 0.02)
      gain.gain.exponentialRampToValueAtTime(0.0001, ctx.currentTime + 0.2)
      oscillator.connect(gain)
      gain.connect(ctx.destination)
      oscillator.start()
      oscillator.stop(ctx.currentTime + 0.21)
      oscillator.onended = () => ctx.close()
    } catch {
      // Browsers may block audio before the first user interaction.
    }
  }

  return {
    status,
    statusLabel,
    unreadCount,
    lastEvent,
    activeContactId,
    toasts,
    notificationPermission,
    start,
    stop,
    syncUnread,
    setUnreadCount,
    setActiveContact,
    requestNotifications,
    dismissToast
  }
})
