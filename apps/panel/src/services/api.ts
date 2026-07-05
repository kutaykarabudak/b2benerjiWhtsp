import axios, { type AxiosInstance } from 'axios'

// The Go backend uses HttpOnly cookies for auth and a double-submit CSRF
// cookie (whm_csrf) that must be echoed on mutating requests.
const API_BASE_URL = import.meta.env.VITE_API_URL || '/api'

export const api: AxiosInstance = axios.create({
  baseURL: API_BASE_URL,
  timeout: 30000,
  withCredentials: true,
  headers: { 'Content-Type': 'application/json' }
})

function getCookie(name: string): string | null {
  const match = document.cookie.match(new RegExp('(?:^|; )' + name + '=([^;]*)'))
  return match ? decodeURIComponent(match[1]) : null
}

api.interceptors.request.use((config) => {
  const method = (config.method || '').toUpperCase()
  if (['POST', 'PUT', 'DELETE', 'PATCH'].includes(method)) {
    const csrf = getCookie('whm_csrf')
    if (csrf) config.headers['X-CSRF-Token'] = csrf
  }
  return config
})

// The backend wraps responses in a fastglue envelope: { status, data, message }.
// Unwrap to the payload so callers work with plain data.
api.interceptors.response.use((res) => {
  if (res.data && typeof res.data === 'object' && 'data' in res.data) {
    res.data = (res.data as { data: unknown }).data
  }
  return res
})
