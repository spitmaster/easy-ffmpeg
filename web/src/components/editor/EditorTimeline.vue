<script setup lang="ts">
import { computed, useTemplateRef } from 'vue'
import { TRACK_AUDIO, TRACK_VIDEO, type Clip, type Track } from '@/api/editor'
import { useEditorStore, Sel } from '@/stores/editor'
import {
  clipDuration,
  clipProgramEnd,
  totalDuration,
  trackClipsKey,
} from '@/utils/timeline'

const store = useEditorStore()

const emit = defineEmits<{
  (e: 'seek', t: number): void
  (e: 'pause-during-scrub'): void
  (e: 'resume-after-scrub'): void
}>()

const PX_MIN = 0.05
const PX_MAX = 80
const SNAP_PX = 8

const scrollEl = useTemplateRef<HTMLDivElement>('scrollEl')
const rulerEl = useTemplateRef<HTMLDivElement>('rulerEl')

const STEPS = [0.05, 0.1, 0.2, 0.5, 1, 2, 5, 10, 15, 30, 60, 120, 300, 600, 900, 1800, 3600, 7200, 14400]
const TARGET_PX = 90

const total = computed(() => totalDuration(store.project))
const trackWidth = computed(() => Math.max(total.value * store.pxPerSecond + 40, 400))

const tickStep = computed(() => {
  const ideal = TARGET_PX / store.pxPerSecond
  for (const s of STEPS) if (s >= ideal) return s
  return STEPS[STEPS.length - 1]
})

const ticks = computed(() => {
  const step = tickStep.value
  const count = Math.floor(total.value / step) + 1
  const out: { t: number; x: number; label: string }[] = []
  for (let i = 0; i <= count; i++) {
    const t = i * step
    if (t > total.value + 0.01) break
    out.push({ t, x: t * store.pxPerSecond, label: fmtTick(t, step) })
  }
  return out
})

function fmtTick(sec: number, step: number): string {
  const decimals = step >= 1 ? 0 : step >= 0.1 ? 1 : 2
  if (sec >= 3600) {
    const h = Math.floor(sec / 3600)
    const m = Math.floor((sec % 3600) / 60)
    const s = (sec % 60).toFixed(decimals)
    const pad = decimals > 0 ? decimals + 3 : 2
    return `${h}:${String(m).padStart(2, '0')}:${s.padStart(pad, '0')}`
  }
  const m = Math.floor(sec / 60)
  const s = (sec % 60).toFixed(decimals)
  const pad = decimals > 0 ? decimals + 3 : 2
  return `${m}:${s.padStart(pad, '0')}`
}

function fmtShort(sec: number): string {
  const s = Math.round(sec)
  const m = Math.floor(s / 60)
  const ss = s % 60
  return `${m}:${ss.toString().padStart(2, '0')}`
}

const playheadX = computed(() => store.playhead * store.pxPerSecond)
const rangeRect = computed(() => {
  const r = store.rangeSelection
  if (!r) return null
  const a = Math.min(r.start, r.end)
  const b = Math.max(r.start, r.end)
  return { left: a * store.pxPerSecond, width: Math.max(1, (b - a) * store.pxPerSecond) }
})

// ---- Fit-to-width (called by EditorView after loadProject) ----

function applyFit() {
  const scroll = scrollEl.value
  if (!scroll || !store.project) return
  const t = total.value
  if (t <= 0) {
    store.pxPerSecond = 8
    return
  }
  const viewW = Math.max(100, scroll.clientWidth - 24)
  store.pxPerSecond = Math.max(PX_MIN, Math.min(PX_MAX, viewW / t))
}

defineExpose({ applyFit })

// ---- Wheel: ctrl/meta zoom around cursor; plain wheel horizontal scroll ----

function onWheel(ev: WheelEvent) {
  if (!scrollEl.value) return
  if (ev.ctrlKey || ev.metaKey) {
    ev.preventDefault()
    if (!store.project) return
    const rect = scrollEl.value.getBoundingClientRect()
    const anchorX = ev.clientX - rect.left + scrollEl.value.scrollLeft
    const anchorTime = anchorX / store.pxPerSecond
    const factor = Math.exp(-ev.deltaY * 0.0015)
    const next = Math.max(PX_MIN, Math.min(PX_MAX, store.pxPerSecond * factor))
    store.pxPerSecond = next
    // Keep the time under the cursor stationary on screen.
    const newAnchorX = anchorTime * next
    scrollEl.value.scrollLeft = newAnchorX - (ev.clientX - rect.left)
  } else if (ev.deltaY !== 0 && ev.deltaX === 0) {
    ev.preventDefault()
    scrollEl.value.scrollLeft += ev.deltaY
  }
}

