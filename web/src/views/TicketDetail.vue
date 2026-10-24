<script setup>
import { ref, computed, onMounted, onUnmounted, nextTick } from 'vue'
import { useRoute } from 'vue-router'
import { api } from '../api'
import { store } from '../store'
import { t } from '../i18n'
import ImageUploader from '../components/ImageUploader.vue'

const route = useRoute()
const ticket = ref(null)
const messages = ref([])
const loading = ref(true)
const err = ref('')
const reply = ref('')
const pendingAtts = ref([])
const sending = ref(false)
const actionBusy = ref(false)
const streamEl = ref(null)
const lightbox = ref(null) // 当前放大的附件 URL
let timer = null

const canReply = computed(() => ticket.value && ticket.value.status !== 'closed')

async function load(silent = false) {
  if (!silent) loading.value = true
  try {
    const res = await api.get(`/api/v1/tickets/${route.params.id}`)
    ticket.value = res.ticket
    messages.value = res.messages
    err.value = ''
    await nextTick()
    scrollBottom()
  } catch (e) {
    err.value = e.message
  } finally {
    loading.value = false
  }
}

function scrollBottom() {
  if (streamEl.value) streamEl.value.scrollTop = streamEl.value.scrollHeight
}

async function send() {
  const text = reply.value.trim()
  const atts = pendingAtts.value.map(a => a.url)
  if ((!text && !atts.length) || sending.value) return
  sending.value = true
  try {
    await api.post(`/api/v1/tickets/${route.params.id}/messages`, {
      content: text, attachments: atts,
    })
    reply.value = ''
    pendingAtts.value = []
    await load(true)
  } catch (e) {
    alert(e.message)
  } finally {
    sending.value = false
  }
}

function msgAtts(m) {
  try { return JSON.parse(m.attachments || '[]') } catch { return [] }
}

async function setStatus(status) {
  actionBusy.value = true
  try {
    await api.post(`/api/v1/tickets/${route.params.id}/status`, { status })
    await load(true)
  } catch (e) {
    alert(e.message)
  } finally {
    actionBusy.value = false
  }
}

function side(msg) {
  // 气泡方向：自己的消息在右侧
  if (msg.sender_role === 'system') return 'center'
  if (msg.sender_role === 'admin') return store.isAdmin ? 'right' : 'left'
  return store.isAdmin ? 'left' : 'right'
}

function roleLabel(msg) {
  if (msg.sender_role === 'system') return t('system')
  if (msg.sender_role === 'admin') return t('admin')
  return store.isAdmin ? t('user') : t('user')
}

function fmtTime(iso) {
  return new Date(iso).toLocaleString([], {
    year: 'numeric', month: '2-digit', day: '2-digit',
    hour: '2-digit', minute: '2-digit',
  })
}

const badgeClass = {
  open: 'bg-emerald-100 text-emerald-700 dark:bg-emerald-900/50 dark:text-emerald-300',
  pending: 'bg-amber-100 text-amber-700 dark:bg-amber-900/50 dark:text-amber-300',
  closed: 'bg-slate-200 text-slate-500 dark:bg-slate-800 dark:text-slate-400',
}

function startPolling() {
  stopPolling()
  timer = setInterval(() => {
    if (!document.hidden) {
      load(true)
    }
  }, 10000)
}

function stopPolling() {
  if (timer) {
    clearInterval(timer)
    timer = null
  }
}

function handleVisibilityChange() {
  if (document.hidden) {
    stopPolling()
  } else {
    load(true)
    startPolling()
  }
}

onMounted(() => {
  load()
  startPolling()
  document.addEventListener('visibilitychange', handleVisibilityChange)
})
onUnmounted(() => {
  stopPolling()
  document.removeEventListener('visibilitychange', handleVisibilityChange)
})
</script>

