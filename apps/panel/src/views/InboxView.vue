<script setup lang="ts">
import { ref, computed, onMounted, onBeforeUnmount, watch, nextTick } from 'vue'
import { useRoute } from 'vue-router'
import {
  listConversations,
  getConversation,
  getMessages,
  sendText,
  sendTemplateMessage,
  sendButtons,
  sendMedia,
  sendLocation,
  sendContactCard,
  sendCatalogMessage,
  markRead,
  updateContactInfo,
  messageBody,
  type Conversation,
  type Message,
  type ChannelType
} from '@/services/inbox'
import { listTemplates, type Template } from '@/services/templates'
import { listCatalogs, listProducts, type Catalog, type Product } from '@/services/catalog'
import { useRealtimeStore } from '@/stores/realtime'

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
const templates = ref<Template[]>([])
const openingTemplate = ref<Template | null>(null)
const openingTemplateParams = ref<Record<string, string>>({})
const openingHeaderFile = ref<File | null>(null)
const openingTemplateSent = ref(false)
const messagesEl = ref<HTMLElement | null>(null)
const draftInput = ref<HTMLTextAreaElement | null>(null)
const realtime = useRealtimeStore()
const attachmentMenuOpen = ref(false)
const fileInput = ref<HTMLInputElement | null>(null)
const cameraInput = ref<HTMLInputElement | null>(null)
const attachmentAccept = ref('*/*')
const pendingFile = ref<File | null>(null)
const pendingMediaType = ref<'image' | 'video' | 'audio' | 'document'>('image')
const pendingCaption = ref('')
const pendingPreviewURL = ref('')
const isDraggingOver = ref(false)
let dragDepth = 0
const showContactCard = ref(false)
const contactCard = ref({ name: '', phone: '', email: '', company: '' })

// Catalog/product message composer
const showCatalogPicker = ref(false)
const catalogPickerMode = ref<'catalog' | 'product' | 'product_list'>('catalog')
const catalogPickerCatalogs = ref<Catalog[]>([])
const catalogPickerCatalogId = ref('')
const catalogPickerProducts = ref<Product[]>([])
const catalogPickerSelected = ref<Set<string>>(new Set())
const catalogPickerSearch = ref('')
const catalogPickerBody = ref('')
const catalogPickerHeaderText = ref('')
const loadingCatalogPicker = ref(false)
const sendingCatalog = ref(false)

// Resolves catalog/product interactive messages in the chat history to their
// actual product (name/image/price) so the bubble shows what was sent
// instead of just a generic "product sent" label.
const productByRetailerId = ref<Map<string, Product>>(new Map())
const catalogsByAccount = new Map<string, Catalog[]>()
const productsLoadedForCatalogId = new Set<string>()

function resolvedProduct(retailerId: string | undefined): Product | undefined {
  if (!retailerId) return undefined
  return productByRetailerId.value.get(retailerId)
}

function resolvedProductListNames(m: Message): string[] {
  const sections = m.interactive_data?.sections || []
  const names: string[] = []
  for (const sec of sections) {
    for (const id of sec.product_retailer_ids || []) {
      const p = productByRetailerId.value.get(id)
      names.push(p ? p.name : id)
    }
  }
  return names
}

async function ensureCatalogProductsResolved() {
  if (!selected.value) return
  const account = selected.value.whatsapp_account || ''
  const metaCatalogIds = new Set<string>()
  for (const m of messages.value) {
    const type = m.interactive_data?.type
    if (type !== 'product' && type !== 'product_list') continue
    const catalogId = m.interactive_data?.catalog_id
    if (catalogId) metaCatalogIds.add(catalogId)
  }
  if (!metaCatalogIds.size) return

  let catalogs = catalogsByAccount.get(account)
  if (!catalogs) {
    try {
      catalogs = await listCatalogs(account)
      catalogsByAccount.set(account, catalogs)
    } catch {
      catalogs = []
    }
  }

  for (const metaCatalogId of metaCatalogIds) {
    const catalog = catalogs.find((c) => c.meta_catalog_id === metaCatalogId)
    if (!catalog || productsLoadedForCatalogId.has(catalog.id)) continue
    productsLoadedForCatalogId.add(catalog.id)
    try {
      const products = await listProducts(catalog.id)
      const next = new Map(productByRetailerId.value)
      for (const p of products) next.set(p.retailer_id, p)
      productByRetailerId.value = next
    } catch {
      // Leave the generic label as fallback if this fails.
    }
  }
}
const supportedDocumentTypes = new Set([
  'application/pdf',
  'text/plain',
  'application/msword',
  'application/vnd.ms-excel',
  'application/vnd.ms-powerpoint',
  'application/vnd.openxmlformats-officedocument.wordprocessingml.document',
  'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet',
  'application/vnd.openxmlformats-officedocument.presentationml.presentation'
])

// Interactive (button) composer — Cloud API channels only.
const buttonMode = ref(false)
const btnBody = ref('')
const btnTitles = ref<string[]>([''])
const canUseButtons = computed(() => selected.value?.channel_type === 'whatsapp')
const freeformLocked = computed(() =>
  selected.value?.channel_type === 'whatsapp' && !selected.value.service_window_open
)
const firstMessageTemplates = computed(() => templates.value.filter((template) =>
  template.status === 'APPROVED' &&
  template.is_first_message &&
  (!selected.value?.whatsapp_account || template.whatsapp_account === selected.value.whatsapp_account)
))

function extractTemplateVariables(template: Template): string[] {
  const names: string[] = []
  const seen = new Set<string>()
  const source = `${template.header_content || ''}\n${template.body_content || ''}`
  for (const match of source.matchAll(/{{\s*([^{}]+?)\s*}}/g)) {
    const name = match[1].trim()
    if (name && !seen.has(name)) {
      seen.add(name)
      names.push(name)
    }
  }
  return names
}

const openingVariables = computed(() =>
  openingTemplate.value ? extractTemplateVariables(openingTemplate.value) : []
)
const openingNeedsMedia = computed(() =>
  ['IMAGE', 'VIDEO', 'DOCUMENT'].includes((openingTemplate.value?.header_type || '').toUpperCase())
)
const canSendOpeningTemplate = computed(() => {
  if (!openingTemplate.value || sending.value) return false
  if (openingNeedsMedia.value && !openingHeaderFile.value) return false
  return openingVariables.value.every((name) => openingTemplateParams.value[name]?.trim())
})

function chooseOpeningTemplate(template: Template) {
  openingTemplate.value = template
  openingTemplateParams.value = Object.fromEntries(extractTemplateVariables(template).map((name) => [name, '']))
  openingHeaderFile.value = null
}

function cancelOpeningTemplate() {
  openingTemplate.value = null
  openingTemplateParams.value = {}
  openingHeaderFile.value = null
}

function onOpeningHeaderFile(event: Event) {
  const input = event.target as HTMLInputElement
  openingHeaderFile.value = input.files?.[0] || null
}

async function sendOpeningTemplate() {
  if (!selected.value || !openingTemplate.value || !canSendOpeningTemplate.value) return
  sending.value = true
  try {
    await sendTemplateMessage(
      selected.value.id,
      openingTemplate.value.id,
      openingTemplateParams.value,
      openingHeaderFile.value
    )
    openingTemplateSent.value = true
    cancelOpeningTemplate()
    await loadMessages()
  } catch (e: any) {
    alert(e?.response?.data?.message || 'İlk mesaj şablonu gönderilemedi.')
  } finally {
    sending.value = false
  }
}

// Contact info (CRM) editor for the open conversation.
const showContactEdit = ref(false)
const savingContact = ref(false)
const cForm = ref({ name: '', company: '', email: '', notes: '', tags: '' })

