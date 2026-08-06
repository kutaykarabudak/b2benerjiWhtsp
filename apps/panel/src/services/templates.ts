import { api } from './api'

export interface Template {
  id: string
  whatsapp_account: string
  name: string
  display_name?: string
  language: string
  category: string
  status: string // APPROVED, PENDING, REJECTED
  quality_rating?: string
  header_type?: string
  header_content?: string
  body_content: string
  footer_content?: string
  buttons?: any[]
  is_first_message: boolean
}

export async function listTemplates(): Promise<Template[]> {
  const res = await api.get('/templates', { params: { limit: '200' } })
  return res.data?.templates ?? []
}

// Pulls the latest templates (and their approval status) from Meta for one account.
export async function syncTemplates(accountName: string): Promise<void> {
  await api.post('/templates/sync', { whatsapp_account: accountName })
}

export interface TemplateButton {
  type: 'QUICK_REPLY' | 'URL' | 'PHONE_NUMBER'
  text: string
  url?: string
  phone_number?: string
}

export interface TemplateInput {
  whatsapp_account: string
  name: string
  language: string
  category: string
  header_type?: string // '' | 'TEXT' | 'IMAGE' | 'VIDEO' | 'DOCUMENT'
  header_content?: string // text for TEXT headers, Meta media handle for IMAGE/VIDEO/DOCUMENT
  body_content: string
  footer_content?: string
  buttons?: TemplateButton[]
}

export interface TemplateHeaderMediaUpload {
  handle: string
  filename: string
  mime_type: string
  size: number
}

// Uploads a header sample file (image/video/document) to Meta and returns the
// resumable-upload handle to use as `header_content` when header_type is
// IMAGE/VIDEO/DOCUMENT.
export async function uploadTemplateHeaderMedia(file: File, account: string): Promise<TemplateHeaderMediaUpload> {
  const form = new FormData()
  form.append('file', file)
  form.append('account', account)
  const res = await api.post('/templates/upload-media', form, {
    headers: { 'Content-Type': undefined as unknown as string }
  })
  return res.data
}

export async function createTemplate(input: TemplateInput): Promise<string> {
  const res = await api.post('/templates', input)
  return res.data?.id ?? res.data?.template?.id ?? ''
}

export async function updateTemplate(id: string, input: TemplateInput): Promise<void> {
  await api.put(`/templates/${id}`, input)
}

// Submits a template to Meta for approval.
export async function submitTemplate(id: string): Promise<void> {
  await api.post(`/templates/${id}/publish`)
}

export async function deleteTemplate(id: string): Promise<void> {
  await api.delete(`/templates/${id}`)
}

export async function setTemplateFirstMessage(id: string, enabled: boolean): Promise<void> {
  await api.put(`/templates/${id}/first-message`, { enabled })
}
