import { api } from './api'

export interface Account {
  name: string
}

export interface Template {
  id: string
  name: string
  display_name?: string
  language?: string
  status: string // APPROVED, PENDING, REJECTED
  header_type?: string // '' | 'TEXT' | 'IMAGE' | 'VIDEO' | 'DOCUMENT'
}

export interface Campaign {
  id: string
  name: string
  whatsapp_account: string
  template_id: string
  template_name?: string
  status: string // draft, queued, processing, completed, failed, paused
  total_recipients: number
  sent_count: number
  delivered_count: number
  read_count: number
  failed_count: number
  header_media_id?: string
  header_media_filename?: string
  created_at?: string
}

export async function listAccounts(): Promise<Account[]> {
  const res = await api.get('/accounts')
  return res.data?.accounts ?? []
}

export async function listApprovedTemplates(): Promise<Template[]> {
  const res = await api.get('/templates', { params: { limit: '200' } })
  const all: Template[] = res.data?.templates ?? []
  return all.filter((t) => t.status === 'APPROVED')
}

export async function listCampaigns(): Promise<Campaign[]> {
  const res = await api.get('/campaigns', { params: { limit: '100' } })
  return res.data?.campaigns ?? []
}

export async function createCampaign(input: {
  name: string
  whatsapp_account: string
  template_id: string
}): Promise<Campaign> {
  const res = await api.post('/campaigns', input)
  return res.data as Campaign
}

// Recipients are added as a JSON list; template params are optional per recipient.
export async function addRecipients(
  campaignId: string,
  recipients: { phone_number: string; template_params?: Record<string, unknown> }[]
): Promise<void> {
  await api.post(`/campaigns/${campaignId}/recipients/import`, { recipients })
}

// Uploads the header image/video/document for a media-header template. Required
// before a campaign using such a template can be started — the template's Meta
// approval only carries an example asset, not a reusable send-time media ID.
export async function uploadCampaignMedia(campaignId: string, file: File): Promise<void> {
  const form = new FormData()
  form.append('file', file)
  await api.post(`/campaigns/${campaignId}/media`, form, {
    headers: { 'Content-Type': undefined as unknown as string }
  })
}

export async function startCampaign(id: string): Promise<void> {
  await api.post(`/campaigns/${id}/start`)
}
export async function pauseCampaign(id: string): Promise<void> {
  await api.post(`/campaigns/${id}/pause`)
}
export async function cancelCampaign(id: string): Promise<void> {
  await api.post(`/campaigns/${id}/cancel`)
}
export async function deleteCampaign(id: string): Promise<void> {
  await api.delete(`/campaigns/${id}`)
}
