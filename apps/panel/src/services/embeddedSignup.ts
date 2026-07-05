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
    const loginOptions: any = {
      config_id: cfg.config_id,
      response_type: 'code',
      override_default_response_type: true,
      extras: coexistence
        ? { setup: {}, featureType: 'whatsapp_business_app_onboarding', sessionInfoVersion: '3', version: 'v3' }
        : { setup: {} }
    }
    window.FB.login((response: any) => {
      if (response.authResponse?.code) {
        resolve({
          code: response.authResponse.code,
          phone_number_id: response.authResponse.phone_number_id,
          waba_id: response.authResponse.waba_id
        })
      } else if (response.error) {
        reject(new Error(response.error.message || 'Facebook hatası'))
      } else {
        reject(new Error('Giriş iptal edildi.'))
      }
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
