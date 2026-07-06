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
}

export async function listTemplates(): Promise<Template[]> {
  const res = await api.get('/templates', { params: { limit: '200' } })
  return res.data?.templates ?? []
}

// Pulls the latest templates (and their approval status) from Meta for one account.
export async function syncTemplates(accountName: string): Promise<void> {
  await api.post('/templates/sync', { whatsapp_account: accountName })
}
