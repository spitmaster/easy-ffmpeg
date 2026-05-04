import { computed, onUnmounted, ref, watch, type ComputedRef, type Ref } from 'vue'
import { multitrackApi, type MultitrackAudioTrack, type MultitrackVideoTrack } from '@/api/multitrack'
import { useMultitrackStore } from '@/stores/multitrack'
import { usePreviewCore, type PreviewStrategy } from '@/composables/usePreviewCore'

/**
 * Multitrack preview adapter.
 *
 * Picture: one `<video>` element per video track. Each track's element is
 * driven independently — `applyVideoForAll(t)` looks up `videoActive(track, t)`
 * for every track, swaps src on source change, corrects currentTime on drift
 * (>0.05s), and broadcasts play/pause. Tracks with no clip at the playhead
 * keep their last src loaded but are paused; the template hides them via
 * v-show when their active source is null. Source switches cost a network
 * preroll (~50–150ms), so we skip redundant src reassignments.
 *
 * Clock arbitration: only the master <video> (the topmost non-hidden track
 * with an active clip — convention v0.5.1+: lowest track index = top of z)
 * drives the playhead via its native `timeupdate`. Other active tracks are
 * "followers" — we set their currentTime when they drift, but they never
 * touch store.playhead. The master changes whenever the topActive track
 * index changes (e.g. clip ends on V1, V2 takes over); a computed
 * `masterVideoRef` propagates this to {@link usePreviewCore}, which auto
 * detaches/attaches its listeners on master swap.
 *
 * Sound: one <audio> element per audio track. Each is independently
 * driven (find-active → src/currentTime/play). A WebAudio GainNode is
 * created per element so per-track volume * global volume can exceed
 * 100% (HTMLMediaElement.volume caps at 1.0).
 *
 * Layout / DOM ordering: the template renders `v-for="(t, i) in videoTracks"`
 * in array order; z-index is set so lower index ends up on top of the
 * stack (matches the export overlay chain: lowest index = top of z).
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
  videoRefs: Ref<(HTMLVideoElement | null)[]>,
  audioRefs: Ref<(HTMLAudioElement | null)[]>,
  videoTracks: ComputedRef<MultitrackVideoTrack[]>,
  audioTracks: ComputedRef<MultitrackAudioTrack[]>,
): MultitrackPreviewAPI {
  const store = useMultitrackStore()

  // Per-track active source id loaded into the corresponding <video>.
  // Empty string means no src has been assigned yet. We use these to skip
  // redundant src reassignments — every src= triggers a network preroll
  // (~50–150ms) and a brief flicker, so we only swap when the active source
  // genuinely changes.
  const activeVideoSourceIds = ref<string[]>([])

  // Master = the <video> whose timeupdate drives store.playhead. It's the
  // topmost active track's video element; recomputed every render so when
  // the top-of-z track changes (clip ended on V1 → V2 picks up), the core
  // re-attaches its listeners to the new master without us doing anything
  // explicit.
  const masterVideoRef = computed<HTMLVideoElement | null>(() => {
    const idx = store.topVideoActiveIndex(store.playhead)
    if (idx < 0) return null
    return videoRefs.value[idx] ?? null
  })

  function sameSrc(node: HTMLMediaElement, url: string): boolean {
    return node.src === url || node.src === location.origin + url
  }

  // ---- Per-track video sync ----

  function applyVideoForTrack(i: number, t: number) {
    const v = videoRefs.value[i]
    const pid = store.project?.id
    const track = videoTracks.value[i]
    if (!v || !pid || !track) return
    const active = store.videoActive(track, t)
    if (!active) {
      activeVideoSourceIds.value[i] = ''
      if (!v.paused) v.pause()
      return
    }
    const url = multitrackApi.sourceUrl(pid, active.source.id)
    if (activeVideoSourceIds.value[i] !== active.source.id || !sameSrc(v, url)) {
      activeVideoSourceIds.value[i] = active.source.id
      v.src = url
      // currentTime is set on loadedmetadata (handled by core's listener) —
      // until then the element ignores assignments.
    }
    if (v.readyState > 0 && Math.abs(v.currentTime - active.srcTime) > 0.05) {
      v.currentTime = active.srcTime
    }
    // Broadcast play/pause: every active track plays in sync; the master's
    // timeupdate drives the program clock, followers ride along.
    if (store.playing && v.paused) v.play().catch(() => {})
    if (!store.playing && !v.paused) v.pause()
  }

  function applyVideoFor(t: number) {
    for (let i = 0; i < videoTracks.value.length; i++) applyVideoForTrack(i, t)
  }

  function pauseAllVideo() {
    for (const v of videoRefs.value) if (v && !v.paused) v.pause()
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
    // Only the master video should drive the playhead. If a follower's
    // timeupdate fires (it's also playing), ignore it — only the master's
    // currentTime is authoritative for the program clock.
    if (v !== masterVideoRef.value) return
    // Map currentTime back to program time via clip alignment.
    const delta = v.currentTime - top.clip.sourceStart
    const newPh = top.clip.programStart + Math.max(0, delta)
    const total = store.programDuration
    store.playhead = Math.max(0, Math.min(total, newPh))
    syncAudio(store.playhead)
    // Followers correct their currentTime if they've drifted from program
    // time (caused by browser-level scheduling jitter on independent
    // <video> elements). 0.08s tolerance keeps the corrections rare.
    syncFollowerVideos(store.playhead)
    // If we've run off the end of this clip and there's no immediate
    // successor on this track, hand off to the gap clock.
    const clipEnd = top.clip.programStart + (top.clip.sourceEnd - top.clip.sourceStart)
    if (v.currentTime >= top.clip.sourceEnd - 0.01 && store.playhead >= clipEnd - 0.02) {
      // Force re-evaluation; if the next instant has no top video, the
      // gap clock takes over.
      setTimeout(() => evaluate(), 0)
    }
  }

  function syncFollowerVideos(t: number) {
    const masterIdx = store.topVideoActiveIndex(t)
    for (let i = 0; i < videoTracks.value.length; i++) {
      if (i === masterIdx) continue
      const v = videoRefs.value[i]
      const track = videoTracks.value[i]
      if (!v || !track) continue
      const active = store.videoActive(track, t)
      if (!active) continue
      if (v.readyState > 0 && Math.abs(v.currentTime - active.srcTime) > 0.08) {
        v.currentTime = active.srcTime
      }
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
    pauseAllAudio: () => {
      pauseAllAudio()
      pauseAllVideo()
    },
    applyAllVolumes,
    onVideoTimeUpdate,
  }

  const core = usePreviewCore(masterVideoRef, strategy)

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
      activeVideoSourceIds.value = []
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

  // Re-evaluate when any clip's timeline state changes (drag on timeline,
  // trim, split, range-delete). Without this watcher, dragging a clip on
  // the timeline doesn't update the <video> elements' src/currentTime —
  // the preview keeps showing whatever frame was active before the drag,
  // even though the playhead now maps to a different source/source-time.
  // We hash a per-clip signature (id + program/source bounds) so the
  // watcher fires exactly when those bounds shift; transform-only edits
  // don't go through here (they don't change which source is active).
  const stopWatchClipBounds = watch(
    () => {
      const p = store.project
      if (!p) return ''
      const parts: string[] = []
      for (const t of p.videoTracks) {
        for (const c of t.clips) {
          parts.push(`v:${c.id}:${c.programStart}:${c.sourceStart}:${c.sourceEnd}:${c.sourceId}`)
        }
      }
      for (const t of p.audioTracks) {
        for (const c of t.clips) {
          parts.push(`a:${c.id}:${c.programStart}:${c.sourceStart}:${c.sourceEnd}:${c.sourceId}`)
        }
      }
      return parts.join('|')
    },
    () => {
      requestAnimationFrame(() => evaluate())
    },
  )

  onUnmounted(() => {
    stopWatchVolumes()
    stopWatchProject()
    stopWatchTracks()
    stopWatchClipBounds()
  })

  function refresh() {
    activeVideoSourceIds.value = []
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
