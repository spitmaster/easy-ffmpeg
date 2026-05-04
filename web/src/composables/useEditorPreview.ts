import { nextTick, onUnmounted, watch, type Ref } from 'vue'
import { editorApi, type Project } from '@/api/editor'
import { useEditorStore } from '@/stores/editor'
import { useAudioGain } from '@/composables/timeline/useAudioGain'
import { usePreviewCore, type PreviewStrategy } from '@/composables/usePreviewCore'
import { collectBoundaries, programToSource, totalDuration } from '@/utils/timeline'

/**
 * Single-video preview adapter. Two media elements: <video> drives picture
 * (muted), <audio> drives sound (independent currentTime so the audio
 * track can be split / reordered relative to the video). Both load the
 * same source URL — set once in {@link loadProject} — and seek
 * independently per their own clip lists.
 *
 * Clock arbitration, video listener wiring, and the public play/pause/
 * toggle/seek surface live in {@link usePreviewCore}; this file just
 * supplies the editor-specific {@link PreviewStrategy} (clip resolution,
 * single-audio-element sync, single-GainNode volume, the next-clip
 * handoff inside `onVideoTimeUpdate`).
 */
export function useEditorPreview(
  videoRef: Ref<HTMLVideoElement | null>,
  audioRef: Ref<HTMLAudioElement | null>,
) {
  const store = useEditorStore()

  // Index into store.project.{videoClips,audioClips}; -1 means "no active
  // clip" (gap / empty). Updated from applyVideoFor / keepAudioInSync —
  // the core's gapTick + onVideoTimeUpdate rely on these staying current.
  let activeVideoIndex = -1
  let activeAudioIndex = -1

  const audioGain = useAudioGain(audioRef, () => store.project?.audioVolume ?? 1)

  function vEl(): HTMLVideoElement | null { return videoRef.value }
  function aEl(): HTMLAudioElement | null { return audioRef.value }

  function sameSrc(node: HTMLMediaElement, url: string): boolean {
    return node.src === url || node.src === location.origin + url
  }

  // ---- Video element sync ----

  function applyVideoFor(t: number) {
    const v = vEl()
    if (!v) return
    const clips = store.project?.videoClips || []
    const pos = programToSource(clips, t)
    if (!pos) {
      activeVideoIndex = -1
      if (!v.paused) v.pause()
      return
    }
    activeVideoIndex = pos.i
    if (v.readyState > 0 && Math.abs(v.currentTime - pos.src) > 0.05) {
      v.currentTime = pos.src
    }
  }

  // ---- Audio element sync ----

  function keepAudioInSync(programTime: number) {
    const a = aEl()
    if (!a) return
    const clips = store.project?.audioClips || []
    const pos = programToSource(clips, programTime)
    if (!pos) {
      if (activeAudioIndex !== -1 || !a.paused) {
        activeAudioIndex = -1
        if (!a.paused) a.pause()
      }
      return
    }
    if (pos.i !== activeAudioIndex) {
      activeAudioIndex = pos.i
      if (a.readyState > 0) a.currentTime = pos.src
      if (store.playing && a.paused) a.play().catch(() => {})
      return
    }
    if (a.readyState > 0 && Math.abs(a.currentTime - pos.src) > 0.15) {
      a.currentTime = pos.src
    }
    if (store.playing && a.paused) a.play().catch(() => {})
  }

  function pauseAudio() {
    const a = aEl()
    if (a && !a.paused) a.pause()
  }

  function onAudioTimeUpdate() {
    const a = aEl()
    if (!a) return
    const clips = store.project?.audioClips || []
    if (!clips.length || activeAudioIndex < 0) return
    const c = clips[activeAudioIndex]
    if (!c) return
    if (a.currentTime >= c.sourceEnd - 0.01) {
      // Force a re-resolve at the new playhead so we either advance to
      // the next audio clip or pause.
      keepAudioInSync(store.playhead)
    }
  }

  // ---- Clip-end transitions on the video element ----

  function onVideoTimeUpdate(v: HTMLVideoElement) {
    const clips = store.project?.videoClips || []
    if (!clips.length || activeVideoIndex < 0) return
    const c = clips[activeVideoIndex]
    if (!c) return

    if (v.currentTime >= c.sourceEnd - 0.01) {
      const sorted = clips.slice().sort((a, b) => a.programStart - b.programStart)
      const curIdx = sorted.findIndex((x) => x.id === c.id)
      const nextClip = sorted[curIdx + 1]
      const programEnd = c.programStart + (c.sourceEnd - c.sourceStart)
      if (nextClip && nextClip.programStart - programEnd < 0.01) {
        // Adjacent next clip on the same source — hand off without
        // dropping into the gap clock so playback stays seamless.
        activeVideoIndex = clips.findIndex((x) => x.id === nextClip.id)
        v.currentTime = nextClip.sourceStart
        store.playhead = nextClip.programStart
        keepAudioInSync(nextClip.programStart)
        return
      }
      store.playhead = programEnd
      activeVideoIndex = -1
      if (!v.paused) v.pause()
      keepAudioInSync(programEnd)
      if (store.playing) core.gapClock.start(store.playhead)
      return
    }

    const delta = v.currentTime - c.sourceStart
    const newPlayhead = c.programStart + Math.max(0, delta)
    store.playhead = newPlayhead
    keepAudioInSync(newPlayhead)
  }

  function onVideoEnded() {
    if (!store.project || !store.playing) {
      store.playing = false
      return
    }
    const total = totalDuration(store.project)
    if (store.playhead < total - 0.01) {
      activeVideoIndex = -1
      core.gapClock.start(store.playhead)
      return
    }
    store.playing = false
  }

  // ---- Strategy + core wiring ----

  const strategy: PreviewStrategy = {
    hasProject: () => !!store.project,
    totalDuration: () => totalDuration(store.project),
    getPlaying: () => store.playing,
    setPlaying: (v) => { store.playing = v },
    getPlayhead: () => store.playhead,
    setPlayhead: (v) => { store.playhead = v },
    activeVideoClip: (t) => {
      const clips = store.project?.videoClips || []
      const pos = programToSource(clips, t)
      if (!pos) return null
      const c = clips[pos.i]
      return { programStart: c.programStart, sourceStart: c.sourceStart, sourceEnd: c.sourceEnd }
    },
    applyVideoFor,
    syncAudio: keepAudioInSync,
    pauseAllAudio: pauseAudio,
    applyAllVolumes: () => audioGain.apply(),
    onVideoTimeUpdate,
    onVideoEnded,
  }

  const core = usePreviewCore(videoRef, strategy)

  // ---- Audio element listener (separate from core's video listener) ----

  function attachAudio(a: HTMLAudioElement) {
    a.muted = false
    a.volume = 1
    a.addEventListener('timeupdate', onAudioTimeUpdate)
  }

  function detachAudio(a: HTMLAudioElement) {
    a.removeEventListener('timeupdate', onAudioTimeUpdate)
  }

  const stopWatchAudio = watch(
    () => audioRef.value,
    (a, prev) => {
      if (prev) detachAudio(prev)
      if (a) attachAudio(a)
    },
    { immediate: true, flush: 'post' },
  )

  onUnmounted(() => {
    stopWatchAudio()
    const a = aEl()
    if (a) detachAudio(a)
  })

  // ---- Editor-specific public surface ----

  /**
   * Async because callers typically invoke this *immediately* after the
   * store flips hasProject from false to true, and the <video>/<audio>
   * elements only mount on the next render tick (they sit inside a
   * v-else block in EditorView). Without the await, videoRef would still
   * be null here and we'd silently skip setting src on both elements —
   * which is exactly the "no picture, only sound" bug.
   */
  async function loadProject(project: Project) {
    if (!project) return
    await nextTick()
    const v = vEl()
    if (!v) return
    const url = editorApi.sourceUrl(project.id)
    if (!sameSrc(v, url)) v.src = url
    const a = aEl()
    if (a && !sameSrc(a, url)) a.src = url
    activeVideoIndex = -1
    activeAudioIndex = -1
    core.gapClock.stop()
    core.videoClock.stop()
    audioGain.apply()
    core.seek(0)
  }

  function play() {
    // Once playback starts, the cursor becomes a global program-time
    // indicator and stays that way after pause too — promote splitScope
    // so visual + cut semantics agree.
    store.splitScope = 'both'
    core.play()
  }

  function seekToBoundary(direction: -1 | 1) {
    if (!store.project) return
    const boundaries = collectBoundaries(store.project.videoClips)
    if (!boundaries.length) return
    const cur = store.playhead
    if (direction < 0) {
      for (let k = boundaries.length - 1; k >= 0; k--) {
        if (boundaries[k] < cur - 0.05) {
          core.seek(boundaries[k])
          return
        }
      }
      core.seek(0)
    } else {
      for (const b of boundaries) {
        if (b > cur + 0.05) {
          core.seek(b)
          return
        }
      }
      core.seek(boundaries[boundaries.length - 1])
    }
  }

  return {
    inGap: core.inGap,
    loadProject,
    play,
    pause: core.pause,
    toggle: core.toggle,
    seek: core.seek,
    seekToBoundary,
    applyAudioVolume: () => audioGain.apply(),
  }
}
