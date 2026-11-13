<script setup>
import { ref } from 'vue'
import { useRouter } from 'vue-router'
import { api } from '@/api'
import { store } from '@/store'
import { t } from '@/i18n'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Card, CardContent, CardDescription, CardHeader, CardTitle,
} from '@/components/ui/card'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Loader2, Ticket, AlertCircle, ShieldCheck } from '@lucide/vue'

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
  <div class="w-full max-w-sm space-y-6">
    <!-- 品牌 -->
    <div class="flex flex-col items-center gap-3 text-center">
      <div class="flex size-11 items-center justify-center rounded-xl bg-primary text-primary-foreground shadow-sm">
        <Ticket class="size-5" />
      </div>
      <div class="space-y-1">
        <h1 class="text-xl font-semibold tracking-tight">{{ t('loginTitle') }}</h1>
        <p class="text-sm text-muted-foreground">{{ t('loginHint') }}</p>
      </div>
    </div>

    <Card>
      <CardHeader class="sr-only">
        <CardTitle>{{ t('loginTitle') }}</CardTitle>
        <CardDescription>{{ t('loginHint') }}</CardDescription>
      </CardHeader>
      <CardContent>
        <form class="space-y-4" @submit.prevent="submit">
          <div class="space-y-2">
            <Label for="email">{{ t('email') }}</Label>
            <Input
              id="email" v-model="email" type="email" required
              autocomplete="username" placeholder="you@example.com" :disabled="busy"
            />
          </div>
          <div class="space-y-2">
            <Label for="password">{{ t('password') }}</Label>
            <Input
              id="password" v-model="password" type="password" required
              autocomplete="current-password" :disabled="busy"
            />
          </div>

          <Alert v-if="err" variant="destructive">
            <AlertCircle />
            <AlertTitle>{{ t('loginFailed') }}</AlertTitle>
            <AlertDescription>{{ err }}</AlertDescription>
          </Alert>

          <Button type="submit" class="w-full" :disabled="busy || !email || !password">
            <Loader2 v-if="busy" class="size-4 animate-spin" />
            {{ t('login') }}
          </Button>
        </form>
      </CardContent>
    </Card>

    <div class="flex items-center justify-center gap-1.5 text-xs text-muted-foreground">
      <ShieldCheck class="size-3.5" />
      sub2api SSO
    </div>
  </div>
</template>
