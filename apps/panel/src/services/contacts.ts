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
}

export interface ImportResult {
  created: number
  updated: number
  skipped: number
  errors: number
  messages: string[]
}

export async function listContacts(search = ''): Promise<Contact[]> {
  const params: Record<string, string> = { limit: '200' }
  if (search.trim()) params.search = search.trim()
  const res = await api.get('/contacts', { params })
  return res.data?.contacts ?? []
}

export async function createContact(input: CreateContactInput): Promise<void> {
  await api.post('/contacts', input)
}

export async function deleteContact(id: string): Promise<void> {
  await api.delete(`/contacts/${id}`)
}

// Uploads a CSV to the shared import endpoint. Required column: phone_number.
// Optional: profile_name, whats_app_account, tags, assigned_user_id.
export async function importContactsCSV(file: File, updateOnDuplicate: boolean): Promise<ImportResult> {
  const form = new FormData()
  form.append('table', 'contacts')
  form.append('update_on_duplicate', String(updateOnDuplicate))
  form.append('file', file)
  // Let the browser set multipart boundary; overriding the JSON default header.
  const res = await api.post('/import', form, { headers: { 'Content-Type': undefined as unknown as string } })
  return res.data as ImportResult
}