function openContactEdit() {
  const c = selected.value
  if (!c) return
  const m = c.metadata || {}
  cForm.value = {
    name: c.name || c.profile_name || '',
    company: m.company || '',
    email: m.email || '',
    notes: m.notes || '',
    tags: (c.tags || []).join(', ')
  }
  showContactEdit.value = true
}

async function saveContactInfo() {
  if (!selected.value) return
  savingContact.value = true
  try {
    const metadata: Record<string, unknown> = { ...(selected.value.metadata || {}) }
    metadata.company = cForm.value.company.trim()
    metadata.email = cForm.value.email.trim()
    metadata.notes = cForm.value.notes.trim()
    await updateContactInfo(selected.value.id, {
      profile_name: cForm.value.name.trim(),
      tags: cForm.value.tags ? cForm.value.tags.split(',').map((t) => t.trim()).filter(Boolean) : [],
      metadata
    })
    showContactEdit.value = false
    await loadConversations()
  } catch (e: any) {
    alert(e?.response?.data?.message || 'Kaydedilemedi.')
  } finally {
    savingContact.value = false
  }
}

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

function chooseAttachment(type: 'image' | 'video' | 'audio' | 'document') {
  pendingMediaType.value = type
  attachmentAccept.value = {
    image: 'image/*',
    video: 'video/*',
    audio: 'audio/*',
    document: '.pdf,.doc,.docx,.xls,.xlsx,.ppt,.pptx,.txt'
  }[type]
  attachmentMenuOpen.value = false
  nextTick(() => fileInput.value?.click())
}

function chooseCamera() {
  pendingMediaType.value = 'image'
  attachmentMenuOpen.value = false
  cameraInput.value?.click()
}

function inferMediaType(file: File): 'image' | 'video' | 'audio' | 'document' {
  if (file.type.startsWith('image/')) return 'image'
  if (file.type.startsWith('video/')) return 'video'
  if (file.type.startsWith('audio/')) return 'audio'
  return 'document'
}

// Shared by the file-picker (click-to-attach) and drag-and-drop paths so
// validation/preview-state logic only lives in one place.
function acceptFile(file: File) {
  if (!selected.value) return
  pendingMediaType.value = inferMediaType(file)
  if (pendingMediaType.value === 'document' && !supportedDocumentTypes.has(file.type)) {
    alert('Bu belge türü WhatsApp tarafından desteklenmiyor. PDF, TXT, DOC/DOCX, XLS/XLSX veya PPT/PPTX seçin.')
    return
  }
  pendingFile.value = file
  pendingCaption.value = draft.value.trim()
  if (pendingPreviewURL.value) URL.revokeObjectURL(pendingPreviewURL.value)
  pendingPreviewURL.value = ['image', 'video', 'audio'].includes(pendingMediaType.value) ? URL.createObjectURL(file) : ''
}

function onAttachmentChosen(e: Event) {
  const input = e.target as HTMLInputElement
  const file = input.files?.[0]
  input.value = ''
  if (!file || !selected.value) return
  acceptFile(file)
}

function onDragEnter(e: DragEvent) {
  if (!selected.value || freeformLocked.value) return
  if (!e.dataTransfer?.types.includes('Files')) return
  dragDepth++
  isDraggingOver.value = true
}

function onDragOver(_e: DragEvent) {
  // .prevent modifier on the listener already allows drop; nothing else to do.
}

function onDragLeave(_e: DragEvent) {
  dragDepth = Math.max(0, dragDepth - 1)
  if (dragDepth === 0) isDraggingOver.value = false
}

function onFileDrop(e: DragEvent) {
  dragDepth = 0
  isDraggingOver.value = false
  if (!selected.value || freeformLocked.value) return
  const file = e.dataTransfer?.files?.[0]
  if (file) acceptFile(file)
}

function cancelAttachment() {
  if (pendingPreviewURL.value) URL.revokeObjectURL(pendingPreviewURL.value)
  pendingFile.value = null
  pendingPreviewURL.value = ''
  pendingCaption.value = ''
}

async function sendAttachment() {
  if (!pendingFile.value || !selected.value || sending.value) return
  sending.value = true
  try {
    await sendMedia(selected.value.id, pendingFile.value, pendingCaption.value.trim(), pendingMediaType.value)
    draft.value = ''
    cancelAttachment()
    await loadMessages()
  } catch (err: any) {
    alert(err?.response?.data?.message || 'Dosya gönderilemedi.')
  } finally {
    sending.value = false
  }
}

async function submitContactCard() {
  if (!selected.value || !contactCard.value.name.trim() || !contactCard.value.phone.trim() || sending.value) return
  sending.value = true
  try {
    await sendContactCard(selected.value.id, {
      name: contactCard.value.name.trim(),
      phone: contactCard.value.phone.trim(),
      email: contactCard.value.email.trim(),
      company: contactCard.value.company.trim()
    })
    showContactCard.value = false
    contactCard.value = { name: '', phone: '', email: '', company: '' }
    await loadMessages()
  } catch (err: any) {
    alert(err?.response?.data?.message || 'Kişi kartı gönderilemedi.')
  } finally {
    sending.value = false
  }
}

const selectedPickerCatalog = computed(() =>
  catalogPickerCatalogs.value.find((c) => c.id === catalogPickerCatalogId.value) || null
)

const filteredCatalogProducts = computed(() => {
  const q = catalogPickerSearch.value.trim().toLowerCase()
  if (!q) return catalogPickerProducts.value
  return catalogPickerProducts.value.filter(
    (p) => p.name.toLowerCase().includes(q) || p.retailer_id.toLowerCase().includes(q)
  )
})

const canSendCatalogSelection = computed(() => {
  if (sendingCatalog.value) return false
  // Meta requires non-empty interactive.body.text for catalog/product/product_list.
  if (!catalogPickerBody.value.trim()) return false
  if (catalogPickerMode.value === 'catalog') return true
  if (!selectedPickerCatalog.value) return false
  if (catalogPickerMode.value === 'product') return catalogPickerSelected.value.size === 1
  return catalogPickerSelected.value.size > 0
})

function setCatalogPickerMode(mode: 'catalog' | 'product' | 'product_list') {
  catalogPickerMode.value = mode
  catalogPickerSelected.value = new Set()
}

async function selectPickerCatalog(catalog: Catalog) {
  catalogPickerCatalogId.value = catalog.id
  catalogPickerSelected.value = new Set()
  loadingCatalogPicker.value = true
  try {
    catalogPickerProducts.value = await listProducts(catalog.id)
  } catch {
    catalogPickerProducts.value = []
  } finally {
    loadingCatalogPicker.value = false
  }
}

function onPickerCatalogChange(e: Event) {
  const id = (e.target as HTMLSelectElement).value
  const catalog = catalogPickerCatalogs.value.find((c) => c.id === id)
  if (catalog) selectPickerCatalog(catalog)
}

async function openCatalogPicker() {
  if (!selected.value) return
  showCatalogPicker.value = true
  catalogPickerMode.value = 'catalog'
  catalogPickerCatalogId.value = ''
  catalogPickerProducts.value = []
  catalogPickerSelected.value = new Set()
  catalogPickerSearch.value = ''
  // Meta rejects catalog/product/product_list interactive sends with
  // "The parameter interactive.body.text is required" if this is empty, so
  // seed a sensible default instead of leaving it blank.
  catalogPickerBody.value = 'Ürünlerimize göz atın:'
  catalogPickerHeaderText.value = 'Ürünler'
  loadingCatalogPicker.value = true
  try {
    catalogPickerCatalogs.value = await listCatalogs(selected.value.whatsapp_account || '')
    if (catalogPickerCatalogs.value.length === 1) {
      await selectPickerCatalog(catalogPickerCatalogs.value[0])
      return
    }
  } catch {
    catalogPickerCatalogs.value = []
  } finally {
    loadingCatalogPicker.value = false
  }
}

