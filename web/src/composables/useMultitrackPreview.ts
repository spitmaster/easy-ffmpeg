import { onUnmounted, ref, watch, type ComputedRef, type Ref } from 'vue'
import { multitrackApi, type MultitrackAudioTrack } from '@/api/multitrack'
import { useMultitrackStore } from '@/stores/multitrack'
import { useGapClock } from '@/composables/timeline/useGapClock'

/**
 * Multitrack preview engine — M6 v1.
 *
 * Picture: a single <video> element. On every tick we ask the store for
 * `topVideoActive(playhead)`; whenever the active source id changes, we
 * swap the element's src and seek into it. Multi-source stacking is
 * predictable but each source change costs a network preroll (~50–150ms),
 * so we keep changes lazy — only when necessary.
 *
 * Sound: one <audio> per audio track. Each one is independently driven
 * to follow its track's clips (find-active → src/currentTime/play). A
 * WebAudio GainNode is created per element so per-track volume * global
 * volume can exceed 100% (HTMLMediaElement.volume caps at 1.0).
 *
 * Time advance:
 *   - When a video clip is active, its timeupdate drives the playhead
 *     (clamped to source range) and the gap clock is parked.
 *   - When in a gap (or no video tracks), a gap-clock rAF advances the
 *     playhead at wall-clock rate; useGapClock owns the loop.
 */

export interface MultitrackPreviewAPI {
  /** Manually re-evaluate the current frame (used after open / reload). */
  refresh: () => void
  play: () => void
  pause: () => void
  toggle: () => void
  seek: (t: number) => void
}

