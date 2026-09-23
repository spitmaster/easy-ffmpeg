import { computed, ref, watch, type ComputedRef, type Ref } from 'vue'
import { useDirsStore } from '@/stores/dirs'
import { useModalsStore } from '@/stores/modals'
import { Path } from '@/utils/path'
import type { ExportSettings } from '@/types/timeline'
import { useJobPanel, type JobPanelOptions } from '@/composables/useJobPanel'

/**
 * Backend response shape returned by *Api.startExport (postJsonRaw). Editor
 * and multitrack diverge in the body type they POST but converge on this
 * response wrapper.
 */
export interface StartExportResponse {
  res: Response
  data: {
    existing?: boolean
    path?: string
    error?: string
    command?: string
  }
}

export interface ExportApi<Body> {
  exportPreview: (body: Body) => Promise<{ command: string; outputPath: string }>
  startExport: (body: Body & { overwrite?: boolean }) => Promise<StartExportResponse>
  cancelExport: () => Promise<void>
}

export interface ExportFlowOptions<Project, Body> {
  /** The project being exported (e.g. editor.project / multitrack.project). */
  getProject: () => Project | null
  /** Default ExportSettings shown when the dialog opens — usually project.export ?? sensible defaults. */
  defaults: ComputedRef<ExportSettings>
  /**
   * Per-export validation. Returns null if ok, or a Chinese error message
   * the caller wants shown to the user. Single-video does a "videoClips
   * leading-gap" check; multitrack walks every video track.
   */
  validate: (project: Project, settings: ExportSettings) => string | null
  /** Persist before submit (autosave flush). */
  flushSave: () => Promise<void>
  /** Build the per-tab API body (projectId + export settings + tab-specific extras). */
  buildBody: (project: Project, settings: ExportSettings) => Body
  /** Tab-specific API client (editorApi / multitrackApi). */
  api: ExportApi<Body>
  /** Total duration in seconds — used for the job panel progress bar. */
  totalDurationSec: () => number
  /**
   * Optional lock setter. Multitrack sets store.exportLocked; single-video
   * doesn't have a lock (the whole editing surface is dimmed via job.running).
   */
  setLocked?: (v: boolean) => void
  /** Stop preview playback before the encoder starts (CPU + I/O contention). */
  pausePreview: () => void
  /** Job panel config (cancelUrl + labels). */
  jobOptions: JobPanelOptions
  /** Picker title for output dir; defaults to "选择输出目录". */
  pickerTitle?: string
}

/**
 * Owns the full export lifecycle so the two views don't keep growing
 * different copies: dialog open/close state, output-dir picker, export
 * submit (flushSave → exportPreview → command modal → startExport with
 * overwrite handling → job.startJob), sidebar close (with cancel-confirm),
 * and the underlying useJobPanel instance.
 *
 * Returns refs the view binds directly to ExportDialog / ExportSidebar
 * props plus actions for the topbar export button.
 */
export function useExportFlow<P, B>(opts: ExportFlowOptions<P, B>) {
  const dirs = useDirsStore()
  const modals = useModalsStore()

  const job = useJobPanel(opts.jobOptions)

  const dialogOpen = ref(false)
  const sidebarOpen = ref(false)

  function openDialog() {
    dialogOpen.value = true
  }

  async function pickOutputDir(current: string): Promise<string | null> {
    const p = await modals.showPicker({
      mode: 'dir',
      title: opts.pickerTitle || '选择输出目录',
      startPath: current || dirs.outputDir,
    })
    if (!p) return null
    await dirs.saveOutput(p)
    return p
  }

  async function submit(settings: ExportSettings) {
    const project = opts.getProject()
    if (!project) return

    const err = opts.validate(project, settings)
    if (err) {
      await modals.showConfirm({
        title: '提示',
        message: err,
        okText: '我知道了',
        hideCancel: true,
      })
      return
    }

    await opts.flushSave()
    const body = opts.buildBody(project, settings)

    let dryRun: { command: string; outputPath: string }
    try {
      dryRun = await opts.api.exportPreview(body)
    } catch (e) {
      await modals.showConfirm({
        title: '生成命令失败',
        message: e instanceof Error ? e.message : String(e),
        okText: '我知道了',
        hideCancel: true,
      })
      return
    }
    if (!(await modals.showCommand(dryRun.command))) return

    dialogOpen.value = false
    sidebarOpen.value = true
    opts.setLocked?.(true)
    opts.pausePreview()

    const sendStart = async (overwrite: boolean): Promise<string> => {
      const { res, data } = await opts.api.startExport({ ...body, overwrite })
      if (res.status === 409 && data.existing) {
        const ok = await modals.showOverwrite(data.path || '')
        if (!ok) throw new Error('已取消覆盖')
        return sendStart(true)
      }
      if (!res.ok) throw new Error(data.error || `HTTP ${res.status}`)
      return data.command || ''
    }

    try {
      await job.startJob({
        outputPath:
          dryRun.outputPath
          || Path.join(settings.outputDir, settings.outputName + '.' + settings.format),
        totalDurationSec: opts.totalDurationSec(),
        request: () => sendStart(false),
      })
    } catch {
      // job.startJob surfaces the error in its own panel; release the lock
      // immediately so the user can fix and retry without a stale gate.
      opts.setLocked?.(false)
    }
  }

  async function closeSidebar() {
    if (job.running.value) {
      if (!confirm('导出仍在进行中，关闭面板将取消导出。确认关闭？')) return
      try {
        await opts.api.cancelExport()
      } catch {
        // server may already be tearing down — fine to swallow.
      }
    }
    sidebarOpen.value = false
    opts.setLocked?.(false)
  }

  // Mirror useJobPanel.running back to setLocked: when ffmpeg finishes
  // (success / error / cancel via JobBus), release the lock so the UI
  // unlocks naturally. Only relevant when a setLocked was provided
  // (multitrack); single-video uses job.running for its dimming overlay.
  if (opts.setLocked) {
    watch(
      () => job.running.value,
      (v) => {
        if (!v) opts.setLocked!(false)
      },
    )
  }

  const runningRef: Ref<boolean> = job.running
  const running = computed(() => runningRef.value)

  return {
    dialogOpen,
    sidebarOpen,
    openDialog,
    submit,
    pickOutputDir,
    closeSidebar,
    job,
    running,
    defaults: opts.defaults,
  }
}
