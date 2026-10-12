<script setup>
import { ref, watch, onMounted, onUnmounted } from 'vue'
import { api } from '../api'
import { store } from '../store'
import { t } from '../i18n'

const status = ref('')
const q = ref('')
const page = ref(1)
const pageSize = 20
const items = ref([])
const total = ref(0)
const statusCount = ref({})
const loading = ref(false)
const err = ref('')
let timer = null

async function load(silent = false) {
  if (!silent) loading.value = true
  try {
    const params = new URLSearchParams({ page: page.value, page_size: pageSize })
    if (status.value) params.set('status', status.value)
    if (q.value) params.set('q', q.value)
    const res = await api.get(`/api/v1/tickets?${params}`)
    items.value = res.items
    total.value = res.total
    statusCount.value = res.status_count || {}
    err.value = ''
  } catch (e) {
    if (!silent) err.value = e.message
  } finally {
    loading.value = false
  }
}

function setStatus(s) {
  status.value = s
  page.value = 1
  load()
}

const tabs = [
  { key: '', label: 'all' },
  { key: 'open', label: 'status_open' },
  { key: 'pending', label: 'status_pending' },
  { key: 'closed', label: 'status_closed' },
]

function countFor(key) {
  if (key === '') return Object.values(statusCount.value).reduce((a, b) => a + b, 0)
  return statusCount.value[key] || 0
}

function fmtTime(iso) {
  if (!iso) return ''
  const d = new Date(iso)
  const now = new Date()
  const sameDay = d.toDateString() === now.toDateString()
  return sameDay
    ? d.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })
    : d.toLocaleDateString([], { month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit' })
}

function badgeClass(s) {
  return {
    open: 'bg-emerald-100 text-emerald-700 dark:bg-emerald-900/50 dark:text-emerald-300',
    pending: 'bg-amber-100 text-amber-700 dark:bg-amber-900/50 dark:text-amber-300',
    closed: 'bg-slate-200 text-slate-500 dark:bg-slate-800 dark:text-slate-400',
  }[s]
}

watch(q, () => { page.value = 1; load() })
onMounted(() => {
  load()
  timer = setInterval(() => load(true), 10000)
})
onUnmounted(() => clearInterval(timer))
</script>

<template>
  <div>
    <div class="flex flex-wrap items-center gap-2 mb-4">
      <div class="flex items-center gap-1 bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-lg p-1">
        <button
          v-for="tb in tabs" :key="tb.key"
          class="px-3 py-1.5 rounded-md text-sm transition-colors cursor-pointer"
          :class="status === tb.key
            ? 'bg-blue-600 text-white'
            : 'text-slate-600 dark:text-slate-300 hover:bg-slate-100 dark:hover:bg-slate-800'"
          @click="setStatus(tb.key)"
        >
          {{ t(tb.label) }}
          <span class="ml-1 text-xs opacity-70">{{ countFor(tb.key) }}</span>
        </button>
      </div>
      <input v-model="q" :placeholder="t('search')" class="input flex-1 min-w-40" />
    </div>

    <div v-if="loading && !items.length" class="py-20 text-center text-slate-400 animate-pulse">…</div>
    <div v-else-if="err" class="card p-8 text-center text-rose-500">{{ err }}</div>

    <div v-else-if="!items.length" class="card p-12 text-center">
      <div class="text-4xl mb-3">📭</div>
      <p class="font-medium text-slate-700 dark:text-slate-200">{{ t('noTickets') }}</p>
      <p class="text-sm text-slate-500 dark:text-slate-400 mt-1">{{ t('noTicketsHint') }}</p>
      <RouterLink to="/new" class="btn-primary mt-5">{{ t('navNew') }}</RouterLink>
    </div>

    <div v-else class="space-y-2">
      <RouterLink
        v-for="tk in items" :key="tk.id" :to="`/tickets/${tk.id}`"
        class="card px-4 py-3 flex items-center gap-3 hover:border-blue-400 dark:hover:border-blue-600 transition-colors"
      >
        <span class="text-slate-400 text-sm font-mono shrink-0">#{{ tk.id }}</span>
        <div class="min-w-0 flex-1">
          <div class="flex items-center gap-2">
            <span class="font-medium truncate">{{ tk.subject }}</span>
            <span
              v-if="store.isAdmin ? tk.unread_admin_count > 0 : tk.unread_user_count > 0"
              class="shrink-0 min-w-5 h-5 px-1.5 rounded-full bg-rose-500 text-white text-xs font-semibold flex items-center justify-center"
            >{{ store.isAdmin ? tk.unread_admin_count : tk.unread_user_count }}</span>
          </div>
          <div class="text-xs text-slate-500 dark:text-slate-400 mt-0.5 flex items-center gap-2">
            <span v-if="store.isAdmin && tk.user">{{ t('userCol') }}: {{ tk.user.username }}</span>
            <span>{{ t('updated') }} {{ fmtTime(tk.last_reply_at) }}</span>
          </div>
        </div>
        <span v-if="tk.priority === 'high' && tk.status !== 'closed'" class="shrink-0 w-2 h-2 rounded-full bg-rose-500" title="high"></span>
        <span class="shrink-0 px-2 py-0.5 rounded-full text-xs font-medium" :class="badgeClass(tk.status)">{{ t('status_' + tk.status) }}</span>
      </RouterLink>

      <div v-if="total > pageSize" class="flex justify-center gap-2 pt-3">
        <button class="btn-ghost" :disabled="page <= 1" @click="page--; load()">←</button>
        <span class="px-3 py-2 text-sm text-slate-500">{{ page }} / {{ Math.ceil(total / pageSize) }}</span>
        <button class="btn-ghost" :disabled="page >= Math.ceil(total / pageSize)" @click="page++; load()">→</button>
      </div>
    </div>
  </div>
</template>
