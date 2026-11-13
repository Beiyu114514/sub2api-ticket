<script setup>
import { ref, onMounted, computed } from 'vue'
import { api } from '@/api'
import { t } from '@/i18n'
import { store } from '@/store'
import { toast } from 'vue-sonner'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { Separator } from '@/components/ui/separator'
import { Skeleton } from '@/components/ui/skeleton'
import { Avatar, AvatarFallback } from '@/components/ui/avatar'
import {
  Card, CardContent, CardDescription, CardHeader, CardTitle,
} from '@/components/ui/card'
import {
  Alert, AlertDescription, AlertTitle,
} from '@/components/ui/alert'
import { initials } from '@/lib/format'
import {
  Loader2, Copy, Check, ExternalLink, Unlink, Info,
  ShieldCheck, Mail, Plane, ArrowLeft,
} from '@lucide/vue'

const bound = ref(false)
const botReady = ref(false)
const botUsername = ref('')
const loading = ref(true)
const bindLink = ref('')
const bindCode = ref('')
const busy = ref(false)
const err = ref('')
const copied = ref(false)

const roleLabel = computed(() => (store.isAdmin ? t('roleAdmin') : t('roleUser')))

async function load() {
  try {
    const res = await api.get('/api/v1/telegram/bind-status')
    bound.value = !!res.bound
    botReady.value = !!res.bot_ready
    botUsername.value = res.bot_username || ''
  } catch (e) {
    err.value = e.message
  } finally {
    loading.value = false
  }
}

async function createBind() {
  busy.value = true
  err.value = ''
  copied.value = false
  try {
    const res = await api.post('/api/v1/telegram/bind')
    bindLink.value = res.link
    bindCode.value = res.code
  } catch (e) {
    err.value = e.message
    toast.error(e.message)
  } finally {
    busy.value = false
  }
}

async function unbind() {
  if (!confirm(t('unbindConfirm'))) return
  busy.value = true
  try {
    await api.del('/api/v1/telegram/bind')
    bound.value = false
    bindLink.value = ''
    toast.success(t('saved'))
  } catch (e) {
    err.value = e.message
    toast.error(e.message)
  } finally {
    busy.value = false
  }
}

async function copyLink() {
  try {
    await navigator.clipboard.writeText(bindLink.value)
    copied.value = true
    toast.success(t('linkCopied'))
  } catch {
    // iframe 内剪贴板受限时退化为手动提示
    window.prompt(t('tgCopyLink'), bindLink.value)
  }
}

onMounted(load)
</script>

