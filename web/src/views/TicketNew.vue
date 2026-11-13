<script setup>
import { ref, computed } from 'vue'
import { useRouter } from 'vue-router'
import { api } from '@/api'
import { t } from '@/i18n'
import { toast } from 'vue-sonner'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Textarea } from '@/components/ui/textarea'
import { Label } from '@/components/ui/label'
import {
  Card, CardContent, CardHeader, CardTitle,
} from '@/components/ui/card'
import { Separator } from '@/components/ui/separator'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import ImageUploader from '@/components/ImageUploader.vue'
import { cn } from '@/lib/utils'
import {
  ArrowLeft, Send, Loader2, Lightbulb, Clock, ArrowDown, Minus, ArrowUp, AlertCircle,
} from '@lucide/vue'

const router = useRouter()

const subject = ref('')
const category = ref('')
const priority = ref('normal')
const content = ref('')
const pendingAtts = ref([])
const busy = ref(false)
const err = ref('')

const priorities = computed(() => ([
  { key: 'low', label: t('prio_low'), icon: ArrowDown },
  { key: 'normal', label: t('prio_normal'), icon: Minus },
  { key: 'high', label: t('prio_high'), icon: ArrowUp },
]))

const canSubmit = computed(() => subject.value.trim().length > 0 && content.value.trim().length > 0 && !busy.value)

const tips = computed(() => [t('newTip1'), t('newTip2'), t('newTip3')])
const flow = computed(() => [t('flow1'), t('flow2'), t('flow3'), t('flow4')])

async function submit() {
  if (!canSubmit.value) return
  err.value = ''
  busy.value = true
  try {
    const res = await api.post('/api/v1/tickets', {
      subject: subject.value.trim(),
      category: category.value.trim(),
      priority: priority.value,
      content: content.value.trim(),
      attachments: pendingAtts.value.map(a => a.url),
    })
    toast.success(t('submitted'))
    router.push(`/tickets/${res.ticket.id}`)
  } catch (e) {
    err.value = e.message
    toast.error(e.message)
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <div class="space-y-6">
    <div class="flex items-center gap-2">
      <Button variant="ghost" size="icon-sm" :aria-label="t('backToList')" @click="router.push('/')">
        <ArrowLeft class="size-4" />
      </Button>
      <div>
        <h1 class="text-2xl font-semibold tracking-tight">{{ t('newTitle') }}</h1>
        <p class="text-sm text-muted-foreground">{{ t('newDesc') }}</p>
      </div>
    </div>

    <div class="grid gap-6 lg:grid-cols-[minmax(0,1fr)_280px]">
      <!-- 主表单 -->
      <Card class="gap-0 py-0">
        <CardContent class="p-6">
          <form class="space-y-5" @submit.prevent="submit">
            <div class="space-y-2">
              <Label for="subject">{{ t('subject') }}</Label>
              <Input
                id="subject" v-model="subject" required maxlength="200"
                :placeholder="t('subjectPh')" :disabled="busy"
              />
            </div>

            <div class="space-y-2">
              <Label for="category">{{ t('category') }}</Label>
              <Input
                id="category" v-model="category" maxlength="50"
                :placeholder="t('categoryPh')" :disabled="busy"
              />
            </div>

            <div class="space-y-2">
              <Label>{{ t('priority') }}</Label>
              <div class="grid grid-cols-3 gap-2">
                <button
                  v-for="p in priorities" :key="p.key"
                  type="button"
                  :disabled="busy"
                  :class="cn(
                    'flex items-center justify-center gap-1.5 rounded-md border px-3 py-2 text-sm transition-all',
                    priority === p.key
                      ? 'border-primary bg-primary/5 font-medium text-primary ring-1 ring-primary/20'
                      : 'border-border text-muted-foreground hover:border-foreground/20 hover:bg-accent/50',
                    p.key === 'high' && priority === p.key && 'border-destructive text-destructive ring-destructive/20 bg-destructive/5',
                  )"
                  @click="priority = p.key"
                >
                  <component :is="p.icon" class="size-3.5" />
                  {{ p.label }}
                </button>
              </div>
            </div>

            <div class="space-y-2">
              <div class="flex items-center justify-between">
                <Label for="content">{{ t('content') }}</Label>
                <span class="text-xs tabular-nums text-muted-foreground">{{ t('charLimit', { n: content.length }) }}</span>
              </div>
              <Textarea
                id="content" v-model="content" required rows="9" maxlength="10000"
                :placeholder="t('contentPh')" :disabled="busy" class="min-h-40 resize-y"
              />
            </div>

            <div class="space-y-2">
              <Label>{{ t('attachments') }}</Label>
              <ImageUploader v-model="pendingAtts" :max="5" :disabled="busy" />
            </div>

            <Alert v-if="err" variant="destructive">
              <AlertCircle />
              <AlertTitle>{{ t('submitFailed') }}</AlertTitle>
              <AlertDescription>{{ err }}</AlertDescription>
            </Alert>

            <Separator />

            <div class="flex flex-wrap items-center justify-between gap-3">
              <p class="text-xs text-muted-foreground">{{ t('firstReplyHint') }}</p>
              <div class="flex gap-2">
                <Button type="button" variant="ghost" :disabled="busy" @click="router.push('/')">
                  {{ t('backToList') }}
                </Button>
                <Button type="submit" :disabled="!canSubmit">
                  <Loader2 v-if="busy" class="size-4 animate-spin" />
                  <Send v-else class="size-4" />
                  {{ busy ? t('submitting') : t('submit') }}
                </Button>
              </div>
            </div>
          </form>
        </CardContent>
      </Card>

      <!-- 侧栏提示 -->
      <div class="space-y-4">
        <Card class="bg-muted/40">
          <CardHeader class="pb-1">
            <CardTitle class="flex items-center gap-2 text-sm">
              <Lightbulb class="size-4 text-warning" />
              {{ t('newTipsTitle') }}
            </CardTitle>
          </CardHeader>
          <CardContent>
            <ul class="space-y-3 text-sm text-muted-foreground">
              <li v-for="(tip, i) in tips" :key="i" class="flex gap-2">
                <span class="mt-1.5 size-1.5 shrink-0 rounded-full bg-primary/50" />
                <span class="leading-relaxed">{{ tip }}</span>
              </li>
            </ul>
          </CardContent>
        </Card>

        <Card>
          <CardHeader class="pb-2">
            <CardTitle class="flex items-center gap-2 text-sm">
              <Clock class="size-4 text-muted-foreground" />
              {{ t('ticketFlow') }}
            </CardTitle>
          </CardHeader>
          <CardContent>
            <ol class="space-y-2.5 text-xs text-muted-foreground">
              <li v-for="(s, i) in flow" :key="i" class="flex items-start gap-2">
                <span class="mt-0.5 flex size-4 shrink-0 items-center justify-center rounded-full bg-muted text-[10px] font-medium tabular-nums">
                  {{ i + 1 }}
                </span>
                <span class="leading-relaxed">{{ s }}</span>
              </li>
            </ol>
          </CardContent>
        </Card>
      </div>
    </div>
  </div>
</template>
