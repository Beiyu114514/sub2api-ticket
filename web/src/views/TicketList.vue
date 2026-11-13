<script setup>
import { ref, computed, watch, onMounted, onUnmounted } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import { api } from '@/api'
import { store } from '@/store'
import { t } from '@/i18n'
import { fmtShortTime, fmtRelative, initials } from '@/lib/format'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Badge } from '@/components/ui/badge'
import { Card } from '@/components/ui/card'
import { Avatar, AvatarFallback } from '@/components/ui/avatar'
import { Skeleton } from '@/components/ui/skeleton'
import {
  Table, TableBody, TableCell, TableHead, TableHeader, TableRow,
} from '@/components/ui/table'
import {
  Empty, EmptyContent, EmptyDescription, EmptyHeader, EmptyMedia, EmptyTitle,
} from '@/components/ui/empty'
import StatusBadge from '@/components/StatusBadge.vue'
import PriorityLabel from '@/components/PriorityLabel.vue'
import {
  Search, Plus, Inbox, Ticket as TicketIcon, CircleDot, Clock, CircleCheck,
  ArrowUpRight, X, RefreshCw, ChevronLeft, ChevronRight,
} from '@lucide/vue'
import { cn } from '@/lib/utils'

const router = useRouter()
const route = useRoute()

const status = ref(String(route.query.status || ''))
const q = ref(String(route.query.q || ''))
const page = ref(1)
const pageSize = ref(20)
const items = ref([])
const total = ref(0)
const statusCount = ref({})
const loading = ref(true)
const refreshing = ref(false)
const err = ref('')
let timer = null
let qTimer = null

const tabs = computed(() => [
  { key: '', label: t('all'), icon: Inbox },
  { key: 'open', label: t('status_open'), icon: CircleDot },
  { key: 'pending', label: t('status_pending'), icon: Clock },
  { key: 'closed', label: t('status_closed'), icon: CircleCheck },
])

const totalPages = computed(() => Math.max(1, Math.ceil(total.value / pageSize.value)))
const hasFilter = computed(() => !!status.value || !!q.value)

const stats = computed(() => {
  const c = statusCount.value || {}
  const sum = Object.values(c).reduce((a, b) => a + b, 0)
  return [
    { key: '', label: t('statTotal'), value: sum, icon: TicketIcon, tone: 'text-foreground' },
    { key: 'open', label: t('statOpen'), value: c.open || 0, icon: CircleDot, tone: 'text-info' },
    { key: 'pending', label: t('statPending'), value: c.pending || 0, icon: Clock, tone: 'text-warning' },
    { key: 'closed', label: t('statClosed'), value: c.closed || 0, icon: CircleCheck, tone: 'text-muted-foreground' },
  ]
})

function syncQuery() {
  const query = {}
  if (status.value) query.status = status.value
  if (q.value) query.q = q.value
  router.replace({ query }).catch(() => {})
}

async function load(silent = false) {
  if (silent) refreshing.value = true
  else loading.value = true
  try {
    const params = new URLSearchParams({ page: page.value, page_size: pageSize.value })
    if (status.value) params.set('status', status.value)
    if (q.value) params.set('q', q.value)
    const res = await api.get(`/api/v1/tickets?${params}`)
    items.value = res.items || []
    total.value = res.total || 0
    statusCount.value = res.status_count || {}
    err.value = ''
  } catch (e) {
    if (!silent) err.value = e.message
  } finally {
    loading.value = false
    refreshing.value = false
  }
}

function setStatus(next) {
  status.value = status.value === next ? '' : next
  page.value = 1
  syncQuery()
  load()
}

function clearFilters() {
  status.value = ''
  q.value = ''
  page.value = 1
  syncQuery()
  load()
}

function goPage(n) {
  if (n < 1 || n > totalPages.value || n === page.value) return
  page.value = n
  load()
}

function openTicket(id) {
  router.push(`/tickets/${id}`)
}

watch(q, () => {
  clearTimeout(qTimer)
  qTimer = setTimeout(() => {
    page.value = 1
    syncQuery()
    load()
  }, 300)
})

onMounted(() => {
  load()
  timer = setInterval(() => {
    if (!document.hidden) load(true)
  }, 15000)
})
onUnmounted(() => {
  clearInterval(timer)
  clearTimeout(qTimer)
})
</script>

