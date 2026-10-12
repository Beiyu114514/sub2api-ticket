<script setup>
import { ref, onMounted } from 'vue'
import { api } from '../api'
import { t } from '../i18n'

const bound = ref(false)
const botReady = ref(false)
const botUsername = ref('')
const loading = ref(true)
const bindLink = ref('')
const bindCode = ref('')
const busy = ref(false)
const err = ref('')
const copied = ref(false)

async function load() {
  try {
    const res = await api.get('/api/v1/telegram/bind-status')
    bound.value = res.bound
    botReady.value = res.bot_ready
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
  } finally {
    busy.value = false
  }
}

async function unbind() {
  if (!confirm(t('tgUnbind') + '?')) return
  busy.value = true
  try {
    await api.del('/api/v1/telegram/bind')
    bound.value = false
    bindLink.value = ''
  } catch (e) {
    err.value = e.message
  } finally {
    busy.value = false
  }
}

async function copyLink() {
  try {
    await navigator.clipboard.writeText(bindLink.value)
    copied.value = true
  } catch {
    // iframe 内剪贴板可能受限，退化为选中提示
    window.prompt(t('tgCopyLink'), bindLink.value)
  }
}

onMounted(load)
</script>

<template>
  <div class="max-w-2xl mx-auto space-y-6">
    <div class="card p-6">
      <h1 class="text-lg font-semibold mb-1">{{ t('settingsTitle') }}</h1>
      <p class="text-sm text-slate-500 dark:text-slate-400">{{ t('tgDesc') }}</p>
    </div>

    <div class="card p-6">
      <div class="flex items-center justify-between mb-4">
        <div class="flex items-center gap-2">
          <span class="text-2xl">✈️</span>
          <h2 class="font-medium">{{ t('tgTitle') }}</h2>
        </div>
        <span
          class="px-2.5 py-1 rounded-full text-xs font-medium"
          :class="bound
            ? 'bg-emerald-100 text-emerald-700 dark:bg-emerald-900/50 dark:text-emerald-300'
            : 'bg-slate-200 text-slate-500 dark:bg-slate-800 dark:text-slate-400'"
        >{{ bound ? t('tgBound') : t('tgUnbound') }}</span>
      </div>

      <div v-if="loading" class="py-6 text-center text-slate-400 animate-pulse">…</div>

      <template v-else>
        <p v-if="!botReady" class="text-sm text-amber-600 dark:text-amber-400 bg-amber-50 dark:bg-amber-950/40 rounded-lg p-3">
          {{ t('tgNotConfigured') }}
        </p>

        <template v-else-if="!bound && !bindLink">
          <p class="text-sm text-slate-500 dark:text-slate-400 mb-4">{{ t('tgBindHint') }}</p>
          <button class="btn-primary" :disabled="busy" @click="createBind">✈️ {{ t('tgBind') }}</button>
        </template>

        <template v-else-if="bindLink">
          <div class="rounded-lg border border-blue-200 dark:border-blue-800 bg-blue-50/60 dark:bg-blue-950/40 p-4 space-y-3">
            <p class="text-sm font-medium text-blue-700 dark:text-blue-300">{{ t('bindCodeLabel') }}: <code class="font-mono">{{ bindCode }}</code></p>
            <div class="flex flex-wrap gap-2">
              <a :href="bindLink" target="_blank" rel="noopener" class="btn-primary">✈️ {{ t('tgOpenLink') }}</a>
              <button class="btn-ghost border border-slate-200 dark:border-slate-700" @click="copyLink">📋 {{ copied ? t('linkCopied') : t('tgCopyLink') }}</button>
            </div>
            <div class="text-xs text-slate-500 dark:text-slate-400 space-y-1">
              <p class="font-medium">{{ t('steps') }}</p>
              <p>{{ t('step1') }}</p>
              <p>{{ t('step2') }}</p>
              <p>{{ t('step3') }}</p>
            </div>
          </div>
          <button class="btn-ghost mt-3 text-sm" @click="bindLink = ''">{{ t('backToList') }}</button>
        </template>

        <template v-else>
          <p class="text-sm text-slate-500 dark:text-slate-400 mb-4">{{ t('tgDesc') }}</p>
          <button class="btn-danger" :disabled="busy" @click="unbind">{{ t('tgUnbind') }}</button>
        </template>

        <p v-if="err" class="text-sm text-rose-600 dark:text-rose-400 mt-3">{{ err }}</p>
      </template>
    </div>
  </div>
</template>
