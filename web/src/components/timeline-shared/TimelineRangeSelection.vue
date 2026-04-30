<script setup lang="ts">
import { computed } from 'vue'
import type { RangeSelection } from '@/types/timeline'

/**
 * Translucent yellow (accent) overlay for a [start,end] program-time
 * selection. Pure presentation — parent passes the range and pxPerSecond,
 * the component renders nothing when range is null.
 */
const props = defineProps<{
  range: RangeSelection | null
  pxPerSecond: number
}>()

const rect = computed(() => {
  if (!props.range) return null
  const a = Math.min(props.range.start, props.range.end)
  const b = Math.max(props.range.start, props.range.end)
  return {
    left: a * props.pxPerSecond,
    width: Math.max(1, (b - a) * props.pxPerSecond),
  }
})
</script>

<template>
  <div
    v-if="rect"
    class="pointer-events-none absolute top-0 bottom-0 z-10 bg-accent/15 ring-1 ring-accent/50"
    :style="{ left: rect.left + 'px', width: rect.width + 'px' }"
  ></div>
</template>
