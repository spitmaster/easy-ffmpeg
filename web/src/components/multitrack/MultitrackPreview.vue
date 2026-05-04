<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, useTemplateRef, watch } from 'vue'
import { useMultitrackStore } from '@/stores/multitrack'
import { useMultitrackPreview } from '@/composables/useMultitrackPreview'

/**
 * Top-half preview pane.
 *
 * Layout: an outer "preview-shell" centers a "preview-canvas" box that
 * scales to fit the available area while preserving the project canvas's
 * aspect ratio (v0.5.1+). The <video> fills the canvas box with
 * object-fill — when a clip's source resolution differs from the canvas,
 * the preview stretches it to the canvas, matching the export's
 * "scale to transform W×H" behavior. Audio elements live unstyled inside
 * the box; useMultitrackPreview drives them.
 *
 * Strategy preserved from v0.5.0: only the topmost video track is shown;
 * cross-track composition (PIP / overlays) only takes effect at export.
 * A small "预览仅显示顶层视频轨" chip appears when ≥2 video clips are
 * active at the playhead.
 */
const store = useMultitrackStore()
const videoRef = useTemplateRef<HTMLVideoElement>('videoRef')
const shellEl = useTemplateRef<HTMLDivElement>('shellEl')
const audioRefs = ref<(HTMLAudioElement | null)[]>([])

const audioTracks = computed(() => store.project?.audioTracks ?? [])
const canvas = computed(() => store.project?.canvas ?? { width: 1920, height: 1080, frameRate: 30 })

// Set audio refs in a manner that works with v-for + dynamic length.
function setAudioRef(i: number, el: Element | null) {
  audioRefs.value[i] = el as HTMLAudioElement | null
}

const preview = useMultitrackPreview(videoRef, audioRefs, audioTracks)

defineExpose({ play: preview.play, pause: preview.pause, toggle: preview.toggle, seek: preview.seek })

// ---- Canvas-box sizing ----
//
// The shell's content area is always smaller than the project canvas
// (which can be 4K or larger), so the box is computed each layout pass:
//   s = min(boxW / canvasW, boxH / canvasH)   // fit-contain ratio
//   width  = canvasW * s
//   height = canvasH * s
// We watch the shell's bounding rect with a ResizeObserver — flexbox
// resizing the column doesn't fire window 'resize'.

const shellW = ref(0)
const shellH = ref(0)
let ro: ResizeObserver | null = null

onMounted(() => {
  preview.refresh()
  if (!shellEl.value) return
  ro = new ResizeObserver((entries) => {
    const r = entries[0]?.contentRect
    if (!r) return
    shellW.value = r.width
    shellH.value = r.height
  })
  ro.observe(shellEl.value)
  // Seed with current size so the first paint isn't 0×0.
  const r = shellEl.value.getBoundingClientRect()
  shellW.value = r.width
  shellH.value = r.height
})

onUnmounted(() => {
  ro?.disconnect()
  ro = null
})

const PADDING = 8 // matches the bg-black margin around the canvas box
const canvasBoxStyle = computed(() => {
  const cw = canvas.value.width
  const ch = canvas.value.height
  const availW = Math.max(0, shellW.value - PADDING * 2)
  const availH = Math.max(0, shellH.value - PADDING * 2)
  if (availW <= 0 || availH <= 0 || cw <= 0 || ch <= 0) {
    return { width: '0px', height: '0px' }
  }
  const s = Math.min(availW / cw, availH / ch)
  return {
    width: `${Math.floor(cw * s)}px`,
    height: `${Math.floor(ch * s)}px`,
  }
})

// Top-layer notice: any time we have ≥2 visible video tracks with a clip
// at playhead, only the topmost is shown. The notice only renders when
// the stacking actually applies (≥2 active video clips at playhead).
const stackedActive = computed(() => {
  const p = store.project
  if (!p) return false
  let hits = 0
  for (const t of p.videoTracks) {
    if (t.hidden) continue
    for (const c of t.clips) {
      const e = c.programStart + (c.sourceEnd - c.sourceStart)
      if (store.playhead + 1e-6 >= c.programStart && store.playhead < e - 1e-6) {
        hits++
        if (hits >= 2) return true
        break // only one clip per track is active at any instant
      }
    }
  }
  return false
})

// Keep audioRefs sized in sync with audioTracks so v-for ref callbacks
// land in the right slots after track add / remove.
watch(audioTracks, (tracks) => {
  audioRefs.value.length = tracks.length
})
</script>

<template>
  <div
    ref="shellEl"
    class="relative flex min-h-0 flex-1 items-center justify-center overflow-hidden bg-black"
  >
    <div
      class="relative bg-black"
      :style="canvasBoxStyle"
    >
      <video
        ref="videoRef"
        preload="auto"
        muted
        class="absolute inset-0 h-full w-full object-fill"
      ></video>
      <!-- Per-track audio elements. Hidden visually; useMultitrackPreview
           drives src / currentTime / play state for each. WebAudio routes
           each through its own GainNode so audio-track volume can exceed
           100% (HTMLMediaElement.volume hard-caps at 1.0). -->
      <audio
        v-for="(t, i) in audioTracks"
        :key="t.id"
        :ref="(el) => setAudioRef(i, el as Element | null)"
        preload="auto"
        style="display: none"
      ></audio>
    </div>

    <div
      v-if="stackedActive"
      class="pointer-events-none absolute right-2 top-2 rounded bg-black/60 px-2 py-0.5 text-[10px] text-white/80"
    >预览仅显示顶层视频轨</div>
  </div>
</template>
