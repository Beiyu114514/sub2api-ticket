<script setup>
import { computed } from 'vue'
import { useRoute } from 'vue-router'
import { store } from '@/store'
import { t } from '@/i18n'
import { useNavGroups } from '@/lib/nav'
import { cn } from '@/lib/utils'
import { initials } from '@/lib/format'
import { Ticket } from '@lucide/vue'
import { Avatar, AvatarFallback } from '@/components/ui/avatar'
import {
  Sheet, SheetContent, SheetHeader, SheetTitle, SheetDescription,
} from '@/components/ui/sheet'
import UserMenu from './UserMenu.vue'

const route = useRoute()
const groups = useNavGroups()

const open = computed({
  get: () => store.sidebarOpen,
  set: (v) => { store.sidebarOpen = v },
})

function isActive(to) {
  if (to === '/') return route.path === '/'
  return route.path === to || route.path.startsWith(`${to}/`)
}
</script>

<template>
  <Sheet v-model:open="open">
    <SheetContent side="left" class="w-72 p-0 lg:hidden">
      <SheetHeader class="flex h-14 flex-row items-center gap-2.5 space-y-0 border-b px-4">
        <div class="flex size-8 items-center justify-center rounded-lg bg-primary text-primary-foreground">
          <Ticket class="size-4" />
        </div>
        <div class="min-w-0 flex-1 pr-6 text-left">
          <SheetTitle class="truncate text-sm font-semibold leading-tight">{{ t('appName') }}</SheetTitle>
          <SheetDescription class="truncate text-xs leading-tight">{{ t('appTagline') }}</SheetDescription>
        </div>
      </SheetHeader>

      <nav class="flex-1 overflow-y-auto px-3 py-4">
        <div v-for="group in groups" :key="group.label" class="mb-6 last:mb-0">
          <p class="mb-1.5 px-2 text-xs font-medium text-muted-foreground">{{ group.label }}</p>
          <ul class="space-y-0.5">
            <li v-for="item in group.items" :key="item.to">
              <RouterLink
                :to="item.to"
                :class="cn(
                  'flex items-center gap-2.5 rounded-md px-2 py-2 text-sm transition-colors',
                  isActive(item.to)
                    ? 'bg-accent font-medium text-accent-foreground'
                    : 'text-muted-foreground hover:bg-accent/60 hover:text-accent-foreground',
                )"
                @click="open = false"
              >
                <component :is="item.icon" class="size-4 shrink-0" />
                {{ item.label }}
              </RouterLink>
            </li>
          </ul>
        </div>
      </nav>

      <div class="border-t p-3">
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
        </UserMenu>
      </div>
    </SheetContent>
  </Sheet>
</template>
