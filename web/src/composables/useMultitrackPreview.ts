import { onUnmounted, ref, watch, type ComputedRef, type Ref } from 'vue'
import { multitrackApi, type MultitrackAudioTrack } from '@/api/multitrack'
import { useMultitrackStore } from '@/stores/multitrack'
import { usePreviewCore, type PreviewStrategy } from '@/composables/usePreviewCore'

/**
 * Multitrack preview adapter.
 *
 * Picture: a single <video> element. On every applyVideoFor we ask
 * `store.topVideoActive(t)`; when the active source id changes, we swap
 * the element's src and seek into it. Source switches cost a network
 * preroll (~50–150ms), so we keep them lazy.
 *
 * Sound: one <audio> element per audio track. Each is independently
 * driven (find-active → src/currentTime/play). A WebAudio GainNode is
 * created per element so per-track volume * global volume can exceed
 * 100% (HTMLMediaElement.volume caps at 1.0).
 *
 * Clock arbitration, video listener wiring, and the public play/pause/
 * toggle/seek surface live in {@link usePreviewCore}; this file only
 * supplies the multitrack {@link PreviewStrategy} (top-clip resolution,
 * per-track audio sync, per-track WebAudio gain) plus multitrack-specific
 * watchers (volume/track/project changes).
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
  // src has been assigned yet. Used to skip redundant src reassignments
  // (a single src change costs a network preroll).
  const activeVideoSourceId = ref<string>('')

  function vEl(): HTMLVideoElement | null { return videoRef.value }

  function sameSrc(node: HTMLMediaElement, url: string): boolean {
    return node.src === url || node.src === location.origin + url
  }

  // ---- Video element sync ----

  function applyVideoFor(t: number) {
    const v = vEl()
    const pid = store.project?.id
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
      // currentTime is set on loadedmetadata (handled by core's listener) —
      // until then the element ignores assignments.
    }
    if (v.readyState > 0 && Math.abs(v.currentTime - top.srcTime) > 0.05) {
      v.currentTime = top.srcTime
    }
  }

  // ---- Per-track audio sync + WebAudio gain ----

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

  function pauseAllAudio() {
    for (const a of audioRefs.value) if (a && !a.paused) a.pause()
  }

  // ---- onVideoTimeUpdate (multitrack-specific clip-end semantics) ----

  function onVideoTimeUpdate(v: HTMLVideoElement) {
    if (!store.project) return
    const top = store.topVideoActive(store.playhead)
    if (!top) return // gap-clock will handle it
    // Map currentTime back to program time via clip alignment.
    const delta = v.currentTime - top.clip.sourceStart
    const newPh = top.clip.programStart + Math.max(0, delta)
    const total = store.programDuration
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

  function evaluate() {
    if (!store.project) return
    applyVideoFor(store.playhead)
    syncAudio(store.playhead)
  }

  // ---- Strategy + core wiring ----

  const strategy: PreviewStrategy = {
    hasProject: () => !!store.project,
    totalDuration: () => store.programDuration,
    getPlaying: () => store.playing,
    setPlaying: (v) => { store.playing = v },
    getPlayhead: () => store.playhead,
    setPlayhead: (v) => { store.playhead = v },
    activeVideoClip: (t) => {
      const top = store.topVideoActive(t)
      if (!top) return null
      const c = top.clip
      return { programStart: c.programStart, sourceStart: c.sourceStart, sourceEnd: c.sourceEnd }
    },
    applyVideoFor,
    syncAudio,
    pauseAllAudio,
    applyAllVolumes,
    onVideoTimeUpdate,
  }

  const core = usePreviewCore(videoRef, strategy)

  // ---- Multitrack-specific reactive plumbing ----

  // Re-apply per-track volume whenever project/track volumes change.
  const stopWatchVolumes = watch(
    [
      () => store.project?.audioVolume,
      () => audioTracks.value.map((t) => t.volume).join(','),
      () => audioTracks.value.length,
    ],
    () => applyAllVolumes(),
    { flush: 'post' },
  )

  // Reset everything when the active project changes.
  const stopWatchProject = watch(
    () => store.project?.id,
    () => {
      core.gapClock.stop()
      core.videoClock.stop()
      core.pauseAll()
      activeVideoSourceId.value = ''
      // Wait for the next render so newly-mounted refs land before we
      // re-evaluate.
      requestAnimationFrame(() => {
        evaluate()
        applyAllVolumes()
      })
    },
  )

  // Re-evaluate when sources/track lists change (after import / drop /
  // add-track) so newly-added <audio> elements get wired and the <video>
  // snaps to the right source.
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
    stopWatchVolumes()
    stopWatchProject()
    stopWatchTracks()
  })

  function refresh() {
    activeVideoSourceId.value = ''
    evaluate()
    applyAllVolumes()
  }

  return {
    refresh,
    play: core.play,
    pause: core.pause,
    toggle: core.toggle,
    seek: core.seek,
  }
}
