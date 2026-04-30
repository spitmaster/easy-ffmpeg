<script setup lang="ts">
import { ref } from 'vue'
import type { MultitrackSource } from '@/api/multitrack'
import MultitrackLibraryItem from './MultitrackLibraryItem.vue'

/**
 * Left-rail source library. The view layer owns import / remove
 * actions — the library is dumb so we can swap it for a sidebar /
 * drawer / floating panel later without touching multitrack data flow.
 */
const props = defineProps<{
  sources: MultitrackSource[]
  /** Disable the import button while a probe is in flight. */
  importing?: boolean
}>()

const emit = defineEmits<{
  (e: 'import'): void
  (e: 'remove', sourceId: string): void
}>()

const lastError = ref('')

defineExpose({
  setError(msg: string) {
    lastError.value = msg
  },
})

function onImport() {
  lastError.value = ''
  emit('import')
}
</script>

<template>
  <aside class="flex h-full w-60 shrink-0 flex-col border-r border-border-base bg-bg-panel">
    <div class="flex shrink-0 items-center gap-2 border-b border-border-base px-3 py-2">
      <span class="text-xs font-medium text-fg-base">素材库</span>
      <button
        class="ml-auto rounded border border-border-strong bg-bg-base px-2 py-0.5 text-[11px] hover:bg-bg-elevated disabled:cursor-not-allowed disabled:opacity-50"
        :disabled="props.importing"
        @click="onImport"
      >+ 导入</button>
    </div>

    <div class="flex-1 overflow-y-auto p-2">
      <div v-if="props.sources.length === 0" class="px-1 py-3 text-[11px] leading-relaxed text-fg-muted">
        暂无素材。<br />
        点击"导入"选择视频或音频文件后，将文件拖到右侧时间轴空白处即可自动建轨。
      </div>
      <div v-else class="flex flex-col gap-1.5">
        <MultitrackLibraryItem
          v-for="s in props.sources"
          :key="s.id"
          :source="s"
          @remove="emit('remove', s.id)"
        />
      </div>
    </div>

    <div v-if="lastError" class="shrink-0 border-t border-border-base bg-danger/10 px-3 py-1.5 text-[11px] text-danger">
      {{ lastError }}
    </div>
  </aside>
</template>
