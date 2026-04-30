<script setup lang="ts">
import { computed, onMounted, ref, useTemplateRef, watch } from 'vue'
import { useMultitrackStore } from '@/stores/multitrack'
import { useMultitrackPreview } from '@/composables/useMultitrackPreview'

/**
 * Top-half preview pane.
 *
 * Layout: a single <video> overlaid by a stack of <audio> elements (one
 * per audio track). Drives them through useMultitrackPreview, which picks
 * the topmost video source for picture and syncs each <audio> to its
 * track's active clip.
 *
 * v1 strategy: only the top-most video track is shown (M2 design lock).
 * When stacked clips overlap a small chip is rendered to inform the user.
 */
const store = useMultitrackStore()
const videoRef = useTemplateRef<HTMLVideoElement>('videoRef')
const audioRefs = ref<(HTMLAudioElement | null)[]>([])

const audioTracks = computed(() => store.project?.audioTracks ?? [])

// Set audio refs in a manner that works with v-for + dynamic length.
function setAudioRef(i: number, el: Element | null) {
  audioRefs.value[i] = el as HTMLAudioElement | null
}

const preview = useMultitrackPreview(videoRef, audioRefs, audioTracks)

defineExpose({ play: preview.play, pause: preview.pause, toggle: preview.toggle, seek: preview.seek })

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

onMounted(() => {
  preview.refresh()
})
</script>

<template>
  <div class="relative flex min-h-0 flex-1 items-center justify-center bg-black">
    <video
      ref="videoRef"
      preload="auto"
      muted
      class="max-h-full max-w-full object-contain"
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

    <div
      v-if="stackedActive"
      class="pointer-events-none absolute right-2 top-2 rounded bg-black/60 px-2 py-0.5 text-[10px] text-white/80"
    >预览仅显示顶层视频轨</div>
  </div>
</template>
