import { nextTick, onUnmounted, ref, watch, type Ref } from 'vue'
import { editorApi, type Project } from '@/api/editor'
import { useEditorStore } from '@/stores/editor'
import { useAudioGain } from '@/composables/timeline/useAudioGain'
import { useGapClock } from '@/composables/timeline/useGapClock'
import { collectBoundaries, programToSource, totalDuration } from '@/utils/timeline'

/**
 * Two-element preview engine: <video> drives picture (muted), <audio>
 * drives sound. Both load the same source URL and seek independently
 * according to videoClips / audioClips. The video element is the master
 * clock — its `timeupdate` advances the program playhead, and the audio
 * element is re-synced on every tick.
 *
 * When the playhead is in a video-track gap (or video clips ended early),
 * we drive the playhead via requestAnimationFrame instead — the "gap clock".
 *
 * Audio volume comes from project.audioVolume; values >100% need WebAudio
 * (HTMLMediaElement.volume hard-caps at 1.0). A GainNode is created lazily
 * on the first applyAudioVolume call.
 */
export function useEditorPreview(
  videoRef: Ref<HTMLVideoElement | null>,
  audioRef: Ref<HTMLAudioElement | null>,
) {
  const store = useEditorStore()

  let activeVideoIndex = -1
  let activeAudioIndex = -1

  // Track whether listeners have been attached so we can be re-init safe.
  const inGap = ref(false)

  // ---- Shared primitives: gap clock + WebAudio gain ----

  const gapClock = useGapClock({
    shouldContinue: () => store.playing && !!store.project,
    onTick: gapTick,
  })

  const audioGain = useAudioGain(audioRef, () => store.project?.audioVolume ?? 1)

  function applyAudioVolume() {
    audioGain.apply()
  }

  function el<T>(r: Ref<T | null>): T | null {
    return r.value
  }

  function sameSrc(elNode: HTMLMediaElement, url: string): boolean {
    return elNode.src === url || elNode.src === location.origin + url
  }

  // Async because callers typically invoke this *immediately* after the
  // store flips hasProject from false to true, and the <video>/<audio>
  // elements only mount on the next render tick (they sit inside a
  // v-else block in EditorView). Without the await, videoRef would still
  // be null here and we'd silently skip setting src on both elements —
  // which is exactly the "no picture, only sound" bug.
  async function loadProject(project: Project) {
    if (!project) return
    await nextTick()
    const v = el(videoRef)
    if (!v) return
    const url = editorApi.sourceUrl(project.id)
    if (!sameSrc(v, url)) v.src = url
    const a = el(audioRef)
    if (a && !sameSrc(a, url)) a.src = url
    activeVideoIndex = -1
    activeAudioIndex = -1
    gapClock.stop()
    inGap.value = false
    applyAudioVolume()
    seek(0)
  }

  function play() {
    if (!store.project) return
    const v = el(videoRef)
    if (!v) return
    v.muted = true
    const a = el(audioRef)
    if (a) a.muted = false
    applyVideoFor(store.playhead)
    applyAudioFor(store.playhead)
    // Once playback starts, the cursor becomes a global program-time
    // indicator and stays that way after pause too — promote splitScope
    // so visual + cut semantics agree.
    store.playing = true
    store.splitScope = 'both'
    if (activeVideoIndex >= 0) {
      gapClock.stop()
      if (v.paused) v.play().catch(() => {})
    } else {
      gapClock.start(store.playhead)
    }
    if (a && a.paused && activeAudioIndex >= 0) a.play().catch(() => {})
  }

  function pause() {
    const v = el(videoRef)
    const a = el(audioRef)
    if (v) v.pause()
    if (a) a.pause()
    gapClock.stop()
    store.playing = false
  }

  function toggle() {
    if (store.playing) pause()
    else play()
  }

  function seek(t: number) {
    if (!store.project) return
    const total = totalDuration(store.project)
    const clamped = Math.max(0, Math.min(t, total))
    store.playhead = clamped
    applyVideoFor(clamped)
    applyAudioFor(clamped)
    if (store.playing) {
      const v = el(videoRef)
      if (activeVideoIndex >= 0 && v) {
        gapClock.stop()
        if (v.paused) v.play().catch(() => {})
      } else {
        gapClock.stop()
        gapClock.start(store.playhead)
      }
    }
  }

  function seekToBoundary(direction: -1 | 1) {
    if (!store.project) return
    const boundaries = collectBoundaries(store.project.videoClips)
    const cur = store.playhead
    if (direction < 0) {
      for (let k = boundaries.length - 1; k >= 0; k--) {
        if (boundaries[k] < cur - 0.05) {
          seek(boundaries[k])
          return
        }
      }
      seek(0)
    } else {
      for (const b of boundaries) {
        if (b > cur + 0.05) {
          seek(b)
          return
        }
      }
      seek(boundaries[boundaries.length - 1])
    }
  }

  // ---- Gap clock tick ----

  // Called by useGapClock per rAF. We update the playhead, then either
  // hand off to the <video> element when we've entered a clip, end the
  // timeline, or keep ticking through the gap.
  function gapTick(newPlayhead: number): 'continue' | 'stop' {
    if (!store.project) return 'stop'
    const v = el(videoRef)
    const total = totalDuration(store.project)
    if (newPlayhead >= total - 1e-3) {
      store.playhead = total
      pause()
      return 'stop'
    }
    store.playhead = newPlayhead
    const videoClips = store.project.videoClips || []
    const pos = programToSource(videoClips, newPlayhead)
    if (pos && v) {
      activeVideoIndex = pos.i
      inGap.value = false
      if (v.readyState > 0) v.currentTime = pos.src
      if (v.paused) v.play().catch(() => {})
      keepAudioInSync(newPlayhead)
      return 'stop'
    }
    keepAudioInSync(newPlayhead)
    return 'continue'
  }

  // ---- Video track ----

  function applyVideoFor(t: number) {
    const v = el(videoRef)
    if (!v) return
    const clips = store.project?.videoClips || []
    const pos = programToSource(clips, t)
    if (!pos) {
      activeVideoIndex = -1
      if (!v.paused) v.pause()
      inGap.value = true
      return
    }
    activeVideoIndex = pos.i
    inGap.value = false
    if (v.readyState > 0 && Math.abs(v.currentTime - pos.src) > 0.05) {
      v.currentTime = pos.src
    }
  }

  function onVideoTimeUpdate() {
    const v = el(videoRef)
    if (!v) return
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
        activeVideoIndex = clips.findIndex((x) => x.id === nextClip.id)
        v.currentTime = nextClip.sourceStart
        store.playhead = nextClip.programStart
        applyAudioFor(nextClip.programStart)
        return
      }
      store.playhead = programEnd
      activeVideoIndex = -1
      inGap.value = true
      if (!v.paused) v.pause()
      keepAudioInSync(programEnd)
      if (store.playing) gapClock.start(store.playhead)
      return
    }

    const delta = v.currentTime - c.sourceStart
    const newPlayhead = c.programStart + Math.max(0, delta)
    store.playhead = newPlayhead
    keepAudioInSync(newPlayhead)
  }

  // ---- Audio track ----

  function applyAudioFor(t: number) {
    const a = el(audioRef)
    if (!a) return
    const clips = store.project?.audioClips || []
    const pos = programToSource(clips, t)
    if (!pos) {
      activeAudioIndex = -1
      if (!a.paused) a.pause()
      return
    }
    activeAudioIndex = pos.i
    if (a.readyState > 0 && Math.abs(a.currentTime - pos.src) > 0.05) {
      a.currentTime = pos.src
    }
    if (store.playing && a.paused) a.play().catch(() => {})
  }

  function keepAudioInSync(programTime: number) {
    const a = el(audioRef)
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

  function onAudioTimeUpdate() {
    const a = el(audioRef)
    if (!a) return
    const clips = store.project?.audioClips || []
    if (!clips.length || activeAudioIndex < 0) return
    const c = clips[activeAudioIndex]
    if (!c) return
    if (a.currentTime >= c.sourceEnd - 0.01) {
      applyAudioFor(store.playhead)
    }
  }

  // ---- Listeners (attach on first valid element) ----

  function attachListeners() {
    const v = el(videoRef)
    if (v) {
      v.muted = true
      v.volume = 0
      v.addEventListener('timeupdate', onVideoTimeUpdate)
      v.addEventListener('ended', onVideoEnded)
      v.addEventListener('loadedmetadata', onLoadedMetadata)
      v.addEventListener('volumechange', onVideoVolumeChange)
    }
    const a = el(audioRef)
    if (a) {
      a.muted = false
      a.volume = 1
      a.addEventListener('timeupdate', onAudioTimeUpdate)
    }
  }

  function detachListeners() {
    const v = el(videoRef)
    if (v) {
      v.removeEventListener('timeupdate', onVideoTimeUpdate)
      v.removeEventListener('ended', onVideoEnded)
      v.removeEventListener('loadedmetadata', onLoadedMetadata)
      v.removeEventListener('volumechange', onVideoVolumeChange)
    }
    const a = el(audioRef)
    if (a) a.removeEventListener('timeupdate', onAudioTimeUpdate)
  }

  function onVideoEnded() {
    if (!store.project || !store.playing) {
      store.playing = false
      return
    }
    const total = totalDuration(store.project)
    if (store.playhead < total - 0.01) {
      activeVideoIndex = -1
      inGap.value = true
      gapClock.start(store.playhead)
      return
    }
    store.playing = false
  }

  function onLoadedMetadata() {
    if (!store.project) return
    applyVideoFor(store.playhead)
    applyAudioFor(store.playhead)
  }

  // Belt-and-braces: keep the video element silent regardless of OS keys
  // or browser caching that might unmute it.
  function onVideoVolumeChange() {
    const v = el(videoRef)
    if (!v) return
    if (!v.muted || v.volume !== 0) {
      v.muted = true
      v.volume = 0
    }
  }

  // Wait for refs to populate (templates mount async).
  const stopWatchAttach = watch(
    () => [videoRef.value, audioRef.value],
    ([v, a]) => {
      if (v || a) {
        detachListeners()
        attachListeners()
      }
    },
    { immediate: true, flush: 'post' },
  )

  onUnmounted(() => {
    stopWatchAttach()
    gapClock.stop()
    detachListeners()
  })

  return {
    inGap,
    loadProject,
    play,
    pause,
    toggle,
    seek,
    seekToBoundary,
    applyAudioVolume,
  }
}
