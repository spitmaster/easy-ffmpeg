import { ref, onUnmounted, type Ref } from 'vue'
import { jobBus, type JobEvent } from '@/api/jobs'
import { fsApi } from '@/api/fs'

export type FinishKind = 'success' | 'error' | 'cancelled'

export interface LogLine {
  text: string
  cls: '' | 'info' | 'error'
  /** Set on lines that look like ffmpeg progress (frame= / size= / video: / Lsize=). */
  isProgress: boolean
}

export interface JobPanelOptions {
  /** Endpoint POSTed to cancel this job. */
  cancelUrl: string
  runningLabel?: string
  idleLabel?: string
  doneLabel?: string
  errorLabel?: string
  cancelledLabel?: string
}

const PROGRESS_RE = /^(frame=|size=|video:|Lsize=)/
const DUR_RE = /Duration:\s*(\d+):(\d+):([\d.]+)/
const TIME_RE = /time=(\d+):(\d+):([\d.]+)/

/**
 * Per-tab job state machine: log lines, progress %, running flag, finish
 * banner. Mirrors the legacy `createJobPanel` factory in app.js but built
 * on Vue refs so the templates can render off it directly.
 *
 * Multiple tabs can call useJobPanel — each instance owns its own `running`
 * flag and listens to JobBus, but only the panel that initiated the
 * current job (set via start()) responds to log/done/error events. This
 * matches the legacy "owning" guard.
 */
export function useJobPanel(opts: JobPanelOptions) {
  const {
    cancelUrl,
    runningLabel = '处理中...',
    idleLabel = '空闲',
    doneLabel = '✓ 完成',
    errorLabel = '✗ 失败',
    cancelledLabel = '! 已取消',
  } = opts

  const running = ref(false)
  const stateLabel = ref(idleLabel)
  const log: Ref<LogLine[]> = ref([])
  const progressVisible = ref(false)
  const progress = ref(0) // 0..1
  const finishVisible = ref(false)
  const finishKind = ref<FinishKind>('success')
  const finishText = ref('')
  const lastOutputPath = ref<string | null>(null)

  let owning = false
  let totalSec = 0

  function setRunning(v: boolean) {
    running.value = v
    stateLabel.value = v ? runningLabel : idleLabel
  }

  function appendLog(text: string, cls: LogLine['cls'] = '') {
    if (!cls) parseForProgress(text)
    const isProgress = !cls && PROGRESS_RE.test(text)
    if (isProgress) {
      const last = log.value[log.value.length - 1]
      if (last && last.isProgress) {
        // Replace the trailing progress line in place so the log doesn't
        // grow unbounded during a long encode.
        log.value[log.value.length - 1] = { text, cls: '', isProgress: true }
        return
      }
    }
    log.value.push({ text, cls, isProgress })
  }

  function parseForProgress(line: string) {
    if (!totalSec) {
      const m = DUR_RE.exec(line)
      if (m) totalSec = +m[1] * 3600 + +m[2] * 60 + parseFloat(m[3])
    }
    const t = TIME_RE.exec(line)
    if (!t || !totalSec) return
    const cur = +t[1] * 3600 + +t[2] * 60 + parseFloat(t[3])
    progress.value = Math.max(0, Math.min(1, cur / totalSec))
  }

  function showFinish(kind: FinishKind, text: string, revealPath: string | null) {
    finishVisible.value = true
    finishKind.value = kind
    finishText.value = text
    lastOutputPath.value = revealPath
  }

  function hideFinish() {
    finishVisible.value = false
    lastOutputPath.value = null
  }

  /**
   * Begin a job. The caller has already shown the command-preview modal
   * and the user confirmed. Reset state, send the POST, and on 409 ask
   * the caller to handle overwrite confirm via the returned hook.
   */
  function startJob(options: {
    outputPath: string | null
    totalDurationSec?: number
    /** POST to start; returns the started command for log header. */
    request: () => Promise<string>
  }) {
    log.value = []
    hideFinish()
    lastOutputPath.value = options.outputPath
    totalSec = options.totalDurationSec && options.totalDurationSec > 0 ? options.totalDurationSec : 0
    progress.value = 0
    progressVisible.value = true
    return (async () => {
      try {
        const cmd = await options.request()
        appendLog('> ' + cmd, 'info')
        owning = true
        setRunning(true)
      } catch (e) {
        progressVisible.value = false
        const msg = e instanceof Error ? e.message : String(e)
        showFinish('error', '✗ 启动失败: ' + msg, null)
      }
    })()
  }

  async function cancel() {
    try {
      await fetch(cancelUrl, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: '{}',
      })
    } catch {
      /* server may already be tearing down */
    }
  }

  async function revealOutput() {
    if (!lastOutputPath.value) return
    try {
      await fsApi.reveal(lastOutputPath.value)
    } catch (e) {
      const msg = e instanceof Error ? e.message : String(e)
      alert('打开失败: ' + msg)
    }
  }

  const unsubscribe = jobBus.subscribe((ev: JobEvent) => {
    if (!owning) return
    switch (ev.type) {
      case 'state':
        setRunning(!!ev.running)
        break
      case 'log':
        appendLog(ev.line)
        break
      case 'done':
        owning = false
        setRunning(false)
        progress.value = 1
        // Brief "100%" hold so the bar doesn't visibly snap back to 0
        // when the next job starts.
        setTimeout(() => (progressVisible.value = false), 600)
        showFinish('success', doneLabel, lastOutputPath.value)
        break
      case 'error':
        owning = false
        setRunning(false)
        progressVisible.value = false
        showFinish('error', `${errorLabel}: ${ev.message || ''}`, null)
        break
      case 'cancelled':
        owning = false
        setRunning(false)
        progressVisible.value = false
        showFinish('cancelled', cancelledLabel, null)
        break
    }
  })

  onUnmounted(unsubscribe)

  return {
    running,
    stateLabel,
    log,
    progress,
    progressVisible,
    finishVisible,
    finishKind,
    finishText,
    lastOutputPath,
    startJob,
    cancel,
    revealOutput,
  }
}
