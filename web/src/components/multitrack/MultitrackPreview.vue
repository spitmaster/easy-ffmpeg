<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, useTemplateRef, watch } from 'vue'
import type { MultitrackTransform, MultitrackVideoTrack } from '@/api/multitrack'
import { useMultitrackStore } from '@/stores/multitrack'
import { useMultitrackPreview } from '@/composables/useMultitrackPreview'
import TransformOverlay from '@/components/multitrack/TransformOverlay.vue'

/**
 * Top-half preview pane.
 *
 * Layout: an outer "preview-shell" centers a "preview-canvas" box that
 * scales to fit the available area while preserving the project canvas's
 * aspect ratio (v0.5.1+). Inside the canvas-box we render one `<video>`
 * element per video track, positioned + sized via the track's currently
 * active clip transform (X, Y, W, H in canvas coords → percentages of
 * the canvas-box). Stacking: lower track index = higher z-index, mirroring
 * the export overlay chain. Tracks with no active clip at the playhead
 * are hidden (v-show:false) so they don't paint a stale frame on top.
 *
 * The canvas-box itself uses a checkerboard pattern (two greys, 16px
 * cells) so the project's drawable area is visible against the black
 * shell background even when no clip occupies a region. Audio elements
 * live unstyled inside the box; useMultitrackPreview drives them.
 *
 * Clock arbitration is handled inside useMultitrackPreview: the topmost
 * (lowest-index) active track's `<video>` is the master driving the
 * playhead via timeupdate; the rest follow via rAF currentTime correction.
 */
const store = useMultitrackStore()
const videoRefs = ref<(HTMLVideoElement | null)[]>([])
const shellEl = useTemplateRef<HTMLDivElement>('shellEl')
const audioRefs = ref<(HTMLAudioElement | null)[]>([])

const videoTracks = computed(() => store.project?.videoTracks ?? [])
const audioTracks = computed(() => store.project?.audioTracks ?? [])
const canvas = computed(() => store.project?.canvas ?? { width: 1920, height: 1080, frameRate: 30 })

// Set refs in a manner that works with v-for + dynamic length.
function setVideoRef(i: number, el: Element | null) {
  videoRefs.value[i] = el as HTMLVideoElement | null
}
function setAudioRef(i: number, el: Element | null) {
  audioRefs.value[i] = el as HTMLAudioElement | null
}

const preview = useMultitrackPreview(videoRefs, audioRefs, videoTracks, audioTracks)

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

// Per-track style: position + size of the <video> inside the canvas-box,
// derived from the track's currently active clip's transform. When the
// track has no active clip at the playhead, `visible` is false and the
// element gets v-show:false — keeps the DOM warm (preserves loaded src,
// avoids preroll on next entry) without painting a stale frame.
//
// z-index convention (v0.5.1+): lower track index = top of z. We set
// z-index = (N - i) so the array order maps directly: tracks[0] sits on
// top of the stack, tracks[N-1] at the bottom.
interface TrackVisual {
  visible: boolean
  style: Record<string, string>
}
function trackVisual(track: MultitrackVideoTrack, i: number, total: number): TrackVisual {
  const cw = canvas.value.width
  const ch = canvas.value.height
  const active = store.videoActive(track, store.playhead)
  if (!active || cw <= 0 || ch <= 0) {
    return { visible: false, style: { zIndex: String(total - i) } }
  }
  const t = active.clip.transform
  return {
    visible: true,
    style: {
      left: `${(t.x / cw) * 100}%`,
      top: `${(t.y / ch) * 100}%`,
      width: `${(t.w / cw) * 100}%`,
      height: `${(t.h / ch) * 100}%`,
      zIndex: String(total - i),
    },
  }
}

const trackVisuals = computed<TrackVisual[]>(() => {
  const tracks = videoTracks.value
  const total = tracks.length
  return tracks.map((tr, i) => trackVisual(tr, i, total))
})

// Keep videoRefs / audioRefs sized in sync so v-for ref callbacks land
// in the right slots after track add / remove.
watch(videoTracks, (tracks) => {
  videoRefs.value.length = tracks.length
})
watch(audioTracks, (tracks) => {
  audioRefs.value.length = tracks.length
})

