<script setup>
import { computed } from 'vue'
import { useRouter } from 'vue-router'
import { store } from '@/store'
import { t } from '@/i18n'
import { Button } from '@/components/ui/button'
import {
  Card, CardContent, CardDescription, CardHeader, CardTitle,
} from '@/components/ui/card'
import { Lock, WifiOff, LogIn, RotateCw } from '@lucide/vue'

const router = useRouter()

const unauthorized = computed(() => store.bootError === 'unauthorized')
const icon = computed(() => (unauthorized.value ? Lock : WifiOff))
const title = computed(() => (unauthorized.value ? t('unauthorizedHint') : t('loadFailed')))
const desc = computed(() => (unauthorized.value ? t('loginHint') : t('loadFailed')))

function retry() {
  location.reload()
}

function toLogin() {
  store.bootError = ''
  router.push('/login')
}
</script>

<template>
  <div class="w-full max-w-md">
    <Card>
      <CardHeader class="items-center text-center">
        <div class="mb-2 flex size-11 items-center justify-center rounded-xl bg-muted text-muted-foreground">
          <component :is="icon" class="size-5" />
        </div>
        <CardTitle class="text-base">{{ title }}</CardTitle>
        <CardDescription>{{ desc }}</CardDescription>
      </CardHeader>
      <CardContent class="flex flex-wrap justify-center gap-2">
        <Button v-if="!unauthorized" variant="outline" size="sm" @click="retry">
          <RotateCw class="size-4" />
          {{ t('retry') }}
        </Button>
        <Button size="sm" @click="toLogin">
          <LogIn class="size-4" />
          {{ t('login') }}
        </Button>
      </CardContent>
    </Card>
  </div>
</template>