// ---- Ruler: left-drag scrub, right-drag range select ----

function clientXToTime(clientX: number, clamp = true): number {
  const r = rulerEl.value
  if (!r) return 0
  const rect = r.getBoundingClientRect()
  const x = clientX - rect.left
  const t = x / store.pxPerSecond
  if (!clamp) return t
  return Math.max(0, Math.min(total.value, t))
}

function startScrubDrag(ev: MouseEvent) {
  ev.preventDefault()
  const wasPlaying = store.playing
  if (wasPlaying) emit('pause-during-scrub')
  emit('seek', clientXToTime(ev.clientX))
  function onMove(e: MouseEvent) {
    emit('seek', clientXToTime(e.clientX))
  }
  function onUp() {
    document.removeEventListener('mousemove', onMove)
    document.removeEventListener('mouseup', onUp)
    if (wasPlaying) emit('resume-after-scrub')
  }
  document.addEventListener('mousemove', onMove)
  document.addEventListener('mouseup', onUp)
}

function onRulerMouseDown(ev: MouseEvent) {
  if (ev.button === 2) {
    startRangeSelect(ev)
    return
  }
  store.splitScope = 'both'
  store.selection = []
  store.rangeSelection = null
  startScrubDrag(ev)
}

function startRangeSelect(ev: MouseEvent) {
  ev.preventDefault()
  if (!store.project) return
  const anchor = clientXToTime(ev.clientX)
  store.rangeSelection = { start: anchor, end: anchor }
  store.selection = []
  store.splitScope = 'both'
  function onMove(e: MouseEvent) {
    store.rangeSelection = { start: anchor, end: clientXToTime(e.clientX) }
  }
  function onUp() {
    document.removeEventListener('mousemove', onMove)
    document.removeEventListener('mouseup', onUp)
    const r = store.rangeSelection
    if (!r) return
    const a = Math.min(r.start, r.end)
    const b = Math.max(r.start, r.end)
    if (b - a < 0.05) store.rangeSelection = null
    else store.rangeSelection = { start: a, end: b }
  }
  document.addEventListener('mousemove', onMove)
  document.addEventListener('mouseup', onUp)
}

function onPlayheadMouseDown(ev: MouseEvent) {
  if (ev.button !== 0) return
  ev.stopPropagation()
  if (store.rangeSelection) store.rangeSelection = null
  startScrubDrag(ev)
}

// ---- Clip interactions: select / trim handle / reorder ----

function onTrackMouseDown(ev: MouseEvent, trackId: Track) {
  if (ev.button !== 0) return
  if (store.rangeSelection) store.rangeSelection = null

  const target = ev.target as HTMLElement
  const clipEl = target.closest('.clip') as HTMLElement | null
  if (!clipEl) {
    // empty area → narrow split scope to this track + scrub.
    store.splitScope = trackId
    store.selection = []
    startScrubDrag(ev)
    return
  }
  const handle = target.closest('.clip-handle') as HTMLElement | null
  const clipId = clipEl.dataset.clipId || ''
  const multi = ev.shiftKey || ev.ctrlKey || ev.metaKey
  store.selection = multi
    ? Sel.toggle(store.selection, trackId, clipId)
    : Sel.replace(trackId, clipId)
  store.splitScope = trackId

  if (handle) {
    const side = handle.dataset.handle as 'left' | 'right'
    startTrimDrag(ev, trackId, clipId, side)
  } else if (!multi) {
    startReorderDrag(ev, trackId, clipId)
  }
}

function startTrimDrag(ev: MouseEvent, trackId: Track, clipId: string, side: 'left' | 'right') {
  ev.preventDefault()
  const project = store.project
  if (!project) return
  const key = trackClipsKey(trackId)
  const original = (project[key] || []).map((c) => ({ ...c }))
  const idx = original.findIndex((c) => c.id === clipId)
  if (idx < 0) return
  const ppS = store.pxPerSecond
  const startX = ev.clientX
  const origClip = { ...original[idx] }
  const sourceMax = project.source?.duration || origClip.sourceEnd + 600

  function onMove(e: MouseEvent) {
    const dx = e.clientX - startX
    const ds = dx / ppS
    const clips = original.map((c) => ({ ...c }))
    const c = clips[idx]
    if (side === 'left') {
      const newStart = Math.max(0, Math.min(origClip.sourceEnd - 0.05, origClip.sourceStart + ds))
      const delta = newStart - origClip.sourceStart
      c.sourceStart = newStart
      c.programStart = Math.max(0, origClip.programStart + delta)
    } else {
      const newEnd = Math.max(origClip.sourceStart + 0.05, Math.min(sourceMax, origClip.sourceEnd + ds))
      c.sourceEnd = newEnd
    }
    store.applyProjectPatch({ [key]: clips }, { save: false })
  }
  function onUp() {
    document.removeEventListener('mousemove', onMove)
    document.removeEventListener('mouseup', onUp)
    store.pushHistory()
    store.scheduleSave()
  }
  document.addEventListener('mousemove', onMove)
  document.addEventListener('mouseup', onUp)
}

