<script setup lang="ts">
import { computed, onMounted, reactive } from 'vue'
import { convertApi, type ConvertBody } from '@/api/convert'
import { fsApi } from '@/api/fs'
import { useDirsStore } from '@/stores/dirs'
import { useModalsStore } from '@/stores/modals'
import { Path } from '@/utils/path'
import { useJobPanel } from '@/composables/useJobPanel'
import JobLog from '@/components/job/JobLog.vue'

const dirs = useDirsStore()
const modals = useModalsStore()

// Form state — kept reactive so the command preview is derived live.
const form = reactive({
  inputPath: '',
  outputDir: '',
  outputName: '',
  videoEncoder: 'h264',
  audioEncoder: 'aac',
  format: 'mp4',
})

const VIDEO_ENCODERS = [
  { v: 'h264', t: 'h264 (H.264/AVC)' },
  { v: 'h265', t: 'h265 (H.265/HEVC)' },
  { v: 'vp9', t: 'vp9 (VP9)' },
  { v: 'av1', t: 'av1 (AV1)' },
  { v: 'mpeg4', t: 'mpeg4 (MPEG-4)' },
  { v: 'copy', t: 'copy (快速拷贝)' },
]
const AUDIO_ENCODERS = [
  { v: 'aac', t: 'aac (AAC)' },
  { v: 'mp3', t: 'mp3 (MP3)' },
  { v: 'libopus', t: 'libopus (Opus)' },
  { v: 'libvorbis', t: 'libvorbis (Vorbis)' },
  { v: 'copy', t: 'copy (拷贝)' },
]
const FORMATS = ['mp4', 'mkv', 'avi', 'mov', 'flv', 'webm', 'm3u8']

function normalizeVideo(v: string): string {
  if (v === 'h264') return 'libx264'
  if (v === 'h265') return 'libx265'
  return v
}

const commandPreview = computed(() => {
  if (!form.inputPath || !form.outputDir || !form.outputName || !form.format) {
    return 'ffmpeg ...'
  }
  const out = Path.join(form.outputDir, `${form.outputName}.${form.format}`)
  const vc = normalizeVideo(form.videoEncoder)
  const ac = form.audioEncoder
  let cmd = `ffmpeg -y -i "${form.inputPath}"`
  cmd += vc === 'copy' && ac === 'copy' ? ' -c copy' : ` -c:v ${vc} -c:a ${ac}`
  cmd += ` "${out}"`
  return cmd
})

const job = useJobPanel({
  cancelUrl: '/api/convert/cancel',
  runningLabel: '转码中...',
  doneLabel: '✓ 转码完成',
  errorLabel: '✗ 转码失败',
  cancelledLabel: '! 转码已取消',
})

async function pickInput() {
  const p = await modals.showPicker({
    mode: 'file',
    title: '选择输入视频',
    startPath: form.inputPath || dirs.inputDir,
  })
  if (!p) return
  form.inputPath = p
  const dir = Path.dirname(p)
  const base = Path.stripExt(Path.basename(p))
  if (!form.outputName) form.outputName = base + '_converted'
  if (dir) await dirs.saveInput(dir)
}

async function pickOutputDir() {
  const p = await modals.showPicker({
    mode: 'dir',
    title: '选择输出目录',
    startPath: form.outputDir || dirs.outputDir,
  })
  if (!p) return
  form.outputDir = p
  await dirs.saveOutput(p)
}

async function openOutputDir() {
  if (!form.outputDir) return
  try {
    await fsApi.reveal(form.outputDir)
  } catch (e) {
    alert('打开失败: ' + (e instanceof Error ? e.message : String(e)))
  }
}

async function start() {
  if (!form.inputPath) return alert('请选择输入文件')
  if (!form.outputDir) return alert('请选择输出目录')
  if (!form.outputName) return alert('请输入输出文件名')

  const body: ConvertBody = {
    inputPath: form.inputPath,
    outputDir: form.outputDir,
    outputName: form.outputName,
    videoEncoder: form.videoEncoder,
    audioEncoder: form.audioEncoder,
    format: form.format,
  }
  const outputPath = Path.join(body.outputDir, `${body.outputName}.${body.format}`)

  // Preview the actual command first so the user can see (and copy) it.
  let preview: string
  try {
    preview = await convertApi.preview(body)
  } catch (e) {
    const msg = e instanceof Error ? e.message : String(e)
    alert('生成命令失败: ' + msg)
    return
  }
  if (!(await modals.showCommand(preview))) return

  // Real start — handle 409 "file exists" by asking confirmation and retrying.
  const sendStart = async (b: ConvertBody): Promise<string> => {
    const { res, data } = await convertApi.start(b)
    if (res.status === 409 && data.existing) {
      const ok = await modals.showOverwrite(data.path || '')
      if (!ok) throw new Error('已取消覆盖')
      return sendStart({ ...b, overwrite: true })
    }
    if (!res.ok) {
      throw new Error(data.error || `HTTP ${res.status}`)
    }
    return data.command || ''
  }

  await job.startJob({
    outputPath,
    request: () => sendStart(body),
  })
}

