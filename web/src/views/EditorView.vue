<script setup lang="ts">
import { computed, onActivated, onBeforeUnmount, onDeactivated, ref, toRef, useTemplateRef, watch } from 'vue'
import {
  editorApi,
  TRACK_AUDIO,
  TRACK_VIDEO,
  type Project,
  type Track,
} from '@/api/editor'
import type { ExportSettings, ProjectsModalItem, RangeSelection, TrackData } from '@/types/timeline'
import { useDirsStore } from '@/stores/dirs'
import { useEditorStore, Sel } from '@/stores/editor'
import { useModalsStore } from '@/stores/modals'
import { useEditorOps } from '@/composables/useEditorOps'
import { useEditorPreview } from '@/composables/useEditorPreview'
import { useJobPanel } from '@/composables/useJobPanel'
import { useTimelineDrag } from '@/composables/timeline/useTimelineDrag'
import { useTimelinePlayback } from '@/composables/timeline/useTimelinePlayback'
import { useTimelineRangeSelect } from '@/composables/timeline/useTimelineRangeSelect'
import { useTimelineZoom } from '@/composables/timeline/useTimelineZoom'
import { totalDuration } from '@/utils/timeline'
import { Path } from '@/utils/path'
import { findMissingSources } from '@/utils/validateSources'
import EditorTopBar from '@/components/editor/EditorTopBar.vue'
import EditorToolbar from '@/components/editor/EditorToolbar.vue'
import ExportDialog from '@/components/timeline-shared/ExportDialog.vue'
import ExportSidebar from '@/components/timeline-shared/ExportSidebar.vue'
import PlayBar from '@/components/timeline-shared/PlayBar.vue'
import ProjectsModal from '@/components/timeline-shared/ProjectsModal.vue'
import TimelinePlayhead from '@/components/timeline-shared/TimelinePlayhead.vue'
import TimelineRangeSelection from '@/components/timeline-shared/TimelineRangeSelection.vue'
import TimelineRuler from '@/components/timeline-shared/TimelineRuler.vue'
import TimelineTrackLabel from '@/components/timeline-shared/TimelineTrackLabel.vue'
import TimelineTrackRow from '@/components/timeline-shared/TimelineTrackRow.vue'

const store = useEditorStore()
const dirs = useDirsStore()
const modals = useModalsStore()
const ops = useEditorOps()

// ---- Refs to mounted DOM ----

const videoRef = ref<HTMLVideoElement | null>(null)
const audioRef = ref<HTMLAudioElement | null>(null)
const scrollEl = useTemplateRef<HTMLDivElement>('scrollEl')
// Vue's defineExpose auto-unwraps refs via proxyRefs, so rulerCmp.value.rootEl
// is the DOM element itself (not a Ref) — typing/access must reflect that.
const rulerCmp = useTemplateRef<{ rootEl: HTMLDivElement | null }>('rulerCmp')

const preview = useEditorPreview(videoRef, audioRef)

// ---- Project / panels ----

const projectsOpen = ref(false)
const exportOpen = ref(false)
const exportSidebarOpen = ref(false)

const job = useJobPanel({
  cancelUrl: '/api/editor/export/cancel',
  runningLabel: '导出中...',
  doneLabel: '✓ 导出完成',
  errorLabel: '✗ 导出失败',
  cancelledLabel: '! 导出已取消',
})

const hasProject = computed(() => !!store.project)

// ---- Timeline computed ----

const total = computed(() => totalDuration(store.project))
const trackWidth = computed(() => Math.max(total.value * store.pxPerSecond + 40, 400))
const playheadX = computed(() => store.playhead * store.pxPerSecond)

const videoTrack = computed<TrackData>(() => ({
  id: TRACK_VIDEO,
  kind: 'video',
  clips: store.project?.videoClips || [],
  tone: 'accent',
}))
const audioTrack = computed<TrackData>(() => ({
  id: TRACK_AUDIO,
  kind: 'audio',
  clips: store.project?.audioClips || [],
  tone: 'success',
}))
const selectedVideoIds = computed(() => Sel.inTrack(store.selection, TRACK_VIDEO))
const selectedAudioIds = computed(() => Sel.inTrack(store.selection, TRACK_AUDIO))

