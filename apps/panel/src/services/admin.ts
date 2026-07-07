import { api } from './api'

export interface Role {
  id: string
  name: string
}

export interface User {
  id: string
  email: string
  full_name: string
  role_id?: string
  role?: { id: string; name: string } | null
  is_active: boolean
  is_super_admin: boolean
}

export interface WhatsAppAccount {
  id: string
  name: string
  app_id?: string
  phone_id: string
  business_id: string
  webhook_verify_token?: string
  api_version?: string
  status?: string
  has_access_token?: boolean
  has_app_secret?: boolean
}

// ---- Users ----
export async function listUsers(): Promise<User[]> {
  const res = await api.get('/users', { params: { limit: '200' } })
  return res.data?.users ?? []
}

export async function createUser(input: {
  email: string
  password: string
  full_name: string
  role_id?: string
}): Promise<void> {
  await api.post('/users', input)
}

export async function setUserActive(id: string, is_active: boolean): Promise<void> {
  await api.put(`/users/${id}`, { is_active })
}

export async function deleteUser(id: string): Promise<void> {
  await api.delete(`/users/${id}`)
}

// Admin sets a new password for a user (forgot-password recovery path).
export async function resetUserPassword(id: string, newPassword: string): Promise<void> {
  await api.post(`/users/${id}/reset-password`, { new_password: newPassword })
}

export async function listRoles(): Promise<Role[]> {
  const res = await api.get('/roles', { params: { limit: '100' } })
  return res.data?.roles ?? []
}

// ---- Channels / WhatsApp accounts ----
export async function listAccounts(): Promise<WhatsAppAccount[]> {
  const res = await api.get('/accounts')
  return res.data?.accounts ?? []
}

export interface AccountInput {
  name: string
  phone_id: string
  business_id: string
  access_token: string
  app_id?: string
  app_secret?: string
  webhook_verify_token?: string
}

export async function createAccount(input: AccountInput): Promise<void> {
  await api.post('/accounts', input)
}

export async function updateAccount(id: string, input: Partial<AccountInput>): Promise<void> {
  await api.put(`/accounts/${id}`, input)
}

export async function deleteAccount(id: string): Promise<void> {
  await api.delete(`/accounts/${id}`)
}

// ---- Meta App integration (per-organization, stored in DB, overrides config.toml) ----
export interface MetaAppSettings {
  meta_app_id: string
  meta_config_id: string
  meta_business_id: string
  has_meta_app_secret: boolean
}

export async function getMetaAppSettings(): Promise<MetaAppSettings> {
  const res = await api.get('/org/settings')
  const s = res.data?.settings || {}
  return {
    meta_app_id: s.meta_app_id || '',
    meta_config_id: s.meta_config_id || '',
    meta_business_id: s.meta_business_id || '',
    has_meta_app_secret: !!s.has_meta_app_secret
  }
}

export async function saveMetaAppSettings(input: {
  meta_app_id: string
  meta_config_id: string
  meta_business_id: string
  meta_app_secret?: string
}): Promise<void> {
  const body: Record<string, string> = {
    meta_app_id: input.meta_app_id,
    meta_config_id: input.meta_config_id,
    meta_business_id: input.meta_business_id
  }
  // Only send the secret when the user typed a new one (empty = keep existing).
  if (input.meta_app_secret) body.meta_app_secret = input.meta_app_secret
  await api.put('/org/settings', body)
}

// ---- Business profile (per WhatsApp number) ----
export interface BusinessProfile {
  about?: string
  address?: string
  description?: string
  email?: string
  vertical?: string
  websites?: string[]
  profile_picture_url?: string
}

export async function getBusinessProfile(accountId: string): Promise<BusinessProfile> {
  const res = await api.get(`/accounts/${accountId}/business_profile`)
  return (res.data as BusinessProfile) ?? {}
}

export async function updateBusinessProfile(accountId: string, input: BusinessProfile): Promise<void> {
  await api.put(`/accounts/${accountId}/business_profile`, {
    about: input.about || '',
    address: input.address || '',
    description: input.description || '',
    email: input.email || '',
    vertical: input.vertical || '',
    websites: input.websites || []
  })
}

export async function testAccount(id: string): Promise<{ ok: boolean; message?: string }> {
  try {
    const res = await api.post(`/accounts/${id}/test`)
    return { ok: true, message: res.data?.message }
  } catch (e: any) {
    return { ok: false, message: e?.response?.data?.message || 'Bağlantı başarısız.' }
  }
}
