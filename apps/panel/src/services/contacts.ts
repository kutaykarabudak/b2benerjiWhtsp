import { api } from './api'
import type { Conversation } from './inbox'

// A contact and a conversation share the same backend record, so we reuse the
// Conversation shape for list rows here.
export type Contact = Conversation

export interface CreateContactInput {
  phone_number: string
  profile_name?: string
  whatsapp_account?: string
  tags?: string[]
  metadata?: Record<string, unknown> // CRM fields: company, email, notes…
  email?: string; company_name?: string; tax_office?: string; tax_number?: string
  address?: string; city?: string; district?: string; postal_code?: string
  purchase_score?: number; has_purchased?: boolean
}

export interface ContactFilters { search?: string; tags?: string; has_purchased?: boolean; min_purchase_score?: number; city?: string; district?: string }

export interface ImportResult {
  created: number
  updated: number
  deleted: number
  skipped: number
  errors: number
  messages: string[]
}

export const CONTACT_SNAPSHOT_COLUMNS = [
  'id', 'customer_id', 'first_name', 'last_name', 'profile_name', 'company_name', 'email',
  'phone_country_code', 'phone_number', 'tax_office', 'tax_number', 'postal_code',
  'city', 'district', 'address', 'purchase_score', 'has_purchased', 'tags', 'note'
]

export async function listContacts(search = '', filters: ContactFilters = {}): Promise<Contact[]> {
  const params: Record<string, string | number | boolean> = { limit: 100 }
  if (search.trim()) params.search = search.trim()
  if (filters.tags) params.tags = filters.tags
  if (filters.has_purchased !== undefined) params.has_purchased = filters.has_purchased
  if (filters.min_purchase_score !== undefined) params.min_purchase_score = filters.min_purchase_score
  if (filters.city) params.city = filters.city
  if (filters.district) params.district = filters.district
  const contacts: Contact[] = []
  let page = 1
  let total = 0
  do {
    const res = await api.get('/contacts', { params: { ...params, page } })
    const batch: Contact[] = res.data?.contacts ?? []
    contacts.push(...batch)
    total = Number(res.data?.total ?? contacts.length)
    if (batch.length < 100) break
    page++
  } while (contacts.length < total)
  return contacts
}

export async function createContact(input: CreateContactInput): Promise<void> {
  await api.post('/contacts', input)
}

export async function updateContact(id: string, input: Partial<CreateContactInput>): Promise<void> {
  await api.put(`/contacts/${id}`, input)
}

export async function deleteContact(id: string): Promise<void> {
  await api.delete(`/contacts/${id}`)
}

export async function downloadContactsCSV(): Promise<Blob> {
  const res = await api.post('/export', {
    table: 'contacts',
    columns: CONTACT_SNAPSHOT_COLUMNS
  }, { responseType: 'blob' })
  return res.data as Blob
}

// Replaces the complete active contact list with the uploaded snapshot.
export async function importContactsCSV(file: File): Promise<ImportResult> {
  const form = new FormData()
  form.append('table', 'contacts')
  form.append('replace_all', 'true')
  form.append('file', file)
  // Let the browser set multipart boundary; overriding the JSON default header.
  const res = await api.post('/import', form, { headers: { 'Content-Type': undefined as unknown as string } })
  return res.data as ImportResult
}