const audioVolume = computed({
  get: () => store.project?.audioVolume ?? 1,
  set: (v: number) => {
    if (!store.project) return
    store.applyProjectPatch({ audioVolume: v })
  },
})

// ---- Timeline composables ----

const zoom = useTimelineZoom({
  pxPerSecond: toRef(store, 'pxPerSecond'),
  totalSec: () => total.value,
  scrollEl,
})

const drag = useTimelineDrag({
  pxPerSecond: () => store.pxPerSecond,
  playhead: () => store.playhead,
  getClips: (trackId) =>
    trackId === TRACK_VIDEO
      ? store.project?.videoClips ?? []
      : store.project?.audioClips ?? [],
  setClips: (trackId, clips) => {
    const key = trackId === TRACK_VIDEO ? 'videoClips' : 'audioClips'
    store.applyProjectPatch({ [key]: clips }, { save: false })
  },
  pushHistory: () => store.pushHistory(),
  scheduleSave: () => store.scheduleSave(),
  // Single-video has one source — both video and audio clips trim against
  // the same source duration.
  sourceMaxFor: () => store.project?.source?.duration ?? 0,
})

// rulerEl is the TimelineRuler component's exposed root <div>, fed to
// useTimelineRangeSelect / clientXToTime for client-x → seconds math.
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
    store.splitScope = 'both'
  },
})

const playback = useTimelinePlayback({
  isLocked: () => job.running.value,
  togglePlay: () => preview.toggle(),
  splitAtPlayhead: () => ops.splitAtPlayhead(),
  deleteSelection: () => ops.deleteSelection(),
  seekBackBoundary: () => preview.seekToBoundary(-1),
  seekForwardBoundary: () => preview.seekToBoundary(1),
  undo: () => store.undo(),
  redo: () => store.redo(),
  clearRangeSelection: () => {
    if (store.rangeSelection) store.rangeSelection = null
  },
})

// ---- Timeline mouse handlers (assemble shared composables) ----

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
  if (wasPlaying) preview.pause()
  preview.seek(clientXToTime(ev.clientX))
  function onMove(e: MouseEvent) {
    preview.seek(clientXToTime(e.clientX))
  }
  function onUp() {
    document.removeEventListener('mousemove', onMove)
    document.removeEventListener('mouseup', onUp)
    if (wasPlaying) preview.play()
  }
  document.addEventListener('mousemove', onMove)
  document.addEventListener('mouseup', onUp)
}

function onRulerMouseDown(ev: MouseEvent) {
  if (ev.button === 2) {
    rangeSelect.start(ev)
    return
  }
  store.splitScope = 'both'
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
  trackId: Track,
  payload: { ev: MouseEvent; clipId?: string; handle?: 'left' | 'right' },
) {
  const { ev, clipId, handle } = payload
  if (ev.button !== 0) return
  if (store.rangeSelection) store.rangeSelection = null

  if (!clipId) {
    // Empty area → narrow split scope to this track + scrub.
    store.splitScope = trackId
    store.selection = []
    startScrubDrag(ev)
    return
  }
  const multi = ev.shiftKey || ev.ctrlKey || ev.metaKey
  store.selection = multi
    ? Sel.toggle(store.selection, trackId, clipId)
    : Sel.replace(trackId, clipId)
  store.splitScope = trackId
  if (handle) {
    drag.startTrim(ev, trackId, clipId, handle)
  } else if (!multi) {
    drag.startReorder(ev, trackId, clipId)
  }
}

// Suppress browser context menu — right-click is repurposed for range select.
function onContextMenu(ev: MouseEvent) {
  ev.preventDefault()
}

// ---- Project lifecycle ----