<template>
  <div class="space-y-6">
    <div class="space-y-1">
      <h1 class="text-2xl font-semibold tracking-tight">{{ t('settingsTitle') }}</h1>
      <p class="text-sm text-muted-foreground">{{ t('settingsDesc') }}</p>
    </div>

    <div class="grid gap-6 lg:grid-cols-[320px_minmax(0,1fr)]">
      <!-- 账号信息 -->
      <Card class="h-fit">
        <CardHeader>
          <CardTitle class="text-base">{{ t('profile') }}</CardTitle>
        </CardHeader>
        <CardContent class="space-y-4">
          <div v-if="store.user" class="flex items-center gap-3">
            <Avatar class="size-11">
              <AvatarFallback class="bg-primary/10 text-sm font-semibold text-primary">
                {{ initials(store.user.username) }}
              </AvatarFallback>
            </Avatar>
            <div class="min-w-0">
              <p class="truncate font-medium leading-tight">{{ store.user.username }}</p>
              <p class="truncate text-xs text-muted-foreground leading-tight">{{ store.user.email }}</p>
            </div>
          </div>
          <Separator />
          <dl class="space-y-2.5 text-sm">
            <div class="flex items-center justify-between gap-2">
              <dt class="inline-flex items-center gap-1.5 text-muted-foreground">
                <ShieldCheck class="size-3.5" />{{ t('role') }}
              </dt>
              <dd>
                <Badge variant="secondary" class="font-medium">{{ roleLabel }}</Badge>
              </dd>
            </div>
            <div class="flex items-center justify-between gap-2">
              <dt class="inline-flex items-center gap-1.5 text-muted-foreground">
                <Mail class="size-3.5" />{{ t('email') }}
              </dt>
              <dd class="max-w-40 truncate text-right">{{ store.user?.email || '—' }}</dd>
            </div>
            <div class="flex items-center justify-between gap-2">
              <dt class="inline-flex items-center gap-1.5 text-muted-foreground">
                <Plane class="size-3.5" />Telegram
              </dt>
              <dd>
                <Badge :variant="bound ? 'default' : 'outline'" class="font-medium">
                  {{ bound ? t('tgBound') : t('tgUnbound') }}
                </Badge>
              </dd>
            </div>
          </dl>
        </CardContent>
      </Card>

      <!-- Telegram 绑定 -->
      <Card class="h-fit">
        <CardHeader>
          <div class="flex items-center gap-2">
            <div class="flex size-8 items-center justify-center rounded-lg bg-info/10 text-info">
              <Plane class="size-4" />
            </div>
            <div class="flex-1">
              <CardTitle class="text-base">{{ t('tgTitle') }}</CardTitle>
              <CardDescription>{{ t('tgDesc') }}</CardDescription>
            </div>
            <Badge :variant="bound ? 'default' : 'secondary'" class="shrink-0">
              {{ bound ? t('tgBound') : t('tgUnbound') }}
            </Badge>
          </div>
        </CardHeader>
        <CardContent class="space-y-4">
          <!-- 加载 -->
          <div v-if="loading" class="space-y-3">
            <Skeleton class="h-4 w-1/2" />
            <Skeleton class="h-9 w-32" />
          </div>

          <template v-else>
            <Alert v-if="!botReady">
              <Info />
              <AlertTitle>{{ t('tgUnavailable') }}</AlertTitle>
              <AlertDescription>{{ t('tgNotConfigured') }}</AlertDescription>
            </Alert>

            <!-- 未绑定：生成绑定链接 -->
            <template v-else-if="!bound && !bindLink">
              <p class="text-sm text-muted-foreground">{{ t('tgBindHint') }}</p>
              <Button :disabled="busy" @click="createBind">
                <Loader2 v-if="busy" class="size-4 animate-spin" />
                <Plane v-else class="size-4" />
                {{ t('tgBind') }}
              </Button>
            </template>

            <!-- 已生成链接 -->
            <template v-else-if="bindLink">
              <div class="rounded-lg border bg-muted/40 p-4">
                <p class="text-sm">
                  <span class="text-muted-foreground">{{ t('bindCodeLabel') }}:</span>
                  <code class="ml-1 rounded bg-background px-1.5 py-0.5 font-mono text-sm">{{ bindCode }}</code>
                </p>
                <div class="mt-3 flex flex-wrap gap-2">
                  <Button as="a" :href="bindLink" target="_blank" rel="noopener" size="sm">
                    <ExternalLink class="size-4" />
                    {{ t('tgOpenLink') }}
                  </Button>
                  <Button variant="outline" size="sm" @click="copyLink">
                    <Check v-if="copied" class="size-4" />
                    <Copy v-else class="size-4" />
                    {{ copied ? t('linkCopied') : t('tgCopyLink') }}
                  </Button>
                </div>
              </div>

              <ol class="space-y-2 text-sm text-muted-foreground">
                <li class="font-medium text-foreground">{{ t('steps') }}</li>
                <li>{{ t('step1') }}</li>
                <li>{{ t('step2') }}</li>
                <li>{{ t('step3') }}</li>
              </ol>

              <Button variant="ghost" size="sm" @click="bindLink = ''">
                <ArrowLeft class="size-4" />
                {{ t('collapse') }}
              </Button>
            </template>

            <!-- 已绑定 -->
            <template v-else>
              <p class="text-sm text-muted-foreground">
                {{ botUsername ? `@${botUsername}` : t('tgDesc') }}
              </p>
              <Button variant="outline" size="sm" class="text-destructive hover:text-destructive" :disabled="busy" @click="unbind">
                <Unlink class="size-4" />
                {{ t('tgUnbind') }}
              </Button>
            </template>

            <p v-if="err" class="text-sm text-destructive">{{ err }}</p>
          </template>
        </CardContent>
      </Card>
    </div>
  </div>
</template>
