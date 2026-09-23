<script setup lang="ts">
import { computed } from 'vue'
import TimelineClip from './TimelineClip.vue'
import type { Clip, TrackData, TrackTone } from '@/types/timeline'

/**
 * One track row: a horizontal lane of clips. Pure presentation — owns no
 * drag/select logic itself, all interactions emit a single 'mousedown'
 * carrying enough metadata for the parent to decide between scrub-on-empty
 * vs select-clip vs trim-handle vs reorder.
 *
 * Height defaults to 48px (h-12) for video and 56px (h-14) for audio in
 * the single-video editor; multitrack will probably keep them uniform.
 * Pass `heightClass` to override.
 */
const props = defineProps<{
  track: TrackData
  pxPerSecond: number
  trackWidth: number
  /** Selected clip ids on this track (used to highlight). */
  selectedIds: string[]
  /** Defaults from the track if omitted; falls back to accent/success. */
  tone?: TrackTone
  /** Tailwind height utility, e.g. 'h-12' or 'h-14'. Default: 'h-12'. */
  heightClass?: string
  /** Add a top border (matches the legacy audio-row separator). */
  topBorder?: boolean
}>()

/**
 * Mousedown on the row. clipId is set when the press landed on a clip;
 * handle is set when it landed on the left/right resize handle.
 */
const emit = defineEmits<{
  (e: 'mousedown', payload: { ev: MouseEvent; clipId?: string; handle?: 'left' | 'right' }): void
}>()

const tone = computed<TrackTone>(() => {
  if (props.tone) return props.tone
  if (props.track.tone) return props.track.tone
  return props.track.kind === 'video' ? 'accent' : 'success'
})

const heightClass = computed(() => props.heightClass ?? 'h-12')

const isSelected = (c: Clip) => props.selectedIds.includes(c.id)

function onMouseDown(ev: MouseEvent) {
  // The clip + handle decision is data-attribute driven so the row stays
  // dumb regardless of how many clips it holds.
  const target = ev.target as HTMLElement
  const clipEl = target.closest('.clip') as HTMLElement | null
  if (!clipEl) {
    emit('mousedown', { ev })
    return
  }
  const clipId = clipEl.dataset.clipId || ''
  const handleEl = target.closest('.clip-handle') as HTMLElement | null
  const handle = handleEl ? (handleEl.dataset.handle as 'left' | 'right') : undefined
  emit('mousedown', { ev, clipId, handle })
}
</script>

<template>
  <div
    class="relative select-none"
    :class="[heightClass, topBorder ? 'border-t border-border-base' : '', track.kind === 'video' ? 'border-b border-border-base' : '']"
    :style="{ width: trackWidth + 'px' }"
    @mousedown="onMouseDown"
  >
    <TimelineClip
      v-for="c in track.clips"
      :key="c.id"
      :clip="c"
      :px-per-second="pxPerSecond"
      :selected="isSelected(c)"
      :tone="tone"
    />
  </div>
</template>
