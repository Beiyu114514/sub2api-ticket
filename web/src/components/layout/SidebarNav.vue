<script setup>
import { useRoute } from 'vue-router'
import { store } from '@/store'
import { t } from '@/i18n'
import { useNavGroups } from '@/lib/nav'
import { cn } from '@/lib/utils'
import { initials } from '@/lib/format'
import { Ticket, ChevronsUpDown, LifeBuoy } from '@lucide/vue'
import { Avatar, AvatarFallback } from '@/components/ui/avatar'
import UserMenu from './UserMenu.vue'

const route = useRoute()
const groups = useNavGroups()

function isActive(to) {
  if (to === '/') return route.path === '/'
  return route.path === to || route.path.startsWith(`${to}/`)
}
</script>

<template>
  <aside class="flex w-64 shrink-0 flex-col border-r border-sidebar-border bg-sidebar text-sidebar-foreground">
    <!-- 品牌区 -->
    <div class="flex h-14 items-center gap-2.5 border-b border-sidebar-border px-4">
      <div class="flex size-8 items-center justify-center rounded-lg bg-primary text-primary-foreground shadow-xs">
        <Ticket class="size-4" />
      </div>
      <div class="min-w-0 flex-1">
        <p class="truncate text-sm font-semibold leading-tight">{{ t('appName') }}</p>
        <p class="truncate text-xs text-muted-foreground leading-tight">{{ t('appTagline') }}</p>
      </div>
    </div>

    <!-- 导航 -->
    <nav class="flex-1 overflow-y-auto px-3 py-4">
      <div v-for="group in groups" :key="group.label" class="mb-6 last:mb-0">
        <p class="mb-1.5 px-2 text-xs font-medium text-muted-foreground">{{ group.label }}</p>
        <ul class="space-y-0.5">
          <li v-for="item in group.items" :key="item.to">
            <RouterLink
              :to="item.to"
              :class="cn(
                'flex items-center gap-2.5 rounded-md px-2 py-1.5 text-sm transition-colors',
                isActive(item.to)
                  ? 'bg-sidebar-accent font-medium text-sidebar-accent-foreground'
                  : 'text-muted-foreground hover:bg-sidebar-accent/60 hover:text-sidebar-accent-foreground',
              )"
            >
              <component :is="item.icon" class="size-4 shrink-0" />
              <span class="truncate">{{ item.label }}</span>
            </RouterLink>
          </li>
        </ul>
      </div>
    </nav>

    <!-- 帮助提示 + 用户 -->
    <div class="border-t border-sidebar-border p-3">
      <div class="mb-3 rounded-lg bg-muted/60 p-3">
        <div class="flex items-center gap-2 text-xs font-medium">
          <LifeBuoy class="size-3.5 text-muted-foreground" />
          {{ t('tgTitle') }}
        </div>
        <p class="mt-1 line-clamp-2 text-xs leading-relaxed text-muted-foreground">{{ t('tgBindHint') }}</p>
      </div>

      <UserMenu v-if="store.user" class="w-full justify-between">
        <div class="flex min-w-0 items-center gap-2">
          <Avatar class="size-8">
            <AvatarFallback class="bg-primary/10 text-xs font-semibold text-primary">
              {{ initials(store.user.username) }}
            </AvatarFallback>
          </Avatar>
          <div class="min-w-0 text-left">
            <p class="truncate text-sm font-medium leading-tight">{{ store.user.username }}</p>
            <p class="truncate text-xs text-muted-foreground leading-tight">
              {{ store.isAdmin ? t('roleAdmin') : t('roleUser') }}
            </p>
          </div>
        </div>
        <ChevronsUpDown class="size-3.5 shrink-0 text-muted-foreground" />
      </UserMenu>
    </div>
  </aside>
</template>
