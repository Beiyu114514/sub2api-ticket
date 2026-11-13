<script setup>
import { ref, computed } from 'vue'
import { t } from '@/i18n'
import { toast } from 'vue-sonner'
import { ImagePlus, Loader2, X } from '@lucide/vue'
import { cn } from '@/lib/utils'

// 多图上传：选择 / 预览 / 移除，上传完成后向父组件回传 Attachment[]
const props = defineProps({
  modelValue: { type: Array, default: () => [] },
  max: { type: Number, default: 5 },
  disabled: { type: Boolean, default: false },
})
const emit = defineEmits(['update:modelValue'])

const uploading = ref(false)
const dragOver = ref(false)
const uploadProgress = ref({ done: 0, total: 0 })
const inputEl = ref(null)

const list = computed(() => props.modelValue || [])
const canAdd = computed(() => !props.disabled && !uploading.value && list.value.length < props.max)
const hintId = `upload-hint-${Math.random().toString(36).slice(2, 8)}`

async function uploadFiles(files) {
  const picked = [...files].filter(f => f.type.startsWith('image/'))
  if (!picked.length) return
  if (list.value.length + picked.length > props.max) {
    toast.error(t('maxImages', { n: props.max }))
    return
  }

  uploading.value = true
  uploadProgress.value = { done: 0, total: picked.length }
  const added = []
  try {
    for (const f of picked) {
      const fd = new FormData()
      fd.append('file', f)
      const res = await fetch('/api/v1/uploads', {
        method: 'POST', body: fd, credentials: 'same-origin',
      })
      const data = await res.json().catch(() => ({}))
      if (!res.ok) throw new Error(data.error || `HTTP ${res.status}`)
      added.push(data.attachment)
      uploadProgress.value = { ...uploadProgress.value, done: uploadProgress.value.done + 1 }
    }
    emit('update:modelValue', [...list.value, ...added])
  } catch (ex) {
    toast.error(ex.message)
    if (added.length) emit('update:modelValue', [...list.value, ...added])
  } finally {
    uploading.value = false
  }
}

function onPick(e) {
  const files = e.target.files
  e.target.value = ''
  if (files?.length) uploadFiles(files)
}

function onDrop(e) {
  dragOver.value = false
  if (!canAdd.value) return
  const files = e.dataTransfer?.files
  if (files?.length) uploadFiles(files)
}

function removeAt(i) {
  const next = [...list.value]
  next.splice(i, 1)
  emit('update:modelValue', next)
}
</script>

<template>
  <div class="w-full space-y-2">
    <!-- 已有附件预览 -->
    <div v-if="list.length" class="flex flex-wrap gap-2">
      <div
        v-for="(a, i) in list" :key="a.url || i"
        class="group relative size-20 overflow-hidden rounded-lg border bg-muted"
      >
        <img :src="a.url" class="size-full object-cover" :alt="t('attachmentAlt')" />
        <button
          v-if="!disabled" type="button"
          class="absolute inset-0 flex items-center justify-center bg-background/70 opacity-0 backdrop-blur-[1px] transition-opacity group-hover:opacity-100"
          :aria-label="t('removeImage')"
          @click="removeAt(i)"
        >
          <span class="flex size-6 items-center justify-center rounded-full bg-destructive text-destructive-foreground">
            <X class="size-3.5" />
          </span>
        </button>
      </div>
    </div>

    <!-- 上传入口：拖拽区 -->
    <div
      :class="cn(
        'flex flex-col items-center justify-center gap-1.5 rounded-lg border border-dashed px-4 py-5 text-center transition-colors',
        canAdd ? 'cursor-pointer hover:border-primary/40 hover:bg-accent/40' : 'cursor-not-allowed opacity-60',
        dragOver && 'border-primary bg-primary/5',
      )"
      role="button"
      :tabindex="canAdd ? 0 : -1"
      :aria-describedby="hintId"
      @click="canAdd && inputEl.click()"
      @keydown.enter.prevent="canAdd && inputEl.click()"
      @keydown.space.prevent="canAdd && inputEl.click()"
      @dragover.prevent="canAdd && (dragOver = true)"
      @dragleave.prevent="dragOver = false"
      @drop.prevent="onDrop"
    >
      <div class="flex items-center gap-2 text-sm font-medium text-muted-foreground">
        <Loader2 v-if="uploading" class="size-4 animate-spin" />
        <ImagePlus v-else class="size-4" />
        <span v-if="uploading">
          {{ t('uploading') }} {{ uploadProgress.done }}/{{ uploadProgress.total }}
        </span>
        <span v-else>{{ t('addImage') }}</span>
      </div>
      <p :id="hintId" class="text-xs text-muted-foreground/80">
        {{ t('dropHint') }} · {{ t('maxImages', { n: max }) }}
      </p>
    </div>

    <input
      ref="inputEl" type="file" accept="image/jpeg,image/png,image/webp,image/gif"
      multiple class="hidden" @change="onPick"
    />
  </div>
</template>
