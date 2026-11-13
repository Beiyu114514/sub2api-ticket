<script setup>
import { ref, computed } from 'vue'
import { useRoute } from 'vue-router'
import { store } from '@/store'
import { t } from '@/i18n'
import { toggleTheme, currentTheme } from '@/theme'
import { Menu, Sun, Moon } from '@lucide/vue'
import { Button } from '@/components/ui/button'
import { useNavGroups } from '@/lib/nav'

const route = useRoute()
const groups = useNavGroups()
const theme = ref(currentTheme())

/** 当前区块名：详情页归到工单列表 */
const current = computed(() => {
  const path = route.path
  const items = groups.value.flatMap(g => g.items)
  const byPath = items.find(i => path === i.to || (i.to !== '/' && path.startsWith(`${i.to}/`)))
  if (byPath) return byPath.label
  if (path.startsWith('/tickets/')) return store.isAdmin ? t('navAllTickets') : t('navTickets')
  return t('appName')
})

function onTheme() {
  theme.value = toggleTheme()
}
</script>

<template>
  <header class="sticky top-0 z-20 flex h-14 shrink-0 items-center gap-2 border-b bg-background/85 px-4 backdrop-blur-sm supports-[backdrop-filter]:bg-background/65">
    <Button variant="ghost" size="icon-sm" class="lg:hidden" @click="store.sidebarOpen = true">
      <Menu class="size-4" />
      <span class="sr-only">{{ t('openMenu') }}</span>
    </Button>

    <!-- 面包屑 -->
    <div class="flex min-w-0 items-center gap-1.5 text-sm">
      <span class="hidden text-muted-foreground sm:inline">{{ t('appName') }}</span>
      <span class="hidden text-muted-foreground/50 sm:inline">/</span>
      <span class="truncate font-medium">{{ current }}</span>
    </div>

    <div class="ml-auto flex items-center gap-1.5">
      <Button variant="ghost" size="icon-sm" :aria-label="t('theme')" @click="onTheme">
        <Sun v-if="theme === 'dark'" class="size-4" />
        <Moon v-else class="size-4" />
      </Button>
    </div>
  </header>
</template>
