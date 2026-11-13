<script setup>
import { ref, computed, onMounted, onUnmounted, nextTick, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { api } from '@/api'
import { store } from '@/store'
import { t } from '@/i18n'
import { fmtDateTime, parseAttachments, initials } from '@/lib/format'
import { toast } from 'vue-sonner'
import { Button } from '@/components/ui/button'
import { Textarea } from '@/components/ui/textarea'
import { Badge } from '@/components/ui/badge'
import { Skeleton } from '@/components/ui/skeleton'
import { Avatar, AvatarFallback } from '@/components/ui/avatar'
import { Separator } from '@/components/ui/separator'
import {
  Card, CardContent, CardHeader, CardTitle,
} from '@/components/ui/card'
import {
  DropdownMenu, DropdownMenuTrigger, DropdownMenuContent, DropdownMenuItem, DropdownMenuLabel,
  DropdownMenuSeparator,
} from '@/components/ui/dropdown-menu'
import { Dialog, DialogContent, DialogTitle, DialogDescription } from '@/components/ui/dialog'
import ImageUploader from '@/components/ImageUploader.vue'
import StatusBadge from '@/components/StatusBadge.vue'
import PriorityLabel from '@/components/PriorityLabel.vue'
import { cn } from '@/lib/utils'
import {
  ArrowLeft, Send, Loader2, Paperclip, Lock, ArrowDown, MoreHorizontal,
  CircleCheck, CircleDot, Clock, Bot, UserRound, ShieldCheck,
  RefreshCw, Tag, CalendarClock, MessageSquare, X,
} from '@lucide/vue'

const route = useRoute()
const router = useRouter()

const ticket = ref(null)
const messages = ref([])
const loading = ref(true)
const err = ref('')
const reply = ref('')
const pendingAtts = ref([])
const sending = ref(false)
const actionBusy = ref(false)
const streamEl = ref(null)
const lightbox = ref(null)
const showJump = ref(false)
let timer = null

const canReply = computed(() => !!ticket.value && ticket.value.status !== 'closed')
const ticketId = computed(() => route.params.id)

async function load(silent = false) {
  if (!silent) loading.value = true
  try {
    const res = await api.get(`/api/v1/tickets/${ticketId.value}`)
    const hadMessages = messages.value.length
    ticket.value = res.ticket
    messages.value = res.messages || []
    err.value = ''
    if (silent && messages.value.length !== hadMessages) {
      await nextTick()
      scrollBottom()
    } else if (!silent) {
      await nextTick()
      scrollBottom()
    }
  } catch (e) {
    err.value = e.message
  } finally {
    loading.value = false
  }
}

function scrollBottom() {
  if (streamEl.value) streamEl.value.scrollTop = streamEl.value.scrollHeight
  showJump.value = false
}

function onStreamScroll() {
  const el = streamEl.value
  if (!el) return
  showJump.value = el.scrollHeight - el.scrollTop - el.clientHeight > 120
}

async function send() {
  const text = reply.value.trim()
  const atts = pendingAtts.value.map(a => a.url)
  if ((!text && !atts.length) || sending.value) return
  sending.value = true
  try {
    await api.post(`/api/v1/tickets/${ticketId.value}/messages`, { content: text, attachments: atts })
    reply.value = ''
    pendingAtts.value = []
    await load(true)
  } catch (e) {
    toast.error(e.message)
  } finally {
    sending.value = false
  }
}

async function setStatus(status) {
  actionBusy.value = true
  try {
    await api.post(`/api/v1/tickets/${ticketId.value}/status`, { status })
    await load(true)
    toast.success(t(`status_${status}`))
  } catch (e) {
    toast.error(e.message)
  } finally {
    actionBusy.value = false
  }
}

/** 气泡方向：自己发的消息靠右 */
function side(m) {
  if (m.sender_role === 'system') return 'center'
  if (m.sender_role === 'assistant') return 'left'
  if (m.sender_role === 'admin') return store.isAdmin ? 'right' : 'left'
  return store.isAdmin ? 'left' : 'right'
}

function roleLabel(m) {
  if (m.sender_role === 'system') return t('system')
  if (m.sender_role === 'assistant') return 'AI'
  if (m.sender_role === 'admin') return t('admin')
  return t('user')
}

function roleIcon(m) {
  if (m.sender_role === 'assistant') return Bot
  if (m.sender_role === 'admin') return ShieldCheck
  if (m.sender_role === 'system') return CircleDot
  return UserRound
}

function attsOf(m) {
  return parseAttachments(m.attachments)
}

function onKeydown(e) {
  if ((e.metaKey || e.ctrlKey) && e.key === 'Enter') {
    e.preventDefault()
    send()
  }
}

function startPolling() {
  stopPolling()
  timer = setInterval(() => {
    if (!document.hidden) load(true)
  }, 10000)
}
function stopPolling() {
  if (timer) { clearInterval(timer); timer = null }
}
function onVisibility() {
  if (document.hidden) stopPolling()
  else { load(true); startPolling() }
}

watch(() => route.params.id, () => { load() })

onMounted(() => {
  load()
  startPolling()
  document.addEventListener('visibilitychange', onVisibility)
})
onUnmounted(() => {
  stopPolling()
  document.removeEventListener('visibilitychange', onVisibility)
})
</script>

<template>
  <div class="space-y-4">
    <!-- 顶部返回 -->
    <div class="flex items-center justify-between gap-2">
      <Button variant="ghost" size="sm" class="-ml-2 text-muted-foreground" @click="router.push('/')">
        <ArrowLeft class="size-4" />
        {{ t('detailBack') }}
      </Button>
      <Button variant="outline" size="sm" :disabled="loading" @click="load()">
        <RefreshCw :class="cn('size-4', loading && 'animate-spin')" />
        <span class="hidden sm:inline">{{ t('refresh') }}</span>
      </Button>
    </div>

    <!-- 加载中 -->
    <div v-if="loading" class="grid gap-4 lg:grid-cols-[minmax(0,1fr)_300px]">
      <Card>
        <div class="space-y-4 p-6">
          <Skeleton class="h-6 w-1/2" />
          <Skeleton class="h-3 w-1/3" />
          <Separator />
          <div class="space-y-3">
            <Skeleton class="h-16 w-3/4" />
            <Skeleton class="ml-auto h-16 w-2/3" />
            <Skeleton class="h-16 w-3/5" />
          </div>
        </div>
      </Card>
      <Card><div class="space-y-3 p-6"><Skeleton class="h-4 w-1/2" /><Skeleton class="h-4 w-2/3" /><Skeleton class="h-4 w-1/3" /></div></Card>
    </div>

    <!-- 错误 -->
    <Card v-else-if="err">
      <div class="flex flex-col items-center gap-3 p-10 text-center">
        <p class="text-sm text-destructive">{{ err }}</p>
        <Button variant="outline" size="sm" @click="router.push('/')">{{ t('backToList') }}</Button>
      </div>
    </Card>

    <template v-else-if="ticket">
      <div class="grid gap-4 lg:grid-cols-[minmax(0,1fr)_300px]">
        <!-- 会话区 -->
        <Card class="min-w-0 gap-0 overflow-hidden py-0">
          <!-- 会话头 -->
          <div class="border-b px-5 py-4">
            <div class="flex flex-wrap items-center gap-2">
              <span class="font-mono text-xs text-muted-foreground">#{{ ticket.id }}</span>
              <h1 class="min-w-0 flex-1 truncate text-base font-semibold">{{ ticket.subject }}</h1>
              <StatusBadge :status="ticket.status" />
              <DropdownMenu>
                <DropdownMenuTrigger as-child>
                  <Button variant="ghost" size="icon-sm">
                    <MoreHorizontal class="size-4" />
                    <span class="sr-only">{{ t('ticketActions') }}</span>
                  </Button>
                </DropdownMenuTrigger>
                <DropdownMenuContent align="end" class="w-48">
                  <DropdownMenuLabel>{{ t('ticketActions') }}</DropdownMenuLabel>
                  <DropdownMenuSeparator />
                  <DropdownMenuItem
                    v-if="ticket.status !== 'closed'"
                    :disabled="actionBusy"
                    @select="setStatus('closed')"
                  >
                    <CircleCheck class="size-4" />
                    {{ t('closeTicket') }}
                  </DropdownMenuItem>
                  <DropdownMenuItem
                    v-if="ticket.status === 'closed'"
                    :disabled="actionBusy"
                    @select="setStatus('open')"
                  >
                    <CircleDot class="size-4" />
                    {{ t('reopenTicket') }}
                  </DropdownMenuItem>
                  <DropdownMenuItem
                    v-if="ticket.status === 'open'"
                    :disabled="actionBusy"
                    @select="setStatus('pending')"
                  >
                    <Clock class="size-4" />
                    {{ t('markPending') }}
                  </DropdownMenuItem>
                  <DropdownMenuItem
                    v-if="ticket.status === 'pending'"
                    :disabled="actionBusy"
                    @select="setStatus('open')"
                  >
                    <CircleDot class="size-4" />
                    {{ t('markOpen') }}
                  </DropdownMenuItem>
                </DropdownMenuContent>
              </DropdownMenu>
            </div>
            <div class="mt-2 flex flex-wrap items-center gap-x-4 gap-y-1 text-xs text-muted-foreground">
              <span v-if="store.isAdmin && ticket.user">
                {{ t('assignee') }}: {{ ticket.user.username }}
              </span>
              <span v-if="ticket.category" class="inline-flex items-center gap-1">
                <Tag class="size-3" />{{ ticket.category }}
              </span>
              <span class="inline-flex items-center gap-1">
                <CalendarClock class="size-3" />{{ fmtDateTime(ticket.created_at) }}
              </span>
              <span class="inline-flex items-center gap-1">
                <MessageSquare class="size-3" />{{ messages.length }} {{ t('replies') }}
              </span>
            </div>
          </div>

          <!-- 消息流 -->
          <div class="relative">
            <div
              ref="streamEl"
              class="max-h-[52vh] min-h-64 space-y-4 overflow-y-auto px-4 py-5 sm:px-5"
              @scroll.passive="onStreamScroll"
            >
              <div v-if="!messages.length" class="py-10 text-center text-sm text-muted-foreground">
                {{ t('noMessages') }}
              </div>

              <template v-for="m in messages" :key="m.id">
                <!-- 系统消息 -->
                <div v-if="side(m) === 'center'" class="flex justify-center">
                  <span class="max-w-[85%] rounded-full bg-muted px-3 py-1 text-xs text-muted-foreground">
                    {{ m.content }}
                  </span>
                </div>

                <!-- 普通气泡 -->
                <div v-else class="flex gap-2.5" :class="side(m) === 'right' ? 'flex-row-reverse' : ''">
                  <Avatar class="mt-0.5 size-7 shrink-0">
                    <AvatarFallback
                      :class="cn(
                        'text-[10px] font-semibold',
                        side(m) === 'right' ? 'bg-primary/10 text-primary' : 'bg-muted text-muted-foreground',
                      )"
                    >
                      {{ initials(m.sender_name) }}
                    </AvatarFallback>
                  </Avatar>

                  <div class="min-w-0 max-w-[85%]" :class="side(m) === 'right' ? 'items-end text-right' : ''">
                    <div
                      class="mb-1 flex flex-wrap items-center gap-1.5 text-[11px] text-muted-foreground"
                      :class="side(m) === 'right' ? 'justify-end' : ''"
                    >
                      <span class="font-medium text-foreground/80">{{ m.sender_name }}</span>
                      <span class="inline-flex items-center gap-0.5">
                        <component :is="roleIcon(m)" class="size-3" />{{ roleLabel(m) }}
                      </span>
                      <Badge v-if="m.via === 'telegram'" variant="outline" class="h-4 px-1 text-[10px]">TG</Badge>
                      <Badge v-if="m.sender_role === 'assistant'" variant="outline" class="h-4 gap-0.5 px-1 text-[10px] text-primary">
                        <Bot class="size-2.5" />{{ t('aiBadge') }}
                      </Badge>
                      <span class="text-muted-foreground/70">{{ fmtDateTime(m.created_at) }}</span>
                    </div>

                    <div
                      :class="cn(
                        'inline-block rounded-2xl px-3.5 py-2 text-left text-sm leading-relaxed',
                        side(m) === 'right'
                          ? 'rounded-br-md bg-primary text-primary-foreground'
                          : 'rounded-bl-md bg-muted text-foreground',
                      )"
                    >
                      <p class="whitespace-pre-wrap break-words">{{ m.content }}</p>

                      <div
                        v-if="attsOf(m).length"
                        class="mt-2 grid gap-1.5"
                        :class="attsOf(m).length > 1 ? 'grid-cols-2' : 'grid-cols-1'"
                      >
                        <button
                          v-for="a in attsOf(m)" :key="a.url"
                          type="button"
                          class="group overflow-hidden rounded-lg border border-black/5 bg-background/40"
                          @click="lightbox = a.url"
                        >
                          <img
                            :src="a.url" :alt="t('attachmentAlt')"
                            class="max-h-40 w-full cursor-zoom-in object-cover transition-transform group-hover:scale-[1.02]"
                            loading="lazy"
                          />
                        </button>
                      </div>
                    </div>
                  </div>
                </div>
              </template>
            </div>

            <!-- 回到最新 -->
            <Transition name="fade">
              <Button
                v-if="showJump"
                size="sm"
                variant="secondary"
                class="absolute bottom-3 left-1/2 -translate-x-1/2 shadow-md"
                @click="scrollBottom"
              >
                <ArrowDown class="size-3.5" />
                {{ t('scrollBottom') }}
              </Button>
            </Transition>
          </div>

          <!-- 回复框 -->
          <div class="border-t bg-muted/20 p-4">
            <template v-if="canReply">
              <Textarea
                v-model="reply" rows="3" :placeholder="t('replyPh')" :disabled="sending"
                class="min-h-20 resize-y bg-background"
                @keydown="onKeydown"
              />
              <div class="mt-2.5">
                <ImageUploader v-model="pendingAtts" :max="5" :disabled="sending" />
              </div>
              <div class="mt-3 flex items-center justify-between gap-2">
                <span class="hidden items-center gap-1 text-xs text-muted-foreground sm:inline-flex">
                  <Paperclip class="size-3" />Ctrl / ⌘ + Enter
                </span>
                <Button
                  :disabled="sending || (!reply.trim() && !pendingAtts.length)"
                  class="ml-auto"
                  @click="send"
                >
                  <Loader2 v-if="sending" class="size-4 animate-spin" />
                  <Send v-else class="size-4" />
                  {{ sending ? t('sending') : t('send') }}
                </Button>
              </div>
            </template>

            <div v-else class="flex flex-col items-center gap-3 py-3 text-center">
              <p class="inline-flex items-center gap-1.5 text-sm text-muted-foreground">
                <Lock class="size-3.5" />{{ t('closed') }} · {{ t('closedHint') }}
              </p>
              <Button variant="outline" size="sm" :disabled="actionBusy" @click="setStatus('open')">
                <CircleDot class="size-4" />
                {{ t('reopenTicket') }}
              </Button>
            </div>
          </div>
        </Card>

        <!-- 右侧信息栏 -->
        <div class="space-y-4">
          <Card>
            <CardHeader class="pb-2">
              <CardTitle class="text-sm">{{ t('ticketInfo') }}</CardTitle>
            </CardHeader>
            <CardContent class="space-y-3 text-sm">
              <div class="flex items-center justify-between gap-2">
                <span class="text-muted-foreground">{{ t('statusCol') }}</span>
                <StatusBadge :status="ticket.status" :show-icon="false" />
              </div>
              <div class="flex items-center justify-between gap-2">
                <span class="text-muted-foreground">{{ t('priority') }}</span>
                <PriorityLabel :priority="ticket.priority" />
              </div>
              <div v-if="ticket.category" class="flex items-center justify-between gap-2">
                <span class="text-muted-foreground">{{ t('category') }}</span>
                <span class="truncate font-medium">{{ ticket.category }}</span>
              </div>
              <Separator />
              <div v-if="store.isAdmin && ticket.user" class="flex items-center gap-2">
                <Avatar class="size-7">
                  <AvatarFallback class="bg-muted text-[10px] font-medium">
                    {{ initials(ticket.user.username) }}
                  </AvatarFallback>
                </Avatar>
                <div class="min-w-0">
                  <p class="truncate font-medium leading-tight">{{ ticket.user.username }}</p>
                  <p class="truncate text-xs text-muted-foreground leading-tight">{{ ticket.user.email }}</p>
                </div>
              </div>
              <div class="space-y-1 text-xs text-muted-foreground">
                <p>{{ t('created') }} {{ fmtDateTime(ticket.created_at) }}</p>
                <p>{{ t('updated') }} {{ fmtDateTime(ticket.last_reply_at || ticket.updated_at) }}</p>
              </div>
            </CardContent>
          </Card>

          <!-- 管理员快捷操作 -->
          <Card v-if="store.isAdmin">
            <CardHeader class="pb-2">
              <CardTitle class="text-sm">{{ t('ticketActions') }}</CardTitle>
            </CardHeader>
            <CardContent class="flex flex-col gap-2">
              <Button
                v-if="ticket.status !== 'closed'"
                variant="destructive" size="sm" :disabled="actionBusy"
                class="justify-start"
                @click="setStatus('closed')"
              >
                <CircleCheck class="size-4" />
                {{ t('closeTicket') }}
              </Button>
              <Button
                v-if="ticket.status === 'open'"
                variant="outline" size="sm" :disabled="actionBusy"
                class="justify-start"
                @click="setStatus('pending')"
              >
                <Clock class="size-4" />
                {{ t('markPending') }}
              </Button>
              <Button
                v-if="ticket.status !== 'open'"
                variant="outline" size="sm" :disabled="actionBusy"
                class="justify-start"
                @click="setStatus('open')"
              >
                <CircleDot class="size-4" />
                {{ ticket.status === 'closed' ? t('reopenTicket') : t('markOpen') }}
              </Button>
            </CardContent>
          </Card>
        </div>
      </div>
    </template>

    <!-- 图片灯箱 -->
    <Dialog :open="!!lightbox" @update:open="(v) => { if (!v) lightbox = null }">
      <DialogContent class="max-w-3xl border-0 bg-transparent p-0 shadow-none" :show-close-button="false">
        <DialogTitle class="sr-only">{{ t('attachmentAlt') }}</DialogTitle>
        <DialogDescription class="sr-only">{{ t('closeLightbox') }}</DialogDescription>
        <img v-if="lightbox" :src="lightbox" :alt="t('attachmentAlt')" class="max-h-[85vh] w-full rounded-lg object-contain" />
        <button
          type="button"
          class="absolute -top-2 right-0 flex size-8 items-center justify-center rounded-full bg-background/90 text-foreground shadow"
          :aria-label="t('closeLightbox')"
          @click="lightbox = null"
        >
          <X class="size-4" />
        </button>
      </DialogContent>
    </Dialog>
  </div>
</template>

<style scoped>
.fade-enter-active,
.fade-leave-active {
  transition: opacity 0.15s ease;
}
.fade-enter-from,
.fade-leave-to {
  opacity: 0;
}
</style>