onMounted(() => {
  // Restore last-used output dir so users don't have to re-pick on every launch.
  if (dirs.outputDir) form.outputDir = dirs.outputDir
})
</script>

<template>
  <section class="mx-auto flex max-w-3xl flex-col gap-4 p-6">
    <!-- Input file -->
    <div>
      <label class="mb-1 block text-xs text-fg-muted">输入文件</label>
      <div class="flex gap-2">
        <button
          class="shrink-0 rounded border border-border-strong bg-bg-elevated px-3 py-1.5 text-xs hover:bg-bg-panel"
          @click="pickInput"
        >
          选择文件
        </button>
        <input
          v-model.trim="form.inputPath"
          type="text"
          placeholder="选择或粘贴输入视频路径..."
          class="flex-1 rounded border border-border-strong bg-bg-base px-2 py-1.5 font-mono text-xs"
        />
      </div>
    </div>

    <!-- Output dir / name -->
    <div>
      <label class="mb-1 block text-xs text-fg-muted">输出目录 / 文件名</label>
      <div class="flex gap-2">
        <button
          class="shrink-0 rounded border border-border-strong bg-bg-elevated px-3 py-1.5 text-xs hover:bg-bg-panel"
          @click="pickOutputDir"
        >
          选择目录
        </button>
        <input
          v-model.trim="form.outputDir"
          type="text"
          placeholder="输出目录..."
          class="flex-[2] rounded border border-border-strong bg-bg-base px-2 py-1.5 font-mono text-xs"
        />
        <button
          class="shrink-0 rounded border border-border-strong bg-bg-elevated px-3 py-1.5 text-xs hover:bg-bg-panel disabled:opacity-40"
          title="在资源管理器中打开输出目录"
          :disabled="!form.outputDir"
          @click="openOutputDir"
        >
          📂
        </button>
        <input
          v-model.trim="form.outputName"
          type="text"
          placeholder="文件名（不含后缀）"
          class="flex-1 rounded border border-border-strong bg-bg-base px-2 py-1.5 text-xs"
        />
      </div>
    </div>

    <!-- Encoder / format -->
    <div>
      <label class="mb-1 block text-xs text-fg-muted">编码器 / 格式</label>
      <div class="flex gap-2">
        <select
          v-model="form.videoEncoder"
          class="flex-1 rounded border border-border-strong bg-bg-base px-2 py-1.5 text-xs"
        >
          <option v-for="o in VIDEO_ENCODERS" :key="o.v" :value="o.v">{{ o.t }}</option>
        </select>
        <select
          v-model="form.audioEncoder"
          class="flex-1 rounded border border-border-strong bg-bg-base px-2 py-1.5 text-xs"
        >
          <option v-for="o in AUDIO_ENCODERS" :key="o.v" :value="o.v">{{ o.t }}</option>
        </select>
        <select
          v-model="form.format"
          class="flex-1 rounded border border-border-strong bg-bg-base px-2 py-1.5 text-xs"
        >
          <option v-for="f in FORMATS" :key="f" :value="f">{{ f }}</option>
        </select>
      </div>
    </div>

    <!-- Command preview -->
    <div>
      <label class="mb-1 block text-xs text-fg-muted">命令预览</label>
      <pre
        class="overflow-auto rounded border border-border-base bg-bg-base p-2 font-mono text-xs leading-relaxed text-fg-base whitespace-pre-wrap break-all"
      >{{ commandPreview }}</pre>
    </div>

    <!-- Action row -->
    <div class="flex items-center gap-2">
      <button
        class="rounded bg-accent px-4 py-1.5 text-xs text-bg-base hover:bg-accent-hover disabled:opacity-50"
        :disabled="job.running.value"
        @click="start"
      >
        开始转码
      </button>
      <button
        class="rounded border border-danger px-4 py-1.5 text-xs text-danger hover:bg-danger/10 disabled:opacity-40"
        :disabled="!job.running.value"
        @click="job.cancel"
      >
        取消
      </button>
      <span class="text-xs text-fg-muted">{{ job.stateLabel.value }}</span>
    </div>

    <!-- Log + finish bar -->
    <JobLog
      label="转码日志"
      :log="job.log.value"
      :progress="job.progress.value"
      :progress-visible="job.progressVisible.value"
      :finish-visible="job.finishVisible.value"
      :finish-kind="job.finishKind.value"
      :finish-text="job.finishText.value"
      :has-output-path="!!job.lastOutputPath.value"
      @reveal="job.revealOutput"
    />
  </section>
</template>
