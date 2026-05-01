<script setup lang="ts">
import { computed, onActivated, onBeforeUnmount, onDeactivated, onMounted, ref, toRef, useTemplateRef, watch } from 'vue'
import {
  SOURCE_VIDEO,
  type MultitrackClip,
  type MultitrackSource,
} from '@/api/multitrack'
import type { ProjectsModalItem, RangeSelection, TrackData, TrackTone } from '@/types/timeline'
import { useDirsStore } from '@/stores/dirs'
import { useMultitrackStore, MultitrackSel } from '@/stores/multitrack'
import { useModalsStore } from '@/stores/modals'
import { useMultitrackOps } from '@/composables/useMultitrackOps'
import { useTimelineDrag } from '@/composables/timeline/useTimelineDrag'
import { useTimelinePlayback } from '@/composables/timeline/useTimelinePlayback'
import { useTimelineRangeSelect } from '@/composables/timeline/useTimelineRangeSelect'
import { useTimelineZoom } from '@/composables/timeline/useTimelineZoom'
import { Path } from '@/utils/path'
import MultitrackLibrary from '@/components/multitrack/MultitrackLibrary.vue'
import MultitrackPreview from '@/components/multitrack/MultitrackPreview.vue'
import MultitrackToolbar from '@/components/multitrack/MultitrackToolbar.vue'
import PlayBar from '@/components/timeline-shared/PlayBar.vue'
import ProjectsModal from '@/components/timeline-shared/ProjectsModal.vue'
import TimelinePlayhead from '@/components/timeline-shared/TimelinePlayhead.vue'
import TimelineRangeSelection from '@/components/timeline-shared/TimelineRangeSelection.vue'
import TimelineRuler from '@/components/timeline-shared/TimelineRuler.vue'
import TimelineTrackRow from '@/components/timeline-shared/TimelineTrackRow.vue'

/**
 * Multitrack editor view — M7.
 *
 * Layout:
 *   topbar (project actions + add-track buttons + undo/redo + library toggle)
 *   library | preview / playbar / timeline (video tracks ↑, audio tracks ↓)
 *
 * Drag/drop:
 *   - Library item dragged out carries `application/x-easy-ffmpeg-source`
 *     with `{ sourceId }`.
 *   - Drop on the timeline area: video source → +video track + +audio track
 *     (when hasAudio); audio source → +audio track. Existing track of the
 *     right kind also accepts a drop, appending the clip at the playhead.
 *   - Cross-track clip drag (V→V / A→A) is wired through useTimelineDrag's
 *     findTargetTrack / onCrossTrack callbacks; V→A and A→V drops are
 *     rejected at findTargetTrack time so onCrossTrack never fires.
 */

const store = useMultitrackStore()
const dirs = useDirsStore()
const modals = useModalsStore()
const ops = useMultitrackOps()

const projectsOpen = ref(false)
const importing = ref(false)
const libraryRef = useTemplateRef<{ setError: (msg: string) => void }>('libraryRef')
const previewRef = useTemplateRef<{ play: () => void; pause: () => void; toggle: () => void; seek: (t: number) => void }>('previewRef')
const scrollEl = useTemplateRef<HTMLDivElement>('scrollEl')
const rulerScrollEl = useTemplateRef<HTMLDivElement>('rulerScrollEl')
const labelsEl = useTemplateRef<HTMLDivElement>('labelsEl')
const bodyEl = useTemplateRef<HTMLDivElement>('bodyEl')
const rulerCmp = useTemplateRef<{ rootEl: HTMLDivElement | null }>('rulerCmp')

/**
 * X-scroll sync: only `scrollEl` shows a horizontal scrollbar; the ruler
 * container at the top mirrors it so ruler and tracks stay aligned at any
 * scrollLeft. The label column is overflow-hidden — it inherits Y from the
 * body Y-scroll container, no X scroll there.
 */
function onTracksScroll() {
  const s = scrollEl.value
  const r = rulerScrollEl.value
  if (s && r && r.scrollLeft !== s.scrollLeft) r.scrollLeft = s.scrollLeft
}
/** Body Y-scroll: keep label column's scrollTop aligned with tracks. */
function onBodyScroll() {
  const b = bodyEl.value
  const l = labelsEl.value
  if (b && l && l.scrollTop !== b.scrollTop) l.scrollTop = b.scrollTop
}