function startReorderDrag(ev: MouseEvent, trackId: Track, clipId: string) {
  ev.preventDefault()
  const project = store.project
  if (!project) return
  const key = trackClipsKey(trackId)
  const original = (project[key] || []).map((c) => ({ ...c }))
  const idx = original.findIndex((c) => c.id === clipId)
  if (idx < 0) return
  const ppS = store.pxPerSecond
  const startX = ev.clientX
  const origProgramStart = original[idx].programStart
  const dur = clipDuration(original[idx])

  const snapPoints: number[] = [0, store.playhead]
  original.forEach((c, i) => {
    if (i === idx) return
    snapPoints.push(c.programStart)
    snapPoints.push(clipProgramEnd(c))
  })

  function snapToNearest(candidateStart: number): number {
    const candidateEnd = candidateStart + dur
    const snapSec = SNAP_PX / ppS
    let bestDelta = Infinity
    let bestStart = candidateStart
    for (const p of snapPoints) {
      const dL = Math.abs(candidateStart - p)
      if (dL < bestDelta && dL <= snapSec) {
        bestDelta = dL
        bestStart = p
      }
      const dR = Math.abs(candidateEnd - p)
      if (dR < bestDelta && dR <= snapSec) {
        bestDelta = dR
        bestStart = p - dur
      }
    }
    return Math.max(0, bestStart)
  }

  function onMove(e: MouseEvent) {
    const dx = e.clientX - startX
    const raw = Math.max(0, origProgramStart + dx / ppS)
    const snapped = snapToNearest(raw)
    const clips = original.map((c) => ({ ...c }))
    clips[idx].programStart = snapped
    store.applyProjectPatch({ [key]: clips }, { save: false })
  }
  function onUp() {
    document.removeEventListener('mousemove', onMove)
    document.removeEventListener('mouseup', onUp)
    const finalClip = (store.project?.[key] || []).find((c) => c.id === clipId)
    if (finalClip && Math.abs(finalClip.programStart - origProgramStart) > 1e-6) {
      store.pushHistory()
    }
    store.scheduleSave()
  }
  document.addEventListener('mousemove', onMove)
  document.addEventListener('mouseup', onUp)
}

function clipClasses(c: Clip, trackId: Track) {
  return {
    selected: Sel.has(store.selection, trackId, c.id),
  }
}

function videoClips(): Clip[] {
  return store.project?.videoClips || []
}
function audioClips(): Clip[] {
  return store.project?.audioClips || []
}

// Suppress browser context menu — right-click is repurposed for range select.
function onContextMenu(ev: MouseEvent) {
  ev.preventDefault()
}
</script>

