<script setup lang="ts">
import { computed } from 'vue'
import type { MultitrackSource } from '@/api/multitrack'
import { Path } from '@/utils/path'

/**
 * One source card in the library: kind icon + name + duration + dimensions.
 * Click "+ 添加" to append the source as new tracks (video → 1 video + 1
 * audio if hasAudio; audio → 1 audio). The drag-to-timeline path was
 * removed because it stacked clips on existing tracks unintuitively.
 */
const props = defineProps<{
  source: MultitrackSource
  /** True while a previous add is still in flight or while exporting. */
  disabled?: boolean
}>()

defineEmits<{
  (e: 'remove'): void
  (e: 'add'): void
}>()

const baseName = computed(() => Path.basename(props.source.path) || props.source.path)

const dims = computed(() => {
  const s = props.source
  if (s.kind === 'video' && s.width && s.height) return `${s.width}×${s.height}`
  return ''
})

const durLabel = computed(() => fmtDur(props.source.duration))

const icon = computed(() => (props.source.kind === 'video' ? '🎬' : '🔊'))

function fmtDur(sec: number): string {
  if (!Number.isFinite(sec) || sec < 0) return '—'
  const total = Math.round(sec)
  const h = Math.floor(total / 3600)
  const m = Math.floor((total % 3600) / 60)
  const s = total % 60
  if (h > 0) return `${h}:${String(m).padStart(2, '0')}:${String(s).padStart(2, '0')}`
  return `${m}:${String(s).padStart(2, '0')}`
}
</script>

<template>
  <div
    class="group flex items-start gap-2 rounded border border-border-base bg-bg-base px-2 py-1.5 hover:border-border-strong"
    :title="source.path"
  >
    <div class="text-base leading-5">{{ icon }}</div>
    <div class="min-w-0 flex-1">
      <div class="truncate text-xs text-fg-base">{{ baseName }}</div>
      <div class="mt-0.5 flex gap-2 text-[10px] text-fg-muted">
        <span>{{ durLabel }}</span>
        <span v-if="dims">{{ dims }}</span>
        <span v-if="source.kind === 'audio'">音频</span>
      </div>
    </div>
    <div class="flex shrink-0 flex-col items-end gap-1">
      <button
        class="rounded border border-accent bg-bg-base px-1.5 py-0.5 text-[10px] text-accent hover:bg-accent hover:text-bg-base disabled:cursor-not-allowed disabled:opacity-40"
        title="添加为新轨道(视频→新建视频轨+音频轨；音频→新建音频轨)"
        :disabled="props.disabled"
        @click.stop="$emit('add')"
      >+ 添加</button>
      <button
        class="invisible rounded px-1 text-[10px] text-fg-muted hover:bg-bg-elevated hover:text-fg-base group-hover:visible"
        title="从工程移除此素材"
        @click.stop="$emit('remove')"
      >×</button>
    </div>
  </div>
</template>
