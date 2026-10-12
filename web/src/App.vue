<script setup>
import { computed } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { store } from './store'
import { api } from './api'
import { t } from './i18n'

const route = useRoute()
const router = useRouter()

const tabs = computed(() => {
  const list = [
    { to: '/', label: store.isAdmin ? t('navAllTickets') : t('navTickets') },
    { to: '/new', label: t('navNew') },
    { to: '/settings', label: t('navSettings') },
  ]
  if (store.isAdmin) list.push({ to: '/admin', label: t('navAdmin') })
  return list
})

async function logout() {
  await api.post('/api/v1/auth/logout').catch(() => {})
  location.href = '/'
}
</script>

<template>
  <div class="min-h-full flex flex-col">
    <!-- 顶部导航 -->
    <header class="sticky top-0 z-20 bg-white/90 dark:bg-slate-900/90 backdrop-blur border-b border-slate-200 dark:border-slate-800">
      <div class="max-w-4xl mx-auto px-4 h-14 flex items-center gap-2">
        <div class="flex items-center gap-2 mr-4 shrink-0">
          <span class="text-xl">🎫</span>
          <span class="font-semibold text-slate-800 dark:text-slate-100 hidden sm:inline">{{ t('appName') }}</span>
        </div>
        <nav class="flex items-center gap-1 overflow-x-auto">
          <RouterLink
            v-for="tb in tabs" :key="tb.to" :to="tb.to"
            class="px-3 py-1.5 rounded-lg text-sm whitespace-nowrap transition-colors"
            :class="route.path === tb.to
              ? 'bg-blue-600 text-white'
              : 'text-slate-600 dark:text-slate-300 hover:bg-slate-100 dark:hover:bg-slate-800'"
          >{{ tb.label }}</RouterLink>
        </nav>
        <div class="ml-auto flex items-center gap-2 shrink-0">
          <span v-if="store.user" class="text-xs text-slate-500 dark:text-slate-400 hidden md:inline">
            {{ store.user.username }}<span v-if="store.isAdmin" class="ml-1 px-1.5 py-0.5 rounded bg-amber-100 dark:bg-amber-900/60 text-amber-700 dark:text-amber-300 font-medium">admin</span>
          </span>
          <button v-if="store.user" class="btn-ghost !px-2.5 !py-1.5 text-xs" @click="logout">{{ t('logout') }}</button>
        </div>
      </div>
    </header>

    <main class="flex-1 max-w-4xl w-full mx-auto px-4 py-6">
      <!-- 启动中 -->
      <div v-if="!store.booted" class="flex items-center justify-center py-32 text-slate-400">
        <div class="animate-pulse">Loading…</div>
      </div>

      <!-- 未登录 -->
      <div v-else-if="store.bootError" class="card p-8 text-center">
        <div class="text-4xl mb-3">{{ store.bootError === 'unauthorized' ? '🔒' : '📡' }}</div>
        <p class="text-slate-600 dark:text-slate-300 mb-4">
          {{ store.bootError === 'unauthorized' ? t('unauthorizedHint') : t('loadFailed') }}
        </p>
        <RouterLink to="/login" class="btn-primary">{{ t('login') }}</RouterLink>
      </div>

      <!-- 未登录但直接访问受保护页：交给路由守卫处理 -->
      <RouterView v-else />
    </main>
  </div>
</template>
