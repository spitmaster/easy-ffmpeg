import type { Ref } from 'vue'
import type { RangeSelection } from '@/types/timeline'

/**
 * Right-drag-on-ruler range selection. Drag installs document-level
 * mousemove/mouseup handlers; on release a sub-50ms (in seconds, i.e. less
 * than ~50px at typical zoom) drag cancels the range. The composable
 * doesn't own the range state — caller passes a setter so single-video
 * and multitrack stores can each track their own.
 */
export interface TimelineRangeSelectOptions {
  rulerEl: Ref<HTMLElement | null>
  pxPerSecond: () => number
  totalSec: () => number
  setRange: (r: RangeSelection | null) => void
  /** Called once when the drag starts. Use to clear selection / split scope. */
  onStart?: () => void
}

const MIN_RANGE_SEC = 0.05

export function useTimelineRangeSelect(opts: TimelineRangeSelectOptions) {
  function clientXToTime(clientX: number, clamp = true): number {
    const r = opts.rulerEl.value
    if (!r) return 0
    const rect = r.getBoundingClientRect()
    const x = clientX - rect.left
    const t = x / opts.pxPerSecond()
    if (!clamp) return t
    return Math.max(0, Math.min(opts.totalSec(), t))
  }

  function start(ev: MouseEvent) {
    ev.preventDefault()
    if (opts.totalSec() <= 0) return
    if (opts.onStart) opts.onStart()
    const anchor = clientXToTime(ev.clientX)
    let lastEnd = anchor
    opts.setRange({ start: anchor, end: anchor })

    function onMove(e: MouseEvent) {
      lastEnd = clientXToTime(e.clientX)
      opts.setRange({ start: anchor, end: lastEnd })
    }
    function onUp() {
      document.removeEventListener('mousemove', onMove)
      document.removeEventListener('mouseup', onUp)
      const a = Math.min(anchor, lastEnd)
      const b = Math.max(anchor, lastEnd)
      if (b - a < MIN_RANGE_SEC) opts.setRange(null)
      else opts.setRange({ start: a, end: b })
    }

    document.addEventListener('mousemove', onMove)
    document.addEventListener('mouseup', onUp)
  }

  return { start, clientXToTime, MIN_RANGE_SEC }
}