async function openVideo() {
  const start = dirs.inputDir || ''
  const p = await modals.showPicker({ mode: 'file', title: '选择要剪辑的视频', startPath: start })
  if (!p) return
  try {
    const project = await editorApi.createProject(p, Path.stripExt(Path.basename(p)))
    const dir = Path.dirname(p)
    if (dir) await dirs.saveInput(dir)
    loadProject(project)
  } catch (e) {
    alert('创建工程失败: ' + (e instanceof Error ? e.message : String(e)))
  }
}

async function loadProjectById(id: string) {
  try {
    const project = await editorApi.getProject(id)
    loadProject(project)
  } catch (e) {
    alert('加载工程失败: ' + (e instanceof Error ? e.message : String(e)))
  }
}

function loadProject(project: Project) {
  store.loadProject(project)
  preview.loadProject(project)
  // Fit-to-width must run after the workspace is visible so clientWidth
  // reads correctly.
  requestAnimationFrame(() => zoom.applyFit())
  warnIfSourceMissing()
}

/**
 * Probe the source URL with HEAD on project load. The backend returns
 * 404 when the on-disk file is gone, which otherwise only surfaces as a
 * silent black preview. Show an explicit warning so the user can restore
 * or pick a new file before trying to play / export.
 */
async function warnIfSourceMissing() {
  const p = store.project
  if (!p?.source?.path) return
  const projectId = p.id
  const missing = await findMissingSources([
    { path: p.source.path, url: editorApi.sourceUrl(projectId) },
  ])
  if (missing.length === 0 || store.project?.id !== projectId) return
  await modals.showConfirm({
    title: '源文件丢失',
    message:
      '该工程的源文件已不存在,无法播放/导出。请先恢复文件,或通过顶栏「📂 打开视频」打开新的视频。',
    detail: missing[0],
    okText: '我知道了',
    hideCancel: true,
  })
}

// ---- Projects modal adapter ----

async function listProjects(): Promise<ProjectsModalItem[]> {
  const ps = (await editorApi.listProjects()) || []
  return ps.map((p) => ({
    id: p.id,
    name: p.name,
    updatedAt: p.updatedAt,
    detail: p.source?.path,
  }))
}

async function deleteProject(id: string) {
  await editorApi.deleteProject(id)
}

// ---- Export ----

const exportDefaults = computed<ExportSettings>(() => {
  const e = store.project?.export
  return {
    format: e?.format || 'mp4',
    videoCodec: e?.videoCodec || 'h264',
    audioCodec: e?.audioCodec || 'aac',
    outputDir: e?.outputDir || dirs.outputDir || '',
    outputName: e?.outputName || store.project?.name || 'edit',
  }
})

async function pickOutputDir(current: string): Promise<string | null> {
  const p = await modals.showPicker({
    mode: 'dir',
    title: '选择输出目录',
    startPath: current || dirs.outputDir,
  })
  if (!p) return null
  await dirs.saveOutput(p)
  return p
}

async function onExportSubmit(settings: ExportSettings) {
  const project = store.project
  if (!project) return
  const vClips = project.videoClips || []
  const aClips = project.audioClips || []
  if (!vClips.length && !aClips.length) {
    alert('时间轴为空，无法导出')
    return
  }
  if (vClips.length) {
    const t = vClips.reduce((m, c) => Math.min(m, c.programStart), Infinity)
    if (t > 0.001) {
      alert(`视频轨道开头必须有内容：第一个 clip 从 ${t.toFixed(2)}s 开始。\n请把它拖到 0 秒再导出。`)
      return
    }
  }

  await store.flushSave()
  const body = { projectId: project.id, export: settings }

  let dryRun: { command: string; outputPath: string }
  try {
    dryRun = await editorApi.exportPreview(body)
  } catch (e) {
    alert('生成命令失败: ' + (e instanceof Error ? e.message : String(e)))
    return
  }
  if (!(await modals.showCommand(dryRun.command))) return

  exportOpen.value = false
  exportSidebarOpen.value = true

  // Stop the preview playback before ffmpeg starts: the in-page video/audio
  // decoder competes with the encoder for CPU and disk I/O on the same
  // source file, and there's no reason to keep playback running during export.
  preview.pause()

  const sendStart = async (overwrite: boolean): Promise<string> => {
    const { res, data } = await editorApi.startExport({ ...body, overwrite })
    if (res.status === 409 && data.existing) {
      const ok = await modals.showOverwrite(data.path || '')
      if (!ok) throw new Error('已取消覆盖')
      return sendStart(true)
    }
    if (!res.ok) throw new Error(data.error || `HTTP ${res.status}`)
    return data.command || ''
  }

  await job.startJob({
    outputPath:
      dryRun.outputPath || Path.join(settings.outputDir, settings.outputName + '.' + settings.format),
    totalDurationSec: totalDuration(project),
    request: () => sendStart(false),
  })
}

