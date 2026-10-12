<script setup>
import { ref } from 'vue'
import { useRouter } from 'vue-router'
import { api } from '../api'
import { store } from '../store'
import { t } from '../i18n'

const router = useRouter()
const email = ref('')
const password = ref('')
const err = ref('')
const busy = ref(false)

async function submit() {
  err.value = ''
  busy.value = true
  try {
    const res = await api.post('/api/v1/auth/login', { email: email.value, password: password.value })
    store.user = res.user
    store.bootError = ''
    router.push('/')
  } catch (e) {
    err.value = e.message || t('loginFailed')
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <div class="max-w-sm mx-auto card p-8">
    <h1 class="text-lg font-semibold mb-1">{{ t('loginTitle') }}</h1>
    <p class="text-xs text-slate-500 dark:text-slate-400 mb-6">{{ t('loginHint') }}</p>
    <form @submit.prevent="submit" class="space-y-4">
      <div>
        <label class="label">{{ t('email') }}</label>
        <input v-model="email" type="email" required class="input" autocomplete="username" />
      </div>
      <div>
        <label class="label">{{ t('password') }}</label>
        <input v-model="password" type="password" required class="input" autocomplete="current-password" />
      </div>
      <p v-if="err" class="text-sm text-rose-600 dark:text-rose-400">{{ err }}</p>
      <button class="btn-primary w-full" :disabled="busy">{{ t('login') }}</button>
    </form>
  </div>
</template>