const hasProject = computed(() => !!store.project)

// ---- Derived timeline data ----

const total = computed(() => Math.max(store.programDuration, 0))
const trackWidth = computed(() => Math.max(total.value * store.pxPerSecond + 40, 400))
const playheadX = computed(() => store.playhead * store.pxPerSecond)

// Color tones cycle so adjacent tracks don't blend together visually.
const VIDEO_TONES: TrackTone[] = ['accent', 'danger']
const AUDIO_TONES: TrackTone[] = ['success', 'accent']

const videoTracksData = computed<TrackData[]>(() =>
  (store.project?.videoTracks ?? []).map((t, i) => ({
    id: t.id,
    kind: 'video',
    clips: t.clips,
    tone: VIDEO_TONES[i % VIDEO_TONES.length],
    label: `视频 ${i + 1}`,
  })),
)
const audioTracksData = computed<TrackData[]>(() =>
  (store.project?.audioTracks ?? []).map((t, i) => ({
    id: t.id,
    kind: 'audio',
    clips: t.clips,
    tone: AUDIO_TONES[i % AUDIO_TONES.length],
    volume: t.volume,
    label: `音频 ${i + 1}`,
  })),
)

function selectedIdsForTrack(trackId: string): string[] {
  return MultitrackSel.inTrack(store.selection, trackId)
}

// Playhead visibility per splitScope.
const showPlayheadAll = computed(() => store.splitScope === 'all')
const showPlayheadVideo = computed(() => store.splitScope === 'video')
const showPlayheadAudio = computed(() => store.splitScope === 'audio')
const ROW_PX = 48
/**
 * Single-track scope playhead top (in body-tracks content coordinates,
 * which start at 0 — the ruler lives in its own container above the body
 * Y-scroll, so the offset is purely the row stack index).
 */
const singleTrackScopeBodyTop = computed<number | null>(() => {
  const s = store.splitScope
  if (typeof s !== 'object') return null
  const p = store.project
  if (!p) return null
  const vIdx = p.videoTracks.findIndex((t) => t.id === s.id)
  if (vIdx >= 0) return vIdx * ROW_PX
  const aIdx = p.audioTracks.findIndex((t) => t.id === s.id)
  if (aIdx >= 0) return p.videoTracks.length * ROW_PX + aIdx * ROW_PX
  return null
})

// ---- Timeline composables ----

const zoom = useTimelineZoom({
  pxPerSecond: toRef(store, 'pxPerSecond'),
  totalSec: () => total.value,
  scrollEl,
})

/**
 * Resolve a track id from any DOM element in the timeline area. Track
 * rows tag themselves with data-mt-track-id and data-mt-track-kind via
 * outer wrappers in the template (see render below).
 */
function findTrackUnderCursor(ev: MouseEvent): { id: string; kind: 'video' | 'audio' } | null {
  const target = ev.target as HTMLElement | null
  if (!target) return null
  const el = target.closest('[data-mt-track-id]') as HTMLElement | null
  if (!el) return null
  const id = el.dataset.mtTrackId
  const kind = el.dataset.mtTrackKind as 'video' | 'audio' | undefined
  if (!id || (kind !== 'video' && kind !== 'audio')) return null
  return { id, kind }
}

/** Track id of the clip currently being dragged — used to gate cross-kind moves. */
let dragKind: 'video' | 'audio' | null = null

const drag = useTimelineDrag<MultitrackClip>({
  pxPerSecond: () => store.pxPerSecond,
  playhead: () => store.playhead,
  getClips: (trackId) => {
    const p = store.project
    if (!p) return []
    return (
      p.videoTracks.find((t) => t.id === trackId)?.clips
      ?? p.audioTracks.find((t) => t.id === trackId)?.clips
      ?? []
    )
  },
  setClips: (trackId, clips) => {
    const p = store.project
    if (!p) return
    if (p.videoTracks.some((t) => t.id === trackId)) {
      const next = p.videoTracks.map((t) => (t.id === trackId ? { ...t, clips } : t))
      store.applyProjectPatch({ videoTracks: next }, { save: false })
    } else {
      const next = p.audioTracks.map((t) => (t.id === trackId ? { ...t, clips } : t))
      store.applyProjectPatch({ audioTracks: next }, { save: false })
    }
  },
  pushHistory: () => store.pushHistory(),
  scheduleSave: () => store.scheduleSave(),
  // Multitrack: each clip resolves source duration via clip.sourceId.
  sourceMaxFor: (clip) => store.sourcesById[clip.sourceId]?.duration ?? 0,
  findTargetTrack: (ev) => {
    const hit = findTrackUnderCursor(ev)
    if (!hit) return null
    if (dragKind && hit.kind !== dragKind) return null
    return hit.id
  },
  onCrossTrack: (from, to, clipId, programStart) => {
    if (!dragKind) return false
    store.moveClipAcrossTracks(dragKind, from, to, clipId, programStart)
    return true
  },
})