export function useMultitrackPreview(
  videoRef: Ref<HTMLVideoElement | null>,
  audioRefs: Ref<(HTMLAudioElement | null)[]>,
  audioTracks: ComputedRef<MultitrackAudioTrack[]>,
): MultitrackPreviewAPI {
  const store = useMultitrackStore()

  // Active source id loaded into the video element. Empty string means no
  // src has been assigned yet; null means we explicitly cleared it.
  const activeVideoSourceId = ref<string>('')

  // ---- Helpers ----

  function el<T>(r: Ref<T | null>): T | null {
    return r.value
  }

  function sameSrc(node: HTMLMediaElement, url: string): boolean {
    return node.src === url || node.src === location.origin + url
  }

  function projectId(): string | null {
    return store.project?.id ?? null
  }

  function totalDuration(): number {
    return store.programDuration
  }

  // ---- Video element ----

  /**
   * Sync the video element to the playhead. If the topmost video track has
   * an active clip, ensure src + currentTime match it. Otherwise, pause
   * the element so a stale frame doesn't keep playing audio-less video.
   */
  function applyVideoFor(t: number) {
    const v = el(videoRef)
    const pid = projectId()
    if (!v || !pid) return
    const top = store.topVideoActive(t)
    if (!top) {
      activeVideoSourceId.value = ''
      if (!v.paused) v.pause()
      return
    }
    const url = multitrackApi.sourceUrl(pid, top.source.id)
    if (activeVideoSourceId.value !== top.source.id || !sameSrc(v, url)) {
      activeVideoSourceId.value = top.source.id
      v.src = url
      // currentTime is set on loadedmetadata (below) — until then the
      // element ignores assignments.
    }
    if (v.readyState > 0 && Math.abs(v.currentTime - top.srcTime) > 0.05) {
      v.currentTime = top.srcTime
    }
  }

  /**
   * Pull the playhead from the video element and refresh audio. Used
   * while a video clip is active and the element is the master clock.
   */
  function onVideoTimeUpdate() {
    const v = el(videoRef)
    if (!v || !store.project) return
    const top = store.topVideoActive(store.playhead)
    if (!top) return // gap-clock will handle it
    // Map currentTime back to program time via clip alignment.
    const delta = v.currentTime - top.clip.sourceStart
    const newPh = top.clip.programStart + Math.max(0, delta)
    const total = totalDuration()
    store.playhead = Math.max(0, Math.min(total, newPh))
    syncAudio(store.playhead)
    // If we've run off the end of this clip and there's no immediate
    // successor on this track, hand off to the gap clock.
    const clipEnd = top.clip.programStart + (top.clip.sourceEnd - top.clip.sourceStart)
    if (v.currentTime >= top.clip.sourceEnd - 0.01 && store.playhead >= clipEnd - 0.02) {
      // Force re-evaluation; if the next instant has no top video, the
      // gap clock takes over.
      setTimeout(() => evaluate(), 0)
    }
  }

  function onLoadedMetadata() {
    if (!store.project) return
    applyVideoFor(store.playhead)
  }

  // Belt-and-braces mute (browsers can re-enable audio after caching).
  function onVideoVolumeChange() {
    const v = el(videoRef)
    if (!v) return
    if (!v.muted || v.volume !== 0) {
      v.muted = true
      v.volume = 0
    }
  }

  // ---- Audio elements (one per track) ----

  type TrackGain = { ctx: AudioContext; gain: GainNode } | null
  // Per-track WebAudio nodes; index aligned with audioTracks/audioRefs.
  const trackGains: TrackGain[] = []

  function ensureGainFor(i: number, audio: HTMLAudioElement): TrackGain {
    if (trackGains[i]) return trackGains[i]
    type AudioCtor = typeof AudioContext
    const w = window as Window & { webkitAudioContext?: AudioCtor }
    const Ctor: AudioCtor | undefined = window.AudioContext || w.webkitAudioContext
    if (!Ctor) {
      trackGains[i] = null
      return null
    }
    try {
      const ctx = new Ctor()
      const src = ctx.createMediaElementSource(audio)
      const gain = ctx.createGain()
      gain.gain.value = 1
      src.connect(gain).connect(ctx.destination)
      trackGains[i] = { ctx, gain }
      return trackGains[i]
    } catch (e) {
      console.warn('[multitrack] WebAudio init failed for track', i, e)
      trackGains[i] = null
      return null
    }
  }

  function applyTrackVolume(i: number) {
    const audio = audioRefs.value[i]
    const track = audioTracks.value[i]
    if (!audio || !track) return
    const project = store.project
    const global = project?.audioVolume ?? 1
    const v = Math.max(0, (track.volume ?? 1) * global)
    const node = ensureGainFor(i, audio)
    if (node) {
      node.gain.gain.value = v
      audio.volume = 1
      if (node.ctx.state === 'suspended') {
        node.ctx.resume().catch(() => {})
      }
    } else {
      audio.volume = Math.min(1, v)
    }
  }

  function applyAllVolumes() {
    for (let i = 0; i < audioTracks.value.length; i++) applyTrackVolume(i)
  }

  /**
   * For each audio track, find the active clip + source at playhead and
   * sync the corresponding <audio> element. Tracks with no active clip
   * are paused; their src is not cleared so the next reactivation can
   * skip the load delay.
   */
  function syncAudio(t: number) {
    const project = store.project
    if (!project) return
    const tracks = audioTracks.value
    const pid = project.id
    for (let i = 0; i < tracks.length; i++) {
      const audio = audioRefs.value[i]
      if (!audio) continue
      const track = tracks[i]
      if (track.muted) {
        if (!audio.paused) audio.pause()
        continue
      }
      const active = store.audioActive(track, t)
      if (!active) {
        if (!audio.paused) audio.pause()
        continue
      }
      const url = multitrackApi.sourceUrl(pid, active.source.id)
      if (!sameSrc(audio, url)) audio.src = url
      if (audio.readyState > 0 && Math.abs(audio.currentTime - active.srcTime) > 0.15) {
        audio.currentTime = active.srcTime
      }
      if (store.playing && audio.paused) audio.play().catch(() => {})
      if (!store.playing && !audio.paused) audio.pause()
    }
  }

  // ---- Master loop ----

  // Gap-clock — advances playhead when no <video> element is driving it
  // (timeline empty, in a gap, or audio-only segment).
  const gapClock = useGapClock({
    shouldContinue: () => store.playing && !!store.project,
    onTick: gapTick,
  })

  // Video-clock smoother. While a <video> element is playing, store.playhead
  // would otherwise only advance on the element's 'timeupdate' event, which
  // browsers throttle to ~4Hz — the timeline cursor and PlayBar timecode look
  // stuttery even though the video itself plays smoothly. This rAF loop
  // reads v.currentTime each frame and re-derives program time via the
  // active top clip's mapping, mirroring onVideoTimeUpdate but at 60Hz.
  // onVideoTimeUpdate still owns clip-end transitions; this clock only
  // smooths the per-frame display.
  const videoClock = useGapClock({
    shouldContinue: () => {
      const v = el(videoRef)
      return store.playing && activeVideoSourceId.value !== '' && !!v && !v.paused
    },
    onTick: () => {
      const v = el(videoRef)
      if (!v || !store.project) return 'stop'
      const top = store.topVideoActive(store.playhead)
      if (!top) return 'stop'
      const delta = v.currentTime - top.clip.sourceStart
      const ph = top.clip.programStart + Math.max(0, delta)
      const total = totalDuration()
      store.playhead = Math.max(0, Math.min(total, ph))
      return 'continue'
    },
  })

  function gapTick(newPh: number): 'continue' | 'stop' {
    if (!store.project) return 'stop'
    const total = totalDuration()
    if (newPh >= total - 1e-3) {
      store.playhead = total
      pauseAll()
      store.playing = false
      return 'stop'
    }
    store.playhead = newPh
    const top = store.topVideoActive(newPh)
    syncAudio(newPh)
    if (top) {
      // We've entered a video clip — hand off to the <video> element.
      applyVideoFor(newPh)
      const v = el(videoRef)
      if (store.playing && v && v.paused) v.play().catch(() => {})
      videoClock.start(0)
      return 'stop'
    }
    return 'continue'
  }

  // ---- Public actions ----

  function evaluate() {
    if (!store.project) return
    applyVideoFor(store.playhead)
    syncAudio(store.playhead)
  }

  function refresh() {
    activeVideoSourceId.value = ''
    evaluate()
    applyAllVolumes()
  }

  function play() {
    if (!store.project) return
    store.playing = true
    const top = store.topVideoActive(store.playhead)
    applyVideoFor(store.playhead)
    syncAudio(store.playhead)
    const v = el(videoRef)
    if (top && v) {
      gapClock.stop()
      if (v.paused) v.play().catch(() => {})
      videoClock.start(0)
    } else {
      // Ensure any stale video pause is in effect, then drive via gap clock.
      if (v && !v.paused) v.pause()
      gapClock.stop()
      videoClock.stop()
      gapClock.start(store.playhead)
    }
  }

  function pauseAll() {
    const v = el(videoRef)
    if (v && !v.paused) v.pause()
    for (const a of audioRefs.value) if (a && !a.paused) a.pause()
  }

  function pause() {
    pauseAll()
    gapClock.stop()
    videoClock.stop()
    store.playing = false
  }

  function toggle() {
    if (store.playing) pause()
    else play()
  }

  function seek(t: number) {
    if (!store.project) return
    const total = totalDuration()
    const clamped = Math.max(0, Math.min(t, total))
    store.playhead = clamped
    const wasPlaying = store.playing
    if (wasPlaying) pauseAll()
    applyVideoFor(clamped)
    syncAudio(clamped)
    if (wasPlaying) {
      const top = store.topVideoActive(clamped)
      const v = el(videoRef)
      if (top && v) {
        gapClock.stop()
        if (v.paused) v.play().catch(() => {})
        videoClock.start(0)
      } else {
        gapClock.stop()
        videoClock.stop()
        gapClock.start(store.playhead)
      }
    }
  }

  // ---- Listener wiring ----

  function attachVideoListeners(v: HTMLVideoElement) {
    v.muted = true
    v.volume = 0
    v.addEventListener('timeupdate', onVideoTimeUpdate)
    v.addEventListener('loadedmetadata', onLoadedMetadata)
    v.addEventListener('volumechange', onVideoVolumeChange)
  }

  function detachVideoListeners(v: HTMLVideoElement) {
    v.removeEventListener('timeupdate', onVideoTimeUpdate)
    v.removeEventListener('loadedmetadata', onLoadedMetadata)
    v.removeEventListener('volumechange', onVideoVolumeChange)
  }

  // Wait for refs to populate (templates mount async).
  const stopWatchVideo = watch(
    () => videoRef.value,
    (v, prev) => {
      if (prev) detachVideoListeners(prev)
      if (v) attachVideoListeners(v)
    },
    { immediate: true, flush: 'post' },
  )

  // Apply per-track volume whenever the project's track volume / global
  // volume / track count changes.
  const stopWatchVolumes = watch(
    [
      () => store.project?.audioVolume,
      () => audioTracks.value.map((t) => t.volume).join(','),
      () => audioTracks.value.length,
    ],
    () => applyAllVolumes(),
    { flush: 'post' },
  )

  // When the active project changes, reset.
  const stopWatchProject = watch(
    () => store.project?.id,
    () => {
      gapClock.stop()
      videoClock.stop()
      pauseAll()
      activeVideoSourceId.value = ''
      // refresh once the next render assigns refs.
      requestAnimationFrame(() => {
        evaluate()
        applyAllVolumes()
      })
    },
  )

  // Re-evaluate when sources or track lists change (e.g. after import /
  // drop / add track) so newly-added <audio> elements get wired and the
  // <video> snaps to the right source.
  const stopWatchTracks = watch(
    () => [
      store.project?.videoTracks.length,
      store.project?.audioTracks.length,
      store.project?.sources.length,
    ],
    () => {
      requestAnimationFrame(() => {
        evaluate()
        applyAllVolumes()
      })
    },
  )

  onUnmounted(() => {
    stopWatchVideo()
    stopWatchVolumes()
    stopWatchProject()
    stopWatchTracks()
    gapClock.stop()
    videoClock.stop()
    pauseAll()
    const v = el(videoRef)
    if (v) detachVideoListeners(v)
  })

  return { refresh, play, pause, toggle, seek }
}
