<script setup lang="ts">
import AudioVolumePopover from './AudioVolumePopover.vue'

/**
 * One left-column label cell for a timeline track row. Renders icon +
 * label + (audio only) volume popover + optional delete-× button on a
 * single horizontal line. Shared between the single-video editor (1 video
 * + 1 audio, !removable) and the multitrack editor (N video + M audio,
 * removable) so the two views' track UIs stay row-for-row identical.
 *
 * Sibling on the right is `TimelineTrackRow` — match `heightClass` between
 * the two so labels line up with their lanes.
 */
const props = defineProps<{
  kind: 'video' | 'audio'
  label: string
  /** Audio-only: current volume (0–2). Omit for video rows. */
  volume?: number
  /** Show trailing delete-× button. Default false. */
  removable?: boolean
  /** Disable interactive elements (e.g. while exporting). */
  disabled?: boolean
  /** Tailwind height utility. Default 'h-12' (matches default TimelineTrackRow height). */
  heightClass?: string
  /** Enable the icon/label region as a drag handle for track reordering.
   *  Default false (single-video editor's two fixed tracks don't reorder). */
  reorderable?: boolean
  /** Visual cue while this row is the active drag source. */
  dragging?: boolean
}>()

const emit = defineEmits<{
  (e: 'update:volume', v: number): void
  (e: 'remove'): void
  /** Fires when the user mouse-downs the drag handle. Parent owns the
   *  mousemove/mouseup pipeline so it can cross-row hit-test against
   *  same-kind targets and apply the final reorder. */
  (e: 'reorder-mousedown', ev: MouseEvent): void
}>()

function onHandleMouseDown(ev: MouseEvent) {
  if (!props.reorderable || props.disabled) return
  if (ev.button !== 0) return
  emit('reorder-mousedown', ev)
}
</script>

<template>
  <div
    class="flex shrink-0 items-center gap-1 border-b border-border-base px-2 transition-opacity"
    :class="[heightClass ?? 'h-12', dragging ? 'opacity-40' : '']"
  >
    <span
      class="min-w-0 flex-1 truncate select-none"
      :class="reorderable && !disabled ? 'cursor-grab active:cursor-grabbing' : ''"
      :title="reorderable && !disabled ? '按住拖动可调整轨道顺序' : ''"
      @mousedown="onHandleMouseDown"
    >{{ kind === 'video' ? '🎬' : '🔊' }} {{ label }}</span>
    <div v-if="kind === 'audio' && volume !== undefined" class="shrink-0">
      <AudioVolumePopover
        :model-value="volume"
        @update:model-value="(v: number) => $emit('update:volume', v)"
      />
    </div>
    <button
      v-if="removable"
      class="shrink-0 rounded px-1 text-fg-muted hover:bg-bg-elevated hover:text-danger disabled:cursor-not-allowed disabled:opacity-40"
      title="删除该轨道"
      :disabled="disabled"
      @click.stop="$emit('remove')"
    >×</button>
  </div>
</template>
