import type { Clip } from '@/types/timeline'

/**
 * Clip drag interactions: trim (left/right edge handle) and reorder (body
 * drag with snapping). The composable mutates clips on a track via a
 * caller-supplied getClips / setClips pair so single-video (videoClips /
 * audioClips on Project) and multitrack (clips inside a VideoTrack /
 * AudioTrack) can both drive the same drag UX.
 *
 * Trim semantics — preserved from the legacy editor:
 *   - left handle: shrinks/extends the source range; programStart shifts
 *     by the same delta so the right edge stays put
 *   - right handle: shrinks/extends the source range; programStart fixed
 *   - both clamp against the source duration (caller supplies via
 *     sourceMaxFor) and a 50ms minimum clip length
 *
 * Reorder snaps the dragged clip's left or right edge to: program start
 * 0, the current playhead, and any other clip's program start/end on the
 * same track. The snap distance is `snapPx` in px (default 8), translated
 * to seconds via the current pxPerSecond.
 */
export interface TimelineDragOptions<C extends Clip = Clip> {
  pxPerSecond: () => number
  playhead: () => number
  getClips: (trackId: string) => C[]
  setClips: (trackId: string, clips: C[]) => void
  pushHistory: () => void
  scheduleSave: () => void
  /**
   * Maximum sourceEnd allowed for a clip during right-handle trim — the
   * source's duration. Single video reads project.source.duration; multi-
   * track resolves via clip.sourceId → sources[id].duration.
   */
  sourceMaxFor: (clip: C, trackId: string) => number
  /** Snap distance in pixels at the current zoom. Defaults to 8. */
  snapPx?: number
  /** Minimum clip length in seconds. Defaults to 0.05. */
  minClipSec?: number
}

const DEFAULT_SNAP_PX = 8
const DEFAULT_MIN_CLIP_SEC = 0.05

export function useTimelineDrag<C extends Clip = Clip>(opts: TimelineDragOptions<C>) {
  const snapPx = opts.snapPx ?? DEFAULT_SNAP_PX
  const minClip = opts.minClipSec ?? DEFAULT_MIN_CLIP_SEC

  function startTrim(ev: MouseEvent, trackId: string, clipId: string, side: 'left' | 'right') {
    ev.preventDefault()
    const original = opts.getClips(trackId).map((c) => ({ ...c }))
    const idx = original.findIndex((c) => c.id === clipId)
    if (idx < 0) return
    const ppS = opts.pxPerSecond()
    const startX = ev.clientX
    const origClip = { ...original[idx] }
    const sourceMax = opts.sourceMaxFor(origClip, trackId) || origClip.sourceEnd + 600

    function onMove(e: MouseEvent) {
      const dx = e.clientX - startX
      const ds = dx / ppS
      const clips = original.map((c) => ({ ...c }))
      const c = clips[idx]
      if (side === 'left') {
        const newStart = Math.max(0, Math.min(origClip.sourceEnd - minClip, origClip.sourceStart + ds))
        const delta = newStart - origClip.sourceStart
        c.sourceStart = newStart
        c.programStart = Math.max(0, origClip.programStart + delta)
      } else {
        const newEnd = Math.max(origClip.sourceStart + minClip, Math.min(sourceMax, origClip.sourceEnd + ds))
        c.sourceEnd = newEnd
      }
      opts.setClips(trackId, clips)
    }
    function onUp() {
      document.removeEventListener('mousemove', onMove)
      document.removeEventListener('mouseup', onUp)
      opts.pushHistory()
      opts.scheduleSave()
    }
    document.addEventListener('mousemove', onMove)
    document.addEventListener('mouseup', onUp)
  }

  function startReorder(ev: MouseEvent, trackId: string, clipId: string) {
    ev.preventDefault()
    const original = opts.getClips(trackId).map((c) => ({ ...c }))
    const idx = original.findIndex((c) => c.id === clipId)
    if (idx < 0) return
    const ppS = opts.pxPerSecond()
    const startX = ev.clientX
    const origProgramStart = original[idx].programStart
    const dur = original[idx].sourceEnd - original[idx].sourceStart

    const snapPoints: number[] = [0, opts.playhead()]
    original.forEach((c, i) => {
      if (i === idx) return
      snapPoints.push(c.programStart)
      snapPoints.push(c.programStart + (c.sourceEnd - c.sourceStart))
    })

    function snapToNearest(candidateStart: number): number {
      const candidateEnd = candidateStart + dur
      const snapSec = snapPx / ppS
      let bestDelta = Infinity
      let bestStart = candidateStart
      for (const p of snapPoints) {
        const dL = Math.abs(candidateStart - p)
        if (dL < bestDelta && dL <= snapSec) {
          bestDelta = dL
          bestStart = p
        }
        const dR = Math.abs(candidateEnd - p)
        if (dR < bestDelta && dR <= snapSec) {
          bestDelta = dR
          bestStart = p - dur
        }
      }
      return Math.max(0, bestStart)
    }

    function onMove(e: MouseEvent) {
      const dx = e.clientX - startX
      const raw = Math.max(0, origProgramStart + dx / ppS)
      const snapped = snapToNearest(raw)
      const clips = original.map((c) => ({ ...c }))
      clips[idx].programStart = snapped
      opts.setClips(trackId, clips)
    }
    function onUp() {
      document.removeEventListener('mousemove', onMove)
      document.removeEventListener('mouseup', onUp)
      const finalClip = opts.getClips(trackId).find((c) => c.id === clipId)
      if (finalClip && Math.abs(finalClip.programStart - origProgramStart) > 1e-6) {
        opts.pushHistory()
      }
      opts.scheduleSave()
    }
    document.addEventListener('mousemove', onMove)
    document.addEventListener('mouseup', onUp)
  }

  return { startTrim, startReorder }
}
