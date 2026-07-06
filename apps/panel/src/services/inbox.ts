import { api } from './api'

// Channels the inbox understands. Kept in sync with internal/channels.
export type ChannelType = 'whatsapp' | 'whatsapp_qr' | 'instagram' | 'messenger' | 'telegram'

export interface Conversation {
  id: string
  phone_number: string
  name: string
  profile_name: string
  channel_type: ChannelType
  last_message_at: string | null
  last_message_preview: string
  unread_count: number
  whatsapp_account?: string
  service_window_open: boolean
}

export interface Message {
  id: string
  contact_id: string
  direction: 'incoming' | 'outgoing'
  message_type: string
  content: { body?: string } | string | null
  interactive_data?: {
    type?: string
    body?: string
    buttons?: { id?: string; title?: string }[]
  } | null
  status: string
  created_at: string
}

export interface ConversationFilters {
  channel?: ChannelType[]
  unread?: boolean
  assigned?: 'me'
  search?: string
}

// The conversation list reuses the existing /contacts endpoint (a contact is a
// conversation thread in this backend), now with channel/unread/assigned filters.
export async function listConversations(filters: ConversationFilters = {}): Promise<Conversation[]> {
  const params: Record<string, string> = { limit: '100' }
  if (filters.channel?.length) params.channel = filters.channel.join(',')
  if (filters.unread) params.unread = 'true'
  if (filters.assigned) params.assigned = filters.assigned
  if (filters.search) params.search = filters.search
  const res = await api.get('/contacts', { params })
  return res.data?.contacts ?? []
}

export async function getMessages(contactId: string): Promise<Message[]> {
  const res = await api.get(`/contacts/${contactId}/messages`)
  return res.data?.messages ?? []
}

export async function sendText(contactId: string, body: string): Promise<void> {
  await api.post(`/contacts/${contactId}/messages`, {
    type: 'text',
    content: { body }
  })
}

// Sends an interactive button message (Cloud API channels only).
export async function sendButtons(
  contactId: string,
  body: string,
  buttons: { id: string; title: string }[]
): Promise<void> {
  await api.post(`/contacts/${contactId}/messages`, {
    type: 'interactive',
    interactive: { type: 'button', body, buttons }
  })
}

export async function markRead(contactId: string): Promise<void> {
  await api.post(`/contacts/${contactId}/mark-read`)
}

export function messageBody(m: Message): string {
  if (m.content && typeof m.content === 'object') return m.content.body ?? ''
  if (typeof m.content === 'string') return m.content
  return ''
}
