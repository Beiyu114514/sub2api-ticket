<script setup>
import { ref, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import { api } from '../api'
import { store } from '../store'
import { t } from '../i18n'

const router = useRouter()
const botToken = ref('')
const adminGroup = ref('')
const notifyUser = ref(true)
const notifyAdmin = ref(true)
const loading = ref(true)
const busy = ref(false)
const msg = ref('')
const err = ref('')
const status = ref(null)

async function load() {
  try {
    const res = await api.get('/api/v1/admin/settings')
    botToken.value = res.tg_bot_token
    adminGroup.value = res.tg_admin_group_id
    notifyUser.value = res.notify_user_enabled
    notifyAdmin.value = res.notify_admin_enabled
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
    await api.put('/api/v1/admin/settings', {
      tg_bot_token: botToken.value,
      tg_admin_group_id: adminGroup.value,
      notify_user_enabled: notifyUser.value,
      notify_admin_enabled: notifyAdmin.value,
    })
    msg.value = t('saved')
    await refreshStatus()
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
          <input v-model="botToken" type="text" class="input font-mono text-xs" spellcheck="false" />
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
