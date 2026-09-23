<script setup lang="ts">
import { computed } from 'vue'
import { useEditorStore } from '@/stores/editor'
import { useEditorOps } from '@/composables/useEditorOps'

const store = useEditorStore()
const ops = useEditorOps()

const PX_MIN = 0.05
const PX_MAX = 80

const deleteDisabled = computed(() => !store.selection.length && !store.rangeSelection)

const scopeLabel = computed(() => {
  switch (store.splitScope) {
    case 'video': return '当前分割范围：视频轨'
    case 'audio': return '当前分割范围：音频轨'
    default: return '当前分割范围：两轨一起'
  }
})

const zoom = computed({
  get: () => store.pxPerSecond,
  set: (v: number) => {
    store.pxPerSecond = Math.max(PX_MIN, Math.min(PX_MAX, v))
  },
})
</script>

<template>
  <div class="flex items-center gap-2 border-t border-border-base bg-bg-panel px-3 py-1.5 text-xs">
    <button
      class="rounded border border-border-strong bg-bg-base px-2 py-1 hover:bg-bg-elevated"
      title="在播放头位置分割 (S)"
      @click="ops.splitAtPlayhead()"
    >✂ 分割</button>
    <button
      class="rounded border border-danger bg-bg-base px-2 py-1 text-danger hover:bg-danger/10 disabled:opacity-40"
      title="删除选中 (Del)"
      :disabled="deleteDisabled"
      @click="ops.deleteSelection()"
    >🗑 删除</button>
    <button
      class="rounded border border-border-strong bg-bg-base px-2 py-1 hover:bg-bg-elevated disabled:opacity-40"
      title="撤销 (Ctrl+Z)"
      :disabled="!store.canUndo"
      @click="store.undo()"
    >↶ 撤销</button>
    <button
      class="rounded border border-border-strong bg-bg-base px-2 py-1 hover:bg-bg-elevated disabled:opacity-40"
      title="重做 (Ctrl+Y)"
      :disabled="!store.canRedo"
      @click="store.redo()"
    >↷ 重做</button>
    <span class="text-fg-muted">{{ scopeLabel }}</span>
    <div class="flex-1"></div>
    <label class="flex items-center gap-2">
      缩放
      <input
        v-model.number="zoom"
        type="range"
        :min="PX_MIN"
        :max="PX_MAX"
        step="0.01"
        class="w-32"
        title="Ctrl+滚轮缩放，滚轮左右滚动"
      />
    </label>
  </div>
</template>