const rulerEl = computed(() => rulerCmp.value?.rootEl ?? null)
const rangeSelect = useTimelineRangeSelect({
  rulerEl,
  pxPerSecond: () => store.pxPerSecond,
  totalSec: () => total.value,
  setRange: (r: RangeSelection | null) => {
    store.rangeSelection = r
  },
  onStart: () => {
    store.selection = []
    store.splitScope = 'all'
  },
})

const playback = useTimelinePlayback({
  isLocked: () => false, // M8 will gate on a multitrack export job
  togglePlay: () => previewRef.value?.toggle(),
  splitAtPlayhead: () => ops.splitAtPlayhead(),
  deleteSelection: () => ops.deleteSelection(),
  seekBackBoundary: () => seekToNearestBoundary(-1),
  seekForwardBoundary: () => seekToNearestBoundary(1),
  undo: () => store.undo(),
  redo: () => store.redo(),
  clearRangeSelection: () => {
    if (store.rangeSelection) store.rangeSelection = null
  },
  extraBindings: [
    {
      keys: ['l', 'L'],
      ctrl: true,
      action: (e) => {
        e.preventDefault()
        store.libraryCollapsed = !store.libraryCollapsed
      },
    },
  ],
})

// ---- Boundary seek (collect all clips' boundaries across all tracks) ----

function seekToNearestBoundary(direction: -1 | 1) {
  const p = store.project
  if (!p) return
  const xs = new Set<number>()
  xs.add(0)
  for (const t of p.videoTracks) {
    for (const c of t.clips) {
      xs.add(c.programStart)
      xs.add(c.programStart + (c.sourceEnd - c.sourceStart))
    }
  }
  for (const t of p.audioTracks) {
    for (const c of t.clips) {
      xs.add(c.programStart)
      xs.add(c.programStart + (c.sourceEnd - c.sourceStart))
    }
  }
  const sorted = Array.from(xs).sort((a, b) => a - b)
  const cur = store.playhead
  if (direction < 0) {
    for (let k = sorted.length - 1; k >= 0; k--) {
      if (sorted[k] < cur - 0.05) {
        previewRef.value?.seek(sorted[k])
        return
      }
    }
    previewRef.value?.seek(0)
  } else {
    for (const b of sorted) {
      if (b > cur + 0.05) {
        previewRef.value?.seek(b)
        return
      }
    }
    if (sorted.length > 0) previewRef.value?.seek(sorted[sorted.length - 1])
  }
}

// ---- Timeline mouse handlers ----

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
  if (wasPlaying) previewRef.value?.pause()
  previewRef.value?.seek(clientXToTime(ev.clientX))
  function onMove(e: MouseEvent) {
    previewRef.value?.seek(clientXToTime(e.clientX))
  }
  function onUp() {
    document.removeEventListener('mousemove', onMove)
    document.removeEventListener('mouseup', onUp)
    if (wasPlaying) previewRef.value?.play()
  }
  document.addEventListener('mousemove', onMove)
  document.addEventListener('mouseup', onUp)
}

function onRulerMouseDown(ev: MouseEvent) {
  if (ev.button === 2) {
    rangeSelect.start(ev)
    return
  }
  store.splitScope = 'all'
  store.selection = []
  store.rangeSelection = null
  startScrubDrag(ev)
}

function onPlayheadMouseDown(ev: MouseEvent) {
  if (ev.button !== 0) return
  ev.stopPropagation()
  if (store.rangeSelection) store.rangeSelection = null
  startScrubDrag(ev)
}

