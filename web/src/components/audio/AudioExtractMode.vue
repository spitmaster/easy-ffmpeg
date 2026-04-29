<script setup lang="ts">
import { computed, onMounted, reactive, ref, watch } from 'vue'
import {
  AudioCodecs,
  BITRATE_OPTIONS,
  CHANNEL_OPTIONS,
  SAMPLE_RATE_OPTIONS,
  audioApi,
  type AudioBody,
  type AudioStream,
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
  audioStreamIndex: 0,
  extractMethod: 'copy' as 'copy' | 'transcode',
  // transcode-only
  tFormat: 'mp3',
  tCodec: 'libmp3lame',
  tBitrate: '192',
  tSampleRate: 0,
  tChannels: 0,
})

const streams = ref<AudioStream[]>([])
const streamHint = ref('')
const detectedFormat = ref('mka')

const transcodeCodecOptions = computed<CodecOption[]>(() => AudioCodecs.codecsFor(form.tFormat))
const transcodeBitrateIgnored = computed(() => AudioCodecs.isBitrateIgnored(form.tFormat, form.tCodec))

watch(
  () => form.tFormat,
  () => {
    const list = transcodeCodecOptions.value
    if (!list.some((c) => c.v === form.tCodec)) form.tCodec = list[0]?.v || ''
  },
)
watch(
  () => form.audioStreamIndex,
  () => {
    const s = streams.value.find((x) => x.index === form.audioStreamIndex)
    detectedFormat.value = s ? AudioCodecs.containerForCodec(s.codecName) : 'mka'
  },
)
watch(
  () => [
    form.inputPath, form.outputDir, form.outputName, form.audioStreamIndex,
    form.extractMethod, form.tFormat, form.tCodec, form.tBitrate, form.tSampleRate, form.tChannels,
  ],
  () => emit('change'),
)

async function probeInput(path: string) {
  streamHint.value = '探测中...'
  try {
    const res = await audioApi.probe(path)
    streams.value = res.streams || []
    streamHint.value = streams.value.length ? `共 ${streams.value.length} 条音轨` : '未找到音频轨'
    if (streams.value.length) {
      form.audioStreamIndex = streams.value[0].index
      detectedFormat.value = AudioCodecs.containerForCodec(streams.value[0].codecName)
    }
  } catch (e) {
    streams.value = []
    streamHint.value = '探测失败: ' + (e instanceof Error ? e.message : String(e))
  }
}

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
  if (!form.outputName) form.outputName = base + '_audio'
  if (dir) await dirs.saveInput(dir)
  await probeInput(p)
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

function streamLabel(s: AudioStream): string {
  const bits = [`#${s.index + 1}`]
  if (s.codecName) bits.push(s.codecName.toUpperCase())
  if (s.channels) bits.push(`${s.channels}ch`)
  if (s.lang && s.lang !== 'und') bits.push(s.lang)
  else if (s.title) bits.push(s.title)
  return bits.join(' · ')
}

function readBody(): AudioBody {
  const base: AudioBody = {
    mode: 'extract',
    inputPath: form.inputPath.trim(),
    outputDir: form.outputDir.trim(),
    outputName: form.outputName.trim(),
    audioStreamIndex: form.audioStreamIndex,
    extractMethod: form.extractMethod,
    format: '',
  }
  if (form.extractMethod === 'copy') {
    return { ...base, format: detectedFormat.value, codec: 'copy' }
  }
  return {
    ...base,
    format: form.tFormat,
    codec: form.tCodec,
    bitrate: transcodeBitrateIgnored.value ? '' : form.tBitrate,
    sampleRate: form.tSampleRate,
    channels: form.tChannels,
  }
}

function validate(body: AudioBody) {
  if (!body.inputPath) throw new Error('请选择输入视频')
  if (!body.outputDir) throw new Error('请选择输出目录')
  if (!body.outputName) throw new Error('请输入输出文件名')
  if (body.extractMethod === 'copy' && !streams.value.length) {
    throw new Error('请等待音轨探测完成或选择有音轨的视频')
  }
}

