<script setup lang="ts">
import type { Clip, TrackTone } from '@/types/timeline'

const props = defineProps<{
  clip: Clip
  pxPerSecond: number
  selected: boolean
  tone: TrackTone
}>()

defineEmits<{
  (e: 'mousedown', ev: MouseEvent): void
}>()

const TONE_CLASSES: Record<TrackTone, { border: string; body: string; hover: string; handle: string; handleHover: string }> = {
  accent:  { border: 'border-accent/60',  body: 'bg-accent/30',  hover: 'hover:bg-accent/50',  handle: 'bg-accent/70',  handleHover: 'hover:bg-accent' },
  success: { border: 'border-success/60', body: 'bg-success/30', hover: 'hover:bg-success/50', handle: 'bg-success/70', handleHover: 'hover:bg-success' },
  danger:  { border: 'border-danger/60',  body: 'bg-danger/30',  hover: 'hover:bg-danger/50',  handle: 'bg-danger/70',  handleHover: 'hover:bg-danger' },
}

function fmtShort(sec: number): string {
  const s = Math.round(sec)
  const m = Math.floor(s / 60)
  const ss = s % 60
  return `${m}:${ss.toString().padStart(2, '0')}`
}
</script>

<template>
  <div
    class="clip absolute top-1 bottom-1 cursor-grab overflow-hidden rounded border px-1 text-[10px] text-fg-base shadow-sm"
    :class="[TONE_CLASSES[tone].border, TONE_CLASSES[tone].body, TONE_CLASSES[tone].hover, { selected }]"
    :data-clip-id="clip.id"
    :style="{
      left: clip.programStart * pxPerSecond + 'px',
      width: Math.max(8, (clip.sourceEnd - clip.sourceStart) * pxPerSecond) + 'px',
    }"
    @mousedown="$emit('mousedown', $event)"
  >
    <span class="clip-label pointer-events-none block truncate">
      {{ fmtShort(clip.sourceStart) }} - {{ fmtShort(clip.sourceEnd) }}
    </span>
    <div
      class="clip-handle absolute inset-y-0 left-0 w-1.5 cursor-ew-resize"
      :class="[TONE_CLASSES[tone].handle, TONE_CLASSES[tone].handleHover]"
      data-handle="left"
    ></div>
    <div
      class="clip-handle absolute inset-y-0 right-0 w-1.5 cursor-ew-resize"
      :class="[TONE_CLASSES[tone].handle, TONE_CLASSES[tone].handleHover]"
      data-handle="right"
    ></div>
  </div>
</template>

<style scoped>
.clip.selected {
  @apply ring-2 ring-fg-base;
}
</style>
