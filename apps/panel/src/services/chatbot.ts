import { api } from './api'

export type MatchType = 'contains' | 'exact' | 'starts_with' | 'regex'

export interface KeywordRule {
  id: string
  name: string
  keywords: string[]
  match_type: MatchType
  response_type: string
  response_content: { body?: string } & Record<string, unknown>
  priority: number
  enabled: boolean
}

export interface RuleButton {
  id: string
  title: string
}

export interface RuleInput {
  name: string
  keywords: string[]
  match_type: MatchType
  reply: string
  buttons: RuleButton[] // up to 3; Cloud API only (ignored on QR channel)
  priority: number
  enabled: boolean
}

export async function listRules(search = ''): Promise<KeywordRule[]> {
  const params: Record<string, string> = { limit: '200' }
  if (search.trim()) params.search = search.trim()
  const res = await api.get('/chatbot/keywords', { params })
  return res.data?.rules ?? []
}

function toPayload(input: RuleInput) {
  const content: Record<string, unknown> = { body: input.reply }
  const buttons = (input.buttons || []).filter((b) => b.title.trim())
  if (buttons.length) {
    content.buttons = buttons.map((b) => ({ id: b.title.trim(), title: b.title.trim() }))
  }
  return {
    name: input.name || input.keywords[0] || 'Kural',
    keywords: input.keywords,
    match_type: input.match_type,
    response_type: 'text',
    response_content: content,
    priority: input.priority,
    enabled: input.enabled
  }
}

export async function createRule(input: RuleInput): Promise<void> {
  await api.post('/chatbot/keywords', toPayload(input))
}

export async function updateRule(id: string, input: RuleInput): Promise<void> {
  await api.put(`/chatbot/keywords/${id}`, toPayload(input))
}

export async function toggleRule(id: string, enabled: boolean): Promise<void> {
  await api.put(`/chatbot/keywords/${id}`, { enabled })
}

export async function deleteRule(id: string): Promise<void> {
  await api.delete(`/chatbot/keywords/${id}`)
}