function getOutputPath(body: AudioBody): string {
  return Path.join(body.outputDir, `${body.outputName}.${body.format}`)
}

function buildPreview(): string {
  const body = readBody()
  if (!body.inputPath || !body.outputDir || !body.outputName) return ''
  const parts = [`ffmpeg -y -i "${body.inputPath}" -vn -map 0:a:${body.audioStreamIndex}`]
  if (body.extractMethod === 'copy') {
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
      <label class="mb-1 block text-xs text-fg-muted">输入视频</label>
      <div class="flex gap-2">
        <button
          class="shrink-0 rounded border border-border-strong bg-bg-elevated px-3 py-1.5 text-xs hover:bg-bg-panel"
          @click="pickInput"
        >选择文件</button>
        <input
          v-model.trim="form.inputPath"
          type="text"
          placeholder="选择或粘贴视频路径..."
          class="flex-1 rounded border border-border-strong bg-bg-base px-2 py-1.5 font-mono text-xs"
        />
      </div>
    </div>

    <div>
      <label class="mb-1 block text-xs text-fg-muted">音轨</label>
      <div class="flex items-center gap-2">
        <select
          v-model.number="form.audioStreamIndex"
          :disabled="!streams.length"
          class="flex-[2] rounded border border-border-strong bg-bg-base px-2 py-1.5 text-xs disabled:opacity-50"
        >
          <option v-if="!streams.length" value="">（请选择视频文件）</option>
          <option v-for="s in streams" :key="s.index" :value="s.index">{{ streamLabel(s) }}</option>
        </select>
        <span class="flex-1 text-xs text-fg-muted">{{ streamHint }}</span>
      </div>
    </div>

    <div>
      <label class="mb-1 block text-xs text-fg-muted">输出方式</label>
      <div class="flex gap-4 text-xs">
        <label class="flex items-center gap-1">
          <input v-model="form.extractMethod" type="radio" value="copy" /> 直接拷贝（无损、秒完成）
        </label>
        <label class="flex items-center gap-1">
          <input v-model="form.extractMethod" type="radio" value="transcode" /> 转码为指定格式
        </label>
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
          title="在资源管理器中打开"
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

    <div v-if="form.extractMethod === 'copy'" class="text-xs text-fg-muted">
      选择视频后根据音轨编码自动选择输出容器：<code class="font-mono text-accent">{{ detectedFormat }}</code>
    </div>

    <template v-else>
      <div>
        <label class="mb-1 block text-xs text-fg-muted">格式 / 编码器 / 码率</label>
        <div class="flex gap-2">
          <select
            v-model="form.tFormat"
            class="flex-1 rounded border border-border-strong bg-bg-base px-2 py-1.5 text-xs"
          >
            <option v-for="f in AudioCodecs.formats" :key="f" :value="f">{{ f }}</option>
          </select>
          <select
            v-model="form.tCodec"
            class="flex-1 rounded border border-border-strong bg-bg-base px-2 py-1.5 text-xs"
          >
            <option v-for="o in transcodeCodecOptions" :key="o.v" :value="o.v">{{ o.t }}</option>
          </select>
          <select
            v-model="form.tBitrate"
            :disabled="transcodeBitrateIgnored"
            :title="transcodeBitrateIgnored ? '当前格式/编码器无需码率' : ''"
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
            v-model.number="form.tSampleRate"
            class="flex-1 rounded border border-border-strong bg-bg-base px-2 py-1.5 text-xs"
          >
            <option v-for="o in SAMPLE_RATE_OPTIONS" :key="o.v" :value="o.v">{{ o.t }}</option>
          </select>
          <select
            v-model.number="form.tChannels"
            class="flex-1 rounded border border-border-strong bg-bg-base px-2 py-1.5 text-xs"
          >
            <option v-for="o in CHANNEL_OPTIONS" :key="o.v" :value="o.v">{{ o.t }}</option>
          </select>
        </div>
      </div>
    </template>
  </div>
</template>
