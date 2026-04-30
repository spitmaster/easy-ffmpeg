<script setup lang="ts">
import { computed } from 'vue'
import { useEditorStore } from '@/stores/editor'
import { totalDuration } from '@/utils/timeline'

const store = useEditorStore()

const props = defineProps<{
  // While an export is running we lock the entire top bar so the user
  // can't open a different project, rename mid-stream, or re-trigger
  // the export dialog. Cancelling lives in the right-hand sidebar.
  locked?: boolean
}>()

const emit = defineEmits<{
  (e: 'open-video'): void
  (e: 'open-projects'): void
  (e: 'open-export'): void
}>()

const exportDisabled = computed(
  () => props.locked || !store.project || totalDuration(store.project) <= 0,
)
const projectName = computed({
  get: () => store.project?.name || '',
  set: (v: string) => {
    if (!store.project) return
    store.applyProjectPatch({ name: v })
  },
})
</script>

<template>
  <div class="flex items-center gap-2 border-b border-border-base bg-bg-elevated px-3 py-2">
    <button
      class="rounded border border-border-strong bg-bg-base px-3 py-1.5 text-xs hover:bg-bg-panel disabled:opacity-50"
      :disabled="locked"
      @click="emit('open-video')"
    >📂 打开视频</button>
    <button
      class="rounded border border-border-strong bg-bg-base px-3 py-1.5 text-xs hover:bg-bg-panel disabled:opacity-50"
      :disabled="locked"
      @click="emit('open-projects')"
    >📋 剪辑记录</button>
    <input
      v-model="projectName"
      type="text"
      placeholder="工程名"
      class="flex-1 rounded border border-border-strong bg-bg-base px-2 py-1.5 text-xs"
      :class="{ 'border-accent': store.dirty }"
      :disabled="!store.project || locked"
    />
    <button
      class="rounded bg-accent px-4 py-1.5 text-xs text-bg-base hover:bg-accent-hover disabled:opacity-50"
      :disabled="exportDisabled"
      @click="emit('open-export')"
    >导出</button>
  </div>
</template>
