<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, useTemplateRef, watch } from 'vue'
import { editorApi, type ExportSettings, type Project } from '@/api/editor'
import { useDirsStore } from '@/stores/dirs'
import { useEditorStore } from '@/stores/editor'
import { useModalsStore } from '@/stores/modals'
import { useEditorOps } from '@/composables/useEditorOps'
import { useEditorPreview } from '@/composables/useEditorPreview'
import { useJobPanel } from '@/composables/useJobPanel'
import { totalDuration } from '@/utils/timeline'
import { Path } from '@/utils/path'
import EditorTopBar from '@/components/editor/EditorTopBar.vue'
import EditorPlayBar from '@/components/editor/EditorPlayBar.vue'
import EditorTimeline from '@/components/editor/EditorTimeline.vue'
import EditorToolbar from '@/components/editor/EditorToolbar.vue'
import EditorAudioVolume from '@/components/editor/EditorAudioVolume.vue'
import EditorProjectsModal from '@/components/editor/EditorProjectsModal.vue'
import EditorExportDialog from '@/components/editor/EditorExportDialog.vue'
import EditorExportSidebar from '@/components/editor/EditorExportSidebar.vue'

const store = useEditorStore()
const dirs = useDirsStore()
const modals = useModalsStore()
const ops = useEditorOps()

const videoRef = ref<HTMLVideoElement | null>(null)
const audioRef = ref<HTMLAudioElement | null>(null)
const timelineRef = useTemplateRef<{ applyFit(): void }>('timelineRef')

const preview = useEditorPreview(videoRef, audioRef)

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
  requestAnimationFrame(() => timelineRef.value?.applyFit())
}

// ---- Export ----

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

  let preview: { command: string; outputPath: string }
  try {
    preview = await editorApi.exportPreview(body)
  } catch (e) {
    alert('生成命令失败: ' + (e instanceof Error ? e.message : String(e)))
    return
  }
  if (!(await modals.showCommand(preview.command))) return

  exportOpen.value = false
  exportSidebarOpen.value = true

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
    outputPath: preview.outputPath || Path.join(settings.outputDir, settings.outputName + '.' + settings.format),
    totalDurationSec: totalDuration(project),
    request: () => sendStart(false),
  })
}

async function closeExportSidebar() {
  if (job.running.value) {
    if (!confirm('导出仍在进行中，关闭面板将取消导出。确认关闭？')) return
    try { await editorApi.cancelExport() } catch { /* server may already be tearing down */ }
  }
  exportSidebarOpen.value = false
}

// ---- Keyboard shortcuts ----

function isEditableFocus(): boolean {
  const a = document.activeElement
  if (!a) return false
  const tag = a.tagName
  return tag === 'INPUT' || tag === 'TEXTAREA' || tag === 'SELECT' || (a as HTMLElement).isContentEditable
}

function onKeyDown(e: KeyboardEvent) {
  if (isEditableFocus()) return
  switch (e.key) {
    case ' ':
      e.preventDefault()
      preview.toggle()
      break
    case 's':
    case 'S':
      ops.splitAtPlayhead()
      break
    case 'Delete':
    case 'Backspace':
      ops.deleteSelection()
      break
    case 'ArrowLeft':
      preview.seekToBoundary(-1)
      break
    case 'ArrowRight':
      preview.seekToBoundary(1)
      break
    case 'z':
    case 'Z':
      if (e.ctrlKey || e.metaKey) {
        e.preventDefault()
        if (e.shiftKey) store.redo()
        else store.undo()
      }
      break
    case 'y':
    case 'Y':
      if (e.ctrlKey || e.metaKey) {
        e.preventDefault()
        store.redo()
      }
      break
    case 'Escape':
      if (store.rangeSelection) store.rangeSelection = null
      break
  }
}

onMounted(() => document.addEventListener('keydown', onKeyDown))
onBeforeUnmount(() => {
  document.removeEventListener('keydown', onKeyDown)
  // Best-effort flush: don't await to keep navigation snappy.
  store.flushSave().catch(() => {})
})

// Re-fit on project switch (covers loadProjectById flow that already calls
// applyFit, plus any future entry points).
watch(
  () => store.project?.id,
  () => {
    if (!store.project) return
    requestAnimationFrame(() => timelineRef.value?.applyFit())
  },
)
</script>

<template>
  <section class="flex h-full flex-col">
    <EditorTopBar
      @open-video="openVideo"
      @open-projects="projectsOpen = true"
      @open-export="exportOpen = true"
    />

    <div class="flex flex-1 overflow-hidden">
      <!-- Main column: preview / playbar / timeline / toolbar -->
      <div class="flex flex-1 flex-col overflow-hidden">
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

          <EditorPlayBar
            @prev="preview.seekToBoundary(-1)"
            @next="preview.seekToBoundary(1)"
            @toggle="preview.toggle()"
          />

          <EditorTimeline
            ref="timelineRef"
            @seek="(t: number) => preview.seek(t)"
            @pause-during-scrub="preview.pause()"
            @resume-after-scrub="preview.play()"
          >
            <template #audio-label>
              <!-- Stack the label and the volume control vertically — the
                   left column is narrow (~96px) and "🔊 音频" + "音量: 100%"
                   on one line wraps. -->
              <span class="flex w-full flex-col items-start gap-1">
                <span class="whitespace-nowrap">🔊 音频</span>
                <EditorAudioVolume />
              </span>
            </template>
          </EditorTimeline>

          <EditorToolbar />
        </div>
      </div>

      <!-- Right: export log sidebar (visible only during/after export) -->
      <EditorExportSidebar
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

    <EditorProjectsModal
      :open="projectsOpen"
      @close="projectsOpen = false"
      @load="loadProjectById"
    />

    <EditorExportDialog
      :open="exportOpen"
      @close="exportOpen = false"
      @submit="onExportSubmit"
    />
  </section>
</template>