function onTrackMouseDown(
  trackId: string,
  kind: 'video' | 'audio',
  payload: { ev: MouseEvent; clipId?: string; handle?: 'left' | 'right' },
) {
  const { ev, clipId, handle } = payload
  if (ev.button !== 0) return
  if (store.rangeSelection) store.rangeSelection = null

  if (!clipId) {
    // Empty area → narrow split scope to this track + scrub.
    store.splitScope = { kind: 'track', id: trackId }
    store.selection = []
    startScrubDrag(ev)
    return
  }
  const multi = ev.shiftKey || ev.ctrlKey || ev.metaKey
  store.selection = multi
    ? MultitrackSel.toggle(store.selection, trackId, clipId)
    : MultitrackSel.replace(trackId, clipId)
  store.splitScope = { kind: 'track', id: trackId }
  if (handle) {
    drag.startTrim(ev, trackId, clipId, handle)
  } else if (!multi) {
    dragKind = kind
    drag.startReorder(ev, trackId, clipId)
  }
}

// Suppress browser context menu — right-click on ruler is range select.
function onContextMenu(ev: MouseEvent) {
  ev.preventDefault()
}

// ---- Project lifecycle ----

async function onCreate() {
  const name = window.prompt('新建多轨工程名称(留空则使用默认)', '')
  if (name === null) return
  try {
    await store.createNew(name)
  } catch (e) {
    alert('新建失败: ' + (e instanceof Error ? e.message : String(e)))
  }
}

async function listForModal(): Promise<ProjectsModalItem[]> {
  const rows = await store.fetchList()
  return rows.map((r) => ({
    id: r.id,
    name: r.name,
    updatedAt: r.updatedAt,
    detail: `${r.sourceCount} 个素材`,
  }))
}

async function removeFromModal(id: string): Promise<void> {
  await store.deleteProject(id)
}

async function onLoad(id: string) {
  try {
    await store.openProject(id)
  } catch (e) {
    alert('打开失败: ' + (e instanceof Error ? e.message : String(e)))
  }
}

async function onClose() {
  await store.flushSave()
  store.closeProject()
}

// ---- Source import / remove ----

async function onImport() {
  if (!store.project) return
  const start = dirs.inputDir || ''
  const p = await modals.showPicker({ mode: 'file', title: '选择视频或音频素材', startPath: start })
  if (!p) return
  importing.value = true
  try {
    const { added, errors } = await store.importSources([p])
    const dir = Path.dirname(p)
    if (dir) await dirs.saveInput(dir)
    if (errors.length > 0 && libraryRef.value) {
      libraryRef.value.setError(`导入失败:${errors[0].error}`)
    } else if (added.length === 0 && libraryRef.value) {
      libraryRef.value.setError('未导入任何素材')
    }
  } catch (e) {
    alert('导入失败: ' + (e instanceof Error ? e.message : String(e)))
  } finally {
    importing.value = false
  }
}

async function onRemoveSource(sourceId: string) {
  if (!store.project) return
  if (!confirm('从当前工程移除此素材?')) return
  try {
    await store.removeSource(sourceId)
  } catch (e) {
    const msg = e instanceof Error ? e.message : String(e)
    alert(msg.includes('still referenced') || msg.includes('in use') ? '该素材仍被时间轴上的片段引用，请先删除相关片段' : '移除失败: ' + msg)
  }
}

// ---- Add / remove tracks ----

function onAddVideoTrack() {
  if (!store.project) return
  store.addVideoTrack()
}

function onAddAudioTrack() {
  if (!store.project) return
  store.addAudioTrack()
}

function onRemoveTrack(kind: 'video' | 'audio', id: string) {
  ops.removeTrack(kind, id)
}

// ---- Drag/drop from library ----

const SOURCE_MIME = 'application/x-easy-ffmpeg-source'

function readSourcePayload(ev: DragEvent): { sourceId: string } | null {
  const raw = ev.dataTransfer?.getData(SOURCE_MIME)
  if (!raw) return null
  try {
    const parsed = JSON.parse(raw)
    if (typeof parsed?.sourceId === 'string' && parsed.sourceId) return { sourceId: parsed.sourceId }
  } catch {
    return null
  }
  return null
}

function findSource(sid: string): MultitrackSource | null {
  return store.project?.sources.find((s) => s.id === sid) ?? null
}

