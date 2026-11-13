<script setup>
import { ref, onMounted, computed } from 'vue'
import { useRouter } from 'vue-router'
import { api } from '@/api'
import { store } from '@/store'
import { t } from '@/i18n'
import { toast } from 'vue-sonner'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Textarea } from '@/components/ui/textarea'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import { Separator } from '@/components/ui/separator'
import { Skeleton } from '@/components/ui/skeleton'
import {
  Card, CardContent, CardDescription, CardHeader, CardTitle,
} from '@/components/ui/card'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import {
  Alert, AlertDescription, AlertTitle,
} from '@/components/ui/alert'
import { cn } from '@/lib/utils'
import {
  Send, Loader2, Bot, HardDrive, Sparkles, Activity, RefreshCw, Webhook,
  CircleCheck, CircleX, Info, Save, Server, KeyRound,
} from '@lucide/vue'

const router = useRouter()

// —— Telegram ——
const botToken = ref('')
const botTokenMasked = ref('')
const hasBotToken = ref(false)
const adminGroup = ref('')
const notifyUser = ref(true)
const notifyAdmin = ref(true)

// —— 存储 ——
const storageMode = ref('local')
const storageLocalDir = ref('')
const storageTTL = ref(168)
const s3 = ref({
  endpoint: '', region: '', bucket: '', access_key: '', secret_key: '',
  use_path_style: false, public_base_url: '',
})
const s3HasSecret = ref(false)

// —— AI 客服 ——
const aiEnabled = ref(false)
const aiBaseURL = ref('')
const aiAPIKey = ref('')
const aiHasKey = ref(false)
const aiModel = ref('')
const aiTemp = ref(0.3)
const aiSilent = ref(5)
const aiMaxReplies = ref(5)
const aiPrompt = ref('')

const loading = ref(true)
const busy = ref(false)
const err = ref('')
const status = ref(null)
const refreshing = ref(false)
const tab = ref('telegram')

const botHealthy = computed(() => !!status.value?.bot_ok)

/** 数字输入兜底：0 是合法值，不能被 || 吞掉 */
function numOr(value, fallback) {
  if (value === '' || value === null || value === undefined) return fallback
  const n = Number(value)
  return Number.isFinite(n) ? n : fallback
}

async function load() {
  try {
    const res = await api.get('/api/v1/admin/settings')
    // 服务端只回显打码后的 Token：输入框留空表示“不修改”
    botToken.value = ''
    botTokenMasked.value = res.tg_bot_token_masked || ''
    hasBotToken.value = !!res.has_bot_token
    adminGroup.value = res.tg_admin_group_id
    notifyUser.value = res.notify_user_enabled
    notifyAdmin.value = res.notify_admin_enabled

    storageMode.value = res.storage_mode || 'local'
    storageLocalDir.value = res.storage_local_dir || ''
    storageTTL.value = res.storage_url_ttl_hrs || 168
    if (res.storage_s3) {
      const c = res.storage_s3
      s3.value = {
        endpoint: c.endpoint || '', region: c.region || '', bucket: c.bucket || '',
        access_key: c.access_key || '', secret_key: '',
        use_path_style: !!c.use_path_style, public_base_url: c.public_base_url || '',
      }
      s3HasSecret.value = !!c.has_secret
    }

    aiEnabled.value = !!res.assistant_enabled
    aiBaseURL.value = res.assistant_base_url || ''
    aiHasKey.value = !!res.assistant_has_api_key
    aiModel.value = res.assistant_model || ''
    aiTemp.value = res.assistant_temperature ?? 0.3
    aiSilent.value = res.assistant_silent_minutes ?? 5
    aiMaxReplies.value = res.assistant_max_replies ?? 5
    aiPrompt.value = res.assistant_system_prompt || ''

    await refreshStatus()
  } catch (e) {
    err.value = e.message
  } finally {
    loading.value = false
  }
}

async function refreshStatus() {
  refreshing.value = true
  try {
    status.value = await api.get('/api/v1/admin/telegram/status')
  } catch {
    status.value = null
  } finally {
    refreshing.value = false
  }
}

