<script setup lang="ts">
import { computed, useTemplateRef } from 'vue'

/**
 * Time ruler — ticks + labels at zoom-aware intervals. The visible
 * range is [0, totalSec]. Steps grow from 50ms up to 4 hours so the
 * tick density stays roughly constant (~90px between labels) regardless
 * of zoom.
 *
 * Exposes the root <div> element via defineExpose so the parent can pass
 * it to useTimelineRangeSelect (which needs a DOM rect to convert
 * client-x → seconds).
 */
const props = defineProps<{
  pxPerSecond: number
  totalSec: number
  trackWidth: number
}>()

defineEmits<{
  (e: 'mousedown', ev: MouseEvent): void
}>()

const rootEl = useTemplateRef<HTMLDivElement>('rootEl')
defineExpose({ rootEl })

const STEPS = [
  0.05, 0.1, 0.2, 0.5, 1, 2, 5, 10, 15, 30, 60, 120, 300, 600, 900, 1800, 3600, 7200, 14400,
]
const TARGET_PX = 90

const tickStep = computed(() => {
  const ideal = TARGET_PX / props.pxPerSecond
  for (const s of STEPS) if (s >= ideal) return s
  return STEPS[STEPS.length - 1]
})

const ticks = computed(() => {
  const step = tickStep.value
  const count = Math.floor(props.totalSec / step) + 1
  const out: { t: number; x: number; label: string }[] = []
  for (let i = 0; i <= count; i++) {
    const t = i * step
    if (t > props.totalSec + 0.01) break
    out.push({ t, x: t * props.pxPerSecond, label: fmtTick(t, step) })
  }
  return out
})

function fmtTick(sec: number, step: number): string {
  const decimals = step >= 1 ? 0 : step >= 0.1 ? 1 : 2
  if (sec >= 3600) {
    const h = Math.floor(sec / 3600)
    const m = Math.floor((sec % 3600) / 60)
    const s = (sec % 60).toFixed(decimals)
    const pad = decimals > 0 ? decimals + 3 : 2
    return `${h}:${String(m).padStart(2, '0')}:${s.padStart(pad, '0')}`
  }
  const m = Math.floor(sec / 60)
  const s = (sec % 60).toFixed(decimals)
  const pad = decimals > 0 ? decimals + 3 : 2
  return `${m}:${s.padStart(pad, '0')}`
}
</script>

<template>
  <div
    ref="rootEl"
    class="relative h-7 cursor-pointer select-none border-b border-border-base bg-bg-panel"
    :style="{ width: trackWidth + 'px' }"
    @mousedown="$emit('mousedown', $event)"
  >
    <template v-for="t in ticks" :key="t.t">
      <div
        class="pointer-events-none absolute top-3 h-2 w-px bg-fg-subtle"
        :style="{ left: t.x + 'px' }"
      ></div>
      <div
        class="pointer-events-none absolute top-0.5 select-none whitespace-nowrap text-[10px] text-fg-muted"
        :style="{ left: t.x + 4 + 'px' }"
      >{{ t.label }}</div>
    </template>
  </div>
</template>
