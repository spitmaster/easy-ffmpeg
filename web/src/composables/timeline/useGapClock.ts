/**
 * Wall-clock-driven playhead advance for moments when the video element is
 * paused (typical cause: the playhead sits in a gap between video clips, or
 * the video track ended early but the audio track has more to go). The
 * pure clock is rAF + an anchor (real time + playhead at start); the caller
 * supplies the per-tick business logic via onTick().
 *
 * Used by useEditorPreview (single video) and (future) useMultitrackPreview.
 */
export interface GapClockOptions {
  /**
   * Cheap predicate checked at the top of every tick — typically
   * `() => store.playing && !!store.project`. Returning false stops the
   * clock without a final onTick call.
   */
  shouldContinue: () => boolean
  /**
   * Per-rAF callback. The argument is the proposed new playhead (anchor +
   * elapsed wall time). The caller is responsible for clamping, side
   * effects (writing back to the store, syncing audio, switching to a
   * video clip when one becomes active), and signalling whether the clock
   * should keep running.
   *
   *   'continue' — keep ticking next frame
   *   'stop'     — stop the clock (caller has handed off to a media element
   *                or reached the end)
   */
  onTick: (newPlayhead: number) => 'continue' | 'stop'
}

export function useGapClock(opts: GapClockOptions) {
  let id: number | null = null
  let anchorReal = 0
  let anchorPlayhead = 0

  function start(currentPlayhead: number) {
    if (id !== null) return
    anchorReal = performance.now()
    anchorPlayhead = currentPlayhead
    id = requestAnimationFrame(tick)
  }

  function stop() {
    if (id !== null) {
      cancelAnimationFrame(id)
      id = null
    }
  }

  function tick() {
    id = null
    if (!opts.shouldContinue()) return
    const elapsed = (performance.now() - anchorReal) / 1000
    const newPlayhead = anchorPlayhead + elapsed
    if (opts.onTick(newPlayhead) === 'continue') {
      id = requestAnimationFrame(tick)
    }
  }

  return { start, stop }
}