async function save() {
  busy.value = true
  err.value = ''
  try {
    const payload = {
      tg_admin_group_id: adminGroup.value,
      notify_user_enabled: notifyUser.value,
      notify_admin_enabled: notifyAdmin.value,
    }
    // 只有真的输入了新 Token 才提交，避免把打码值写回数据库
    if (botToken.value.trim() !== '') payload.tg_bot_token = botToken.value.trim()

    payload.storage_mode = storageMode.value
    payload.storage_local_dir = storageLocalDir.value
    payload.storage_url_ttl_hrs = numOr(storageTTL.value, 168)
    const s3c = { ...s3.value }
    // secret 留空 = 保持原值，不提交该字段
    if (String(s3c.secret_key || '').trim() === '') delete s3c.secret_key
    payload.storage_s3 = s3c

    payload.assistant_enabled = aiEnabled.value
    payload.assistant_base_url = aiBaseURL.value.trim()
    payload.assistant_model = aiModel.value.trim()
    payload.assistant_temperature = numOr(aiTemp.value, 0.3)
    payload.assistant_silent_minutes = numOr(aiSilent.value, 5)
    payload.assistant_max_replies = numOr(aiMaxReplies.value, 5)
    payload.assistant_system_prompt = aiPrompt.value
    if (aiAPIKey.value.trim() !== '') payload.assistant_api_key = aiAPIKey.value.trim()
    aiAPIKey.value = ''

    await api.put('/api/v1/admin/settings', payload)
    botToken.value = ''
    toast.success(t('saved'))
    await load()
  } catch (e) {
    err.value = e.message
    toast.error(e.message)
  } finally {
    busy.value = false
  }
}

async function setupWebhook() {
  busy.value = true
  try {
    await api.post('/api/v1/admin/telegram/setup-webhook')
    toast.success(t('webhookOn'))
    await refreshStatus()
  } catch (e) {
    toast.error(e.message)
  } finally {
    busy.value = false
  }
}

async function stopWebhook() {
  busy.value = true
  try {
    await api.post('/api/v1/admin/telegram/stop-webhook')
    toast.success(t('webhookOff'))
    await refreshStatus()
  } catch (e) {
    toast.error(e.message)
  } finally {
    busy.value = false
  }
}

async function testSend() {
  busy.value = true
  try {
    await api.post('/api/v1/admin/telegram/test')
    toast.success(t('testSent'))
  } catch (e) {
    toast.error(e.message)
  } finally {
    busy.value = false
  }
}

onMounted(() => {
  if (!store.isAdmin) {
    router.replace('/')
    return
  }
  load()
})
</script>

