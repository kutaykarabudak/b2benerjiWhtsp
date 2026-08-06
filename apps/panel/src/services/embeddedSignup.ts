import { api } from './api'

// Meta Embedded Signup config, served by the backend from config.toml [whatsapp].
export interface EmbeddedSignupConfig {
  app_id: string
  config_id: string
  api_version: string
}

declare global {
  interface Window {
    FB?: any
    fbAsyncInit?: () => void
  }
}

let sdkLoaded = false

export async function getEmbeddedSignupConfig(): Promise<EmbeddedSignupConfig | null> {
  try {
    const res = await api.get('/embedded-signup/config')
    const d = res.data || {}
    const cfg: EmbeddedSignupConfig = {
      app_id: d.whatsapp_app_id || '',
      config_id: d.whatsapp_config_id || '',
      api_version: d.whatsapp_api_version || 'v21.0'
    }
    return cfg.app_id && cfg.config_id ? cfg : null
  } catch {
    return null
  }
}

// Loads the Facebook JS SDK once and initialises it with the given app.
export function loadFacebookSDK(cfg: EmbeddedSignupConfig): Promise<void> {
  return new Promise((resolve) => {
    if (sdkLoaded && window.FB) {
      resolve()
      return
    }
    const script = document.createElement('script')
    script.src = 'https://connect.facebook.net/en_US/sdk.js'
    script.async = true
    script.defer = true
    script.onload = () => {
      window.FB.init({ appId: cfg.app_id, cookie: true, xfbml: true, version: cfg.api_version })
      sdkLoaded = true
      resolve()
    }
    document.body.appendChild(script)
  })
}

export interface SignupResult {
  code: string
  phone_number_id?: string
  waba_id?: string
}

// Session info Meta posts from the Embedded Signup popup (WA_EMBEDDED_SIGNUP).
// The definitive waba_id/phone_number_id arrive here, NOT in authResponse.
interface SessionInfo {
  event?: string // FINISH | FINISH_ONLY_WABA | CANCEL | ERROR
  phone_number_id?: string
  waba_id?: string
  business_id?: string
  current_step?: string
  error_message?: string
}

let lastSessionInfo: SessionInfo | null = null
let sessionListenerAttached = false
let sessionInfoWaiter: ((info: SessionInfo | null) => void) | null = null

function isFacebookOrigin(origin: string): boolean {
  try {
    const hostname = new URL(origin).hostname
    return hostname === 'facebook.com' || hostname.endsWith('.facebook.com')
  } catch {
    return false
  }
}

function waitForSessionInfo(timeoutMs = 1500): Promise<SessionInfo | null> {
  if (lastSessionInfo) return Promise.resolve(lastSessionInfo)
  return new Promise((resolve) => {
    const timeout = window.setTimeout(() => {
      if (sessionInfoWaiter === finish) sessionInfoWaiter = null
      resolve(lastSessionInfo)
    }, timeoutMs)
    const finish = (info: SessionInfo | null) => {
      window.clearTimeout(timeout)
      if (sessionInfoWaiter === finish) sessionInfoWaiter = null
      resolve(info)
    }
    sessionInfoWaiter = finish
  })
}

function attachSessionInfoListener() {
  if (sessionListenerAttached) return
  sessionListenerAttached = true
  window.addEventListener('message', (event: MessageEvent) => {
    if (typeof event.origin !== 'string' || !isFacebookOrigin(event.origin)) return
    try {
      const data = typeof event.data === 'string' ? JSON.parse(event.data) : event.data
      if (data?.type === 'WA_EMBEDDED_SIGNUP') {
        lastSessionInfo = { ...(data.data || {}), event: data.event }
        sessionInfoWaiter?.(lastSessionInfo)
      }
    } catch {
      /* non-JSON messages from facebook.com are irrelevant */
    }
  })
}

// Launches Meta's Embedded Signup popup. coexistence=true onboards a number that
// stays live on the WhatsApp Business app (whatsapp_business_app_onboarding).
export function launchWhatsAppSignup(
  cfg: EmbeddedSignupConfig,
  coexistence = true
): Promise<SignupResult> {
  return new Promise((resolve, reject) => {
    if (!window.FB) {
      reject(new Error('Facebook SDK henüz yüklenmedi.'))
      return
    }
    attachSessionInfoListener()
    lastSessionInfo = null
    const loginOptions: any = {
      config_id: cfg.config_id,
      response_type: 'code',
      override_default_response_type: true,
      extras: coexistence
        ? { setup: {}, featureType: 'whatsapp_business_app_onboarding', sessionInfoVersion: '3' }
        : { setup: {}, sessionInfoVersion: '3' }
    }
    // Meta's JS SDK explicitly validates this callback as a plain Function.
    // An `async` callback has the internal type "asyncfunction" and recent SDK
    // builds reject it before opening the popup. Keep this callback synchronous
    // and continue the asynchronous session-info wait through a Promise chain.
    window.FB.login((response: any) => {
      void Promise.resolve(lastSessionInfo || waitForSessionInfo()).then((si) => {
        // The OAuth callback and WA_EMBEDDED_SIGNUP postMessage are separate
        // browser events; either one can arrive first.
      if (response.authResponse?.code) {
        resolve({
          code: response.authResponse.code,
          phone_number_id: si?.phone_number_id || response.authResponse.phone_number_id,
          waba_id: si?.waba_id || response.authResponse.waba_id
        })
      } else if (response.error) {
        reject(new Error(response.error.message || 'Facebook hatası'))
      } else if (si?.event === 'ERROR' && si.error_message) {
        reject(new Error('Meta hatası: ' + si.error_message))
      } else if (si?.event === 'CANCEL' && si.current_step) {
        reject(new Error(`Akış "${si.current_step}" adımında yarıda kaldı.`))
      } else {
        reject(new Error('Giriş iptal edildi (popup kapandı veya izin verilmedi).'))
      }
      }).catch(reject)
    }, loginOptions)
  })
}

export interface ExchangeResult {
  status?: string
  pin?: string
}

export async function exchangeToken(result: SignupResult): Promise<ExchangeResult> {
  const res = await api.post('/accounts/exchange-token', {
    code: result.code,
    phone_id: result.phone_number_id,
    waba_id: result.waba_id
  })
  const d = res.data || {}
  return { status: d.account?.status, pin: d.pin }
}
