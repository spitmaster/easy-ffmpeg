<script setup lang="ts">
import { computed, onActivated, onBeforeUnmount, onDeactivated, onMounted, ref, toRef, useTemplateRef } from 'vue'
import {
  multitrackApi,
  SOURCE_VIDEO,
  type MultitrackClip,
  type MultitrackExportBody,
  type MultitrackProject,
  type MultitrackSource,
} from '@/api/multitrack'
import type { ExportSettings, ProjectsModalItem, RangeSelection, TrackData, TrackTone } from '@/types/timeline'
import { useDirsStore } from '@/stores/dirs'
import { useMultitrackStore, MultitrackSel } from '@/stores/multitrack'
import { useModalsStore } from '@/stores/modals'
import { useExportFlow } from '@/composables/useExportFlow'
import { useMultitrackOps } from '@/composables/useMultitrackOps'
import { useTimelineDrag } from '@/composables/timeline/useTimelineDrag'
import { useTimelineLifecycle } from '@/composables/timeline/useTimelineLifecycle'
import { useTimelineMouseHandlers } from '@/composables/timeline/useTimelineMouseHandlers'
import { useTimelinePlayback } from '@/composables/timeline/useTimelinePlayback'
import { useTimelineRangeSelect } from '@/composables/timeline/useTimelineRangeSelect'
import { useTimelineZoom } from '@/composables/timeline/useTimelineZoom'
import { Path } from '@/utils/path'
import { findMissingSources } from '@/utils/validateSources'
import CanvasSettingsDialog from '@/components/multitrack/CanvasSettingsDialog.vue'
import MultitrackInspector from '@/components/multitrack/MultitrackInspector.vue'
import MultitrackLibrary from '@/components/multitrack/MultitrackLibrary.vue'
import MultitrackPreview from '@/components/multitrack/MultitrackPreview.vue'
import MultitrackToolbar from '@/components/multitrack/MultitrackToolbar.vue'
import ExportDialog from '@/components/timeline-shared/ExportDialog.vue'
import ExportSidebar from '@/components/timeline-shared/ExportSidebar.vue'
import PlayBar from '@/components/timeline-shared/PlayBar.vue'
import ProjectsModal from '@/components/timeline-shared/ProjectsModal.vue'
import TimelinePlayhead from '@/components/timeline-shared/TimelinePlayhead.vue'
import TimelineRangeSelection from '@/components/timeline-shared/TimelineRangeSelection.vue'
import TimelineRuler from '@/components/timeline-shared/TimelineRuler.vue'
import TimelineTrackLabel from '@/components/timeline-shared/TimelineTrackLabel.vue'
import TimelineTrackRow from '@/components/timeline-shared/TimelineTrackRow.vue'

/**
 * Multitrack editor view.
 *
 * Layout:
 *   topbar (project actions + dirty indicator + export)
 *   library | preview / playbar / timeline | export sidebar
 *   bottom toolbar (split / delete / undo / redo / scope / zoom / +track)
 *
 * Source → tracks:
 *   - The library card's "+ 添加" button calls onAddSource(): video source
 *     creates 1 video track + 1 audio track (when hasAudio); audio source
 *     creates 1 audio track. Drag-from-library was removed because it
 *     stacked clips on existing tracks unintuitively.
 *   - Cross-track clip drag (V→V / A→A) is still wired through
 *     useTimelineDrag's findTargetTrack / onCrossTrack callbacks; V→A and
 *     A→V moves are rejected at findTargetTrack time.
 */

const store = useMultitrackStore()
const dirs = useDirsStore()
const modals = useModalsStore()
const ops = useMultitrackOps()

const projectsOpen = ref(false)
const canvasOpen = ref(false)
const importing = ref(false)
const inspectorCollapsed = ref(false)

const libraryRef = useTemplateRef<{ setError: (msg: string) => void }>('libraryRef')
const previewRef = useTemplateRef<{ play: () => void; pause: () => void; toggle: () => void; seek: (t: number) => void }>('previewRef')
const scrollEl = useTemplateRef<HTMLDivElement>('scrollEl')
const rulerScrollEl = useTemplateRef<HTMLDivElement>('rulerScrollEl')
const labelsEl = useTemplateRef<HTMLDivElement>('labelsEl')
const rulerCmp = useTemplateRef<{ rootEl: HTMLDivElement | null }>('rulerCmp')