<template>
  <div class="space-y-6">
    <div class="flex flex-wrap items-start justify-between gap-3">
      <div class="space-y-1">
        <h1 class="text-2xl font-semibold tracking-tight">{{ t('adminTitle') }}</h1>
        <p class="text-sm text-muted-foreground">{{ t('adminDesc') }}</p>
      </div>
      <Button :disabled="busy || loading" @click="save">
        <Loader2 v-if="busy" class="size-4 animate-spin" />
        <Save v-else class="size-4" />
        {{ busy ? t('saving') : t('save') }}
      </Button>
    </div>

    <Alert v-if="err" variant="destructive">
      <CircleX />
      <AlertTitle>{{ t('saved_error') }}</AlertTitle>
      <AlertDescription>{{ err }}</AlertDescription>
    </Alert>

    <div v-if="loading" class="space-y-4">
      <Skeleton class="h-9 w-72" />
      <Card><CardContent class="space-y-3 py-6"><Skeleton class="h-4 w-1/3" /><Skeleton class="h-9 w-full" /><Skeleton class="h-9 w-full" /></CardContent></Card>
    </div>

    <Tabs v-else v-model="tab" class="gap-4">
      <TabsList class="w-full max-w-md">
        <TabsTrigger value="telegram">
          <Send class="size-4" />
          <span class="hidden sm:inline">{{ t('tabTelegram') }}</span>
        </TabsTrigger>
        <TabsTrigger value="storage">
          <HardDrive class="size-4" />
          <span class="hidden sm:inline">{{ t('tabStorage') }}</span>
        </TabsTrigger>
        <TabsTrigger value="ai">
          <Sparkles class="size-4" />
          <span class="hidden sm:inline">{{ t('tabAI') }}</span>
        </TabsTrigger>
        <TabsTrigger value="status">
          <Activity class="size-4" />
          <span class="hidden sm:inline">{{ t('tabStatus') }}</span>
        </TabsTrigger>
      </TabsList>

      <!-- Telegram -->
      <TabsContent value="telegram" class="space-y-4">
        <Card>
          <CardHeader>
            <CardTitle class="text-base">{{ t('tabTelegram') }}</CardTitle>
            <CardDescription>{{ t('tgDesc') }}</CardDescription>
          </CardHeader>
          <CardContent class="space-y-5">
            <div class="space-y-2">
              <Label for="botToken" class="flex items-center gap-1.5">
                <KeyRound class="size-3.5 text-muted-foreground" />{{ t('botToken') }}
              </Label>
              <Input
                id="botToken" v-model="botToken" spellcheck="false"
                class="font-mono text-xs"
                :placeholder="hasBotToken ? botTokenMasked || t('botTokenConfigured') : '123456789:AAxxxxxxxxxxxxxxxxxxxx'"
              />
              <p class="text-xs text-muted-foreground">{{ t('botTokenHint') }}</p>
            </div>

            <div class="space-y-2">
              <Label for="adminGroup">{{ t('adminGroup') }}</Label>
              <Input id="adminGroup" v-model="adminGroup" class="font-mono text-sm" placeholder="-1001234567890" />
              <p class="text-xs text-muted-foreground">{{ t('adminGroupHint') }}</p>
            </div>

            <Separator />

            <div class="space-y-3">
              <div class="flex items-start justify-between gap-4">
                <div class="space-y-0.5">
                  <Label for="notifyAdmin">{{ t('notifyAdmin') }}</Label>
                  <p class="text-xs text-muted-foreground">{{ t('notifyAdminHint') }}</p>
                </div>
                <Switch id="notifyAdmin" v-model="notifyAdmin" />
              </div>
              <div class="flex items-start justify-between gap-4">
                <div class="space-y-0.5">
                  <Label for="notifyUser">{{ t('notifyUser') }}</Label>
                  <p class="text-xs text-muted-foreground">{{ t('notifyUserHint') }}</p>
                </div>
                <Switch id="notifyUser" v-model="notifyUser" />
              </div>
            </div>
          </CardContent>
        </Card>
      </TabsContent>

      <!-- 存储 -->
      <TabsContent value="storage" class="space-y-4">
        <Card>
          <CardHeader>
            <CardTitle class="text-base">{{ t('storageMode') }}</CardTitle>
            <CardDescription>{{ t('storageFallback') }}</CardDescription>
          </CardHeader>
          <CardContent class="space-y-5">
            <div class="grid gap-2 sm:grid-cols-2">
              <button
                v-for="mode in [{ key: 'local', label: t('storageLocal'), icon: HardDrive }, { key: 's3', label: t('storageS3'), icon: Server }]"
                :key="mode.key"
                type="button"
                :class="cn(
                  'flex items-center gap-2 rounded-lg border p-3 text-left text-sm transition-all',
                  storageMode === mode.key
                    ? 'border-primary bg-primary/5 font-medium ring-1 ring-primary/20'
                    : 'hover:border-foreground/20 hover:bg-accent/50',
                )"
                @click="storageMode = mode.key"
              >
                <component :is="mode.icon" class="size-4 text-muted-foreground" />
                {{ mode.label }}
              </button>
            </div>

            <div v-if="storageMode === 'local'" class="space-y-2">
              <Label for="localDir">{{ t('localDir') }}</Label>
              <Input id="localDir" v-model="storageLocalDir" class="font-mono text-xs" placeholder="data/uploads" />
            </div>

            <template v-else>
              <div class="grid gap-4 sm:grid-cols-2">
                <div class="space-y-2 sm:col-span-2">
                  <Label for="s3Endpoint">Endpoint</Label>
                  <Input id="s3Endpoint" v-model="s3.endpoint" class="font-mono text-xs"
                    placeholder="https://s3.amazonaws.com 或 MinIO / R2 地址" />
                </div>
                <div class="space-y-2">
                  <Label for="s3Region">Region</Label>
                  <Input id="s3Region" v-model="s3.region" class="font-mono text-xs" placeholder="us-east-1" />
                </div>
                <div class="space-y-2">
                  <Label for="s3Bucket">Bucket</Label>
                  <Input id="s3Bucket" v-model="s3.bucket" class="font-mono text-xs" />
                </div>
                <div class="space-y-2">
                  <Label for="s3Key">Access Key</Label>
                  <Input id="s3Key" v-model="s3.access_key" class="font-mono text-xs" />
                </div>
                <div class="space-y-2">
                  <Label for="s3Secret">Secret Key</Label>
                  <Input id="s3Secret" v-model="s3.secret_key" type="password" class="font-mono text-xs"
                    :placeholder="s3HasSecret ? t('secretConfigured') : ''" />
                </div>
                <div class="space-y-2 sm:col-span-2">
                  <Label for="s3Public">{{ t('publicBaseURL') }}</Label>
                  <Input id="s3Public" v-model="s3.public_base_url" class="font-mono text-xs"
                    placeholder="https://cdn.example.com/bucket" />
                  <p class="text-xs text-muted-foreground">{{ t('publicBaseURLHint') }}</p>
                </div>
              </div>
              <div class="flex items-center justify-between gap-4 rounded-lg border p-3">
                <Label for="pathStyle" class="text-sm font-normal">{{ t('pathStyle') }}</Label>
                <Switch id="pathStyle" v-model="s3.use_path_style" />
              </div>
            </template>

            <Separator />

            <div class="space-y-2">
              <Label for="ttl">{{ t('urlTTL') }}</Label>
              <Input id="ttl" v-model.number="storageTTL" type="number" min="1" max="720" class="w-32" />
            </div>
          </CardContent>
        </Card>
      </TabsContent>

      <!-- AI 客服 -->
      <TabsContent value="ai" class="space-y-4">
        <Card>
          <CardHeader>
            <div class="flex items-start justify-between gap-4">
              <div class="space-y-1">
                <CardTitle class="text-base">{{ t('tabAI') }}</CardTitle>
                <CardDescription>{{ t('aiEnableHint') }}</CardDescription>
              </div>
              <Switch v-model="aiEnabled" />
            </div>
          </CardHeader>
          <CardContent :class="cn('space-y-5 transition-opacity', !aiEnabled && 'pointer-events-none opacity-50')">
            <div class="grid gap-4 sm:grid-cols-2">
              <div class="space-y-2">
                <Label for="aiBaseURL">{{ t('aiBaseURL') }}</Label>
                <Input id="aiBaseURL" v-model="aiBaseURL" class="font-mono text-xs" placeholder="https://api.openai.com/v1" />
              </div>
              <div class="space-y-2">
                <Label for="aiModel">{{ t('aiModel') }}</Label>
                <Input id="aiModel" v-model="aiModel" class="font-mono text-xs" placeholder="gpt-4o-mini" />
              </div>
              <div class="space-y-2 sm:col-span-2">
                <Label for="aiKey">{{ t('aiKey') }}</Label>
                <Input id="aiKey" v-model="aiAPIKey" type="password" class="font-mono text-xs"
                  :placeholder="aiHasKey ? t('secretConfigured') : 'sk-...'" />
              </div>
            </div>

            <Separator />

            <div class="grid gap-4 sm:grid-cols-3">
              <div class="space-y-2">
                <Label for="aiTemp">{{ t('aiTemp') }}</Label>
                <Input id="aiTemp" v-model.number="aiTemp" type="number" min="0" max="2" step="0.1" />
              </div>
              <div class="space-y-2">
                <Label for="aiSilent">{{ t('aiSilent') }}</Label>
                <Input id="aiSilent" v-model.number="aiSilent" type="number" min="1" />
              </div>
              <div class="space-y-2">
                <Label for="aiMax">{{ t('aiMaxReplies') }}</Label>
                <Input id="aiMax" v-model.number="aiMaxReplies" type="number" min="1" />
              </div>
            </div>

            <div class="space-y-2">
              <Label for="aiPrompt">{{ t('aiPrompt') }}</Label>
              <Textarea id="aiPrompt" v-model="aiPrompt" rows="5" class="min-h-28 resize-y font-mono text-xs" />
            </div>
          </CardContent>
        </Card>
      </TabsContent>

      <!-- 运行状态 -->
      <TabsContent value="status" class="space-y-4">
        <Card>
          <CardHeader>
            <div class="flex items-center justify-between gap-2">
              <div>
                <CardTitle class="text-base">{{ t('tgStatus') }}</CardTitle>
                <CardDescription>{{ t('tgStatusDesc') }}</CardDescription>
              </div>
              <Button variant="outline" size="sm" :disabled="refreshing" @click="refreshStatus">
                <RefreshCw :class="cn('size-4', refreshing && 'animate-spin')" />
                {{ t('refresh') }}
              </Button>
            </div>
          </CardHeader>
          <CardContent class="space-y-4">
            <div v-if="!status?.configured" class="rounded-lg border border-dashed p-6 text-center text-sm text-muted-foreground">
              {{ t('tgStatusUnset') }}
            </div>

            <template v-else>
              <div class="grid gap-3 sm:grid-cols-2">
                <div class="rounded-lg border p-3">
                  <p class="text-xs text-muted-foreground">{{ t('botAccount') }}</p>
                  <p class="mt-1 flex items-center gap-1.5 text-sm font-medium">
                    <component :is="botHealthy ? CircleCheck : CircleX" :class="cn('size-4', botHealthy ? 'text-success' : 'text-destructive')" />
                    <span v-if="botHealthy">@{{ status.bot_username }}</span>
                    <span v-else>{{ t('tgStatusBad') }}</span>
                  </p>
                  <p v-if="botHealthy" class="mt-0.5 font-mono text-xs text-muted-foreground">id: {{ status.bot_id }}</p>
                  <p v-else-if="status.error" class="mt-0.5 text-xs text-destructive">{{ status.error }}</p>
                </div>

                <div class="rounded-lg border p-3">
                  <p class="text-xs text-muted-foreground">{{ t('webhook') }}</p>
                  <p class="mt-1 flex items-center gap-1.5 text-sm font-medium">
                    <Webhook class="size-4 text-muted-foreground" />
                    {{ status.webhook_url ? t('webhookOn') : t('webhookOff') }}
                  </p>
                  <p v-if="status.webhook_url" class="mt-0.5 truncate font-mono text-xs text-muted-foreground" :title="status.webhook_url">
                    {{ status.webhook_url }}
                  </p>
                </div>
              </div>

              <Alert v-if="status.last_error">
                <Info />
                <AlertTitle>{{ t('tgStatus') }}</AlertTitle>
                <AlertDescription>{{ status.last_error }}</AlertDescription>
              </Alert>

              <p v-if="!status.webhook_url && !status.bot_ok" class="text-xs text-muted-foreground">
                {{ t('needPublicBaseURL') }}
              </p>

              <div class="flex flex-wrap gap-2">
                <Button size="sm" :disabled="busy" @click="setupWebhook">
                  <Webhook class="size-4" />
                  {{ t('setupWebhook') }}
                </Button>
                <Button variant="outline" size="sm" :disabled="busy" @click="stopWebhook">
                  {{ t('stopWebhook') }}
                </Button>
                <Button variant="outline" size="sm" :disabled="busy" @click="testSend">
                  <Send class="size-4" />
                  {{ t('testSend') }}
                </Button>
              </div>
            </template>
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle class="flex items-center gap-2 text-base">
              <Info class="size-4 text-muted-foreground" />
              {{ t('deployTips') }}
            </CardTitle>
          </CardHeader>
          <CardContent>
            <ul class="space-y-2 text-sm text-muted-foreground">
              <li class="flex gap-2"><span class="mt-1.5 size-1.5 shrink-0 rounded-full bg-muted-foreground/40" />{{ t('tipGroup') }}</li>
              <li class="flex gap-2"><span class="mt-1.5 size-1.5 shrink-0 rounded-full bg-muted-foreground/40" />{{ t('tipPublicURL') }}</li>
              <li class="flex gap-2"><span class="mt-1.5 size-1.5 shrink-0 rounded-full bg-muted-foreground/40" />{{ t('tipIdCmd') }}</li>
            </ul>
          </CardContent>
        </Card>
      </TabsContent>
    </Tabs>
  </div>
</template>
