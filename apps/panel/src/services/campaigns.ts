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