<template>
  <!-- shrink-0 + h-48 makes the timeline a fixed bottom bar regardless of
       viewport height, so a wide-but-short window (laptop in horizontal
       orientation) doesn't squeeze it to nothing. The preview takes all
       remaining vertical space via flex-1 in EditorView. -->
  <div
    class="relative flex h-48 shrink-0 overflow-hidden border-t border-border-base bg-bg-base"
    @contextmenu="onContextMenu"
  >
    <!-- Left: track labels. Each row's height matches the right-hand
         ruler / track heights so the labels stay aligned with their
         tracks. -->
    <div class="flex w-24 shrink-0 flex-col border-r border-border-base bg-bg-panel text-xs">
      <div class="h-7 shrink-0 border-b border-border-base"></div>
      <div class="flex h-12 shrink-0 items-center px-2">🎬 视频</div>
      <!-- Audio row is taller — the slot stacks the label + volume button
           vertically, h-12 was too tight. Right-side audio track height
           below mirrors this so labels and tracks stay aligned. -->
      <div class="flex h-14 shrink-0 items-center border-t border-border-base px-2 py-1">
        <slot name="audio-label">🔊 音频</slot>
      </div>
    </div>

    <!-- Right: scroll -->
    <div
      ref="scrollEl"
      class="relative flex-1 overflow-x-auto overflow-y-hidden"
      @wheel="onWheel"
    >
      <!-- Ruler -->
      <div
        ref="rulerEl"
        class="relative h-7 cursor-pointer select-none border-b border-border-base bg-bg-panel"
        :style="{ width: trackWidth + 'px' }"
        @mousedown="onRulerMouseDown"
      >
        <template v-for="t in ticks" :key="t.t">
          <div
            class="pointer-events-none absolute top-3 h-2 w-px bg-fg-subtle"
            :style="{ left: t.x + 'px' }"
          ></div>
          <div
            class="pointer-events-none absolute top-0.5 select-none whitespace-nowrap text-[10px] text-fg-muted"
            :style="{ left: t.x + 4 + 'px' }"
          >{{ t.label }}</div>
        </template>
      </div>

      <!-- Video track -->
      <div
        class="relative h-12 select-none border-b border-border-base"
        :style="{ width: trackWidth + 'px' }"
        @mousedown="onTrackMouseDown($event, TRACK_VIDEO)"
      >
        <div
          v-for="c in videoClips()"
          :key="c.id"
          class="clip absolute top-1 bottom-1 cursor-grab overflow-hidden rounded border border-accent/60 bg-accent/30 px-1 text-[10px] text-fg-base shadow-sm hover:bg-accent/50"
          :class="clipClasses(c, TRACK_VIDEO)"
          :data-clip-id="c.id"
          :style="{ left: c.programStart * store.pxPerSecond + 'px', width: Math.max(8, clipDuration(c) * store.pxPerSecond) + 'px' }"
        >
          <span class="clip-label pointer-events-none block truncate">
            {{ fmtShort(c.sourceStart) }} - {{ fmtShort(c.sourceEnd) }}
          </span>
          <div class="clip-handle absolute inset-y-0 left-0 w-1.5 cursor-ew-resize bg-accent/70 hover:bg-accent" data-handle="left"></div>
          <div class="clip-handle absolute inset-y-0 right-0 w-1.5 cursor-ew-resize bg-accent/70 hover:bg-accent" data-handle="right"></div>
        </div>
      </div>

      <!-- Audio track. Height matches the audio label row on the left. -->
      <div
        class="relative h-14 select-none"
        :style="{ width: trackWidth + 'px' }"
        @mousedown="onTrackMouseDown($event, TRACK_AUDIO)"
      >
        <div
          v-for="c in audioClips()"
          :key="c.id"
          class="clip absolute top-1 bottom-1 cursor-grab overflow-hidden rounded border border-success/60 bg-success/30 px-1 text-[10px] text-fg-base shadow-sm hover:bg-success/50"
          :class="clipClasses(c, TRACK_AUDIO)"
          :data-clip-id="c.id"
          :style="{ left: c.programStart * store.pxPerSecond + 'px', width: Math.max(8, clipDuration(c) * store.pxPerSecond) + 'px' }"
        >
          <span class="clip-label pointer-events-none block truncate">
            {{ fmtShort(c.sourceStart) }} - {{ fmtShort(c.sourceEnd) }}
          </span>
          <div class="clip-handle absolute inset-y-0 left-0 w-1.5 cursor-ew-resize bg-success/70 hover:bg-success" data-handle="left"></div>
          <div class="clip-handle absolute inset-y-0 right-0 w-1.5 cursor-ew-resize bg-success/70 hover:bg-success" data-handle="right"></div>
        </div>
      </div>

      <!-- Range selection overlay -->
      <div
        v-if="rangeRect"
        class="pointer-events-none absolute top-0 bottom-0 z-10 bg-accent/15 ring-1 ring-accent/50"
        :style="{ left: rangeRect.left + 'px', width: rangeRect.width + 'px' }"
      ></div>

      <!-- Playheads -->
      <div
        v-show="store.project && store.splitScope === 'both'"
        class="pointer-events-auto absolute top-0 bottom-0 z-20 w-px cursor-ew-resize bg-danger"
        :style="{ left: playheadX + 'px' }"
        @mousedown="onPlayheadMouseDown"
      ></div>
      <div
        v-show="store.project && store.splitScope === TRACK_VIDEO"
        class="pointer-events-auto absolute top-7 z-20 h-12 w-px cursor-ew-resize bg-danger"
        :style="{ left: playheadX + 'px' }"
        @mousedown="onPlayheadMouseDown"
      ></div>
      <div
        v-show="store.project && store.splitScope === TRACK_AUDIO"
        class="pointer-events-auto absolute top-[76px] z-20 h-14 w-px cursor-ew-resize bg-danger"
        :style="{ left: playheadX + 'px' }"
        @mousedown="onPlayheadMouseDown"
      ></div>
    </div>
  </div>
</template>

<style scoped>
.clip.selected {
  @apply ring-2 ring-fg-base;
}
</style>
