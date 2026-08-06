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
  tags?: string[]
  metadata?: Record<string, any>
  email?: string
  company_name?: string
  tax_office?: string
  tax_number?: string
  address?: string
  city?: string
  district?: string
  postal_code?: string
  purchase_score?: number
  has_purchased?: boolean
}

// Update a contact's CRM info (name, tags, company/email/notes in metadata).
export async function updateContactInfo(
  contactId: string,
  data: { profile_name?: string; tags?: string[]; metadata?: Record<string, unknown> }
): Promise<void> {
  await api.put(`/contacts/${contactId}`, data)
}

export interface Message {
  id: string
  contact_id: string
  direction: 'incoming' | 'outgoing'
  message_type: string
  content: { body?: string } | string | null
  media_url?: string
  media_filename?: string
  media_mime_type?: string
  interactive_data?: {
    type?: string
    body?: string
    buttons?: { id?: string; title?: string }[]
    catalog_id?: string
    product_retailer_id?: string
    thumbnail_product_retailer_id?: string
    header?: string
    sections?: { title?: string; product_retailer_ids?: string[] }[]
    text?: string
    item_count?: number
    total?: number
    currency?: string
    items?: OrderItem[]
  } | null
  status: string
  created_at: string
}

export interface OrderItem {
  retailer_id: string
  name?: string
  image_url?: string
  meta_product_id?: string
  quantity: number
  item_price: string | number
  line_total: number
  currency: string
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
  const params: Record<string, string> = { limit: '100', has_messages: 'true' }
  if (filters.channel?.length) params.channel = filters.channel.join(',')
  if (filters.unread) params.unread = 'true'
  if (filters.assigned) params.assigned = filters.assigned
  if (filters.search) params.search = filters.search
  const res = await api.get('/contacts', { params })
  return res.data?.contacts ?? []
}

export async function getConversation(contactId: string): Promise<Conversation> {
  const res = await api.get(`/contacts/${contactId}`)
  return res.data as Conversation
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

export async function sendTemplateMessage(
  contactId: string,
  templateId: string,
  templateParams: Record<string, string>,
  headerFile?: File | null
): Promise<void> {
  if (headerFile) {
    const form = new FormData()
    form.append('contact_id', contactId)
    form.append('template_id', templateId)
    form.append('template_params', JSON.stringify(templateParams))
    form.append('header_params', JSON.stringify(templateParams))
    form.append('header_file', headerFile)
    form.append('header_media_filename', headerFile.name)
    await api.post('/messages/template', form, {
      headers: { 'Content-Type': undefined as unknown as string }
    })
    return
  }
  await api.post('/messages/template', {
    contact_id: contactId,
    template_id: templateId,
    template_params: templateParams,
    header_params: templateParams
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

// Shares the WhatsApp account's connected catalog, a single product, or a
// multi-section product list in a free-form (session) message. Only usable
// while the 24h service window is open — same restriction as the rest of the
// composer, and enforced again server-side regardless.
export type CatalogSendPayload =
  | { mode: 'catalog'; body: string; thumbnailRetailerId?: string }
  | { mode: 'product'; body: string; catalogId: string; productRetailerId: string }
  | {
      mode: 'product_list'
      body: string
      catalogId: string
      headerText: string
      sections: { title: string; productRetailerIds: string[] }[]
    }

export async function sendCatalogMessage(contactId: string, payload: CatalogSendPayload): Promise<void> {
  const interactive: Record<string, unknown> = { type: payload.mode, body: payload.body }
  if (payload.mode === 'catalog') {
    if (payload.thumbnailRetailerId) interactive.thumbnail_retailer_id = payload.thumbnailRetailerId
  } else if (payload.mode === 'product') {
    interactive.catalog_id = payload.catalogId
    interactive.product_retailer_id = payload.productRetailerId
  } else {
    interactive.catalog_id = payload.catalogId
    interactive.header = payload.headerText
    interactive.sections = payload.sections.map((s) => ({
      title: s.title,
      product_retailer_ids: s.productRetailerIds
    }))
  }
  await api.post(`/contacts/${contactId}/messages`, { type: 'interactive', interactive })
}

export async function markRead(contactId: string): Promise<void> {
  await api.post(`/contacts/${contactId}/mark-read`)
}

// Sends the user's current location (Cloud API channels only).
export async function sendLocation(
  contactId: string,
  latitude: number,
  longitude: number,
  name = ''
): Promise<void> {
  await api.post(`/contacts/${contactId}/location`, { latitude, longitude, name })
}

export async function sendContactCard(
  contactId: string,
  card: { name: string; phone: string; email?: string; company?: string }
): Promise<void> {
  await api.post(`/contacts/${contactId}/contact-card`, card)
}

// Sends an image (or other media) via the multipart media endpoint.
export async function sendMedia(
  contactId: string,
  file: File,
  caption = '',
  type = 'image'
): Promise<void> {
  const form = new FormData()
  form.append('contact_id', contactId)
  form.append('type', type)
  if (caption) form.append('caption', caption)
  form.append('file', file)
  await api.post('/messages/media', form, { headers: { 'Content-Type': undefined as unknown as string } })
}

export function messageBody(m: Message): string {
  if (m.content && typeof m.content === 'object') return m.content.body ?? ''
  if (typeof m.content === 'string') return m.content
  return ''
}
