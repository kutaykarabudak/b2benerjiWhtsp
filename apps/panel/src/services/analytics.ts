import { api } from './api'

export interface DashboardStats {
  total_messages: number
  messages_change: number
  total_contacts: number
  contacts_change: number
  chatbot_sessions: number
  chatbot_change: number
  campaigns_sent: number
  campaigns_change: number
}

export async function getDashboardStats(): Promise<DashboardStats | null> {
  const res = await api.get('/analytics/dashboard')
  return (res.data?.stats as DashboardStats) ?? null
}
