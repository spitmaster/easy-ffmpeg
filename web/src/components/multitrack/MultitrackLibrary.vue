<script setup lang="ts">
import { ref } from 'vue'
import type { MultitrackSource } from '@/api/multitrack'
import MultitrackLibraryItem from './MultitrackLibraryItem.vue'

/**
 * Left-rail source library. The view layer owns import / remove / add
 * actions — the library is dumb so we can swap it for a sidebar /
 * drawer / floating panel later without touching multitrack data flow.
 */
const props = defineProps<{
  sources: MultitrackSource[]
  /** Disable the import button while a probe is in flight. */
  importing?: boolean
  /** Disable per-item add button while exporting. */
  addDisabled?: boolean
}>()

const emit = defineEmits<{
  (e: 'import'): void
  (e: 'remove', sourceId: string): void
  (e: 'add', sourceId: string): void
  (e: 'collapse'): void
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
  <!-- Region: LIBRARY (cols 1–3 of 15-col body grid ≈ 20% width).
       See docs/tabs/multitrack/product.md §3.1.1.
       min-w-0 is critical: flex items default to min-width:auto, which
       refuses to shrink below the intrinsic min-content width of the
       header row (素材库 + 导入按钮 + 折叠按钮) and library items, so
       without it the aside gets stuck at ~160px when the viewport
       shrinks. overflow-hidden clips header content when basis drops
       below intrinsic width. -->
  <aside class="flex h-full min-w-0 grow-0 basis-[20%] flex-col overflow-hidden border-r border-border-base bg-bg-panel">
    <div class="flex shrink-0 items-center gap-2 border-b border-border-base px-3 py-2">
      <span class="text-xs font-medium text-fg-base">素材库</span>
      <button
        class="ml-auto rounded border border-border-strong bg-bg-base px-2 py-0.5 text-[11px] hover:bg-bg-elevated disabled:cursor-not-allowed disabled:opacity-50"
        :disabled="props.importing"
        @click="onImport"
      >+ 导入</button>
      <button
        class="rounded px-1.5 py-0.5 text-[11px] text-fg-muted hover:bg-bg-elevated hover:text-fg-base"
        title="收起素材库 (Ctrl+L)"
        @click="emit('collapse')"
      >«</button>
    </div>

    <div class="flex-1 overflow-y-auto p-2">
      <div v-if="props.sources.length === 0" class="px-1 py-3 text-[11px] leading-relaxed text-fg-muted">
        暂无素材。<br />
        点击"导入"选择视频或音频文件,然后点每个素材右上角的"+ 添加"按钮加到时间轴。
      </div>
      <div v-else class="flex flex-col gap-1.5">
        <MultitrackLibraryItem
          v-for="s in props.sources"
          :key="s.id"
          :source="s"
          :disabled="props.addDisabled"
          @remove="emit('remove', s.id)"
          @add="emit('add', s.id)"
        />
      </div>
    </div>

    <div v-if="lastError" class="shrink-0 border-t border-border-base bg-danger/10 px-3 py-1.5 text-[11px] text-danger">
      {{ lastError }}
    </div>
  </aside>
</template>
