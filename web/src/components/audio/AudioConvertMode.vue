<script setup lang="ts">
import { computed, onMounted, reactive, watch } from 'vue'
import {
  AudioCodecs,
  BITRATE_OPTIONS,
  CHANNEL_OPTIONS,
  SAMPLE_RATE_OPTIONS,
  type AudioBody,
  type CodecOption,
} from '@/api/audio'
import { fsApi } from '@/api/fs'
import { useDirsStore } from '@/stores/dirs'
import { useModalsStore } from '@/stores/modals'
import { Path } from '@/utils/path'

const emit = defineEmits<{ (e: 'change'): void }>()

const dirs = useDirsStore()
const modals = useModalsStore()

const form = reactive({
  inputPath: '',
  outputDir: '',
  outputName: '',
  format: 'mp3',
  codec: 'libmp3lame',
  bitrate: '192',
  sampleRate: 0,
  channels: 0,
})

const codecOptions = computed<CodecOption[]>(() => AudioCodecs.codecsFor(form.format))
const bitrateIgnored = computed(() => AudioCodecs.isBitrateIgnored(form.format, form.codec))

watch(
  () => form.format,
  () => {
    const list = codecOptions.value
    if (!list.some((c) => c.v === form.codec)) {
      form.codec = list[0]?.v || ''
    }
  },
)
watch([() => form.format, () => form.codec, () => form.inputPath, () => form.outputDir, () => form.outputName, () => form.bitrate, () => form.sampleRate, () => form.channels], () => emit('change'))

async function pickInput() {
  const p = await modals.showPicker({
    mode: 'file',
    title: '选择输入音频',
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
    await modals.showConfirm({
      title: '打开失败',
      message: e instanceof Error ? e.message : String(e),
      okText: '我知道了',
      hideCancel: true,
    })
  }
}

function readBody(): AudioBody {
  return {
    mode: 'convert',
    inputPath: form.inputPath.trim(),
    outputDir: form.outputDir.trim(),
    outputName: form.outputName.trim(),
    format: form.format,
    codec: form.codec,
    bitrate: bitrateIgnored.value ? '' : form.bitrate,
    sampleRate: form.sampleRate,
    channels: form.channels,
  }
}

function validate(body: AudioBody) {
  if (!body.inputPath) throw new Error('请选择输入文件')
  if (!body.outputDir) throw new Error('请选择输出目录')
  if (!body.outputName) throw new Error('请输入输出文件名')
  if (!body.format) throw new Error('请选择输出格式')
}

function getOutputPath(body: AudioBody): string {
  return Path.join(body.outputDir, `${body.outputName}.${body.format}`)
}

function buildPreview(): string {
  const body = readBody()
  if (!body.inputPath || !body.outputDir || !body.outputName) return ''
  const parts = [`ffmpeg -y -i "${body.inputPath}" -vn`]
  if (body.codec === 'copy') {
    parts.push(`-c:a copy`)
  } else {
    parts.push(`-c:a ${body.codec}`)
    if (body.bitrate && body.bitrate !== 'copy') parts.push(`-b:a ${body.bitrate}k`)
    if (body.sampleRate) parts.push(`-ar ${body.sampleRate}`)
    if (body.channels) parts.push(`-ac ${body.channels}`)
  }
  parts.push(`"${getOutputPath(body)}"`)
  return parts.join(' ')
}

defineExpose({ readBody, validate, getOutputPath, buildPreview })

onMounted(() => {
  if (dirs.outputDir) form.outputDir = dirs.outputDir
})
</script>

<template>
  <div class="flex flex-col gap-4">
    <div>
      <label class="mb-1 block text-xs text-fg-muted">输入音频</label>
      <div class="flex gap-2">
        <button
          class="shrink-0 rounded border border-border-strong bg-bg-elevated px-3 py-1.5 text-xs hover:bg-bg-panel"
          @click="pickInput"
        >选择文件</button>
        <input
          v-model.trim="form.inputPath"
          type="text"
          placeholder="选择或粘贴输入音频路径..."
          class="flex-1 rounded border border-border-strong bg-bg-base px-2 py-1.5 font-mono text-xs"
        />
      </div>
    </div>

    <div>
      <label class="mb-1 block text-xs text-fg-muted">输出目录 / 文件名</label>
      <div class="flex gap-2">
        <button
          class="shrink-0 rounded border border-border-strong bg-bg-elevated px-3 py-1.5 text-xs hover:bg-bg-panel"
          @click="pickOutputDir"
        >选择目录</button>
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
        >📂</button>
        <input
          v-model.trim="form.outputName"
          type="text"
          placeholder="文件名（不含后缀）"
          class="flex-1 rounded border border-border-strong bg-bg-base px-2 py-1.5 text-xs"
        />
      </div>
    </div>

    <div>
      <label class="mb-1 block text-xs text-fg-muted">格式 / 编码器 / 码率</label>
      <div class="flex gap-2">
        <select
          v-model="form.format"
          class="flex-1 rounded border border-border-strong bg-bg-base px-2 py-1.5 text-xs"
        >
          <option v-for="f in AudioCodecs.formats" :key="f" :value="f">{{ f }}</option>
        </select>
        <select
          v-model="form.codec"
          class="flex-1 rounded border border-border-strong bg-bg-base px-2 py-1.5 text-xs"
        >
          <option v-for="o in codecOptions" :key="o.v" :value="o.v">{{ o.t }}</option>
        </select>
        <select
          v-model="form.bitrate"
          :disabled="bitrateIgnored"
          :title="bitrateIgnored ? '当前格式/编码器无需码率' : ''"
          class="flex-1 rounded border border-border-strong bg-bg-base px-2 py-1.5 text-xs disabled:opacity-50"
        >
          <option v-for="b in BITRATE_OPTIONS" :key="b" :value="b">
            {{ b === 'copy' ? '保持原码率' : `${b} kbps` }}
          </option>
        </select>
      </div>
    </div>

    <div>
      <label class="mb-1 block text-xs text-fg-muted">采样率 / 声道</label>
      <div class="flex gap-2">
        <select
          v-model.number="form.sampleRate"
          class="flex-1 rounded border border-border-strong bg-bg-base px-2 py-1.5 text-xs"
        >
          <option v-for="o in SAMPLE_RATE_OPTIONS" :key="o.v" :value="o.v">{{ o.t }}</option>
        </select>
        <select
          v-model.number="form.channels"
          class="flex-1 rounded border border-border-strong bg-bg-base px-2 py-1.5 text-xs"
        >
          <option v-for="o in CHANNEL_OPTIONS" :key="o.v" :value="o.v">{{ o.t }}</option>
        </select>
      </div>
    </div>
  </div>
</template>