/**
 * scrollEl owns BOTH X and Y scroll so the scrollbars stay anchored at
 * the bottom-right of the visible tracks area (rather than at the bottom
 * of the full content stack, which goes off-screen as more tracks are
 * added). On scroll we mirror:
 *   - X → ruler.scrollLeft (top time-axis stays aligned with tracks)
 *   - Y → labels.scrollTop  (left label column scrolls with the tracks)
 * labels keeps overflow-hidden (no scrollbar visible) and is moved
 * programmatically; ruler the same.
 */
function onTracksScroll() {
  const s = scrollEl.value
  if (!s) return
  const r = rulerScrollEl.value
  if (r && r.scrollLeft !== s.scrollLeft) r.scrollLeft = s.scrollLeft
  const l = labelsEl.value
  if (l && l.scrollTop !== s.scrollTop) l.scrollTop = s.scrollTop
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

// ---- Track reorder via label-drag ----
//
// Pressing the icon/label of a TimelineTrackLabel and dragging vertically
// reorders that track among its same-kind siblings. The label emits
// 'reorder-mousedown'; we attach document mousemove/mouseup, hit-test the
// same-kind row under the cursor, and on mouseup commit via store.reorder*.
//
// Visual feedback:
//   - Source row gets opacity-40 via the `dragging` prop on its label
//   - A 2-px insertion indicator slides between rows in the labels column,
//     positioned by reorderInsertY (computed from the cursor's slot index)

interface ReorderState {
  kind: 'video' | 'audio'
  fromIdx: number
  /** insertion slot 0..tracks.length; -1 = no valid drop target yet */
  toIdx: number
}
const reorder = ref<ReorderState | null>(null)

const reorderingTrackId = computed(() => {
  if (!reorder.value) return null
  const arr = reorder.value.kind === 'video' ? videoTracksData.value : audioTracksData.value
  return arr[reorder.value.fromIdx]?.id ?? null
})

/** Y position (in the labels column's local coordinates) where the
 *  insertion indicator should render. Returns null when no drop target
 *  is valid (cursor outside the same-kind block). */
const reorderInsertY = computed<number | null>(() => {
  const r = reorder.value
  if (!r || r.toIdx < 0) return null
  const videoCount = videoTracksData.value.length
  const audioCount = audioTracksData.value.length
  // Labels column stack (top → bottom): [video rows][audio rows]
  if (r.kind === 'video') {
    if (r.toIdx > videoCount) return null
    return r.toIdx * ROW_PX
  }
  if (r.toIdx > audioCount) return null
  return videoCount * ROW_PX + r.toIdx * ROW_PX
})

function onLabelReorderMouseDown(
  kind: 'video' | 'audio',
  trackId: string,
  ev: MouseEvent,
) {
  if (store.exportLocked) return
  const arr = kind === 'video' ? videoTracksData.value : audioTracksData.value
  const fromIdx = arr.findIndex((t) => t.id === trackId)
  if (fromIdx < 0) return
  ev.preventDefault()
  reorder.value = { kind, fromIdx, toIdx: fromIdx }
  document.addEventListener('mousemove', onReorderMouseMove)
  document.addEventListener('mouseup', onReorderMouseUp, { once: true })
  document.body.style.cursor = 'grabbing'
}

function onReorderMouseMove(ev: MouseEvent) {
  const r = reorder.value
  if (!r) return
  const labels = labelsEl.value
  const container = labels?.firstElementChild as HTMLElement | null
  if (!labels || !container) return
  const rect = container.getBoundingClientRect()
  const y = ev.clientY - rect.top
  const videoCount = videoTracksData.value.length
  const audioCount = audioTracksData.value.length
  const videoRegionEnd = videoCount * ROW_PX
  const audioRegionEnd = videoRegionEnd + audioCount * ROW_PX
  // Find which slot the cursor is over within its own kind. Slots are
  // 0..count (count = "below the last row"). Outside the same-kind
  // region → toIdx = -1 (no valid drop).
  if (r.kind === 'video') {
    if (y < 0 || y > videoRegionEnd) {
      r.toIdx = -1
    } else {
      // Snap to nearest gap. Slot k sits at y = k * ROW_PX (top edge).
      r.toIdx = Math.max(0, Math.min(videoCount, Math.round(y / ROW_PX)))
    }
  } else {
    if (y < videoRegionEnd || y > audioRegionEnd) {
      r.toIdx = -1
    } else {
      const local = y - videoRegionEnd
      r.toIdx = Math.max(0, Math.min(audioCount, Math.round(local / ROW_PX)))
    }
  }
}

function onReorderMouseUp() {
  document.removeEventListener('mousemove', onReorderMouseMove)
  document.body.style.cursor = ''
  const r = reorder.value
  reorder.value = null
  if (!r || r.toIdx < 0) return
  if (r.kind === 'video') {
    store.reorderVideoTrack(r.fromIdx, r.toIdx)
  } else {
    store.reorderAudioTrack(r.fromIdx, r.toIdx)
  }
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

// ---- Export flow (dialog + sidebar + job, all in one composable) ----

/**
 * Defaults for the export dialog. Mirrors the editor's pattern: pull the
 * persisted ExportSettings from the project, falling back to system
 * defaults + the saved output dir + the project name. Recomputed when the
 * user reopens the dialog so a "save then reopen" cycle picks up edits.
 */
const exportDefaults = computed<ExportSettings>(() => {
  const e = store.project?.export
  return {
    format: e?.format || 'mp4',
    videoCodec: e?.videoCodec || 'h264',
    audioCodec: e?.audioCodec || 'aac',
    outputDir: e?.outputDir || dirs.outputDir || '',
    outputName: e?.outputName || store.project?.name || 'multitrack',
  }
})

/** "Project has at least one clip on at least one track" — mirrors the
 * backend's "no clips" guard so the button can disable cleanly. */
const hasAnyClip = computed(() => {
  const p = store.project
  if (!p) return false
  for (const t of p.videoTracks) if (t.clips.length) return true
  for (const t of p.audioTracks) if (t.clips.length) return true
  return false
})

const exportFlow = useExportFlow<MultitrackProject, MultitrackExportBody>({
  getProject: () => store.project,
  defaults: exportDefaults,
  validate: (project, _settings) => {
    let hasAny = false
    for (const t of project.videoTracks) if (t.clips.length) { hasAny = true; break }
    if (!hasAny) {
      for (const t of project.audioTracks) if (t.clips.length) { hasAny = true; break }
    }
    if (!hasAny) return '时间轴为空，无法导出'
    // Frontend-side leading-gap pre-check so the user doesn't have to wait
    // for the dryRun roundtrip to learn they're offending. Backend re-checks.
    for (let i = 0; i < project.videoTracks.length; i++) {
      const clips = project.videoTracks[i].clips
      if (!clips.length) continue
      const earliest = clips.reduce((m, c) => Math.min(m, c.programStart), Infinity)
      if (earliest > 0.001) {
        return `视频轨 ${i + 1} 开头必须有内容：第一个 clip 从 ${earliest.toFixed(2)}s 开始。\n请把它拖到 0 秒再导出。`
      }
    }
    return null
  },
  flushSave: () => store.flushSave(),
  buildBody: (project, settings) => ({ projectId: project.id, export: settings }),
  api: {
    exportPreview: multitrackApi.exportPreview,
    startExport: multitrackApi.startExport,
    cancelExport: multitrackApi.cancelExport,
  },
  totalDurationSec: () => store.programDuration,
  setLocked: (v) => { store.exportLocked = v },
  pausePreview: () => previewRef.value?.pause(),
  jobOptions: {
    cancelUrl: '/api/multitrack/export/cancel',
    runningLabel: '导出中...',
    doneLabel: '✓ 导出完成',
    errorLabel: '✗ 导出失败',
    cancelledLabel: '! 导出已取消',
  },
})

const exportDisabled = computed(
  () => !hasProject.value || !hasAnyClip.value || store.exportLocked,
)

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
  // Gate every shortcut on the export lock so Space / S / Delete /
  // Ctrl+Z don't slip through while ffmpeg is running.
  isLocked: () => store.exportLocked,
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

// ---- Timeline mouse handlers (delegated) ----

const previewLike = {
  play: () => previewRef.value?.play(),
  pause: () => previewRef.value?.pause(),
  seek: (t: number) => previewRef.value?.seek(t),
}

const mouse = useTimelineMouseHandlers({
  rulerEl,
  pxPerSecond: () => store.pxPerSecond,
  totalSec: () => total.value,
  isPlaying: () => store.playing,
  preview: previewLike,
  isLocked: () => store.exportLocked,
  onRangeStart: (ev) => rangeSelect.start(ev),
  beforeScrubFromRuler: () => {
    store.splitScope = 'all'
    store.selection = []
    store.rangeSelection = null
  },
  beforeScrubFromPlayhead: () => {
    if (store.rangeSelection) store.rangeSelection = null
  },
})

function onTrackMouseDown(
  trackId: string,
  kind: 'video' | 'audio',
  payload: { ev: MouseEvent; clipId?: string; handle?: 'left' | 'right' },
) {
  const { ev, clipId, handle } = payload
  if (ev.button !== 0) return
  if (store.exportLocked) return
  if (store.rangeSelection) store.rangeSelection = null

  if (!clipId) {
    // Empty area → narrow split scope to this track + scrub.
    store.splitScope = { kind: 'track', id: trackId }
    store.selection = []
    mouse.startScrubDrag(ev)
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

// ---- Project lifecycle ----

async function onCreate() {
  const name = window.prompt('新建多轨工程名称(留空则使用默认)', '')
  if (name === null) return
  try {
    await store.createNew(name)
  } catch (e) {
    await modals.showConfirm({
      title: '新建失败',
      message: e instanceof Error ? e.message : String(e),
      okText: '我知道了',
      hideCancel: true,
    })
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
    await modals.showConfirm({
      title: '打开失败',
      message: e instanceof Error ? e.message : String(e),
      okText: '我知道了',
      hideCancel: true,
    })
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
    await modals.showConfirm({
      title: '导入失败',
      message: e instanceof Error ? e.message : String(e),
      okText: '我知道了',
      hideCancel: true,
    })
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
    const inUse = msg.includes('still referenced') || msg.includes('in use')
    await modals.showConfirm({
      title: '移除失败',
      message: inUse ? '该素材仍被时间轴上的片段引用，请先删除相关片段' : msg,
      okText: '我知道了',
      hideCancel: true,
    })
  }
}

// ---- Track removal ----

function onRemoveTrack(kind: 'video' | 'audio', id: string) {
  ops.removeTrack(kind, id)
}

// ---- Per-audio-track volume ----

function onSetTrackVolume(trackId: string, v: number) {
  if (!Number.isFinite(v)) return
  store.setAudioTrackVolume(trackId, v)
}

// ---- Library "+ 添加" → create new tracks for this source ----

function findSource(sid: string): MultitrackSource | null {
  return store.project?.sources.find((s) => s.id === sid) ?? null
}

function newClipID(): string {
  return 'c' + Math.random().toString(16).slice(2, 8)
}

function makeClip(src: MultitrackSource, programStart: number): MultitrackClip {
  // Default Transform to full canvas — same behavior as v0.5.0 (clip
  // stretches to fill the frame). Inspector / TransformOverlay (M5) is
  // the path that lets the user customise it.
  const canvas = store.project?.canvas
  const w = canvas?.width ?? 1920
  const h = canvas?.height ?? 1080
  return {
    id: newClipID(),
    sourceId: src.id,
    sourceStart: 0,
    sourceEnd: src.duration > 0 ? src.duration : 0.001,
    programStart,
    transform: { x: 0, y: 0, w, h },
  }
}

/**
 * Click "+ 添加" on a library card → always create new tracks holding
 * the full clip. Video source → 1 video track + 1 audio track (when
 * hasAudio); audio source → 1 audio track. Clips start at program time 0.
 */
function onAddSource(sourceId: string) {
  if (!store.project || store.exportLocked) return
  const src = findSource(sourceId)
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

// Suppress browser context menu — right-click on ruler is range select.
function onContextMenu(ev: MouseEvent) {
  mouse.onContextMenu(ev)
}

// ---- Lifecycle ----

onMounted(() => {
  store.fetchList().catch(() => {})
})

useTimelineLifecycle({
  attach: () => playback.attach(),
  detach: () => playback.detach(),
  flushSave: () => store.flushSave(),
  projectId: () => store.project?.id,
  applyFit: () => zoom.applyFit(),
  onProjectChange: () => warnIfSourcesMissing(),
})

/**
 * Probe each source URL with HEAD on project open. The backend returns
 * 404 when the on-disk file is gone, which otherwise only surfaces as a
 * silent black preview. Showing the list lets the user restore / re-link
 * the files before trying to play or export.
 */
async function warnIfSourcesMissing() {
  const p = store.project
  if (!p || p.sources.length === 0) return
  const projectId = p.id
  const missing = await findMissingSources(
    p.sources.map((s) => ({
      path: s.path,
      url: multitrackApi.sourceUrl(projectId, s.id),
    })),
  )
  // Bail if the user closed / switched projects while HEAD was in flight.
  if (missing.length === 0 || store.project?.id !== projectId) return
  await modals.showConfirm({
    title: '部分源文件丢失',
    message:
      '该工程引用的以下源文件已不存在,无法播放/导出。请先恢复文件,或在素材库中将其移除后重新导入。',
    detail: missing.join('\n'),
    okText: '我知道了',
    hideCancel: true,
  })
}

// ---- Canvas settings (M4) ----

/**
 * Topbar pill label for the canvas button: `1920×1080 @ 30fps`. Empty
 * when no project is open (button itself is `v-if="hasProject"` so the
 * label only renders when the canvas is real).
 */
const canvasLabel = computed(() => {
  const c = store.project?.canvas
  if (!c) return ''
  // Drop trailing zeros so "30fps" beats "30.000fps" etc.
  const fr = Number.isInteger(c.frameRate) ? `${c.frameRate}` : `${c.frameRate.toFixed(2)}`
  return `${c.width}×${c.height} @ ${fr}fps`
})

function onCanvasSubmit(canvas: { width: number; height: number; frameRate: number }) {
  store.setCanvas(canvas)
  canvasOpen.value = false
}

// ---- Selected-clip nudge keyboard shortcuts (M5) ----
//
// Hooks into the document keydown stream BEFORE useTimelinePlayback's
// listener so ArrowLeft / ArrowRight nudge the selected video clip's
// transform instead of seeking. Capture phase + stopImmediatePropagation
// is the trick — useTimelinePlayback's listener is bubble-phase and only
// fires when our handler doesn't claim the event.
//
// Bindings (only when a video clip is selected and not in an editable):
//   ←/→/↑/↓        nudge X / Y by 1px
//   Shift + ←/→/↑/↓  nudge by 10px
//   Ctrl + 0       reset to full canvas
// Without a video selection or while the export is locked, we don't
// claim the keys — playback's seek bindings continue to work as before.

function isInEditable(target: EventTarget | null): boolean {
  const a = target as HTMLElement | null
  if (!a) return false
  const tag = a.tagName
  return tag === 'INPUT' || tag === 'TEXTAREA' || tag === 'SELECT' || a.isContentEditable
}

function nudgeSelected(dx: number, dy: number): boolean {
  const sel = store.selectedVideoClip
  if (!sel) return false
  const t = sel.clip.transform
  store.commitClipTransform(sel.trackId, sel.clipId, {
    x: t.x + dx,
    y: t.y + dy,
    w: t.w,
    h: t.h,
  })
  return true
}

function resetSelectedToFullCanvas(): boolean {
  const sel = store.selectedVideoClip
  const c = store.project?.canvas
  if (!sel || !c) return false
  const cur = sel.clip.transform
  if (cur.x === 0 && cur.y === 0 && cur.w === c.width && cur.h === c.height) return true
  store.commitClipTransform(sel.trackId, sel.clipId, { x: 0, y: 0, w: c.width, h: c.height })
  return true
}

function onTransformKeyCapture(ev: KeyboardEvent) {
  if (store.exportLocked) return
  if (isInEditable(ev.target)) return
  // Ctrl+0 → reset (only when a video clip is selected; otherwise let
  // the browser / other handlers see it).
  if ((ev.ctrlKey || ev.metaKey) && ev.key === '0') {
    if (resetSelectedToFullCanvas()) {
      ev.preventDefault()
      ev.stopImmediatePropagation()
    }
    return
  }
  // Arrow nudge — only intercept when there's a video clip to nudge,
  // otherwise playback gets to see Arrow* for seek-to-boundary.
  if (!store.selectedVideoClip) return
  const step = ev.shiftKey ? 10 : 1
  let handled = false
  switch (ev.key) {
    case 'ArrowLeft':
      handled = nudgeSelected(-step, 0)
      break
    case 'ArrowRight':
      handled = nudgeSelected(step, 0)
      break
    case 'ArrowUp':
      handled = nudgeSelected(0, -step)
      break
    case 'ArrowDown':
      handled = nudgeSelected(0, step)
      break
  }
  if (handled) {
    ev.preventDefault()
    ev.stopImmediatePropagation()
  }
}

function attachTransformKey() {
  document.addEventListener('keydown', onTransformKeyCapture, { capture: true })
}
function detachTransformKey() {
  document.removeEventListener('keydown', onTransformKeyCapture, { capture: true })
}

// Ctrl/Cmd + wheel outside the timeline scroll area would otherwise trigger
// the browser's page-zoom (or trackpad pinch on macOS). The timeline's own
// `@wheel="zoom.onWheel"` handles its own preventDefault, so we only step in
// when the wheel event lands outside scrollEl. Capture-phase + passive:false
// is required for preventDefault on wheel to actually take effect.
function onGlobalWheel(ev: WheelEvent) {
  if (!(ev.ctrlKey || ev.metaKey)) return
  const scroll = scrollEl.value
  const target = ev.target as Node | null
  if (scroll && target && scroll.contains(target)) return
  ev.preventDefault()
}
function attachGlobalWheel() {
  document.addEventListener('wheel', onGlobalWheel, { capture: true, passive: false })
}
function detachGlobalWheel() {
  document.removeEventListener('wheel', onGlobalWheel, { capture: true })
}

onActivated(() => { attachTransformKey(); attachGlobalWheel() })
onDeactivated(() => { detachTransformKey(); detachGlobalWheel() })
onBeforeUnmount(() => { detachTransformKey(); detachGlobalWheel() })

</script>

<template>
  <section class="flex h-full flex-col">
    <!-- ============================================================
         Region: TOP-BAR  (1 row × full width)
         Left:  新建工程 / 工程列表 / 关闭工程
         Right: 脏标 / 导出
         Spec:  docs/tabs/multitrack/product.md §3.1.1
         ============================================================ -->
    <div class="flex shrink-0 items-center gap-2 border-b border-border-base bg-bg-panel px-4 py-2 text-sm">
      <button
        class="rounded border border-border-strong px-3 py-1 hover:bg-bg-elevated"
        @click="onCreate"
      >新建工程</button>
      <button
        class="rounded border border-border-strong px-3 py-1 hover:bg-bg-elevated"
        @click="projectsOpen = true"
      >工程列表</button>
      <button
        v-if="hasProject"
        class="rounded px-2 py-1 text-xs text-fg-muted hover:bg-bg-elevated hover:text-fg-base disabled:opacity-50"
        :disabled="store.exportLocked"
        @click="onClose"
      >关闭工程</button>
      <button
        v-if="hasProject"
        class="rounded border border-border-strong px-2 py-1 font-mono text-xs hover:bg-bg-elevated disabled:opacity-50"
        :disabled="store.exportLocked"
        :title="`点击修改工程画布(分辨率 / 帧率)`"
        @click="canvasOpen = true"
      >画布: {{ canvasLabel }} ▾</button>
      <div v-if="!hasProject" class="ml-2 truncate text-xs text-fg-muted">暂无打开的工程</div>
      <div class="ml-auto flex items-center gap-2">
        <span
          v-if="hasProject && store.dirty"
          class="text-xs text-warning"
          title="有未保存的改动"
        >●</span>
        <button
          v-if="hasProject"
          class="rounded bg-accent px-3 py-1 text-xs text-bg-base hover:bg-accent-hover disabled:opacity-50"
          :disabled="exportDisabled"
          @click="exportFlow.openDialog()"
        >导出</button>
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

    <!-- ============================================================
         Body grid (15 rows × 15 cols, see product.md §3.1.1):
           Region: LIBRARY  (cols  1– 3, full body height)
           Region: PREVIEW  (cols  4–15, top    half — 0.5)
           Region: TRACKS   (cols  4–15, bottom half — 0.5)
         Implementation note:
           - The grid is descriptive, not enforced via CSS Grid; the layout
             below uses flex with proportional basis (3 : 12 = 20% : 80%)
             for LIBRARY vs PREVIEW+TRACKS, and flex-1 inside each of
             PREVIEW / TRACKS for the 0.5 / 0.5 split.
           - PlayBar lives inside PREVIEW (controls the preview), Toolbar
             inside TRACKS (operates on tracks); both take natural height
             and eat into their own region — they do not break the 0.5
             split between regions.
         ============================================================ -->
    <div v-else class="flex flex-1 overflow-hidden">
      <!-- Region: LIBRARY (cols 1–3 ≈ 20%) -->
      <MultitrackLibrary
        v-if="!store.libraryCollapsed"
        ref="libraryRef"
        :sources="store.project!.sources"
        :importing="importing"
        :add-disabled="store.exportLocked"
        @import="onImport"
        @remove="onRemoveSource"
        @add="onAddSource"
        @collapse="store.libraryCollapsed = true"
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

      <!-- Editor column (cols 4–15 ≈ 80%): split 0.5 / 0.5 vertically. -->
      <div class="flex flex-1 flex-col overflow-hidden">
        <!-- Region: PREVIEW (top 0.5) — preview pane + PlayBar -->
        <div class="flex min-h-0 flex-1 flex-col overflow-hidden">
        <MultitrackPreview ref="previewRef" />

        <PlayBar
          :playhead="store.playhead"
          :total-sec="total"
          :playing="store.playing"
          @prev="seekToNearestBoundary(-1)"
          @next="seekToNearestBoundary(1)"
          @toggle="previewRef?.toggle()"
        />
        </div>
        <!-- /Region: PREVIEW -->

        <!-- Region: TRACKS (bottom 0.5) — timeline area + Toolbar -->
        <div class="flex min-h-0 flex-1 flex-col overflow-hidden">
        <!-- Timeline area: ruler row (fixed at top) + body (Y-scroll
             label column + X-scroll tracks). flex-1 + min-h so the area
             grows with available vertical space — fixed h-64 used to push
             extra tracks off screen when the project had many of them. -->
        <div
          class="relative flex min-h-[12rem] flex-1 flex-col overflow-hidden border-t border-border-base bg-bg-base"
          @contextmenu="onContextMenu"
        >
          <!-- Header row: corner + ruler. The ruler container's X-scroll is
               kept in sync with the tracks via onTracksScroll, so this row
               always shows the same time range as what's below. -->
          <div class="flex shrink-0">
            <div class="h-7 w-40 shrink-0 border-b border-r border-border-base bg-bg-panel"></div>
            <div
              ref="rulerScrollEl"
              class="relative flex-1 overflow-hidden"
            >
              <TimelineRuler
                ref="rulerCmp"
                :px-per-second="store.pxPerSecond"
                :total-sec="total"
                :track-width="trackWidth"
                @mousedown="mouse.onRulerMouseDown"
              />
              <!-- Playhead segments that should overlap the ruler row. They
                   ride the ruler's X-scroll context so they stay aligned
                   with the tracks below at any scrollLeft. -->
              <TimelinePlayhead
                v-show="showPlayheadAll && store.project && total > 0"
                :x="playheadX"
                top="0"
                height="28px"
                @mousedown="mouse.onPlayheadMouseDown"
              />
            </div>
          </div>

          <!-- Body: pure flex-row container (labels | scrollEl). No scroll
               here — scrollEl owns BOTH X and Y so its scrollbars stay
               anchored at the bottom-right of the visible tracks area
               instead of riding the content off-screen. labels stretches
               to body height via default align-items:stretch, content
               inside is clipped by overflow-hidden and moved
               programmatically via onTracksScroll → labels.scrollTop. -->
          <div class="flex min-h-0 flex-1 overflow-hidden">
            <!-- Labels column. Inner div lets content grow taller than the
                 visible column so it scrolls with the body. Audio rows put
                 the volume popover button inline next to the label so all
                 controls (label + 音量:NNN% button + delete-×) fit on a
                 single h-12 line. Width is sized to the audio row's
                 content; the label has min-w-0 + truncate so double-digit
                 track indices ("音频 10+") elide gracefully. -->
            <div
              ref="labelsEl"
              class="w-40 shrink-0 overflow-hidden border-r border-border-base bg-bg-panel text-xs"
            >
              <div class="relative flex flex-col">
                <TimelineTrackLabel
                  v-for="t in videoTracksData"
                  :key="'lv-' + t.id"
                  kind="video"
                  :label="t.label ?? ''"
                  removable
                  reorderable
                  :dragging="reorderingTrackId === t.id"
                  :disabled="store.exportLocked"
                  @remove="onRemoveTrack('video', t.id)"
                  @reorder-mousedown="(ev: MouseEvent) => onLabelReorderMouseDown('video', t.id, ev)"
                />
                <TimelineTrackLabel
                  v-for="t in audioTracksData"
                  :key="'la-' + t.id"
                  kind="audio"
                  :label="t.label ?? ''"
                  :volume="t.volume ?? 1"
                  removable
                  reorderable
                  :dragging="reorderingTrackId === t.id"
                  :disabled="store.exportLocked"
                  @update:volume="(v: number) => onSetTrackVolume(t.id, v)"
                  @remove="onRemoveTrack('audio', t.id)"
                  @reorder-mousedown="(ev: MouseEvent) => onLabelReorderMouseDown('audio', t.id, ev)"
                />
                <div v-if="videoTracksData.length + audioTracksData.length === 0" class="px-2 py-3 text-[11px] text-fg-muted">
                  从素材库点"+ 添加"建轨
                </div>
                <!-- Track-reorder insertion indicator. A 2px accent bar
                     anchored at the gap between rows where dropping would
                     land the dragged track. Hidden when the cursor is
                     outside the same-kind region (toIdx = -1). -->
                <div
                  v-if="reorderInsertY !== null"
                  class="pointer-events-none absolute left-0 right-0 z-20 h-0.5 -translate-y-1/2 bg-accent shadow-[0_0_4px_rgba(0,0,0,0.6)]"
                  :style="{ top: reorderInsertY + 'px' }"
                ></div>
              </div>
            </div>

            <!-- Tracks: owns BOTH X and Y scroll so the X scrollbar stays
                 fixed at the bottom of the visible tracks frame instead
                 of being pushed off-screen by tall content. onTracksScroll
                 mirrors X→ruler and Y→labels.

                 The inner wrapper is the `position: relative` containing
                 block for the absolute overlays (range selection,
                 playheads). It sizes to natural content height (sum of
                 track rows) — using `relative` directly on scrollEl makes
                 absolute children resolve `bottom: 0` against the viewport
                 (clientHeight), so range selection and the full-height
                 playhead would only cover the visible part of the tracks
                 and not the rows scrolled below. minHeight: 100% keeps the
                 empty-state placeholder visible when there are no tracks. -->
            <div
              ref="scrollEl"
              class="flex-1 overflow-auto"
              @wheel="zoom.onWheel"
              @scroll="onTracksScroll"
            >
              <div
                class="relative"
                :style="{ width: trackWidth + 'px', minHeight: '100%' }"
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
                  />
                </div>

                <div
                  v-if="videoTracksData.length + audioTracksData.length === 0"
                  class="absolute inset-0 flex items-center justify-center text-[11px] text-fg-muted"
                >从左侧素材库点"+ 添加"开始建轨</div>

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
                  @mousedown="mouse.onPlayheadMouseDown"
                />
                <TimelinePlayhead
                  v-show="showPlayheadVideo && store.project && total > 0 && videoTracksData.length > 0"
                  :x="playheadX"
                  top="0"
                  :height="(videoTracksData.length * ROW_PX) + 'px'"
                  @mousedown="mouse.onPlayheadMouseDown"
                />
                <TimelinePlayhead
                  v-show="showPlayheadAudio && store.project && total > 0 && audioTracksData.length > 0"
                  :x="playheadX"
                  :top="(videoTracksData.length * ROW_PX) + 'px'"
                  :height="(audioTracksData.length * ROW_PX) + 'px'"
                  @mousedown="mouse.onPlayheadMouseDown"
                />
                <TimelinePlayhead
                  v-if="singleTrackScopeBodyTop !== null"
                  v-show="store.project && total > 0"
                  :x="playheadX"
                  :top="singleTrackScopeBodyTop + 'px'"
                  :height="ROW_PX + 'px'"
                  @mousedown="mouse.onPlayheadMouseDown"
                />
              </div>
            </div>
          </div>
        </div>

        <MultitrackToolbar />
        </div>
        <!-- /Region: TRACKS -->
      </div>
      <!-- /Editor column -->

      <!-- Inspector: right rail for canvas + selected-clip transform.
           Mutually exclusive with ExportSidebar (no transform editing
           while ffmpeg renders), so v-show toggles on the export flag.
           Inspector renders only when a project is open. -->
      <MultitrackInspector
        v-if="hasProject"
        v-show="!exportFlow.sidebarOpen.value"
        :collapsed="inspectorCollapsed"
        @toggle="inspectorCollapsed = !inspectorCollapsed"
        @open-canvas-dialog="canvasOpen = true"
      />

      <!-- Right: export log sidebar (visible only during/after export). The
           ExportSidebar component lays itself out as an inset column —
           same as the editor — so we drop it as a sibling of the main
           column rather than overlaying. -->
      <ExportSidebar
        :open="exportFlow.sidebarOpen.value"
        :running="exportFlow.job.running.value"
        :state-label="exportFlow.job.stateLabel.value"
        :log="exportFlow.job.log.value"
        :progress="exportFlow.job.progress.value"
        :progress-visible="exportFlow.job.progressVisible.value"
        :finish-visible="exportFlow.job.finishVisible.value"
        :finish-kind="exportFlow.job.finishKind.value"
        :finish-text="exportFlow.job.finishText.value"
        :has-output-path="!!exportFlow.job.lastOutputPath.value"
        @close="exportFlow.closeSidebar"
        @cancel="exportFlow.job.cancel"
        @reveal="exportFlow.job.revealOutput"
      />
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

    <ExportDialog
      :open="exportFlow.dialogOpen.value"
      :defaults="exportDefaults"
      :pick-dir="exportFlow.pickOutputDir"
      title="导出多轨工程"
      @close="exportFlow.dialogOpen.value = false"
      @submit="exportFlow.submit"
    />

    <CanvasSettingsDialog
      v-if="store.project"
      :open="canvasOpen"
      :defaults="store.project.canvas"
      :project="store.project"
      @close="canvasOpen = false"
      @submit="onCanvasSubmit"
    />
  </section>
</template>
