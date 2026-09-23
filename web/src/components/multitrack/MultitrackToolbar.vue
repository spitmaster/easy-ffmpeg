<script setup lang="ts">
import { computed } from 'vue'
import { useMultitrackStore } from '@/stores/multitrack'
import { useMultitrackOps } from '@/composables/useMultitrackOps'

/**
 * Bottom edit toolbar for the multitrack timeline. Mirrors EditorToolbar
 * (split / delete / undo / redo / scope label / zoom) but layered over
 * the multitrack store so the scope label can show "all / video / audio /
 * track:<id>" and the actions hit useMultitrackOps. Also hosts the
 * +video / +audio track buttons so creation lives next to the tracks.
 */

const store = useMultitrackStore()
const ops = useMultitrackOps()

const PX_MIN = 0.05
const PX_MAX = 80

const deleteDisabled = computed(() => !store.selection.length && !store.rangeSelection)

const scopeLabel = computed(() => {
  const s = store.splitScope
  if (s === 'all') return '当前分割范围:全部轨道'
  if (s === 'video') return '当前分割范围:全部视频轨'
  if (s === 'audio') return '当前分割范围:全部音频轨'
  // single track — surface a human label so the user knows which one
  const p = store.project
  if (!p) return '当前分割范围:单条轨道'
  const vIdx = p.videoTracks.findIndex((t) => t.id === s.id)
  if (vIdx >= 0) return `当前分割范围:视频 ${vIdx + 1}`
  const aIdx = p.audioTracks.findIndex((t) => t.id === s.id)
  if (aIdx >= 0) return `当前分割范围:音频 ${aIdx + 1}`
  return `当前分割范围:轨道 ${s.id}`
})

const zoom = computed({
  get: () => store.pxPerSecond,
  set: (v: number) => {
    store.pxPerSecond = Math.max(PX_MIN, Math.min(PX_MAX, v))
  },
})

function onAddVideoTrack() {
  if (!store.project) return
  store.addVideoTrack()
}

function onAddAudioTrack() {
  if (!store.project) return
  store.addAudioTrack()
}
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
    <span class="mx-1 h-4 w-px bg-border-base"></span>
    <button
      class="rounded border border-border-strong bg-bg-base px-2 py-1 hover:bg-bg-elevated disabled:opacity-40"
      :disabled="!store.project || store.exportLocked"
      @click="onAddVideoTrack"
    >+ 视频轨</button>
    <button
      class="rounded border border-border-strong bg-bg-base px-2 py-1 hover:bg-bg-elevated disabled:opacity-40"
      :disabled="!store.project || store.exportLocked"
      @click="onAddAudioTrack"
    >+ 音频轨</button>
  </div>
</template>
