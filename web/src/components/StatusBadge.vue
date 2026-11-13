<script setup>
import { computed } from 'vue'
import { t } from '@/i18n'
import { Badge } from '@/components/ui/badge'
import { cn } from '@/lib/utils'
import { CircleDot, Clock, CircleCheck } from '@lucide/vue'

const props = defineProps({
  status: { type: String, default: 'open' },
  class: { type: String, default: '' },
  showIcon: { type: Boolean, default: true },
})

const meta = computed(() => ({
  open: {
    label: t('status_open'),
    icon: CircleDot,
    cls: 'border-info/25 bg-info/10 text-info',
    dot: 'bg-info',
  },
  pending: {
    label: t('status_pending'),
    icon: Clock,
    cls: 'border-warning/30 bg-warning/10 text-warning',
    dot: 'bg-warning',
  },
  closed: {
    label: t('status_closed'),
    icon: CircleCheck,
    cls: 'border-border bg-muted text-muted-foreground',
    dot: 'bg-muted-foreground',
  },
}[props.status] || {
  label: props.status,
  icon: CircleDot,
  cls: 'border-border bg-muted text-muted-foreground',
  dot: 'bg-muted-foreground',
}))

const icon = computed(() => meta.value.icon)
</script>

<template>
  <Badge variant="outline" :class="cn(meta.cls, 'gap-1 font-medium', props.class)">
    <component :is="icon" v-if="showIcon" class="size-3" />
    <span v-else :class="cn('size-1.5 rounded-full', meta.dot)" />
    {{ meta.label }}
  </Badge>
</template>
