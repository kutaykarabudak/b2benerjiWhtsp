import { api } from './api'

export interface WSEvent {
  type: string
  payload: any
}

// Realtime channel to the backend. Auth is a short-lived token (fetched over the
// authenticated cookie session) sent as the first WS message. Auto-reconnects.
export type RealtimeStatus = 'connecting' | 'connected' | 'disconnected'

export function createRealtime(
  onEvent: (ev: WSEvent) => void,
  onStatus: (status: RealtimeStatus) => void = () => {}
) {
  let ws: WebSocket | null = null
  let closed = false
  let reconnectTimer: number | undefined
  let pingTimer: number | undefined

  async function connect() {
    if (closed) return
    onStatus('connecting')
    let token: string | null = null
    let wsUrl = ''
    try {
      const res = await api.get('/auth/ws-token')
      token = res.data?.token ?? null
      wsUrl = res.data?.ws_url ?? ''
    } catch {
      scheduleReconnect()
      return
    }
    if (!token) {
      scheduleReconnect()
      return
    }
    // Prefer the server-provided WS URL (e.g. direct Cloud Run, since Firebase
    // Hosting can't proxy WebSockets). Fall back to same origin for local dev.
    let endpoint: string
    if (wsUrl) {
      endpoint = wsUrl.replace(/^http/, 'ws').replace(/\/$/, '') + '/ws'
    } else {
      const proto = location.protocol === 'https:' ? 'wss' : 'ws'
      endpoint = `${proto}://${location.host}/ws`
    }
    try {
      ws = new WebSocket(endpoint)
    } catch {
      scheduleReconnect()
      return
    }
    ws.onopen = () => {
      onStatus('connected')
      ws?.send(JSON.stringify({ type: 'auth', payload: { token } }))
      pingTimer = window.setInterval(() => {
        if (ws?.readyState === WebSocket.OPEN) ws.send(JSON.stringify({ type: 'ping' }))
      }, 25000)
    }
    ws.onmessage = (e) => {
      try {
        onEvent(JSON.parse(e.data) as WSEvent)
      } catch {
        /* ignore malformed */
      }
    }
    ws.onclose = () => {
      onStatus('disconnected')
      window.clearInterval(pingTimer)
      scheduleReconnect()
    }
    ws.onerror = () => ws?.close()
  }

  function scheduleReconnect() {
    if (closed) return
    window.clearTimeout(reconnectTimer)
    reconnectTimer = window.setTimeout(connect, 3000)
  }

  connect()

  return {
    close() {
      closed = true
      onStatus('disconnected')
      window.clearTimeout(reconnectTimer)
      window.clearInterval(pingTimer)
      ws?.close()
    }
  }
}