<template>
  <div>
    <div v-if="loading" class="py-20 text-center text-slate-400 animate-pulse">…</div>
    <div v-else-if="err" class="card p-8 text-center">
      <p class="text-rose-500 mb-4">{{ err }}</p>
      <RouterLink to="/" class="btn-ghost">{{ t('backToList') }}</RouterLink>
    </div>

    <template v-else-if="ticket">
      <!-- 头部信息 -->
      <div class="card p-4 mb-4">
        <div class="flex flex-wrap items-center gap-2">
          <span class="text-slate-400 font-mono text-sm">#{{ ticket.id }}</span>
          <h1 class="font-semibold text-lg flex-1 min-w-40">{{ ticket.subject }}</h1>
          <span class="px-2 py-0.5 rounded-full text-xs font-medium" :class="badgeClass[ticket.status]">
            {{ t('status_' + ticket.status) }}
          </span>
        </div>
        <div class="text-xs text-slate-500 dark:text-slate-400 mt-2 flex flex-wrap gap-x-4 gap-y-1">
          <span v-if="store.isAdmin && ticket.user">{{ t('userCol') }}: {{ ticket.user.username }} ({{ ticket.user.email }})</span>
          <span v-if="ticket.category">{{ t('category') }}: {{ ticket.category }}</span>
          <span>{{ t('created') }} {{ fmtTime(ticket.created_at) }}</span>
          <span>{{ messages.length }} {{ t('replies') }}</span>
        </div>
        <!-- 管理员状态操作 -->
        <div v-if="store.isAdmin" class="flex flex-wrap gap-2 mt-3 pt-3 border-t border-slate-100 dark:border-slate-800">
          <button v-if="ticket.status !== 'closed'" class="btn-danger !py-1.5 text-xs" :disabled="actionBusy" @click="setStatus('closed')">
            {{ t('closeTicket') }}
          </button>
          <button v-if="ticket.status === 'closed'" class="btn-primary !py-1.5 text-xs" :disabled="actionBusy" @click="setStatus('open')">
            {{ t('reopenTicket') }}
          </button>
          <button v-if="ticket.status === 'open'" class="btn-ghost !py-1.5 text-xs border border-slate-200 dark:border-slate-700" :disabled="actionBusy" @click="setStatus('pending')">
            {{ t('markPending') }}
          </button>
        </div>
      </div>

      <!-- 消息流 -->
      <div ref="streamEl" class="space-y-3 mb-4 max-h-[55vh] overflow-y-auto px-1 py-2">
        <div v-for="m in messages" :key="m.id">
          <!-- 系统消息居中 -->
          <p v-if="side(m) === 'center'" class="text-center text-xs text-slate-400 dark:text-slate-500 py-1">
            {{ m.content }}
          </p>
          <div v-else class="flex" :class="side(m) === 'right' ? 'justify-end' : 'justify-start'">
            <div
              class="max-w-[85%] rounded-2xl px-4 py-2.5"
              :class="side(m) === 'right'
                ? 'bg-blue-600 text-white rounded-br-sm'
                : 'bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-bl-sm'"
            >
              <div class="flex items-center gap-2 text-xs mb-1"
                :class="side(m) === 'right' ? 'text-blue-100' : 'text-slate-400'">
                <span class="font-medium">{{ m.sender_name }}</span>
                <span>{{ roleLabel(m) }}</span>
                <span v-if="m.via === 'telegram'" class="px-1 rounded bg-sky-500/20 text-sky-600 dark:text-sky-300 text-[10px]">TG</span>
                <span>{{ fmtTime(m.created_at) }}</span>
              </div>
              <p class="whitespace-pre-wrap break-words text-sm leading-relaxed">{{ m.content }}</p>
              <div v-if="msgAtts(m).length" class="grid gap-1 mt-1"
                :class="msgAtts(m).length > 1 ? 'grid-cols-3' : 'grid-cols-1'">
                <img v-for="a in msgAtts(m)" :key="a.url" :src="a.url"
                  class="rounded-lg cursor-zoom-in object-cover w-full max-h-48"
                  :class="msgAtts(m).length > 1 ? 'h-24' : 'max-h-72'"
                  alt="" @click="lightbox = a.url" />
              </div>
            </div>
          </div>
        </div>
      </div>

      <!-- 回复框 / 关闭态 -->
      <div class="card p-4">
        <template v-if="canReply">
          <textarea
            v-model="reply" rows="3" :placeholder="t('replyPh')"
            class="input resize-y" @keydown.ctrl.enter="send" @keydown.meta.enter="send"
          ></textarea>
          <div class="mt-2">
            <ImageUploader v-model="pendingAtts" :max="5" :disabled="sending" />
          </div>
          <div class="flex items-center justify-between mt-3">
            <span class="text-xs text-slate-400 hidden sm:inline">Ctrl/⌘ + Enter</span>
            <button class="btn-primary" :disabled="sending || (!reply.trim() && !pendingAtts.length)" @click="send">
              {{ sending ? t('sending') : t('send') }}
            </button>
          </div>
        </template>
        <template v-else>
          <div class="text-center py-2">
            <p class="text-sm text-slate-500 dark:text-slate-400 mb-3">🔒 {{ t('closed') }} · {{ t('closedHint') }}</p>
            <button class="btn-primary !py-1.5 text-sm" :disabled="actionBusy" @click="setStatus('open')">
              {{ t('reopenTicket') }}
            </button>
          </div>
        </template>
      </div>
    </template>

    <!-- 图片灯箱 -->
    <div v-if="lightbox" class="fixed inset-0 z-50 bg-black/80 flex items-center justify-center p-6"
      @click="lightbox = null">
      <img :src="lightbox" class="max-w-full max-h-full rounded-lg" alt="" @click.stop />
      <button class="absolute top-4 right-6 text-white/80 text-3xl leading-none" @click="lightbox = null">×</button>
    </div>
  </div>
</template>
