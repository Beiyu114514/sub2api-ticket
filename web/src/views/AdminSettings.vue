<script setup>
import { ref, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import { api } from '../api'
import { store } from '../store'
import { t } from '../i18n'

const router = useRouter()
const botToken = ref('')
const botTokenMasked = ref('')
const hasBotToken = ref(false)
const adminGroup = ref('')
const notifyUser = ref(true)
const notifyAdmin = ref(true)
// 存储配置
const storageMode = ref('local')
const storageLocalDir = ref('')
const storageTTL = ref(168)
const s3 = ref({ endpoint: '', region: '', bucket: '', access_key: '', secret_key: '', use_path_style: false, public_base_url: '' })
const s3HasSecret = ref(false)

// AI 客服
const aiEnabled = ref(false)
const aiBaseURL = ref("")
const aiAPIKey = ref("")
const aiHasKey = ref(false)
const aiModel = ref("")
const aiTemp = ref(0.3)
const aiSilent = ref(5)
const aiMaxReplies = ref(5)
const aiPrompt = ref("")
const loading = ref(true)
const busy = ref(false)
const msg = ref('')
const err = ref('')
const status = ref(null)

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
    aiEnabled.value = !!res.assistant_enabled
    aiBaseURL.value = res.assistant_base_url || ""
    aiHasKey.value = !!res.assistant_has_api_key
    aiModel.value = res.assistant_model || ""
    aiTemp.value = res.assistant_temperature ?? 0.3
    aiSilent.value = res.assistant_silent_minutes ?? 5
    aiMaxReplies.value = res.assistant_max_replies ?? 5
    aiPrompt.value = res.assistant_system_prompt || ""
    }
    await refreshStatus()
  } catch (e) {
    err.value = e.message
  } finally {
    loading.value = false
  }
}

async function refreshStatus() {
  try {
    status.value = await api.get('/api/v1/admin/telegram/status')
  } catch { status.value = null }
}

async function save() {
  busy.value = true
  msg.value = ''
  err.value = ''
  try {
    const payload = {
      tg_admin_group_id: adminGroup.value,
      notify_user_enabled: notifyUser.value,
      notify_admin_enabled: notifyAdmin.value,
    }
    // 只有用户真的输入了新 Token 才提交，避免把打码值写回数据库
    if (botToken.value.trim() !== '') payload.tg_bot_token = botToken.value.trim()
    // 存储配置
    payload.storage_mode = storageMode.value
    payload.storage_local_dir = storageLocalDir.value
    payload.storage_url_ttl_hrs = Number(storageTTL.value) || 168
    const s3c = { ...s3.value }
    // secret 留空 = 保持原值，不提交该字段
    if (s3c.secret_key.trim() === '') delete s3c.secret_key
    payload.storage_s3 = s3c
    // AI 客服
    payload.assistant_enabled = aiEnabled.value
    payload.assistant_base_url = aiBaseURL.value.trim()
    payload.assistant_model = aiModel.value.trim()
    payload.assistant_temperature = Number(aiTemp.value) || 0.3
    payload.assistant_silent_minutes = Number(aiSilent.value) || 5
    payload.assistant_max_replies = Number(aiMaxReplies.value) || 5
    payload.assistant_system_prompt = aiPrompt.value
    if (aiAPIKey.value.trim() !== "") payload.assistant_api_key = aiAPIKey.value.trim()
    aiAPIKey.value = ""
    await api.put('/api/v1/admin/settings', payload)
    botToken.value = ''
    msg.value = t('saved')
    await load()
  } catch (e) {
    err.value = e.message
  } finally {
    busy.value = false
  }
}

async function setupWebhook() {
  busy.value = true
  msg.value = ''
  err.value = ''
  try {
    await api.post('/api/v1/admin/telegram/setup-webhook')
    msg.value = t('webhookOn')
    await refreshStatus()
  } catch (e) {
    err.value = e.message
  } finally {
    busy.value = false
  }
}

async function stopWebhook() {
  busy.value = true
  try {
    await api.post('/api/v1/admin/telegram/stop-webhook')
    await refreshStatus()
  } catch (e) {
    err.value = e.message
  } finally {
    busy.value = false
  }
}