<template>
  <div class="space-y-6">
    <!-- 页头 -->
    <div class="flex flex-wrap items-start justify-between gap-3">
      <div class="min-w-0 space-y-1">
        <h1 class="text-2xl font-semibold tracking-tight">{{ store.isAdmin ? t('navAllTickets') : t('navTickets') }}</h1>
        <p class="text-sm text-muted-foreground">
          {{ store.isAdmin ? t('listDescAdmin') : t('listDesc') }}
        </p>
      </div>
      <div class="flex items-center gap-2">
        <Button variant="outline" size="sm" :disabled="refreshing" @click="load(true)">
          <RefreshCw :class="cn('size-4', refreshing && 'animate-spin')" />
          <span class="hidden sm:inline">{{ t('refresh') }}</span>
        </Button>
        <Button size="sm" @click="router.push('/new')">
          <Plus class="size-4" />
          {{ t('navNew') }}
        </Button>
      </div>
    </div>

    <!-- 统计卡片 -->
    <div class="grid grid-cols-2 gap-3 lg:grid-cols-4">
      <button
        v-for="s in stats" :key="s.key"
        type="button"
        :class="cn(
          'group rounded-xl border bg-card p-4 text-left transition-all hover:border-primary/30 hover:shadow-sm',
          status === s.key && 'border-primary/40 ring-1 ring-primary/20',
        )"
        @click="setStatus(s.key)"
      >
        <div class="flex items-center justify-between">
          <span class="text-xs font-medium text-muted-foreground">{{ s.label }}</span>
          <component :is="s.icon" :class="cn('size-4', s.tone)" />
        </div>
        <div class="mt-2">
          <span class="text-2xl font-semibold leading-none tabular-nums">{{ s.value }}</span>
        </div>
      </button>
    </div>

    <!-- 列表卡片 -->
    <Card class="overflow-hidden py-0">
      <!-- 工具栏 -->
      <div class="flex flex-col gap-3 border-b p-4 sm:flex-row sm:items-center sm:justify-between">
        <div class="flex items-center gap-1 overflow-x-auto">
          <button
            v-for="tb in tabs" :key="tb.key"
            type="button"
            :class="cn(
              'inline-flex shrink-0 items-center gap-1.5 rounded-md px-2.5 py-1.5 text-sm transition-colors',
              status === tb.key
                ? 'bg-accent font-medium text-accent-foreground'
                : 'text-muted-foreground hover:bg-accent/60 hover:text-accent-foreground',
            )"
            @click="setStatus(tb.key)"
          >
            {{ tb.label }}
            <Badge
              variant="secondary"
              class="h-5 min-w-5 px-1 text-[10px] tabular-nums"
              :class="status === tb.key ? 'bg-background/70' : ''"
            >
              {{ tb.key === '' ? stats[0].value : statusCount[tb.key] || 0 }}
            </Badge>
          </button>
        </div>

        <div class="relative sm:w-64">
          <Search class="pointer-events-none absolute left-2.5 top-1/2 size-3.5 -translate-y-1/2 text-muted-foreground" />
          <Input v-model="q" :placeholder="t('search')" class="h-8 pl-8 pr-8 text-sm" />
          <button
            v-if="q" type="button"
            class="absolute right-2 top-1/2 -translate-y-1/2 text-muted-foreground hover:text-foreground"
            @click="q = ''"
          >
            <X class="size-3.5" />
          </button>
        </div>
      </div>

      <!-- 加载骨架 -->
      <div v-if="loading" class="divide-y">
        <div v-for="i in 5" :key="i" class="flex items-center gap-4 p-4">
          <Skeleton class="size-9 rounded-full" />
          <div class="flex-1 space-y-2">
            <Skeleton class="h-3.5 w-2/5" />
            <Skeleton class="h-3 w-1/4" />
          </div>
          <Skeleton class="h-5 w-16 rounded-full" />
        </div>
      </div>

      <!-- 错误 -->
      <div v-else-if="err" class="p-6">
        <div class="rounded-lg border border-destructive/30 bg-destructive/5 p-4 text-sm text-destructive">
          {{ err }}
        </div>
      </div>

      <!-- 空状态 -->
      <Empty v-else-if="!items.length" class="border-0">
        <EmptyMedia variant="icon">
          <Inbox />
        </EmptyMedia>
        <EmptyHeader>
          <EmptyTitle>{{ hasFilter ? t('noTicketsSearch') : t('noTickets') }}</EmptyTitle>
          <EmptyDescription>{{ hasFilter ? t('noTicketsSearchHint') : t('noTicketsHint') }}</EmptyDescription>
        </EmptyHeader>
        <EmptyContent>
          <div class="flex gap-2">
            <Button v-if="hasFilter" variant="outline" size="sm" @click="clearFilters">
              {{ t('clearFilters') }}
            </Button>
            <Button size="sm" @click="router.push('/new')">
              <Plus class="size-4" />
              {{ t('navNew') }}
            </Button>
          </div>
        </EmptyContent>
      </Empty>

      <!-- 数据表 -->
      <template v-else>
        <!-- 移动端：卡片列表 -->
        <ul class="divide-y sm:hidden">
          <li v-for="tk in items" :key="tk.id">
            <button type="button" class="flex w-full flex-col gap-2 p-4 text-left active:bg-accent/40" @click="openTicket(tk.id)">
              <div class="flex items-start gap-2">
                <span class="mt-0.5 shrink-0 font-mono text-xs text-muted-foreground">#{{ tk.id }}</span>
                <span class="min-w-0 flex-1 line-clamp-2 text-sm font-medium">{{ tk.subject }}</span>
                <Badge
                  v-if="(store.isAdmin ? tk.unread_admin_count : tk.unread_user_count) > 0"
                  class="h-4 min-w-4 shrink-0 px-1 text-[10px] tabular-nums"
                >{{ store.isAdmin ? tk.unread_admin_count : tk.unread_user_count }}</Badge>
              </div>
              <div class="flex flex-wrap items-center gap-x-3 gap-y-1.5 pl-8 text-xs text-muted-foreground">
                <StatusBadge :status="tk.status" class="h-5" />
                <PriorityLabel :priority="tk.priority" />
                <span v-if="store.isAdmin && tk.user" class="truncate">{{ tk.user.username }}</span>
                <span class="ml-auto shrink-0" :title="fmtShortTime(tk.last_reply_at)">{{ fmtRelative(tk.last_reply_at) }}</span>
              </div>
            </button>
          </li>
        </ul>

        <!-- 桌面端：表格 -->
        <div class="hidden sm:block">
        <Table>
          <TableHeader>
            <TableRow class="hover:bg-transparent">
              <TableHead class="w-[45%] pl-4">{{ t('colTicket') }}</TableHead>
              <TableHead v-if="store.isAdmin" class="hidden md:table-cell">{{ t('userCol') }}</TableHead>
              <TableHead>{{ t('priority') }}</TableHead>
              <TableHead>{{ t('statusCol') }}</TableHead>
              <TableHead class="pr-4 text-right">{{ t('colUpdated') }}</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            <TableRow
              v-for="tk in items" :key="tk.id"
              class="group cursor-pointer"
              tabindex="0"
              @click="openTicket(tk.id)"
              @keydown.enter="openTicket(tk.id)"
            >
              <TableCell class="max-w-0 pl-4">
                <div class="flex items-center gap-2.5">
                  <span class="shrink-0 font-mono text-xs text-muted-foreground">#{{ tk.id }}</span>
                  <div class="min-w-0">
                    <div class="flex items-center gap-2">
                      <span class="truncate font-medium">{{ tk.subject }}</span>
                      <Badge
                        v-if="(store.isAdmin ? tk.unread_admin_count : tk.unread_user_count) > 0"
                        class="h-4 min-w-4 shrink-0 px-1 text-[10px] tabular-nums"
                      >
                        {{ store.isAdmin ? tk.unread_admin_count : tk.unread_user_count }}
                        <span class="sr-only">{{ t('rowUnread') }}</span>
                      </Badge>
                    </div>
                    <p v-if="tk.category" class="truncate text-xs text-muted-foreground">{{ tk.category }}</p>
                  </div>
                </div>
              </TableCell>

              <TableCell v-if="store.isAdmin" class="hidden md:table-cell">
                <div v-if="tk.user" class="flex items-center gap-2">
                  <Avatar class="size-6">
                    <AvatarFallback class="bg-muted text-[10px] font-medium">
                      {{ initials(tk.user.username) }}
                    </AvatarFallback>
                  </Avatar>
                  <span class="max-w-32 truncate text-sm">{{ tk.user.username }}</span>
                </div>
                <span v-else class="text-sm text-muted-foreground">—</span>
              </TableCell>

              <TableCell>
                <PriorityLabel :priority="tk.priority" />
              </TableCell>

              <TableCell>
                <StatusBadge :status="tk.status" />
              </TableCell>

              <TableCell class="pr-4 text-right">
                <div class="flex items-center justify-end gap-1.5 text-xs text-muted-foreground">
                  <span :title="fmtShortTime(tk.last_reply_at)">{{ fmtRelative(tk.last_reply_at) }}</span>
                  <ArrowUpRight class="size-3.5 opacity-0 transition-opacity group-hover:opacity-100" />
                </div>
              </TableCell>
            </TableRow>
          </TableBody>
        </Table>
        </div>

        <!-- 分页 -->
        <div class="flex flex-wrap items-center justify-between gap-3 border-t px-4 py-3">
          <p class="text-xs text-muted-foreground">{{ t('totalCount', { n: total }) }}</p>
          <div class="flex items-center gap-1.5">
            <Button
              variant="outline" size="icon-sm"
              :disabled="page <= 1" :aria-label="t('prevPage')"
              @click="goPage(page - 1)"
            >
              <ChevronLeft class="size-4" />
            </Button>
            <span class="min-w-16 text-center text-sm tabular-nums text-muted-foreground">
              {{ page }} / {{ totalPages }}
            </span>
            <Button
              variant="outline" size="icon-sm"
              :disabled="page >= totalPages" :aria-label="t('nextPage')"
              @click="goPage(page + 1)"
            >
              <ChevronRight class="size-4" />
            </Button>
          </div>
        </div>
      </template>
    </Card>
  </div>
</template>
