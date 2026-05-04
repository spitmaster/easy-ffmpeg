import { computed, onUnmounted, watch, type Ref } from 'vue'
import { useGapClock } from '@/composables/timeline/useGapClock'

/**
 * Resolved active "top" video clip at a given program time, returned by
 * `PreviewStrategy.activeVideoClip()`. The core uses the program/source
 * mapping for the videoClock smoother and the gap-clock handoff; nothing
 * else about the clip leaks through. Strategies are free to tag the
 * source / track id internally (multitrack does, single-video doesn't
 * need to).
 */
export interface ActiveVideoClipInfo {
  programStart: number
  sourceStart: number
  sourceEnd: number
}

/**
 * Pluggable behaviour for the preview engine. Single-video and multitrack
 * implement this against their own store shapes — the core is unaware of
 * Pinia, project schemas, or how many <audio> elements exist.
 */
export interface PreviewStrategy {
  // ---- Project state ----
  hasProject(): boolean
  totalDuration(): number

  // ---- Playhead / playing (delegated to per-store refs) ----
  getPlaying(): boolean
  setPlaying(v: boolean): void
  getPlayhead(): number
  setPlayhead(v: number): void

  // ---- Active clip resolution ----
  /**
   * The top video clip currently active at time t, or null if t falls in a
   * gap (or there's no video track). The core uses this for two things:
   *   1. The rAF videoClock smoother — to map v.currentTime back to program
   *      time at 60Hz instead of waiting for the throttled `timeupdate`.
   *   2. The gap-clock → video handoff in gapTick + play + seek.
   */
  activeVideoClip(t: number): ActiveVideoClipInfo | null

  // ---- Element-level sync (strategy implements) ----
  /** Set <video> src/currentTime/pause as needed for time t. */
  applyVideoFor(t: number): void
  /** Sync all <audio> elements to time t (single-video has 1, multitrack has N). */
  syncAudio(t: number): void
  /** Pause every audio element managed by this strategy. */
  pauseAllAudio(): void
  /** Reapply audio volumes (single GainNode, or per-track WebAudio). */
  applyAllVolumes(): void

  // ---- Strategy-owned listeners ----
  /**
   * The video element's `timeupdate` is browser-throttled to ~4Hz; the
   * core supplies a 60Hz videoClock smoother for *display* but the
   * throttled event still owns *clip-end* transitions, which differ:
   *   - editor: handoff to the next adjacent clip on the same track
   *   - multitrack: setTimeout(evaluate) to re-arbitrate sources
   * That's why this stays per-strategy.
   */
  onVideoTimeUpdate(v: HTMLVideoElement): void

  // ---- Optional hooks ----
  /** Fires on the <video> element's 'ended' event. Editor uses it to hand off to the gap clock. */
  onVideoEnded?(): void
  /** Run extra setup after the core attaches its listeners (e.g. set initial state). */
  onAttach?(v: HTMLVideoElement): void
  /** Run extra teardown before the core detaches its listeners. */
  onDetach?(v: HTMLVideoElement): void
}

export interface PreviewCore {
  /** True iff there's no active top video clip at the current playhead. */
  inGap: Ref<boolean>
  play: () => void
  pause: () => void
  toggle: () => void
  seek: (t: number) => void
  /** Pause <video> + every audio element managed by the strategy. */
  pauseAll: () => void
  /** rAF-driven gap clock used while no video element is the master. */
  gapClock: ReturnType<typeof useGapClock>
  /**
   * rAF-driven video clock — runs while the <video> element is playing
   * and pulls the program playhead forward by reading v.currentTime each
   * frame. Without this, store.playhead would only advance on the
   * <video>'s 'timeupdate' event, which browsers throttle to ~4Hz —
   * making the timeline cursor and PlayBar timecode look stuttery even
   * though the video itself plays smoothly.
   */
  videoClock: ReturnType<typeof useGapClock>
}

/**
 * Shared preview engine. Owns the rAF clock arbitration (gap-clock vs
 * video-clock vs <video> element), the video listener attach/detach
 * dance, and the public play/pause/toggle/seek surface. Per-store details
 * (project shape, audio topology, volume handling, clip-end semantics)
 * are injected via {@link PreviewStrategy}.
 *
 * History note: editor and multitrack used to ship two parallel
 * implementations; the videoClock 60Hz smoother landed in editor's
 * version during M2-M5 and was forgotten when multitrack was written from
 * a pre-smoother snapshot, producing a months-long jittery-cursor bug
 * (M9 era). Centralising the clock arbitration here is what stops that
 * pattern from happening again.
 */
