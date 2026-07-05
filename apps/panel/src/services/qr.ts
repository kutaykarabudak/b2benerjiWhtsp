import { api } from './api'

export interface QRStatus {
  state: 'disconnected' | 'qr' | 'connected'
  phone?: string
  qr?: string
}

export async function getQRStatus(): Promise<QRStatus> {
  const res = await api.get('/qr/status')
  return res.data as QRStatus
}

export async function qrConnect(): Promise<QRStatus> {
  const res = await api.post('/qr/connect')
  return res.data as QRStatus
}

export async function qrLogout(): Promise<void> {
  await api.post('/qr/logout')
}
