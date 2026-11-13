<script setup>
import { useRouter } from 'vue-router'
import { api } from '@/api'
import { store } from '@/store'
import { t } from '@/i18n'
import { LogOut, Settings, SlidersHorizontal, Languages, Check } from '@lucide/vue'
import {
  DropdownMenu,
  DropdownMenuTrigger,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuGroup,
} from '@/components/ui/dropdown-menu'
import { availableLangs, lang, setLang } from '@/i18n'

const router = useRouter()

async function logout() {
  await api.post('/api/v1/auth/logout').catch(() => {})
  store.user = null
  location.href = '/'
}

function go(to) {
  router.push(to)
}
</script>

<template>
  <DropdownMenu>
    <DropdownMenuTrigger class="flex items-center gap-2 rounded-md px-2 py-1.5 text-sm transition-colors hover:bg-accent data-[state=open]:bg-accent">
      <slot />
    </DropdownMenuTrigger>
    <DropdownMenuContent align="end" side="top" class="w-56">
      <DropdownMenuLabel class="flex items-center justify-between">
        <span class="truncate">{{ store.user?.email || store.user?.username }}</span>
      </DropdownMenuLabel>
      <DropdownMenuSeparator />
      <DropdownMenuGroup>
        <DropdownMenuItem @select="go('/settings')">
          <Settings class="size-4" />
          {{ t('navSettings') }}
        </DropdownMenuItem>
        <DropdownMenuItem v-if="store.isAdmin" @select="go('/admin')">
          <SlidersHorizontal class="size-4" />
          {{ t('navAdmin') }}
        </DropdownMenuItem>
      </DropdownMenuGroup>
      <DropdownMenuSeparator />
      <DropdownMenuLabel class="text-xs text-muted-foreground">{{ t('language') }}</DropdownMenuLabel>
      <DropdownMenuGroup>
        <DropdownMenuItem
          v-for="l in availableLangs" :key="l.key"
          @select="setLang(l.key)"
        >
          <Languages class="size-4" />
          <span class="flex-1">{{ l.label }}</span>
          <Check v-if="lang === l.key" class="size-4" />
        </DropdownMenuItem>
      </DropdownMenuGroup>
      <DropdownMenuSeparator />
      <DropdownMenuItem variant="destructive" @select="logout">
        <LogOut class="size-4" />
        {{ t('logout') }}
      </DropdownMenuItem>
    </DropdownMenuContent>
  </DropdownMenu>
</template>
