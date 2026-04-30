import type { Ref } from 'vue'

/**
 * Zoom and horizontal-scroll handling for a timeline. The composable owns
 * neither the px-per-second value nor the total duration — both are passed
 * by reference so single-video and multitrack share the same wheel/fit
 * semantics over their own state.
 *
 *   onWheel — Ctrl/Cmd + wheel zooms around the cursor (cursor stays put);
 *             plain vertical wheel scrolls the timeline horizontally.
 *   applyFit — fit the entire program duration into the viewport width;
 *             clamps to [pxMin, pxMax].
 */
export interface TimelineZoomOptions {
  pxPerSecond: Ref<number>
  totalSec: () => number
  scrollEl: Ref<HTMLElement | null>
  pxMin?: number
  pxMax?: number
}

const DEFAULT_PX_MIN = 0.05
const DEFAULT_PX_MAX = 80
const DEFAULT_FALLBACK = 8

export function useTimelineZoom(opts: TimelineZoomOptions) {
  const pxMin = opts.pxMin ?? DEFAULT_PX_MIN
  const pxMax = opts.pxMax ?? DEFAULT_PX_MAX

  function applyFit() {
    const scroll = opts.scrollEl.value
    if (!scroll) return
    const total = opts.totalSec()
    if (total <= 0) {
      opts.pxPerSecond.value = DEFAULT_FALLBACK
      return
    }
    const viewW = Math.max(100, scroll.clientWidth - 24)
    opts.pxPerSecond.value = Math.max(pxMin, Math.min(pxMax, viewW / total))
  }

  function onWheel(ev: WheelEvent) {
    const scroll = opts.scrollEl.value
    if (!scroll) return
    if (ev.ctrlKey || ev.metaKey) {
      ev.preventDefault()
      if (opts.totalSec() <= 0) return
      const rect = scroll.getBoundingClientRect()
      const anchorX = ev.clientX - rect.left + scroll.scrollLeft
      const anchorTime = anchorX / opts.pxPerSecond.value
      const factor = Math.exp(-ev.deltaY * 0.0015)
      const next = Math.max(pxMin, Math.min(pxMax, opts.pxPerSecond.value * factor))
      opts.pxPerSecond.value = next
      // Keep the time under the cursor stationary on screen.
      const newAnchorX = anchorTime * next
      scroll.scrollLeft = newAnchorX - (ev.clientX - rect.left)
    } else if (ev.deltaY !== 0 && ev.deltaX === 0) {
      ev.preventDefault()
      scroll.scrollLeft += ev.deltaY
    }
  }

  return { applyFit, onWheel }
}
