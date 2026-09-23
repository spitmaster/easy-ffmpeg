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
 *
 * Cross-track reorder (multitrack) is gated on two optional callbacks:
 *   - `findTargetTrack(ev)` — returns the track id the cursor is over
 *     (or null when over a non-droppable area / different-kind track);
 *   - `onCrossTrack(from, to, clipId, programStart)` — commits the move
 *     in the store. When both are provided and a move is accepted, the
 *     drag continues against the new track (snap points recomputed).
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
  /** Cross-track drag: identify the track id under the cursor. Return null
   * to keep the clip on its current track (e.g. cursor over a different-
   * kind track that should reject the drop). */
  findTargetTrack?: (ev: MouseEvent) => string | null
  /** Cross-track drag: commit the move. Should remove the clip from
   * `from` and append it to `to` at `newProgramStart`. Return true to
   * accept the move; false rejects it and the drag continues on `from`. */
  onCrossTrack?: (
    fromTrackId: string,
    toTrackId: string,
    clipId: string,
    newProgramStart: number,
  ) => boolean
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
    const ppS = opts.pxPerSecond()

    // Mutable drag state — gets replaced after a successful cross-track move.
    let currentTrackId = trackId
    let original = opts.getClips(currentTrackId).map((c) => ({ ...c }))
    let idx = original.findIndex((c) => c.id === clipId)
    if (idx < 0) return
    let origProgramStart = original[idx].programStart
    const dur = original[idx].sourceEnd - original[idx].sourceStart
    let baseX = ev.clientX
    let snapPoints = computeSnapPoints()
    const initialProgramStart = origProgramStart
    const initialTrackId = trackId

    function computeSnapPoints(): number[] {
      const pts: number[] = [0, opts.playhead()]
      original.forEach((c, i) => {
        if (i === idx) return
        pts.push(c.programStart)
        pts.push(c.programStart + (c.sourceEnd - c.sourceStart))
      })
      return pts
    }

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
      const dx = e.clientX - baseX
      const raw = Math.max(0, origProgramStart + dx / ppS)
      const snapped = snapToNearest(raw)

      // Cross-track move attempt — only when both callbacks are wired and
      // the cursor identifies a *different* track.
      if (opts.findTargetTrack && opts.onCrossTrack) {
        const target = opts.findTargetTrack(e)
        if (target && target !== currentTrackId) {
          const accepted = opts.onCrossTrack(currentTrackId, target, clipId, snapped)
          if (accepted) {
            // Re-baseline the drag against the new track. The store has
            // already moved the clip; reload our cached `original` so
            // future setClips writes target the destination track.
            currentTrackId = target
            original = opts.getClips(currentTrackId).map((c) => ({ ...c }))
            idx = original.findIndex((c) => c.id === clipId)
            if (idx < 0) return
            origProgramStart = snapped
            baseX = e.clientX
            snapPoints = computeSnapPoints()
            return
          }
        }
      }

      const clips = original.map((c) => ({ ...c }))
      if (idx >= 0 && idx < clips.length) {
        clips[idx].programStart = snapped
        opts.setClips(currentTrackId, clips)
      }
    }
    function onUp() {
      document.removeEventListener('mousemove', onMove)
      document.removeEventListener('mouseup', onUp)
      const finalClip = opts.getClips(currentTrackId).find((c) => c.id === clipId)
      const moved =
        currentTrackId !== initialTrackId ||
        (finalClip && Math.abs(finalClip.programStart - initialProgramStart) > 1e-6)
      if (moved) {
        opts.pushHistory()
      }
      opts.scheduleSave()
    }
    document.addEventListener('mousemove', onMove)
    document.addEventListener('mouseup', onUp)
  }

  return { startTrim, startReorder }
}