async function closeExportSidebar() {
  if (job.running.value) {
    if (!confirm('导出仍在进行中，关闭面板将取消导出。确认关闭？')) return
    try {
      await editorApi.cancelExport()
    } catch {
      // server may already be tearing down
    }
  }
  exportSidebarOpen.value = false
}

// ---- Lifecycle ----

// Use activated/deactivated rather than mounted/beforeUnmount: with
// KeepAlive in App.vue this view stays mounted across tab switches, but
// the document-level shortcuts (Space, S, Delete, Ctrl+Z, …) must only
// fire while the editor is the visible tab — otherwise hitting Space
// in Convert/Audio would toggle preview playback in the background.
onActivated(() => playback.attach())
onDeactivated(() => playback.detach())
onBeforeUnmount(() => {
  playback.detach()
  // Best-effort flush: don't await to keep navigation snappy.
  store.flushSave().catch(() => {})
})

// Re-fit on project switch.
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
    <EditorTopBar
      :locked="job.running.value"
      @open-video="openVideo"
      @open-projects="projectsOpen = true"
      @open-export="exportOpen = true"
    />

    <div class="flex flex-1 overflow-hidden">
      <!-- Main column: preview / playbar / timeline / toolbar -->
      <div class="relative flex flex-1 flex-col overflow-hidden">
        <div v-if="!hasProject" class="flex flex-1 flex-col items-center justify-center gap-2 text-fg-muted">
          <div class="text-base">尚未导入视频</div>
          <div class="text-xs">
            点击顶栏「📂 打开视频」选择要剪辑的视频文件，或从「📋 剪辑记录」打开历史工程
          </div>
        </div>

        <div v-else class="flex flex-1 flex-col overflow-hidden">
          <!-- Preview. min-h-0 + max-h on the video are *both* required:
               flex children default to min-height: auto, and a <video> with
               h-full has a content-size of the source's natural pixels, so
               without these the preview grows to match a 1080p source and
               pushes the timeline off the bottom of the screen. -->
          <div class="flex min-h-0 flex-1 items-center justify-center bg-black">
            <video
              ref="videoRef"
              preload="auto"
              muted
              class="max-h-full max-w-full object-contain"
              :class="{ invisible: preview.inGap.value }"
            ></video>
            <audio ref="audioRef" preload="auto" style="display: none"></audio>
          </div>

          <PlayBar
            :playhead="store.playhead"
            :total-sec="total"
            :playing="store.playing"
            @prev="preview.seekToBoundary(-1)"
            @next="preview.seekToBoundary(1)"
            @toggle="preview.toggle()"
          />

          <!-- Timeline (was EditorTimeline.vue; now composed inline from
               timeline-shared/* atoms so the multitrack view can compose
               the same atoms with N tracks). -->
          <div
            class="relative flex h-48 shrink-0 overflow-hidden border-t border-border-base bg-bg-base"
            @contextmenu="onContextMenu"
          >
            <!-- Left: track labels. Each row's height matches the right-hand
                 ruler / track heights so the labels stay aligned. Width is
                 sized to the audio row's content (label + inline volume
                 popover button) — same single-line layout multitrack uses,
                 minus the delete-× since this editor's two tracks are fixed.
                 Narrower than the multitrack column for that reason. -->
            <div class="flex w-36 shrink-0 flex-col border-r border-border-base bg-bg-panel text-xs">
              <div class="h-7 shrink-0 border-b border-border-base"></div>
              <TimelineTrackLabel kind="video" label="视频" />
              <TimelineTrackLabel
                kind="audio"
                label="音频"
                :volume="audioVolume"
                @update:volume="audioVolume = $event"
              />
            </div>

            <!-- Right: scrolling area -->
            <div
              ref="scrollEl"
              class="relative flex-1 overflow-x-auto overflow-y-hidden"
              @wheel="zoom.onWheel"
            >
              <TimelineRuler
                ref="rulerCmp"
                :px-per-second="store.pxPerSecond"
                :total-sec="total"
                :track-width="trackWidth"
                @mousedown="onRulerMouseDown"
              />

              <TimelineTrackRow
                :track="videoTrack"
                :px-per-second="store.pxPerSecond"
                :track-width="trackWidth"
                :selected-ids="selectedVideoIds"
                height-class="h-12"
                @mousedown="(payload) => onTrackMouseDown(TRACK_VIDEO, payload)"
              />

              <TimelineTrackRow
                :track="audioTrack"
                :px-per-second="store.pxPerSecond"
                :track-width="trackWidth"
                :selected-ids="selectedAudioIds"
                height-class="h-12"
                @mousedown="(payload) => onTrackMouseDown(TRACK_AUDIO, payload)"
              />

              <TimelineRangeSelection
                :range="store.rangeSelection"
                :px-per-second="store.pxPerSecond"
              />

              <TimelinePlayhead
                v-show="store.project && store.splitScope === 'both'"
                :x="playheadX"
                @mousedown="onPlayheadMouseDown"
              />
              <TimelinePlayhead
                v-show="store.project && store.splitScope === TRACK_VIDEO"
                :x="playheadX"
                top="28px"
                height="48px"
                @mousedown="onPlayheadMouseDown"
              />
              <TimelinePlayhead
                v-show="store.project && store.splitScope === TRACK_AUDIO"
                :x="playheadX"
                top="76px"
                height="48px"
                @mousedown="onPlayheadMouseDown"
              />
            </div>
          </div>

          <EditorToolbar />
        </div>

        <!-- Lock the editing surface (preview + playbar + timeline +
             toolbar) while an export is running. The TopBar is disabled
             via :locked above, the right-hand export sidebar (with the
             cancel button) sits outside this overlay, and the global
             TabNav stays interactive — so the user can switch tabs or
             cancel the export, but can't accidentally edit clips while
             ffmpeg is running. -->
        <div
          v-if="job.running.value"
          class="pointer-events-auto absolute inset-0 z-10 flex flex-col items-center justify-center gap-2 bg-bg-base/60 backdrop-blur-[2px]"
        >
          <div class="text-sm text-fg-muted">导出中,编辑已锁定</div>
          <div class="text-xs text-fg-subtle">
            可在右侧面板取消导出,或切到其他 Tab。导出结束后自动解锁。
          </div>
        </div>
      </div>

      <!-- Right: export log sidebar (visible only during/after export) -->
      <ExportSidebar
        :open="exportSidebarOpen"
        :running="job.running.value"
        :state-label="job.stateLabel.value"
        :log="job.log.value"
        :progress="job.progress.value"
        :progress-visible="job.progressVisible.value"
        :finish-visible="job.finishVisible.value"
        :finish-kind="job.finishKind.value"
        :finish-text="job.finishText.value"
        :has-output-path="!!job.lastOutputPath.value"
        @close="closeExportSidebar"
        @cancel="job.cancel"
        @reveal="job.revealOutput"
      />
    </div>

    <ProjectsModal
      :open="projectsOpen"
      title="剪辑记录"
      empty-text="暂无剪辑工程"
      :list="listProjects"
      :remove="deleteProject"
      @close="projectsOpen = false"
      @load="loadProjectById"
    />

    <ExportDialog
      :open="exportOpen"
      :defaults="exportDefaults"
      :pick-dir="pickOutputDir"
      @close="exportOpen = false"
      @submit="onExportSubmit"
    />
  </section>
</template>
