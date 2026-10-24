<script setup>
import { ref, computed } from 'vue'
import { t } from '../i18n'

// 多图上传组件：选择/预览/移除，上传后向父组件输出 Attachment 对象数组
const props = defineProps({
  modelValue: { type: Array, default: () => [] }, // Attachment[]
  max: { type: Number, default: 5 },
  disabled: { type: Boolean, default: false },
})
const emit = defineEmits(['update:modelValue'])

const uploading = ref(false)
const err = ref('')
const inputEl = ref(null)

const list = computed(() => props.modelValue || [])

async function onPick(e) {
  const files = [...e.target.files]
  e.target.value = ''
  if (!files.length) return
  if (list.value.length + files.length > props.max) {
    err.value = `最多 ${props.max} 张图片`
    return
  }
  err.value = ''
  uploading.value = true
  const added = []
  try {
    for (const f of files) {
      const fd = new FormData()
      fd.append('file', f)
      const res = await fetch('/api/v1/uploads', {
        method: 'POST', body: fd, credentials: 'same-origin',
      })
      const data = await res.json().catch(() => ({}))
      if (!res.ok) throw new Error(data.error || `HTTP ${res.status}`)
      added.push(data.attachment)
    }
    emit('update:modelValue', [...list.value, ...added])
  } catch (ex) {
    err.value = ex.message
  } finally {
    uploading.value = false
  }
}

function removeAt(i) {
  const next = [...list.value]
  next.splice(i, 1)
  emit('update:modelValue', next)
}
</script>

<template>
  <div class="w-full">
    <div v-if="list.length" class="flex flex-wrap gap-2 mb-2">
      <div v-for="(a, i) in list" :key="a.url"
        class="relative w-16 h-16 rounded-lg overflow-hidden border border-slate-200 dark:border-slate-700 group">
        <img :src="a.url" class="w-full h-full object-cover" alt="" />
        <button v-if="!disabled" type="button"
          class="absolute inset-0 hidden group-hover:flex items-center justify-center bg-black/50 text-white text-xs"
          @click="removeAt(i)">✕</button>
      </div>
    </div>
    <div class="flex items-center gap-2">
      <button type="button" class="btn-ghost !py-1.5 text-xs border border-slate-200 dark:border-slate-700"
        :disabled="disabled || uploading || list.length >= max" @click="inputEl.click()">
        {{ uploading ? '上传中…' : '📷 添加图片' }}
      </button>
      <span v-if="err" class="text-xs text-rose-500">{{ err }}</span>
    </div>
    <input ref="inputEl" type="file" accept="image/jpeg,image/png,image/webp,image/gif"
      multiple class="hidden" @change="onPick" />
  </div>
</template>