function onTimelineDragOver(ev: DragEvent) {
  if (!ev.dataTransfer) return
  if (Array.from(ev.dataTransfer.types).includes(SOURCE_MIME)) {
    ev.preventDefault()
    ev.dataTransfer.dropEffect = 'copy'
  }
}

function newClipID(): string {
  return 'c' + Math.random().toString(16).slice(2, 8)
}

function makeClip(src: MultitrackSource, programStart: number): MultitrackClip {
  return {
    id: newClipID(),
    sourceId: src.id,
    sourceStart: 0,
    sourceEnd: src.duration > 0 ? src.duration : 0.001,
    programStart,
  }
}

/**
 * Drop onto blank timeline space → auto-create the right tracks.
 * Video source → 1 video track + 1 audio track if it has audio.
 * Audio source → 1 audio track.
 */
function onTimelineDropEmpty(ev: DragEvent) {
  ev.preventDefault()
  if (!store.project) return
  const payload = readSourcePayload(ev)
  if (!payload) return
  const src = findSource(payload.sourceId)
  if (!src) return
  if (src.kind === SOURCE_VIDEO) {
    const vid = store.addVideoTrack()
    store.appendClip('video', vid, makeClip(src, 0))
    if (src.hasAudio) {
      const aid = store.addAudioTrack()
      store.appendClip('audio', aid, makeClip(src, 0))
    }
  } else {
    const aid = store.addAudioTrack()
    store.appendClip('audio', aid, makeClip(src, 0))
  }
}

/** Drop onto a specific track row → append the clip at the playhead. */
function onTrackDrop(ev: DragEvent, kind: 'video' | 'audio', trackId: string) {
  ev.preventDefault()
  if (!store.project) return
  const payload = readSourcePayload(ev)
  if (!payload) return
  const src = findSource(payload.sourceId)
  if (!src) return
  if (kind === 'video' && src.kind !== SOURCE_VIDEO) return
  store.appendClip(kind, trackId, makeClip(src, store.playhead))
}

// ---- Lifecycle ----

onMounted(() => {
  store.fetchList().catch(() => {})
})

onActivated(() => playback.attach())
onDeactivated(() => {
  playback.detach()
  // Best-effort flush on tab switch so unsaved drops persist.
  store.flushSave().catch(() => {})
})
onBeforeUnmount(() => {
  playback.detach()
  store.flushSave().catch(() => {})
})

watch(
  () => store.project?.id,
  () => {
    if (!store.project) return
    requestAnimationFrame(() => zoom.applyFit())
  },
)

</script>