// ---- Selected-clip transform overlay (v0.5.1 / M5) ----
//
// The overlay only renders for video-track selections. Audio selections
// don't have a transform concept; the store's selectedVideoClip filters
// these out for us. Hide during export so the user can't reposition while
// ffmpeg renders. The overlay sits above the video stack via z-30 — high
// enough to clear any realistic video track count (videoStyle uses
// zIndex = N - i, < 30 for any sane N) but lower than dialogs / modals
// (z-40 for CanvasSettingsDialog/ExportDialog/ProjectsModal, z-50 for the
// confirm modals), so opening a dialog while a clip is selected doesn't
// leave the selection box on top.
const overlayVisible = computed(
  () => !!store.selectedVideoClip && !store.exportLocked,
)

// Source aspect ratio for the selected clip — fed to TransformOverlay so
// that Shift-drag locks against the original shape (not the current
// transform). Undefined when source dimensions aren't known; the overlay
// then falls back to the live transform's ratio.
const selectedSourceRatio = computed<number | undefined>(() => {
  const sel = store.selectedVideoClip
  if (!sel) return undefined
  const src = store.sourcesById[sel.clip.sourceId]
  if (!src || !src.width || !src.height || src.width <= 0 || src.height <= 0) return undefined
  return src.width / src.height
})

function onTransformPreview(t: MultitrackTransform) {
  const sel = store.selectedVideoClip
  if (!sel) return
  store.previewClipTransform(sel.trackId, sel.clipId, t)
}

function onTransformCommit(t: MultitrackTransform) {
  const sel = store.selectedVideoClip
  if (!sel) return
  store.commitClipTransform(sel.trackId, sel.clipId, t)
}

// Click on a track's <video> to select that track's currently-active clip.
// Only fires when the click lands on the visible <video> rectangle (so the
// tinted-canvas background and gap regions don't accidentally select). The
// TransformOverlay sits on top via z-30; clicks on its handles are caught
// there before reaching the video, so resizing doesn't double-select.
function onVideoClick(track: MultitrackVideoTrack) {
  if (store.exportLocked) return
  const active = store.videoActive(track, store.playhead)
  if (!active) return
  store.selection = [{ trackId: track.id, clipId: active.clip.id }]
}
</script>

<template>
  <div
    ref="shellEl"
    class="relative flex min-h-0 flex-1 items-center justify-center overflow-hidden bg-black"
  >
    <div
      class="canvas-box relative"
      :style="canvasBoxStyle"
    >
      <!-- Per-track <video> stack. Each track's element is positioned by
           its current active clip's transform. Tracks with no active clip
           are hidden via v-show so they don't paint a frame on top. -->
      <video
        v-for="(t, i) in videoTracks"
        :key="t.id"
        v-show="trackVisuals[i]?.visible"
        :ref="(el) => setVideoRef(i, el as Element | null)"
        preload="auto"
        muted
        class="absolute cursor-pointer object-fill"
        :style="trackVisuals[i]?.style"
        @click="onVideoClick(t)"
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

      <!-- Selection box for the currently selected video clip (v0.5.1).
           Draws above all <video> elements via z-index in the component. -->
      <TransformOverlay
        v-if="overlayVisible && store.selectedVideoClip"
        :canvas="canvas"
        :transform="store.selectedVideoClip.clip.transform"
        :source-ratio="selectedSourceRatio"
        @update="onTransformPreview"
        @commit="onTransformCommit"
      />
    </div>
  </div>
</template>

<style scoped>
/* Photoshop-style transparency checkerboard so the canvas-box reads as a
   distinct surface against the black shell. Two greys (#2b2b2b / #1f1f1f)
   alternate in 16px cells — clearly visible but not loud enough to fight
   the video content drawn on top. */
.canvas-box {
  background-color: #2b2b2b;
  background-image:
    linear-gradient(45deg, #1f1f1f 25%, transparent 25%),
    linear-gradient(-45deg, #1f1f1f 25%, transparent 25%),
    linear-gradient(45deg, transparent 75%, #1f1f1f 75%),
    linear-gradient(-45deg, transparent 75%, #1f1f1f 75%);
  background-size: 16px 16px;
  background-position: 0 0, 0 8px, 8px -8px, 8px 0;
}
</style>