async function testSend() {
  busy.value = true
  msg.value = ''
  err.value = ''
  try {
    await api.post('/api/v1/admin/telegram/test')
    msg.value = '✅'
  } catch (e) {
    err.value = e.message
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
  <div class="max-w-2xl mx-auto space-y-6">
    <div class="card p-6">
      <h1 class="text-lg font-semibold mb-6">{{ t('adminTitle') }}</h1>
      <div v-if="loading" class="py-10 text-center text-slate-400 animate-pulse">…</div>
      <form v-else @submit.prevent="save" class="space-y-5">
        <div>
          <label class="label">{{ t('botToken') }}</label>
          <input v-model="botToken" type="text" class="input font-mono text-xs" spellcheck="false"
                 :placeholder="hasBotToken ? botTokenMasked + '（已配置，留空则不修改）' : '123456789:AAxxxxxxxxxxxxxxxxxxxx'" />
          <p class="text-xs text-slate-400 mt-1">{{ t('botTokenHint') }}</p>
        </div>
        <div>
          <label class="label">{{ t('adminGroup') }}</label>
          <input v-model="adminGroup" type="text" class="input font-mono" placeholder="-1001234567890" />
          <p class="text-xs text-slate-400 mt-1">{{ t('adminGroupHint') }}</p>
        </div>
        <label class="flex items-center gap-2 text-sm cursor-pointer">
          <input v-model="notifyAdmin" type="checkbox" class="w-4 h-4 accent-blue-600" />
          {{ t('notifyAdmin') }}
        </label>
        <label class="flex items-center gap-2 text-sm cursor-pointer">
          <input v-model="notifyUser" type="checkbox" class="w-4 h-4 accent-blue-600" />
          {{ t('notifyUser') }}
        </label>
        <p v-if="msg" class="text-sm text-emerald-600 dark:text-emerald-400">{{ msg }}</p>
        <p v-if="err" class="text-sm text-rose-600 dark:text-rose-400">{{ err }}</p>
        <button class="btn-primary" :disabled="busy">{{ t('save') }}</button>
      </form>
    </div>

    <!-- 存储配置 -->
    <div class="card p-6">
      <h2 class="font-medium mb-4">图片存储</h2>
      <div class="space-y-5">
        <div>
          <label class="label">存储模式</label>
          <div class="flex gap-2">
            <button type="button" class="btn flex-1 border"
              :class="storageMode === 'local' ? 'border-blue-500 bg-blue-50 dark:bg-blue-950 text-blue-700 dark:text-blue-300' : 'border-slate-200 dark:border-slate-700 text-slate-600 dark:text-slate-300'"
              @click="storageMode = 'local'">本地磁盘</button>
            <button type="button" class="btn flex-1 border"
              :class="storageMode === 's3' ? 'border-blue-500 bg-blue-50 dark:bg-blue-950 text-blue-700 dark:text-blue-300' : 'border-slate-200 dark:border-slate-700 text-slate-600 dark:text-slate-300'"
              @click="storageMode = 's3'">S3 兼容</button>
          </div>
          <p class="text-xs text-slate-400 mt-1">S3 配置不完整时自动回退到本地存储</p>
        </div>
        <div v-if="storageMode === 'local'">
          <label class="label">本地目录</label>
          <input v-model="storageLocalDir" class="input font-mono text-xs" placeholder="data/uploads" />
        </div>
        <div v-else class="space-y-4">
          <div class="grid grid-cols-1 sm:grid-cols-2 gap-4">
            <div>
              <label class="label">Endpoint</label>
              <input v-model="s3.endpoint" class="input font-mono text-xs" placeholder="https://s3.amazonaws.com 或 MinIO/R2 地址" />
            </div>
            <div>
              <label class="label">Region</label>
              <input v-model="s3.region" class="input font-mono text-xs" placeholder="us-east-1" />
            </div>
            <div>
              <label class="label">Bucket</label>
              <input v-model="s3.bucket" class="input font-mono text-xs" />
            </div>
            <div>
              <label class="label">Access Key</label>
              <input v-model="s3.access_key" class="input font-mono text-xs" />
            </div>
          </div>
          <div>
            <label class="label">Secret Key</label>
            <input v-model="s3.secret_key" type="password" class="input font-mono text-xs"
              :placeholder="s3HasSecret ? '已配置，留空则不修改' : ''" />
          </div>
          <div>
            <label class="label">公开访问 Base URL（可选）</label>
            <input v-model="s3.public_base_url" class="input font-mono text-xs"
              placeholder="https://cdn.example.com/bucket" />
            <p class="text-xs text-slate-400 mt-1">留空则使用签名 URL 访问</p>
          </div>
          <label class="flex items-center gap-2 text-sm cursor-pointer">
            <input v-model="s3.use_path_style" type="checkbox" class="w-4 h-4 accent-blue-600" />
            使用 Path Style（MinIO /自建 需要）
          </label>
        </div>
        <div>
          <label class="label">签名 URL 有效期（小时）</label>
          <input v-model.number="storageTTL" type="number" min="1" max="720" class="input" />
        </div>
      </div>
    </div>

    <!-- AI 客服 -->
    <div class="card p-6">
      <h2 class="font-medium mb-4">AI 临时客服</h2>
      <div class="space-y-5">
        <label class="flex items-center gap-2 text-sm cursor-pointer">
          <input v-model="aiEnabled" type="checkbox" class="w-4 h-4 accent-blue-600" />
          启用 LLM 自动回复（OpenAI 兼容 API）
        </label>
        <p class="text-xs text-slate-400">用户发消息静默数分钟后无人回复时，AI 自动应答；人工回复后自动让位，每单最多回复有限次。</p>
        <div class="grid grid-cols-1 sm:grid-cols-2 gap-4">
          <div>
            <label class="label">API Base URL</label>
            <input v-model="aiBaseURL" class="input font-mono text-xs" placeholder="https://api.openai.com/v1" />
          </div>
          <div>
            <label class="label">模型</label>
            <input v-model="aiModel" class="input font-mono text-xs" placeholder="gpt-4o-mini" />
          </div>
        </div>
        <div>
          <label class="label">API Key</label>
          <input v-model="aiAPIKey" type="password" class="input font-mono text-xs"
            :placeholder="aiHasKey ? '已配置，留空则不修改' : 'sk-...'" />
        </div>
        <div class="grid grid-cols-1 sm:grid-cols-3 gap-4">
          <div>
            <label class="label">温度 (0-2)</label>
            <input v-model.number="aiTemp" type="number" min="0" max="2" step="0.1" class="input" />
          </div>
          <div>
            <label class="label">静默触发（分钟）</label>
            <input v-model.number="aiSilent" type="number" min="1" class="input" />
          </div>
          <div>
            <label class="label">每单最多回复</label>
            <input v-model.number="aiMaxReplies" type="number" min="1" class="input" />
          </div>
        </div>
        <div>
          <label class="label">System Prompt</label>
          <textarea v-model="aiPrompt" rows="4" class="input font-mono text-xs"></textarea>
        </div>
      </div>
    </div>

    <!-- 连接状态与操作 -->
    <div class="card p-6">
      <h2 class="font-medium mb-4">{{ t('tgStatus') }}</h2>
      <div v-if="!status?.configured" class="text-sm text-slate-500 dark:text-slate-400">
        {{ t('tgStatusUnset') }}
      </div>
      <template v-else>
        <div class="space-y-2 text-sm">
          <p :class="status.bot_ok ? 'text-emerald-600 dark:text-emerald-400' : 'text-rose-500'">
            {{ status.bot_ok ? `✅ @${status.bot_username} (id: ${status.bot_id})` : `❌ ${t('tgStatusBad')}: ${status.error}` }}
          </p>
          <p v-if="status.last_error" class="text-xs text-amber-600 dark:text-amber-400">⚠️ {{ status.last_error }}</p>
          <p class="text-slate-500 dark:text-slate-400">
            {{ status.webhook_url ? `🔗 ${t('webhookOn')}: ${status.webhook_url}` : `🔌 ${t('webhookOff')}` }}
          </p>
          <p v-if="!status.webhook_url && !status.bot_ok" class="text-xs text-slate-400">{{ t('needPublicBaseURL') }}</p>
        </div>
        <div class="flex flex-wrap gap-2 mt-5">
          <button class="btn-primary !py-1.5 text-sm" :disabled="busy" @click="setupWebhook">{{ t('setupWebhook') }}</button>
          <button class="btn-ghost !py-1.5 text-sm border border-slate-200 dark:border-slate-700" :disabled="busy" @click="stopWebhook">{{ t('stopWebhook') }}</button>
          <button class="btn-ghost !py-1.5 text-sm border border-slate-200 dark:border-slate-700" :disabled="busy" @click="testSend">{{ t('testSend') }}</button>
        </div>
      </template>
    </div>

    <!-- 部署提示 -->
    <div class="card p-6 text-sm text-slate-500 dark:text-slate-400 space-y-2">
      <p class="font-medium text-slate-700 dark:text-slate-200">📌</p>
      <p>· 管理员群需为<b>超级群</b>并开启「话题 / Topics」功能，bot 加入群并设为管理员（需管理话题权限）。</p>
      <p>· 启用 Webhook 前需在服务端环境变量配置 PUBLIC_BASE_URL（HTTPS 地址）。</p>
      <p>· 在群里发送 <code class="font-mono">/id</code> 可获取群 chat_id。</p>
    </div>
  </div>
</template>