export function usePreviewCore(
  videoRef: Ref<HTMLVideoElement | null>,
  strategy: PreviewStrategy,
): PreviewCore {
  // Reactive: re-derives whenever the active clip resolution at the
  // current playhead changes. EditorView's template uses this to hide
  // the <video> element while in a gap so a stale frame doesn't show.
  const inGap = computed(() => {
    if (!strategy.hasProject()) return false
    return strategy.activeVideoClip(strategy.getPlayhead()) === null
  })

  function el(): HTMLVideoElement | null {
    return videoRef.value
  }

  function pauseAll() {
    const v = el()
    if (v && !v.paused) v.pause()
    strategy.pauseAllAudio()
  }

  // ---- Clocks ----

  // The gap clock advances the playhead at wall-clock rate while no
  // video element is the master (timeline empty, in a gap, audio-only
  // segment). On every tick we re-check whether we've entered a video
  // clip — if so, we hand off to the video element + start the video
  // clock smoother.
  const gapClock = useGapClock({
    shouldContinue: () => strategy.getPlaying() && strategy.hasProject(),
    onTick: gapTick,
  })

  // The video clock smoother runs at 60Hz while a <video> element is
  // playing. v.currentTime is the authoritative source of truth during
  // video playback; this loop just re-derives program time and writes
  // the playhead so the cursor + timecode update smoothly. Strategy's
  // onVideoTimeUpdate still owns clip-end transitions.
  const videoClock = useGapClock({
    shouldContinue: () => {
      const v = el()
      if (!v || v.paused) return false
      if (!strategy.getPlaying()) return false
      return strategy.activeVideoClip(strategy.getPlayhead()) !== null
    },
    onTick: () => {
      const v = el()
      if (!v || !strategy.hasProject()) return 'stop'
      const c = strategy.activeVideoClip(strategy.getPlayhead())
      if (!c) return 'stop'
      const delta = v.currentTime - c.sourceStart
      const ph = c.programStart + Math.max(0, delta)
      const total = strategy.totalDuration()
      strategy.setPlayhead(Math.max(0, Math.min(total, ph)))
      return 'continue'
    },
  })

  function gapTick(newPh: number): 'continue' | 'stop' {
    if (!strategy.hasProject()) return 'stop'
    const total = strategy.totalDuration()
    if (newPh >= total - 1e-3) {
      strategy.setPlayhead(total)
      pauseAll()
      strategy.setPlaying(false)
      return 'stop'
    }
    strategy.setPlayhead(newPh)
    const active = strategy.activeVideoClip(newPh)
    strategy.syncAudio(newPh)
    if (active) {
      // Entered a video clip — hand off to the <video> element so it
      // becomes the master clock at native frame rate. The video clock
      // smoother then takes over per-frame playhead updates.
      strategy.applyVideoFor(newPh)
      const v = el()
      if (strategy.getPlaying() && v && v.paused) v.play().catch(() => {})
      videoClock.start(0)
      return 'stop'
    }
    return 'continue'
  }

  // ---- Public actions ----

  function play() {
    if (!strategy.hasProject()) return
    strategy.setPlaying(true)
    const t = strategy.getPlayhead()
    strategy.applyVideoFor(t)
    strategy.syncAudio(t)
    const active = strategy.activeVideoClip(t)
    const v = el()
    if (active && v) {
      gapClock.stop()
      if (v.paused) v.play().catch(() => {})
      videoClock.start(0)
    } else {
      if (v && !v.paused) v.pause()
      videoClock.stop()
      gapClock.stop()
      gapClock.start(t)
    }
  }

  function pause() {
    pauseAll()
    gapClock.stop()
    videoClock.stop()
    strategy.setPlaying(false)
  }

  function toggle() {
    if (strategy.getPlaying()) pause()
    else play()
  }

  function seek(t: number) {
    if (!strategy.hasProject()) return
    const total = strategy.totalDuration()
    const clamped = Math.max(0, Math.min(t, total))
    strategy.setPlayhead(clamped)
    const wasPlaying = strategy.getPlaying()
    if (wasPlaying) pauseAll()
    strategy.applyVideoFor(clamped)
    strategy.syncAudio(clamped)
    if (wasPlaying) {
      const active = strategy.activeVideoClip(clamped)
      const v = el()
      if (active && v) {
        gapClock.stop()
        if (v.paused) v.play().catch(() => {})
        videoClock.start(0)
      } else {
        gapClock.stop()
        videoClock.stop()
        gapClock.start(clamped)
      }
    }
  }

  // ---- Listeners ----

  function handleTimeUpdate() {
    const v = el()
    if (v) strategy.onVideoTimeUpdate(v)
  }

  function handleLoadedMetadata() {
    if (!strategy.hasProject()) return
    const t = strategy.getPlayhead()
    strategy.applyVideoFor(t)
    strategy.syncAudio(t)
  }

  // Belt-and-braces: keep the video element silent regardless of OS keys
  // or browser caching that might unmute it.
  function handleVolumeChange() {
    const v = el()
    if (!v) return
    if (!v.muted || v.volume !== 0) {
      v.muted = true
      v.volume = 0
    }
  }

  function handleEnded() {
    strategy.onVideoEnded?.()
  }

  function attachVideo(v: HTMLVideoElement) {
    v.muted = true
    v.volume = 0
    v.addEventListener('timeupdate', handleTimeUpdate)
    v.addEventListener('loadedmetadata', handleLoadedMetadata)
    v.addEventListener('volumechange', handleVolumeChange)
    v.addEventListener('ended', handleEnded)
    strategy.onAttach?.(v)
  }

  function detachVideo(v: HTMLVideoElement) {
    strategy.onDetach?.(v)
    v.removeEventListener('timeupdate', handleTimeUpdate)
    v.removeEventListener('loadedmetadata', handleLoadedMetadata)
    v.removeEventListener('volumechange', handleVolumeChange)
    v.removeEventListener('ended', handleEnded)
  }

  // Wait for the videoRef to populate (templates mount async).
  const stopWatchVideo = watch(
    () => videoRef.value,
    (v, prev) => {
      if (prev) detachVideo(prev)
      if (v) attachVideo(v)
    },
    { immediate: true, flush: 'post' },
  )

  onUnmounted(() => {
    stopWatchVideo()
    gapClock.stop()
    videoClock.stop()
    pauseAll()
    const v = el()
    if (v) detachVideo(v)
  })

  return {
    inGap,
    play,
    pause,
    toggle,
    seek,
    pauseAll,
    gapClock,
    videoClock,
  }
}