function closeCatalogPicker() {
  showCatalogPicker.value = false
}

function toggleProductSelect(retailerId: string) {
  const next = new Set(catalogPickerSelected.value)
  if (next.has(retailerId)) {
    next.delete(retailerId)
  } else {
    if (catalogPickerMode.value === 'product') next.clear()
    next.add(retailerId)
  }
  catalogPickerSelected.value = next
}

async function sendCatalogSelection() {
  if (!selected.value || !canSendCatalogSelection.value) return
  sendingCatalog.value = true
  try {
    const body = catalogPickerBody.value.trim()
    if (catalogPickerMode.value === 'catalog') {
      await sendCatalogMessage(selected.value.id, { mode: 'catalog', body })
    } else if (catalogPickerMode.value === 'product') {
      const retailerId = Array.from(catalogPickerSelected.value)[0]
      await sendCatalogMessage(selected.value.id, {
        mode: 'product',
        body,
        catalogId: selectedPickerCatalog.value!.meta_catalog_id,
        productRetailerId: retailerId
      })
    } else {
      // Sort alphabetically by product name — Set iteration order is click
      // order, which would otherwise send products in a scrambled sequence.
      const nameByRetailerId = new Map(catalogPickerProducts.value.map((p) => [p.retailer_id, p.name]))
      const sortedRetailerIds = Array.from(catalogPickerSelected.value).sort((a, b) =>
        (nameByRetailerId.get(a) || a).localeCompare(nameByRetailerId.get(b) || b, 'tr')
      )
      await sendCatalogMessage(selected.value.id, {
        mode: 'product_list',
        body,
        catalogId: selectedPickerCatalog.value!.meta_catalog_id,
        headerText: catalogPickerHeaderText.value.trim() || 'Ürünler',
        sections: [{ title: 'Ürünler', productRetailerIds: sortedRetailerIds }]
      })
    }
    closeCatalogPicker()
    await loadMessages()
  } catch (err: any) {
    alert(err?.response?.data?.message || 'Katalog mesajı gönderilemedi.')
  } finally {
    sendingCatalog.value = false
  }
}

let fallbackTimer: number | undefined
let searchTimer: number | undefined

function channelMeta(t: ChannelType) {
  return CHANNELS.find((c) => c.type === t) ?? { type: t, label: t, icon: '•' }
}

function money(value: string | number | undefined, currency = 'TRY') {
  const amount = Number(value || 0)
  try {
    return new Intl.NumberFormat('tr-TR', { style: 'currency', currency: currency || 'TRY' }).format(amount)
  } catch {
    return `${amount.toFixed(2)} ${currency}`
  }
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
    if (activeChannel.value === 'all' && !unreadOnly.value && !assignedToMe.value && !search.value.trim()) {
      realtime.setUnreadCount(conversations.value.reduce((total, item) => total + (item.unread_count || 0), 0))
    }
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
  openingTemplateSent.value = false
  cancelOpeningTemplate()
  realtime.setActiveContact(c.id)
  await loadMessages()
  c.unread_count = 0
  try {
    await markRead(c.id)
    realtime.syncUnread()
  } catch {
    // The message list still opens if the read receipt cannot be sent.
  }
}

function backToList() {
  selected.value = null
  realtime.setActiveContact(null)
}

async function loadMessages(scroll = true) {
  if (!selected.value) return
  loadingMessages.value = true
  try {
    messages.value = await getMessages(selected.value.id)
    ensureCatalogProductsResolved()
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
    nextTick(resizeDraftInput)
    await loadMessages()
  } catch (e: any) {
    alert(e?.response?.data?.message || 'Mesaj gönderilemedi.')
  } finally {
    sending.value = false
  }
}

// Grows the composer textarea with its content (up to a cap) instead of
// scrolling internally, so pasted multi-line text stays visible while typing.
function resizeDraftInput() {
  const el = draftInput.value
  if (!el) return
  el.style.height = 'auto'
  el.style.height = `${Math.min(el.scrollHeight, 140)}px`
}

