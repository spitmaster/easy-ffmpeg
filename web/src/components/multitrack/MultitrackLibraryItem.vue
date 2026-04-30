<script setup lang="ts">
import { computed } from 'vue'
import type { MultitrackSource } from '@/api/multitrack'
import { Path } from '@/utils/path'

/**
 * One source card in the library: kind icon + name + duration + dimensions.
 * The card is `draggable=true`; the parent doesn't need to know about
 * native drag — we set the `dataTransfer` payload right here.
 */
const props = defineProps<{
  source: MultitrackSource
}>()

defineEmits<{
  (e: 'remove'): void
}>()

const baseName = computed(() => Path.basename(props.source.path) || props.source.path)

const dims = computed(() => {
  const s = props.source
  if (s.kind === 'video' && s.width && s.height) return `${s.width}×${s.height}`
  return ''
})

const durLabel = computed(() => fmtDur(props.source.duration))

const icon = computed(() => (props.source.kind === 'video' ? '🎬' : '🔊'))

function onDragStart(ev: DragEvent) {
  if (!ev.dataTransfer) return
  ev.dataTransfer.effectAllowed = 'copy'
  // Custom mime type so internal drops can identify our payload while
  // ignoring foreign drags (e.g. files dragged in from the OS shell).
  ev.dataTransfer.setData(
    'application/x-easy-ffmpeg-source',
    JSON.stringify({ sourceId: props.source.id }),
  )
}

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
    class="group flex cursor-grab items-start gap-2 rounded border border-border-base bg-bg-base px-2 py-1.5 hover:border-border-strong"
    :title="source.path"
    draggable="true"
    @dragstart="onDragStart"
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
    <button
      class="invisible shrink-0 rounded px-1 text-[10px] text-fg-muted hover:bg-bg-elevated hover:text-fg-base group-hover:visible"
      title="从工程移除此素材"
      @click.stop="$emit('remove')"
    >×</button>
  </div>
</template>
