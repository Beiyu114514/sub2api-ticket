<script setup>
import { useRoute } from 'vue-router'
import { store } from './store'
import { t } from './i18n'
import SidebarNav from './components/layout/SidebarNav.vue'
import AppTopbar from './components/layout/AppTopbar.vue'
import MobileNav from './components/layout/MobileNav.vue'
import BootError from './views/BootError.vue'
import { Toaster } from './components/ui/sonner'
import { TooltipProvider } from './components/ui/tooltip'

const route = useRoute()
</script>

<template>
  <TooltipProvider>
    <div class="h-full">
      <!-- 启动中 -->
      <div v-if="!store.booted" class="flex h-full items-center justify-center">
        <div class="flex flex-col items-center gap-3 text-muted-foreground">
          <div class="size-6 animate-spin rounded-full border-2 border-muted border-t-primary" />
          <span class="text-sm">{{ t('loading') }}</span>
        </div>
      </div>

      <!-- 启动失败 -->
      <div v-else-if="store.bootError" class="flex min-h-full items-center justify-center px-4 py-12">
        <BootError />
      </div>

      <!-- 登录页（无侧边栏） -->
      <div v-else-if="route.path === '/login'" class="flex min-h-full items-center justify-center px-4 py-12">
        <RouterView />
      </div>

      <!-- 主壳：侧边栏 + 顶栏 + 内容 -->
      <div v-else class="flex h-full overflow-hidden bg-background">
        <SidebarNav class="hidden lg:flex" />
        <MobileNav />

        <div class="flex min-w-0 flex-1 flex-col">
          <AppTopbar />
          <main class="flex-1 overflow-y-auto">
            <div class="mx-auto w-full max-w-6xl px-4 py-6 sm:px-6 lg:px-8 lg:py-8">
              <RouterView />
            </div>
          </main>
        </div>
      </div>

      <Toaster position="top-center" :duration="2400" />
    </div>
  </TooltipProvider>
</template>