<template>
  <section class="flex h-full flex-col">
    <!-- Top bar: project actions + add-track buttons + undo/redo + library toggle -->
    <div class="flex shrink-0 items-center gap-2 border-b border-border-base bg-bg-panel px-4 py-2 text-sm">
      <button
        class="rounded border border-border-strong px-3 py-1 hover:bg-bg-elevated"
        @click="onCreate"
      >新建工程</button>
      <button
        class="rounded border border-border-strong px-3 py-1 hover:bg-bg-elevated"
        @click="projectsOpen = true"
      >工程列表</button>
      <template v-if="hasProject">
        <span class="mx-2 h-4 w-px bg-border-base"></span>
        <button
          class="rounded border border-border-strong px-2 py-1 text-xs hover:bg-bg-elevated"
          @click="onAddVideoTrack"
        >+ 视频轨</button>
        <button
          class="rounded border border-border-strong px-2 py-1 text-xs hover:bg-bg-elevated"
          @click="onAddAudioTrack"
        >+ 音频轨</button>
        <span class="mx-2 h-4 w-px bg-border-base"></span>
        <button
          class="rounded border border-border-strong px-2 py-1 text-xs hover:bg-bg-elevated"
          title="折叠/展开素材库 (Ctrl+L)"
          @click="store.libraryCollapsed = !store.libraryCollapsed"
        >{{ store.libraryCollapsed ? '展开素材库' : '收起素材库' }}</button>
        <button
          class="rounded px-2 py-1 text-xs text-fg-muted hover:bg-bg-elevated hover:text-fg-base"
          @click="onClose"
        >关闭工程</button>
      </template>
      <div class="ml-auto truncate text-xs text-fg-muted">
        <template v-if="hasProject">
          当前:<span class="text-fg-base">{{ store.project!.name }}</span>
          <span v-if="store.dirty" class="ml-2 text-warning">●</span>
        </template>
        <template v-else>暂无打开的工程</template>
      </div>
    </div>

    <!-- Body -->
    <div v-if="!hasProject" class="flex flex-1 items-center justify-center text-fg-muted">
      <div class="text-center">
        <div class="text-base">尚未打开工程</div>
        <div class="mt-2 text-xs">
          点击顶栏「新建工程」开始,或从「工程列表」打开既有工程
        </div>
      </div>
    </div>

    <div v-else class="flex flex-1 overflow-hidden">
      <MultitrackLibrary
        v-if="!store.libraryCollapsed"
        ref="libraryRef"
        :sources="store.project!.sources"
        :importing="importing"
        @import="onImport"
        @remove="onRemoveSource"
      />
      <div
        v-else
        class="flex h-full w-8 shrink-0 items-start justify-center border-r border-border-base bg-bg-panel pt-2"
        title="展开素材库 (Ctrl+L)"
      >
        <button
          class="rounded px-1 py-0.5 text-xs text-fg-muted hover:bg-bg-elevated hover:text-fg-base"
          @click="store.libraryCollapsed = false"
        >»</button>
      </div>

      <div class="flex flex-1 flex-col overflow-hidden">
        <MultitrackPreview ref="previewRef" />

        <PlayBar
          :playhead="store.playhead"
          :total-sec="total"
          :playing="store.playing"
          @prev="seekToNearestBoundary(-1)"
          @next="seekToNearestBoundary(1)"
          @toggle="previewRef?.toggle()"
        />

        <!-- Timeline area: ruler row (fixed at top) + body (Y-scroll
             label column + X-scroll tracks). -->
        <div
          class="relative flex h-64 shrink-0 flex-col overflow-hidden border-t border-border-base bg-bg-base"
          @contextmenu="onContextMenu"
          @dragover="onTimelineDragOver"
          @drop="onTimelineDropEmpty"
        >
          <!-- Header row: corner + ruler. The ruler container's X-scroll is
               kept in sync with the tracks via onTracksScroll, so this row
               always shows the same time range as what's below. -->
          <div class="flex shrink-0">
            <div class="h-7 w-28 shrink-0 border-b border-r border-border-base bg-bg-panel"></div>
            <div
              ref="rulerScrollEl"
              class="relative flex-1 overflow-hidden"
            >
              <TimelineRuler
                ref="rulerCmp"
                :px-per-second="store.pxPerSecond"
                :total-sec="total"
                :track-width="trackWidth"
                @mousedown="onRulerMouseDown"
              />
              <!-- Playhead segments that should overlap the ruler row. They
                   ride the ruler's X-scroll context so they stay aligned
                   with the tracks below at any scrollLeft. -->
              <TimelinePlayhead
                v-show="showPlayheadAll && store.project && total > 0"
                :x="playheadX"
                top="0"
                height="28px"
                @mousedown="onPlayheadMouseDown"
              />
            </div>
          </div>

          <!-- Body: shared Y-scroll for labels + tracks. The labels column
               itself is overflow-hidden, scrollTop synced from the body. -->
          <div ref="bodyEl" class="flex flex-1 overflow-y-auto overflow-x-hidden" @scroll="onBodyScroll">
            <!-- Labels column. Inner div lets content grow taller than the
                 visible column so it scrolls with the body. -->
            <div
              ref="labelsEl"
              class="w-28 shrink-0 overflow-hidden border-r border-border-base bg-bg-panel text-xs"
            >
              <div class="flex flex-col">
                <div
                  v-for="t in videoTracksData"
                  :key="'lv-' + t.id"
                  class="group flex h-12 shrink-0 items-center gap-1 border-b border-border-base px-2"
                >
                  <span class="truncate">🎬 {{ t.label }}</span>
                  <button
                    class="ml-auto rounded px-1 text-fg-muted opacity-0 hover:bg-bg-elevated hover:text-danger group-hover:opacity-100"
                    title="删除该轨道"
                    @click.stop="onRemoveTrack('video', t.id)"
                  >×</button>
                </div>
                <div
                  v-for="t in audioTracksData"
                  :key="'la-' + t.id"
                  class="group flex h-12 shrink-0 items-center gap-1 border-b border-border-base px-2"
                >
                  <span class="truncate">🔊 {{ t.label }}</span>
                  <button
                    class="ml-auto rounded px-1 text-fg-muted opacity-0 hover:bg-bg-elevated hover:text-danger group-hover:opacity-100"
                    title="删除该轨道"
                    @click.stop="onRemoveTrack('audio', t.id)"
                  >×</button>
                </div>
                <div v-if="videoTracksData.length + audioTracksData.length === 0" class="px-2 py-3 text-[11px] text-fg-muted">
                  拖入素材即建轨
                </div>
              </div>
            </div>

            <!-- Tracks: X-scroll only. Y-scroll is owned by the parent body. -->
            <div
              ref="scrollEl"
              class="relative flex-1 overflow-x-auto overflow-y-hidden"
              @wheel="zoom.onWheel"
              @scroll="onTracksScroll"
            >
              <div
                v-for="t in videoTracksData"
                :key="'v-' + t.id"
                :data-mt-track-id="t.id"
                data-mt-track-kind="video"
              >
                <TimelineTrackRow
                  :track="t"
                  :px-per-second="store.pxPerSecond"
                  :track-width="trackWidth"
                  :selected-ids="selectedIdsForTrack(t.id)"
                  height-class="h-12"
                  @mousedown="(payload) => onTrackMouseDown(t.id, 'video', payload)"
                  @dragover="onTimelineDragOver"
                  @drop.stop="(ev: DragEvent) => onTrackDrop(ev, 'video', t.id)"
                />
              </div>

              <div
                v-for="t in audioTracksData"
                :key="'a-' + t.id"
                :data-mt-track-id="t.id"
                data-mt-track-kind="audio"
              >
                <TimelineTrackRow
                  :track="t"
                  :px-per-second="store.pxPerSecond"
                  :track-width="trackWidth"
                  :selected-ids="selectedIdsForTrack(t.id)"
                  height-class="h-12"
                  top-border
                  @mousedown="(payload) => onTrackMouseDown(t.id, 'audio', payload)"
                  @dragover="onTimelineDragOver"
                  @drop.stop="(ev: DragEvent) => onTrackDrop(ev, 'audio', t.id)"
                />
              </div>

              <!-- Empty-timeline drop hint inside the scroll area so the
                   dragover hit-test always lands on something useful. -->
              <div
                v-if="videoTracksData.length + audioTracksData.length === 0"
                class="absolute inset-0 flex items-center justify-center text-[11px] text-fg-muted"
              >拖入素材到此处自动建轨</div>

              <TimelineRangeSelection
                :range="store.rangeSelection"
                :px-per-second="store.pxPerSecond"
              />

              <!-- Playheads inside the body cover the tracks. Heights are
                   computed inline so they align to the row stack regardless
                   of the body Y-scroll position (top is in tracks-content
                   coordinates). -->
              <TimelinePlayhead
                v-show="showPlayheadAll && store.project && total > 0"
                :x="playheadX"
                top="0"
                @mousedown="onPlayheadMouseDown"
              />
              <TimelinePlayhead
                v-show="showPlayheadVideo && store.project && total > 0 && videoTracksData.length > 0"
                :x="playheadX"
                top="0"
                :height="(videoTracksData.length * ROW_PX) + 'px'"
                @mousedown="onPlayheadMouseDown"
              />
              <TimelinePlayhead
                v-show="showPlayheadAudio && store.project && total > 0 && audioTracksData.length > 0"
                :x="playheadX"
                :top="(videoTracksData.length * ROW_PX) + 'px'"
                :height="(audioTracksData.length * ROW_PX) + 'px'"
                @mousedown="onPlayheadMouseDown"
              />
              <TimelinePlayhead
                v-if="singleTrackScopeBodyTop !== null"
                v-show="store.project && total > 0"
                :x="playheadX"
                :top="singleTrackScopeBodyTop + 'px'"
                :height="ROW_PX + 'px'"
                @mousedown="onPlayheadMouseDown"
              />
            </div>
          </div>
        </div>

        <MultitrackToolbar />
      </div>
    </div>

    <ProjectsModal
      :open="projectsOpen"
      title="多轨工程列表"
      :list="listForModal"
      :remove="removeFromModal"
      :empty-text='`暂无多轨工程,点上方"新建工程"创建`'
      @close="projectsOpen = false"
      @load="onLoad"
    />
  </section>
</template>
