import type { Ref } from 'vue'

/**
 * Shared timeline mouse plumbing for single-video and multitrack views.
 *
 * Both views convert client-x to program time the same way, scrub the
 * playhead the same way, suppress the browser context menu the same way,
 * and gate ruler-mousedown on right-click for range selection. The pieces
 * that genuinely differ (which splitScope value to set, what selection to
 * clear, whether to honour an export lock) are passed in as callbacks so
 * the same body of code drives both views and a fix in scrub/range/lock
 * behaviour applies everywhere.
 */
export interface PreviewLike {
  play(): void
  pause(): void
  seek(t: number): void
}

export interface TimelineMouseHandlersOptions {
  rulerEl: Ref<HTMLElement | null>
  pxPerSecond: () => number
  totalSec: () => number
  isPlaying: () => boolean
  preview: PreviewLike
  /** Right-click on ruler → start range selection. Wired to rangeSelect.start. */
  onRangeStart: (ev: MouseEvent) => void
  /**
   * Called once at the start of a ruler-mousedown scrub (left-click), before
   * any drag movement. Typical body: clear selection, clear range, reset
   * splitScope to the "all tracks" value.
   */
  beforeScrubFromRuler: () => void
  /**
   * Called once at the start of a playhead-mousedown scrub. Typical body:
   * clear range only.
   */
  beforeScrubFromPlayhead: () => void
  /** Optional gate (e.g. multitrack's exportLocked). Defaults to never locked. */
  isLocked?: () => boolean
}

export function useTimelineMouseHandlers(opts: TimelineMouseHandlersOptions) {
  function clientXToTime(clientX: number, clamp = true): number {
    const r = opts.rulerEl.value
    if (!r) return 0
    const rect = r.getBoundingClientRect()
    const x = clientX - rect.left
    const t = x / opts.pxPerSecond()
    if (!clamp) return t
    return Math.max(0, Math.min(opts.totalSec(), t))
  }

  function startScrubDrag(ev: MouseEvent) {
    ev.preventDefault()
    const wasPlaying = opts.isPlaying()
    if (wasPlaying) opts.preview.pause()
    opts.preview.seek(clientXToTime(ev.clientX))
    function onMove(e: MouseEvent) {
      opts.preview.seek(clientXToTime(e.clientX))
    }
    function onUp() {
      document.removeEventListener('mousemove', onMove)
      document.removeEventListener('mouseup', onUp)
      if (wasPlaying) opts.preview.play()
    }
    document.addEventListener('mousemove', onMove)
    document.addEventListener('mouseup', onUp)
  }

  function onRulerMouseDown(ev: MouseEvent) {
    if (opts.isLocked?.()) return
    if (ev.button === 2) {
      opts.onRangeStart(ev)
      return
    }
    opts.beforeScrubFromRuler()
    startScrubDrag(ev)
  }

  function onPlayheadMouseDown(ev: MouseEvent) {
    if (ev.button !== 0) return
    ev.stopPropagation()
    opts.beforeScrubFromPlayhead()
    startScrubDrag(ev)
  }

  // Suppress browser context menu — right-click is repurposed for range select.
  function onContextMenu(ev: MouseEvent) {
    ev.preventDefault()
  }

  return {
    clientXToTime,
    startScrubDrag,
    onRulerMouseDown,
    onPlayheadMouseDown,
    onContextMenu,
  }
}
