<script setup>
import { computed } from 'vue'
import { t } from '@/i18n'
import { cn } from '@/lib/utils'
import { ArrowDown, Minus, ArrowUp } from '@lucide/vue'

const props = defineProps({
  priority: { type: String, default: 'normal' },
  class: { type: String, default: '' },
})

const meta = computed(() => ({
  low: { label: t('prio_low'), icon: ArrowDown, cls: 'text-muted-foreground' },
  normal: { label: t('prio_normal'), icon: Minus, cls: 'text-muted-foreground' },
  high: { label: t('prio_high'), icon: ArrowUp, cls: 'text-destructive' },
}[props.priority] || { label: props.priority, icon: Minus, cls: 'text-muted-foreground' }))

const icon = computed(() => meta.value.icon)
</script>

<template>
  <span :class="cn('inline-flex items-center gap-1 text-xs font-medium', meta.cls, props.class)">
    <component :is="icon" class="size-3.5" />
    {{ meta.label }}
  </span>
</template>
