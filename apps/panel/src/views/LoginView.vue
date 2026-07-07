<script setup lang="ts">
import { ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useAuthStore } from '@/stores/auth'

const auth = useAuthStore()
const router = useRouter()
const route = useRoute()

const email = ref('')
const password = ref('')
const loading = ref(false)
const error = ref('')
const showForgot = ref(false)

async function submit() {
  error.value = ''
  loading.value = true
  try {
    await auth.login(email.value, password.value)
    const redirect = (route.query.redirect as string) || '/inbox'
    router.push(redirect)
  } catch (e: any) {
    error.value = e?.response?.data?.message || 'Giriş başarısız. Bilgileri kontrol edin.'
  } finally {
    loading.value = false
  }
}
</script>

<template>
  <div class="login-wrap">
    <form class="card login-card" @submit.prevent="submit">
      <div class="login-brand"><span class="brand-mark">B</span></div>
      <h1>B2B Panel</h1>
      <p class="muted">Devam etmek için giriş yapın</p>

      <label>Kullanıcı adı / E-posta</label>
      <input v-model="email" type="text" autocomplete="username" required />

      <label>Şifre</label>
      <input v-model="password" type="password" autocomplete="current-password" required />

      <p v-if="error" class="error">{{ error }}</p>

      <button class="primary" type="submit" :disabled="loading">
        {{ loading ? 'Giriş yapılıyor…' : 'Giriş Yap' }}
      </button>

      <button type="button" class="forgot-link" @click="showForgot = !showForgot">
        Şifremi unuttum?
      </button>
      <div v-if="showForgot" class="forgot-box muted">
        Şifreni yöneticin sıfırlayabilir: <b>Yönetim → Kullanıcılar → Şifre Sıfırla</b>.
        Yönetici sensen, admin hesabınla girip kendi/kullanıcı şifrelerini oradan sıfırlayabilirsin.
      </div>
    </form>
  </div>
</template>

<style scoped>
.login-wrap {
  min-height: 100%;
  display: grid;
  place-items: center;
  padding: 24px;
  background:
    radial-gradient(1000px 500px at 15% -10%, rgba(13, 150, 104, 0.14), transparent 60%),
    radial-gradient(900px 500px at 100% 110%, rgba(13, 150, 104, 0.1), transparent 55%),
    var(--bg);
}
.forgot-link { background: transparent; border: none; box-shadow: none; color: var(--brand); font-size: 13px; margin-top: 10px; padding: 4px; cursor: pointer; align-self: center; }
.forgot-link:hover { background: transparent; text-decoration: underline; }
.forgot-box { font-size: 13px; line-height: 1.5; margin-top: 4px; padding: 12px; background: var(--bg-2); border: 1px solid var(--border); border-radius: var(--radius); }
.login-card {
  width: 100%;
  max-width: 380px;
  display: flex;
  flex-direction: column;
  gap: 8px;
  padding: 30px 28px;
  border-radius: var(--radius-lg);
  box-shadow: var(--shadow-lg);
}
.login-brand { display: flex; justify-content: center; margin-bottom: 6px; }
.login-brand .brand-mark {
  display: grid; place-items: center;
  width: 52px; height: 52px; border-radius: 15px;
  background: var(--brand); color: #fff; font-size: 26px; font-weight: 700;
  box-shadow: 0 6px 18px rgba(13, 150, 104, 0.4);
}
.login-card h1 { margin: 0; font-size: 23px; text-align: center; }
.login-card p { margin: 0 0 10px; text-align: center; }
.login-card label { margin-top: 8px; font-size: 13px; color: var(--muted); }
.login-card > button.primary { margin-top: 18px; padding: 11px; font-size: 15px; }
.error { color: var(--danger); margin-top: 8px; text-align: center; }
</style>
