<script setup>
import { ref } from 'vue'
import { useRouter } from 'vue-router'
import { api } from '../api'
import { t } from '../i18n'
import ImageUploader from '../components/ImageUploader.vue'

const router = useRouter()
const subject = ref('')
const category = ref('')
const priority = ref('normal')
const content = ref('')
const pendingAtts = ref([])
const busy = ref(false)
const err = ref('')

const priorities = [
  { key: 'low', label: 'prio_low', dot: 'bg-slate-400' },
  { key: 'normal', label: 'prio_normal', dot: 'bg-amber-400' },
  { key: 'high', label: 'prio_high', dot: 'bg-rose-500' },
]

async function submit() {
  if (!subject.value.trim() || !content.value.trim()) return
  err.value = ''
  busy.value = true
  try {
    const res = await api.post('/api/v1/tickets', {
      subject: subject.value,
      category: category.value,
      priority: priority.value,
      content: content.value,
      attachments: pendingAtts.value.map(a => a.url),
    })
    router.push(`/tickets/${res.ticket.id}`)
  } catch (e) {
    err.value = e.message
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <div class="max-w-2xl mx-auto card p-6">
    <h1 class="text-lg font-semibold mb-6">{{ t('navNew') }}</h1>
    <form @submit.prevent="submit" class="space-y-5">
      <div>
        <label class="label">{{ t('subject') }}</label>
        <input v-model="subject" required maxlength="200" class="input" />
      </div>
      <div>
        <label class="label">{{ t('category') }}</label>
        <input v-model="category" :placeholder="t('categoryPh')" maxlength="50" class="input" />
      </div>
      <div>
        <label class="label">{{ t('priority') }}</label>
        <div class="flex gap-2">
          <button
            v-for="p in priorities" :key="p.key" type="button"
            class="btn flex-1 border"
            :class="priority === p.key
              ? 'border-blue-500 bg-blue-50 dark:bg-blue-950 text-blue-700 dark:text-blue-300'
              : 'border-slate-200 dark:border-slate-700 text-slate-600 dark:text-slate-300'"
            @click="priority = p.key"
          >
            <span class="w-2 h-2 rounded-full" :class="p.dot"></span>
            {{ t(p.label) }}
          </button>
        </div>
      </div>
      <div>
        <label class="label">{{ t('content') }}</label>
        <textarea v-model="content" required rows="7" :placeholder="t('contentPh')"
          class="input resize-y font-normal"></textarea>
        <p class="text-xs text-slate-400 mt-1 text-right">{{ t('charLimit', { n: content.length }) }}</p>
        <div class="mt-2">
          <ImageUploader v-model="pendingAtts" :max="5" :disabled="busy" />
        </div>
      </div>
      <p v-if="err" class="text-sm text-rose-600 dark:text-rose-400">{{ err }}</p>
      <div class="flex items-center justify-between">
        <p class="text-xs text-slate-400">{{ t('firstReplyHint') }}</p>
        <button class="btn-primary" :disabled="busy || !subject.trim() || !content.trim()">
          {{ busy ? t('submitting') : t('submit') }}
        </button>
      </div>
    </form>
  </div>
</template>