// Enter sends (matches the old single-line input's behavior); Shift+Enter
// inserts a newline like every other chat app.
function onDraftKeydown(e: KeyboardEvent) {
  if (e.key === 'Enter' && !e.shiftKey) {
    e.preventDefault()
    send()
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
async function onRealtime(ev: { type: string; payload: any }) {
  if (ev.type === 'new_message') {
    await loadConversations()
    const cid = ev.payload?.contact_id
    if (selected.value && cid && String(cid) === String(selected.value.id)) {
      await loadMessages()
      if (ev.payload?.direction === 'incoming' && document.visibilityState === 'visible' && document.hasFocus()) {
        try {
          await markRead(selected.value.id)
          realtime.syncUnread()
        } catch { /* non-critical */ }
      }
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
watch(() => realtime.lastEvent, (event) => {
  if (event) onRealtime(event)
})

const route = useRoute()

async function openRequestedContact(value: unknown) {
  const id = typeof value === 'string' ? value : ''
  if (!id || selected.value?.id === id) return
  let conversation = conversations.value.find((item) => item.id === id)
  if (!conversation) {
    // A CRM/CSV contact has no conversation row yet, so fetch it directly.
    // It becomes part of the inbox list after the first message is sent.
    try {
      conversation = await getConversation(id)
    } catch {
      conversation = undefined
    }
  }
  if (conversation) await openConversation(conversation)
}

watch(() => route.query.contact, openRequestedContact)

onMounted(async () => {
  realtime.start()
  await Promise.all([
    loadConversations(),
    listTemplates().then((items) => { templates.value = items }).catch(() => { templates.value = [] })
  ])
  // Opened from Contacts → "Sohbet": select that conversation.
  await openRequestedContact(route.query.contact)
  // Safety net if WebSocket can't get through (e.g. some proxies): periodic sync.
  fallbackTimer = window.setInterval(() => {
    loadConversations()
    if (selected.value && !loadingMessages.value) loadMessages(false)
  }, 15000)
})
onBeforeUnmount(() => {
  if (pendingPreviewURL.value) URL.revokeObjectURL(pendingPreviewURL.value)
  realtime.setActiveContact(null)
  window.clearInterval(fallbackTimer)
  window.clearTimeout(searchTimer)
})

function fmtTime(iso: string | null): string {
  if (!iso) return ''
  const d = new Date(iso)
  return d.toLocaleString('tr-TR', { hour: '2-digit', minute: '2-digit', day: '2-digit', month: '2-digit' })
}

// media_url is a server-side relative storage path. Load it through the
// authenticated endpoint instead of resolving it below the current /inbox URL.
function messageMediaURL(message: Message) {
  return `/api/media/${encodeURIComponent(message.id)}`
}

// Copies the raw message text (not the rendered DOM selection) so line
// breaks and spacing survive the round trip when pasted into another chat.
const copiedMessageId = ref<string | null>(null)
async function copyMessage(m: Message) {
  const text = messageBody(m) || m.interactive_data?.body || ''
  if (!text) return
  try {
    await navigator.clipboard.writeText(text)
    copiedMessageId.value = m.id
    setTimeout(() => {
      if (copiedMessageId.value === m.id) copiedMessageId.value = null
    }, 1500)
  } catch {
    // Clipboard access denied or unavailable — nothing to recover from here.
  }
}
</script>

<template>
  <div class="inbox" :class="{ 'chat-open': !!selected }">
    <!-- Top filter bar -->
    <div class="filterbar">
      <div class="inbox-title">
        <div><span class="eyebrow">MESAJLAR</span><b>Gelen Kutusu</b></div>
        <span class="realtime-state" :class="realtime.status"><i></i>{{ realtime.status === 'connected' ? 'Canlı' : 'Bağlanıyor' }}</span>
      </div>
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
        <button
          v-if="realtime.notificationPermission === 'default'"
          class="notify-button"
          title="Masaüstü bildirimlerini aç"
          @click="realtime.requestNotifications"
        >🔔 Bildirimleri aç</button>
        <label class="toggle"><input type="checkbox" v-model="unreadOnly" /> Okunmamış</label>
        <label class="toggle"><input type="checkbox" v-model="assignedToMe" /> Bana atanan</label>
        <div class="search-wrap"><span>⌕</span><input class="search" v-model="search" placeholder="İsim veya telefon ara" /></div>
      </div>
    </div>

    <div class="body">
      <!-- Conversation list -->
      <div class="list">
        <div v-if="loadingList && !conversations.length" class="hint muted">Yükleniyor…</div>
        <div v-else-if="listError" class="hint error">{{ listError }}</div>
        <div v-else-if="!conversations.length" class="hint empty-list"><span>💬</span><b>Konuşma bulunamadı</b><small>Yeni mesajlar burada görünecek.</small></div>
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
              <span :class="['conv-time', { unread: c.unread_count }]">{{ fmtTime(c.last_message_at) }}</span>
            </div>
            <div class="conv-bottom">
              <span :class="['conv-preview', { unread: c.unread_count }]">
                <span class="ch-icon">{{ channelMeta(c.channel_type).icon }}</span>
                {{ c.last_message_preview || '—' }}
              </span>
              <span v-if="c.unread_count" class="badge">{{ c.unread_count }}</span>
            </div>
          </div>
        </button>
      </div>

      <!-- Chat pane -->
      <div
        class="chat"
        @dragenter.prevent="onDragEnter"
        @dragover.prevent="onDragOver"
        @dragleave.prevent="onDragLeave"
        @drop.prevent="onFileDrop"
      >
        <div v-if="isDraggingOver" class="drop-overlay">
          <div class="drop-overlay-inner">📎 Dosyayı buraya bırakın</div>
        </div>
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
            <span v-if="selected.service_window_open && selected.channel_type === 'whatsapp'" class="window-open"><i></i> Yanıt penceresi açık</span>
            <span v-else-if="selected.channel_type === 'whatsapp'" class="window-closed">24s kapalı</span>
            <button class="edit-contact-btn" title="Kişi bilgisi" @click="openContactEdit">✏️</button>
          </div>

          <!-- Contact CRM editor -->
          <div v-if="showContactEdit" class="contact-edit">
            <div class="ce-row">
              <input v-model="cForm.name" placeholder="İsim" />
              <input v-model="cForm.company" placeholder="Şirket" />
            </div>
            <div class="ce-row">
              <input v-model="cForm.email" placeholder="E-posta" />
              <input v-model="cForm.tags" placeholder="Etiketler (virgülle)" />
            </div>
            <input v-model="cForm.notes" placeholder="Not…" />
            <div class="ce-actions">
              <button type="button" @click="showContactEdit = false">İptal</button>
              <button class="primary" :disabled="savingContact" @click="saveContactInfo">Kaydet</button>
            </div>
          </div>

          <div ref="messagesEl" class="messages">
            <div v-if="loadingMessages && !messages.length" class="hint muted">Yükleniyor…</div>
            <div
              v-for="m in messages"
              :key="m.id"
              :class="['msg', m.direction === 'outgoing' ? 'out' : 'in']"
            >
              <div class="bubble">
                <img
                  v-if="m.media_url && m.message_type === 'image'"
                  :src="messageMediaURL(m)"
                  :alt="messageBody(m) || 'Gelen görsel'"
                  class="bubble-img"
                />
                <video
                  v-else-if="m.media_url && m.message_type === 'video'"
                  :src="messageMediaURL(m)"
                  class="bubble-video"
                  controls
                  preload="metadata"
                />
                <audio
                  v-else-if="m.media_url && m.message_type === 'audio'"
                  :src="messageMediaURL(m)"
                  class="bubble-audio"
                  controls
                  preload="metadata"
                />
                <a
                  v-else-if="m.media_url && m.message_type === 'document'"
                  :href="messageMediaURL(m)"
                  target="_blank"
                  rel="noopener"
                  class="bubble-document"
                >📄 {{ m.media_filename || 'Belgeyi aç' }}</a>
                <!-- Template messages with an IMAGE/VIDEO/DOCUMENT header store the
                     same media_url as a plain media message, but message_type stays
                     'template' — so it needs its own branch, keyed off the MIME type
                     since there's no dedicated message_type to switch on. -->
                <img
                  v-else-if="m.media_url && m.message_type === 'template' && (m.media_mime_type || '').startsWith('image/')"
                  :src="messageMediaURL(m)"
                  :alt="messageBody(m) || 'Şablon görseli'"
                  class="bubble-img"
                />
                <video
                  v-else-if="m.media_url && m.message_type === 'template' && (m.media_mime_type || '').startsWith('video/')"
                  :src="messageMediaURL(m)"
                  class="bubble-video"
                  controls
                  preload="metadata"
                />
                <a
                  v-else-if="m.media_url && m.message_type === 'template'"
                  :href="messageMediaURL(m)"
                  target="_blank"
                  rel="noopener"
                  class="bubble-document"
                >📄 {{ m.media_filename || 'Şablon dokümanını aç' }}</a>
                <div v-else-if="m.message_type === 'order'" class="order-card">
                  <div class="order-head">
                    <span class="order-icon">🛍️</span>
                    <div><b>Yeni sipariş</b><small>{{ m.interactive_data?.items?.length || 0 }} ürün kalemi</small></div>
                  </div>
                  <div v-if="m.interactive_data?.items?.length" class="order-items">
                    <div v-for="(item, index) in m.interactive_data.items" :key="item.retailer_id || index" class="order-item">
                      <img v-if="item.image_url" :src="item.image_url" :alt="item.name || item.retailer_id" />
                      <div v-else class="order-image-empty">▦</div>
                      <div class="order-item-copy">
                        <b>{{ item.name || 'Ürün ' + item.retailer_id }}</b>
                        <small>SKU: {{ item.retailer_id }} · {{ item.quantity }} adet</small>
                      </div>
                      <strong>{{ money(item.line_total, item.currency) }}</strong>
                    </div>
                  </div>
                  <div v-else class="order-legacy">Sipariş ayrıntısı bu eski mesajda kaydedilmemiş.</div>
                  <div v-if="m.interactive_data?.items?.length" class="order-total">
                    <span>Toplam</span><strong>{{ money(m.interactive_data.total, m.interactive_data.currency) }}</strong>
                  </div>
                  <p v-if="m.interactive_data?.text" class="order-note">{{ m.interactive_data.text }}</p>
                </div>
                <div v-else-if="m.interactive_data?.type === 'catalog'" class="catalog-msg-label">
                  📦 Katalog mesajı gönderildi
                </div>
                <div
                  v-else-if="m.interactive_data?.type === 'product' && resolvedProduct(m.interactive_data.product_retailer_id)"
                  class="sent-product-card"
                >
                  <img
                    v-if="resolvedProduct(m.interactive_data.product_retailer_id)!.image_url"
                    :src="resolvedProduct(m.interactive_data.product_retailer_id)!.image_url"
                    :alt="resolvedProduct(m.interactive_data.product_retailer_id)!.name"
                  />
                  <div v-else class="order-image-empty">▦</div>
                  <div class="sent-product-copy">
                    <b>{{ resolvedProduct(m.interactive_data.product_retailer_id)!.name }}</b>
                    <small>{{ money(resolvedProduct(m.interactive_data.product_retailer_id)!.price / 100, resolvedProduct(m.interactive_data.product_retailer_id)!.currency) }}</small>
                  </div>
                </div>
                <div v-else-if="m.interactive_data?.type === 'product'" class="catalog-msg-label">
                  🛍️ Ürün gönderildi{{ m.interactive_data.product_retailer_id ? ' (SKU: ' + m.interactive_data.product_retailer_id + ')' : '' }}
                </div>
                <div v-else-if="m.interactive_data?.type === 'product_list'" class="catalog-msg-label">
                  🛍️ Ürün listesi gönderildi<template v-if="resolvedProductListNames(m).length">: {{ resolvedProductListNames(m).join(', ') }}</template>
                </div>
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
                <div
                  v-if="(['image', 'video', 'document'].includes(m.message_type) || (m.message_type === 'template' && m.media_url)) && messageBody(m)"
                  class="bubble-text media-caption"
                >{{ messageBody(m) }}</div>
                <div v-if="m.interactive_data?.buttons?.length" class="bubble-buttons">
                  <span v-for="(b, i) in m.interactive_data.buttons" :key="i" class="bubble-btn">{{ b.title }}</span>
                </div>
                <div v-if="m.direction === 'outgoing' && m.status === 'failed'" class="bubble-failed">
                  ✕ Gönderilemedi{{ m.error_message ? ': ' + m.error_message : '' }}
                </div>
                <div class="bubble-meta">
                  <button
                    v-if="messageBody(m) || m.interactive_data?.body"
                    type="button"
                    class="bubble-copy-btn"
                    :title="copiedMessageId === m.id ? 'Kopyalandı' : 'Mesajı kopyala'"
                    @click="copyMessage(m)"
                  >{{ copiedMessageId === m.id ? '✓' : '📋' }}</button>
                  {{ fmtTime(m.created_at) }}
                </div>
              </div>
            </div>
          </div>

          <!-- Interactive (button) composer -->
          <div v-if="buttonMode && !freeformLocked" class="composer btn-composer">
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

          <form v-else-if="!freeformLocked" class="composer" @submit.prevent="send">
            <div class="attachment-wrap">
              <button
                type="button"
                class="btn-toggle"
                title="Ek gönder"
                aria-label="Ek gönder"
                @click="attachmentMenuOpen = !attachmentMenuOpen"
              >📎</button>
              <div v-if="attachmentMenuOpen" class="attachment-menu">
                <button type="button" @click="chooseAttachment('image')"><span>🖼️</span> Fotoğraf</button>
                <button type="button" @click="chooseCamera"><span>📷</span> Kamera</button>
                <button type="button" @click="chooseAttachment('video')"><span>🎥</span> Video</button>
                <button type="button" @click="chooseAttachment('document')"><span>📄</span> Belge</button>
                <button type="button" @click="chooseAttachment('audio')"><span>🎵</span> Ses</button>
                <button
                  v-if="canUseButtons"
                  type="button"
                  @click="attachmentMenuOpen = false; shareLocation()"
                ><span>📍</span> Konum</button>
                <button
                  v-if="canUseButtons"
                  type="button"
                  @click="attachmentMenuOpen = false; showContactCard = true"
                ><span>👤</span> Kişi</button>
                <button
                  v-if="canUseButtons"
                  type="button"
                  @click="attachmentMenuOpen = false; openCatalogPicker()"
                ><span>🛒</span> Katalog</button>
              </div>
            </div>
            <input
              ref="fileInput"
              type="file"
              :accept="attachmentAccept"
              hidden
              @change="onAttachmentChosen"
            />
            <input
              ref="cameraInput"
              type="file"
              accept="image/*"
              capture="environment"
              hidden
              @change="onAttachmentChosen"
            />
            <button
              v-if="canUseButtons"
              type="button"
              class="btn-toggle"
              title="Çoktan seçmeli buton gönder"
              @click="buttonMode = true"
            >
              ⊞
            </button>
            <textarea
              ref="draftInput"
              v-model="draft"
              class="draft-input"
              rows="1"
              placeholder="Mesaj yazın…"
              @keydown="onDraftKeydown"
              @input="resizeDraftInput"
            ></textarea>
            <button class="primary send-btn" type="submit" :disabled="sending || !draft.trim()">Gönder</button>
          </form>
          <section v-else class="closed-window-composer">
            <div class="closed-window-head">
              <div>
                <b>24 saatlik yanıt penceresi kapalı</b>
                <small>Serbest mesaj gönderimi kilitlendi. Yalnızca “ilk mesaj” olarak açılmış onaylı şablonlar kullanılabilir.</small>
              </div>
              <span v-if="openingTemplateSent" class="waiting-reply">✓ Gönderildi · müşteri yanıtı bekleniyor</span>
            </div>

            <div v-if="!openingTemplate" class="opening-template-list">
              <button
                v-for="template in firstMessageTemplates"
                :key="template.id"
                type="button"
                class="opening-template-card"
                @click="chooseOpeningTemplate(template)"
              >
                <span>
                  <b>{{ template.display_name || template.name }}</b>
                  <small>{{ template.body_content }}</small>
                </span>
                <strong>Seç ›</strong>
              </button>
              <div v-if="!firstMessageTemplates.length" class="no-opening-template">
                Kullanılabilir ilk mesaj şablonu yok. Şablonlar ekranında onaylı bir şablon için
                <b>“İlk mesaj olarak kullan”</b> seçeneğini açın.
              </div>
            </div>

            <div v-else class="opening-template-form">
              <div class="opening-template-preview">
                <b>{{ openingTemplate.display_name || openingTemplate.name }}</b>
                <p>{{ openingTemplate.body_content }}</p>
              </div>
              <label v-for="name in openingVariables" :key="name">
                <span>{{ name }}</span>
                <input v-model="openingTemplateParams[name]" :placeholder="`${name} değeri`" />
              </label>
              <label v-if="openingNeedsMedia">
                <span>Şablon başlık dosyası *</span>
                <input type="file" @change="onOpeningHeaderFile" />
              </label>
              <div class="opening-template-actions">
                <button type="button" @click="cancelOpeningTemplate">Geri</button>
                <button type="button" class="primary" :disabled="!canSendOpeningTemplate" @click="sendOpeningTemplate">
                  {{ sending ? 'Gönderiliyor…' : 'Şablonu gönder' }}
                </button>
              </div>
            </div>
          </section>
        </template>
        <div v-else class="empty-chat">
          <div class="empty-chat-icon">💬</div>
          <h2>Mesajlarınız burada</h2>
          <p>Görüşmeye devam etmek için soldan bir konuşma seçin. Yeni mesajlar anlık olarak ekrana düşer.</p>
          <span :class="['empty-live', realtime.status]"><i></i>{{ realtime.statusLabel }}</span>
        </div>
      </div>
    </div>

    <div v-if="pendingFile" class="media-modal-backdrop" @click.self="cancelAttachment">
      <section class="media-modal" role="dialog" aria-modal="true" aria-label="Dosya önizleme">
        <header>
          <div>
            <b>{{ pendingFile.name }}</b>
            <small>{{ (pendingFile.size / 1024 / 1024).toFixed(1) }} MB</small>
          </div>
          <button type="button" aria-label="Kapat" @click="cancelAttachment">×</button>
        </header>
        <div class="media-preview">
          <img v-if="pendingMediaType === 'image'" :src="pendingPreviewURL" alt="Gönderilecek görsel" />
          <video v-else-if="pendingMediaType === 'video'" :src="pendingPreviewURL" controls />
          <audio v-else-if="pendingMediaType === 'audio'" :src="pendingPreviewURL" controls />
          <div v-else class="document-preview">📄<span>{{ pendingFile.name }}</span></div>
        </div>
        <textarea
          v-if="pendingMediaType !== 'audio'"
          v-model="pendingCaption"
          maxlength="1024"
          rows="3"
          placeholder="Açıklama ekleyin…"
        ></textarea>
        <div class="media-modal-actions">
          <button type="button" @click="cancelAttachment">İptal</button>
          <button type="button" class="primary" :disabled="sending" @click="sendAttachment">
            {{ sending ? 'Gönderiliyor…' : 'Gönder' }}
          </button>
        </div>
      </section>
    </div>

    <div v-if="showContactCard" class="media-modal-backdrop" @click.self="showContactCard = false">
      <section class="media-modal contact-card-modal" role="dialog" aria-modal="true" aria-label="Kişi kartı gönder">
        <header>
          <div><b>Kişi kartı gönder</b><small>Ad ve telefon zorunludur.</small></div>
          <button type="button" aria-label="Kapat" @click="showContactCard = false">×</button>
        </header>
        <input v-model="contactCard.name" placeholder="Ad soyad" autocomplete="name" />
        <input v-model="contactCard.phone" placeholder="Telefon (ülke koduyla)" inputmode="tel" autocomplete="tel" />
        <input v-model="contactCard.email" placeholder="E-posta (isteğe bağlı)" type="email" autocomplete="email" />
        <input v-model="contactCard.company" placeholder="Şirket (isteğe bağlı)" autocomplete="organization" />
        <div class="media-modal-actions">
          <button type="button" @click="showContactCard = false">İptal</button>
          <button
            type="button"
            class="primary"
            :disabled="sending || !contactCard.name.trim() || !contactCard.phone.trim()"
            @click="submitContactCard"
          >{{ sending ? 'Gönderiliyor…' : 'Gönder' }}</button>
        </div>
      </section>
    </div>

    <div v-if="showCatalogPicker" class="media-modal-backdrop" @click.self="closeCatalogPicker">
      <section class="media-modal catalog-picker-modal" role="dialog" aria-modal="true" aria-label="Katalog gönder">
        <header>
          <div><b>Katalog gönder</b><small>Yalnızca açık yanıt penceresinde gönderilebilir.</small></div>
          <button type="button" aria-label="Kapat" @click="closeCatalogPicker">×</button>
        </header>

        <div class="catalog-picker-modes">
          <button type="button" :class="{ on: catalogPickerMode === 'catalog' }" @click="setCatalogPickerMode('catalog')">Tüm Katalog</button>
          <button type="button" :class="{ on: catalogPickerMode === 'product' }" @click="setCatalogPickerMode('product')">Tek Ürün</button>
          <button type="button" :class="{ on: catalogPickerMode === 'product_list' }" @click="setCatalogPickerMode('product_list')">Ürün Listesi</button>
        </div>

        <select v-if="catalogPickerCatalogs.length > 1" :value="catalogPickerCatalogId" @change="onPickerCatalogChange">
          <option v-for="c in catalogPickerCatalogs" :key="c.id" :value="c.id">{{ c.name }} ({{ c.product_count }})</option>
        </select>

        <div v-if="!loadingCatalogPicker && !catalogPickerCatalogs.length" class="no-opening-template">
          Bu hesapta senkronize katalog yok. Önce Katalog ekranından bir katalog senkronlayın.
        </div>

        <textarea v-model="catalogPickerBody" rows="2" placeholder="Mesaj metni *"></textarea>
        <input v-if="catalogPickerMode === 'product_list'" v-model="catalogPickerHeaderText" placeholder="Başlık (ör. Ürünlerimiz)" />

        <template v-if="catalogPickerMode !== 'catalog' && catalogPickerCatalogs.length">
          <input v-model="catalogPickerSearch" class="catalog-picker-search" placeholder="Ürün ara…" />
          <div v-if="loadingCatalogPicker" class="hint muted">Yükleniyor…</div>
          <div v-else class="catalog-picker-products">
            <label v-for="p in filteredCatalogProducts" :key="p.retailer_id" class="catalog-picker-product">
              <input
                :type="catalogPickerMode === 'product' ? 'radio' : 'checkbox'"
                name="catalog-picker-product"
                :checked="catalogPickerSelected.has(p.retailer_id)"
                @change="toggleProductSelect(p.retailer_id)"
              />
              <img v-if="p.image_url" :src="p.image_url" :alt="p.name" />
              <div v-else class="order-image-empty">▦</div>
              <div class="catalog-picker-product-copy">
                <b>{{ p.name }}</b>
                <small>{{ money(p.price / 100, p.currency) }}</small>
              </div>
            </label>
            <div v-if="!filteredCatalogProducts.length" class="hint muted">Ürün bulunamadı.</div>
          </div>
        </template>

        <div class="media-modal-actions">
          <button type="button" @click="closeCatalogPicker">İptal</button>
          <button type="button" class="primary" :disabled="!canSendCatalogSelection" @click="sendCatalogSelection">
            {{ sendingCatalog ? 'Gönderiliyor…' : 'Gönder' }}
          </button>
        </div>
      </section>
    </div>
  </div>
</template>

<style scoped>
.inbox { display: flex; flex-direction: column; height: 100%; }

.filterbar {
  display: flex; align-items: center; gap: 18px;
  min-height: 74px; padding: 11px 18px; background: var(--panel); border-bottom: 1px solid var(--border); flex-wrap: wrap;
}
.inbox-title { display: flex; align-items: center; gap: 10px; padding-right: 18px; border-right: 1px solid var(--border); }
.inbox-title > div { display: flex; flex-direction: column; line-height: 1.1; }.inbox-title b { font-size: 17px; margin-top: 3px; }.inbox-title .eyebrow { font-size: 9px; }
.realtime-state { display: flex; align-items: center; gap: 5px; padding: 4px 8px; border-radius: 999px; color: var(--muted); background: var(--bg-2); font-size: 10px; font-weight: 650; }.realtime-state i { width: 6px; height: 6px; border-radius: 50%; background: #a9b4b8; }.realtime-state.connected { color: #087a55; background: var(--brand-soft); }.realtime-state.connected i { background: #20c77a; }
.chips { display: flex; gap: 6px; flex-wrap: wrap; }
.chip { min-height: 34px; padding: 6px 12px; border-radius: 999px; font-size: 12px; box-shadow: none; }
.chip.on { background: var(--brand-soft); border-color: rgba(11,149,103,.18); color: var(--brand); font-weight: 650; }
.filters-right { display: flex; align-items: center; gap: 10px; margin-left: auto; }
.toggle { display: flex; align-items: center; gap: 6px; font-size: 13px; color: var(--muted); white-space: nowrap; }
.toggle input { width: auto; }
.notify-button { min-height: 34px; padding: 7px 10px; color: #9b6b0b; border-color: #efdba4; background: #fff9e9; font-size: 11px; }
.search-wrap { display: flex; align-items: center; width: 220px; padding-left: 10px; border: 1px solid var(--border-strong); border-radius: 11px; background: var(--bg-2); }.search-wrap > span { color: var(--muted); font-size: 18px; }.search { width: 100%; min-height: 38px; border: 0; box-shadow: none !important; background: transparent; }

.body { flex: 1; display: flex; min-height: 0; }

.list { width: 365px; flex-shrink: 0; border-right: 1px solid var(--border); background: var(--panel); overflow-y: auto; }
.hint { padding: 20px; text-align: center; }
.empty-list { min-height: 260px; display: flex; flex-direction: column; align-items: center; justify-content: center; }.empty-list > span { width: 56px; height: 56px; display: grid; place-items: center; margin-bottom: 10px; border-radius: 18px; background: var(--brand-soft); font-size: 24px; }.empty-list small { margin-top: 3px; color: var(--muted); }
.conv {
  width: 100%; text-align: left; border: none; border-bottom: 1px solid var(--border);
  border-radius: 0; background: transparent; padding: 12px 15px; display: flex; align-items: center; gap: 12px; box-shadow: none;
}
.conv:hover { background: var(--bg-2); transform: none; box-shadow: none; }
.conv.active { background: linear-gradient(90deg,var(--brand-soft),rgba(11,149,103,.03)); box-shadow: inset 3px 0 0 var(--brand); }
.avatar {
  width: 44px; height: 44px; border-radius: 50%; background: linear-gradient(145deg,#35c991,#0b9567); color: #fff;
  display: grid; place-items: center; font-weight: 700; flex-shrink: 0; font-size: 17px;
  box-shadow: 0 3px 10px rgba(11,149,103,.16);
}
.avatar.sm { width: 34px; height: 34px; font-size: 14px; }
.conv-main { flex: 1; min-width: 0; }
.conv-top, .conv-bottom { display: flex; justify-content: space-between; align-items: center; gap: 8px; }
.conv-name { font-weight: 600; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.conv-time { color: var(--muted); font-size: 10px; flex-shrink: 0; }.conv-time.unread { color: var(--brand); font-weight: 700; }
.conv-preview { color: var(--muted); font-size: 12px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }.conv-preview.unread { color: #34444a; font-weight: 600; }
.ch-icon { margin-right: 2px; }
.badge { background: var(--brand); color: #fff; font-size: 11px; font-weight: 600; border-radius: 999px; padding: 1px 7px; flex-shrink: 0; }

.chat { flex: 1; display: flex; flex-direction: column; min-width: 0; position: relative; background-color: #edf2ef; background-image: radial-gradient(rgba(68,102,89,.055) 1px,transparent 1px); background-size: 18px 18px; }
.drop-overlay { position: absolute; inset: 8px; z-index: 50; display: grid; place-items: center; border: 3px dashed var(--brand); border-radius: 16px; background: rgba(11,149,103,.12); pointer-events: none; }
.drop-overlay-inner { padding: 14px 22px; border-radius: 14px; background: #fff; box-shadow: 0 10px 30px rgba(11,149,103,.25); color: var(--brand); font-weight: 650; }
.empty-chat { width: min(430px,80%); margin: auto; display: flex; flex-direction: column; align-items: center; text-align: center; }.empty-chat-icon { width: 78px; height: 78px; display: grid; place-items: center; border-radius: 25px; background: linear-gradient(145deg,#dff8ec,#ccecdf); font-size: 34px; box-shadow: 0 10px 30px rgba(11,149,103,.12); }.empty-chat h2 { margin: 18px 0 5px; font-size: 20px; }.empty-chat p { margin: 0; color: var(--muted); font-size: 13px; }.empty-live { display: flex; align-items: center; gap: 6px; margin-top: 17px; padding: 6px 10px; border-radius: 999px; color: var(--muted); background: rgba(255,255,255,.7); font-size: 10px; }.empty-live i { width: 7px; height: 7px; border-radius: 50%; background: #a9b4b8; }.empty-live.connected { color: #087a55; }.empty-live.connected i { background: #20c77a; box-shadow: 0 0 0 4px rgba(32,199,122,.12); }
.chat-head {
  display: flex; align-items: center; gap: 10px;
  min-height: 66px; padding: 10px 17px; border-bottom: 1px solid var(--border); background: rgba(255,255,255,.96); backdrop-filter: blur(8px);
}
.chat-head-info { flex: 1; min-width: 0; }
.chat-title { font-weight: 600; }
.small { font-size: 12px; }
.window-closed { font-size: 12px; color: var(--danger); }
.window-open { display: flex; align-items: center; gap: 5px; padding: 5px 8px; border-radius: 999px; color: #087a55; background: var(--brand-soft); font-size: 10px; }.window-open i { width: 6px; height: 6px; border-radius: 50%; background: #20c77a; }
.edit-contact-btn { min-height: 36px; width: 36px; border: 1px solid var(--border); background: var(--bg-2); font-size: 15px; padding: 0; cursor: pointer; }
.contact-edit { display: flex; flex-direction: column; gap: 6px; padding: 10px 16px; background: var(--panel); border-bottom: 1px solid var(--border); }
.ce-row { display: flex; gap: 6px; }
.ce-row input { flex: 1; }
.ce-actions { display: flex; justify-content: flex-end; gap: 8px; }
.back-btn { display: none; border: none; background: transparent; font-size: 22px; padding: 0 4px; line-height: 1; }

.messages { flex: 1; overflow-y: auto; padding: 22px max(18px,4vw); display: flex; flex-direction: column; gap: 7px; }
.msg { display: flex; }
.msg.out { justify-content: flex-end; }
.bubble { max-width: 72%; padding: 8px 11px; border-radius: 5px 14px 14px 14px; background: #fff; box-shadow: 0 2px 5px rgba(20,44,35,.07); }
.msg.out .bubble { border-radius: 14px 5px 14px 14px; background: #d8f8e7; }
.bubble-img, .bubble-video { max-width: 320px; max-height: 320px; border-radius: 6px; display: block; margin-bottom: 4px; }
.bubble-audio { display: block; width: min(320px, 65vw); }
.bubble-document { display: flex; align-items: center; gap: 8px; min-width: 180px; padding: 10px; border-radius: 8px; background: rgba(0,0,0,.045); color: #1d5f4a; text-decoration: none; font-weight: 600; }
.bubble-loc { color: #027eb5; text-decoration: none; font-weight: 600; }
.bubble-text { white-space: pre-wrap; word-break: break-word; }
.media-caption { margin-top: 5px; }
.bubble-meta { font-size: 10px; color: var(--muted); text-align: right; margin-top: 2px; display: flex; align-items: center; justify-content: flex-end; gap: 6px; }
.bubble-copy-btn { opacity: 0; transition: opacity .15s; background: none; border: none; cursor: pointer; font-size: 11px; line-height: 1; padding: 0; color: var(--muted); }
.bubble:hover .bubble-copy-btn { opacity: .7; }
.bubble-copy-btn:hover { opacity: 1 !important; }
.bubble-failed { margin-top: 5px; padding-top: 5px; border-top: 1px solid rgba(224,75,75,.2); color: var(--danger); font-size: 11px; font-weight: 600; }
.bubble-buttons { display: flex; flex-direction: column; gap: 4px; margin-top: 6px; border-top: 1px solid rgba(0,0,0,0.08); padding-top: 6px; }
.bubble-btn { text-align: center; font-size: 13px; color: #027eb5; padding: 5px; border-radius: 6px; background: rgba(0,0,0,0.03); }
.order-card { width: min(390px, 68vw); }
.order-head { display: flex; align-items: center; gap: 10px; padding-bottom: 9px; border-bottom: 1px solid rgba(13, 67, 49, .1); }
.order-head > div { display: flex; flex-direction: column; }
.order-head small, .order-item-copy small { color: var(--muted); font-size: 11px; }
.order-icon { display: grid; place-items: center; width: 34px; height: 34px; border-radius: 10px; background: #e6f8ef; }
.order-items { display: flex; flex-direction: column; }
.order-item { display: grid; grid-template-columns: 42px minmax(0, 1fr) auto; align-items: center; gap: 9px; padding: 9px 0; border-bottom: 1px solid rgba(13, 67, 49, .08); }
.order-item img, .order-image-empty { width: 42px; height: 42px; border-radius: 8px; object-fit: cover; background: #edf4f1; }
.order-image-empty { display: grid; place-items: center; color: var(--muted); }
.order-item-copy { display: flex; flex-direction: column; min-width: 0; }
.order-item-copy b { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; font-size: 13px; }
.order-item > strong { font-size: 12px; white-space: nowrap; }
.order-total { display: flex; justify-content: space-between; padding-top: 9px; font-size: 14px; }
.order-note { margin: 7px 0 0; color: var(--muted); font-size: 12px; }
.order-legacy { padding: 10px 0 2px; color: var(--muted); font-size: 12px; }

.btn-composer { flex-direction: column; align-items: stretch; gap: 6px; }
.btn-line { display: flex; }
.btn-composer-actions { display: flex; gap: 8px; justify-content: flex-end; }
.btn-toggle { border: 1px solid var(--border); background: var(--panel); padding: 0 12px; font-size: 18px; border-radius: var(--radius); }

.composer { display: flex; gap: 8px; padding: 11px 16px; border-top: 1px solid var(--border); background: rgba(255,255,255,.97); }
.composer input, .composer textarea { flex: 1; }
.composer > input:not([type=file]) { border-radius: 999px; padding-left: 17px; background: var(--bg-2); }
.draft-input { resize: none; max-height: 140px; overflow-y: auto; line-height: 1.4; border-radius: 20px; padding-left: 17px; padding-top: 10px; background: var(--bg-2); white-space: pre-wrap; }
.send-btn { border-radius: 999px; padding-left: 19px; padding-right: 19px; }
.attachment-wrap { position: relative; display: flex; }
.attachment-menu {
  position: absolute; left: 0; bottom: calc(100% + 10px); z-index: 20;
  width: 190px; padding: 7px; border: 1px solid var(--border); border-radius: 14px;
  background: #fff; box-shadow: 0 12px 35px rgba(18,48,38,.2);
}
.attachment-menu button {
  display: flex; align-items: center; gap: 10px; width: 100%; padding: 9px 10px;
  border: 0; background: transparent; box-shadow: none; text-align: left; color: inherit;
}
.attachment-menu button:hover { background: var(--brand-soft); transform: none; }
.attachment-menu span { width: 24px; text-align: center; font-size: 18px; }
.closed-window-composer { padding: 14px 16px 16px; border-top: 1px solid #edcf91; background: #fffaf0; }
.closed-window-head { display: flex; align-items: flex-start; justify-content: space-between; gap: 12px; margin-bottom: 10px; }
.closed-window-head > div { display: flex; flex-direction: column; gap: 2px; }
.closed-window-head small { color: var(--muted); line-height: 1.35; }
.waiting-reply { padding: 5px 9px; border-radius: 999px; color: #087a55; background: #e7f8ef; font-size: 10px; white-space: nowrap; }
.opening-template-list { display: flex; gap: 8px; overflow-x: auto; }
.opening-template-card { min-width: 230px; max-width: 320px; display: flex; align-items: center; justify-content: space-between; gap: 10px; padding: 10px 12px; text-align: left; background: #fff; }
.opening-template-card > span { min-width: 0; display: flex; flex-direction: column; gap: 3px; }
.opening-template-card small { max-height: 32px; overflow: hidden; color: var(--muted); font-weight: 400; line-height: 1.35; }
.opening-template-card strong { color: var(--brand); white-space: nowrap; font-size: 11px; }
.no-opening-template { width: 100%; padding: 10px 12px; border: 1px dashed #dfbf7e; border-radius: 10px; color: #79571d; font-size: 12px; background: #fff; }
.opening-template-form { display: grid; grid-template-columns: minmax(200px,1fr) repeat(2,minmax(140px,220px)); gap: 8px; align-items: end; }
.opening-template-form label { display: flex; flex-direction: column; gap: 4px; }
.opening-template-form label span { color: var(--muted); font-size: 10px; font-weight: 650; }
.opening-template-preview { align-self: stretch; padding: 9px 11px; border-radius: 10px; background: #fff; }
.opening-template-preview p { max-height: 44px; margin: 3px 0 0; overflow: hidden; color: var(--muted); font-size: 11px; white-space: pre-wrap; }
.opening-template-actions { display: flex; justify-content: flex-end; gap: 7px; grid-column: 1/-1; }
.media-modal-backdrop {
  position: fixed; inset: 0; z-index: 100; display: grid; place-items: center;
  padding: 18px; background: rgba(9,25,20,.62); backdrop-filter: blur(3px);
}
.media-modal { width: min(560px, 100%); max-height: 92vh; overflow: auto; padding: 16px; border-radius: 16px; background: #fff; box-shadow: 0 20px 55px rgba(0,0,0,.3); }
.media-modal header { display: flex; align-items: center; justify-content: space-between; gap: 12px; margin-bottom: 12px; }
.media-modal header > div { display: flex; flex-direction: column; min-width: 0; }
.media-modal header b { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.media-modal header small { color: var(--muted); }
.media-modal header button { border: 0; background: transparent; box-shadow: none; font-size: 26px; line-height: 1; }
.media-preview { min-height: 160px; max-height: 55vh; display: grid; place-items: center; overflow: hidden; border-radius: 12px; background: #edf2ef; }
.media-preview img, .media-preview video { display: block; max-width: 100%; max-height: 55vh; }
.media-preview audio { width: min(420px, 90%); }
.document-preview { display: flex; flex-direction: column; align-items: center; gap: 10px; padding: 30px; font-size: 42px; }
.document-preview span { max-width: 100%; color: var(--muted); font-size: 13px; overflow-wrap: anywhere; }
.media-modal textarea { width: 100%; margin-top: 12px; resize: vertical; }
.media-modal-actions { display: flex; justify-content: flex-end; gap: 8px; margin-top: 12px; }
.contact-card-modal { display: flex; flex-direction: column; gap: 9px; width: min(440px, 100%); }
.contact-card-modal header { margin-bottom: 3px; }
.catalog-msg-label { padding: 4px 2px; font-weight: 600; }
.sent-product-card { display: grid; grid-template-columns: 44px minmax(0,1fr); align-items: center; gap: 9px; width: min(280px, 60vw); }
.sent-product-card img, .sent-product-card .order-image-empty { width: 44px; height: 44px; border-radius: 8px; object-fit: cover; background: #edf4f1; }
.sent-product-copy { display: flex; flex-direction: column; min-width: 0; }
.sent-product-copy b { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; font-size: 13px; }
.sent-product-copy small { color: var(--muted); font-size: 11px; }
.catalog-picker-modal { display: flex; flex-direction: column; gap: 9px; width: min(480px, 100%); }
.catalog-picker-modal header { margin-bottom: 3px; }
.catalog-picker-modes { display: flex; gap: 6px; }
.catalog-picker-modes button { flex: 1; min-height: 34px; padding: 6px 8px; font-size: 12px; }
.catalog-picker-modes button.on { background: var(--brand-soft); border-color: rgba(11,149,103,.18); color: var(--brand); font-weight: 650; }
.catalog-picker-search { margin-top: 2px; }
.catalog-picker-products { display: flex; flex-direction: column; gap: 2px; max-height: 260px; overflow-y: auto; }
.catalog-picker-product { display: grid; grid-template-columns: auto 38px minmax(0,1fr); align-items: center; gap: 9px; padding: 7px 4px; border-radius: 8px; cursor: pointer; }
.catalog-picker-product:hover { background: var(--bg-2); }
.catalog-picker-product input { width: auto; }
.catalog-picker-product img, .catalog-picker-product .order-image-empty { width: 38px; height: 38px; border-radius: 8px; object-fit: cover; background: #edf4f1; }
.catalog-picker-product-copy { display: flex; flex-direction: column; min-width: 0; }
.catalog-picker-product-copy b { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; font-size: 13px; }
.catalog-picker-product-copy small { color: var(--muted); font-size: 11px; }

/* --- Mobile: WhatsApp-style single column --- */
@media (max-width: 768px) {
  .list { width: 100%; }
  .chat { display: none; }
  .inbox.chat-open .list { display: none; }
  .inbox.chat-open .filterbar { display: none; }
  .inbox.chat-open .chat { display: flex; }
  .back-btn { display: inline-flex; }
  .filters-right { width: 100%; }
  .search-wrap { flex: 1; width: auto; }
  .chip-label { display: none; }
  .inbox-title { border-right: 0; padding-right: 0; }
  .filterbar { gap: 10px; }
  .notify-button { display: none; }
}
</style>
