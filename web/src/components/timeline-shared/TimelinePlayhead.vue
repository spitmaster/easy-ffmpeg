<script setup lang="ts">
/**
 * Vertical line indicating program time. Pure presentation — the parent
 * computes x (in px) from the playhead value and decides which slice of
 * the timeline (full height vs single-track) to render this for.
 *
 * `top` and `bottom` are CSS values (e.g. '0', '7px', '76px'); `bottom`
 * defaults to '0' so the line spans to the bottom of the parent.
 */
defineProps<{
  /** X position in px from the left edge of the timeline. */
  x: number
  /** CSS top — string so callers can pass '0', '28px', etc. */
  top?: string
  /** Optional explicit height; takes precedence over bottom. */
  height?: string
}>()

defineEmits<{
  (e: 'mousedown', ev: MouseEvent): void
}>()
</script>

<template>
  <div
    class="pointer-events-auto absolute z-20 w-px cursor-ew-resize bg-danger"
    :style="{
      left: x + 'px',
      top: top ?? '0',
      height: height ?? undefined,
      bottom: height ? undefined : '0',
    }"
    @mousedown="$emit('mousedown', $event)"
  ></div>
</template>
