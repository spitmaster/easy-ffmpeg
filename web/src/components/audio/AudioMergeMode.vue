<script setup lang="ts">
import { computed, onMounted, reactive, ref, watch } from 'vue'
import {
  AudioCodecs,
  BITRATE_OPTIONS,
  audioApi,
  type AudioBody,
  type CodecOption,
} from '@/api/audio'
import { fsApi } from '@/api/fs'
import { useDirsStore } from '@/stores/dirs'
import { useModalsStore } from '@/stores/modals'
import { Path } from '@/utils/path'

interface MergeItem {
  path: string
  meta?: {
    codec?: string
    channels?: number
    sampleRate?: number
    bitRate?: number
    duration?: number
  }
}

const emit = defineEmits<{ (e: 'change'): void }>()

const dirs = useDirsStore()
const modals = useModalsStore()

const items = ref<MergeItem[]>([])

const form = reactive({
  outputDir: '',
  outputName: 'merged',
  format: 'mp3',
  codec: 'libmp3lame',
  bitrate: '192',
  strategy: 'auto' as 'auto' | 'copy' | 'reencode',
})

const codecOptions = computed<CodecOption[]>(() => AudioCodecs.codecsFor(form.format))
const bitrateIgnored = computed(() => AudioCodecs.isBitrateIgnored(form.format, form.codec))

watch(
  () => form.format,
  () => {
    const list = codecOptions.value
    if (!list.some((c) => c.v === form.codec)) form.codec = list[0]?.v || ''
  },
)
watch(
  () => [
    items.value.length,
    form.outputDir, form.outputName, form.format, form.codec, form.bitrate, form.strategy,
  ],
  () => emit('change'),
  { deep: true },
)

function humanDuration(sec?: number): string {
  if (!sec) return ''
  const s = Math.round(sec)
  const h = Math.floor(s / 3600)
  const m = Math.floor((s % 3600) / 60)
  const ss = s % 60
  if (h > 0) return `${h}:${String(m).padStart(2, '0')}:${String(ss).padStart(2, '0')}`
  return `${m}:${String(ss).padStart(2, '0')}`
}

function formatMeta(meta: MergeItem['meta']): string {
  if (!meta) return ''
  const bits: string[] = []
  if (meta.codec) bits.push(meta.codec.toUpperCase())
  if (meta.channels) bits.push(`${meta.channels}ch`)
  if (meta.bitRate) bits.push(`${Math.round(meta.bitRate / 1000)} kbps`)
  if (meta.duration) bits.push(humanDuration(meta.duration))
  return bits.join(' · ')
}

async function addFile() {
  const start = dirs.inputDir || ''
  const p = await modals.showPicker({ mode: 'file', title: '选择音频文件', startPath: start })
  if (!p) return
  const dir = Path.dirname(p)
  if (dir) await dirs.saveInput(dir)
  let meta: MergeItem['meta']
  try {
    const res = await audioApi.probe(p)
    const s = res.streams?.[0]
    if (s) {
      meta = {
        codec: s.codecName,
        channels: s.channels,
        sampleRate: s.sampleRate,
        bitRate: s.bitRate,
        duration: res.format?.duration,
      }
    }
  } catch {
    /* ignore — we still allow adding */
  }
  items.value.push({ path: p, meta })
}

function move(i: number, delta: number) {
  const j = i + delta
  if (j < 0 || j >= items.value.length) return
  const a = items.value[i]
  items.value[i] = items.value[j]
  items.value[j] = a
}

function remove(i: number) {
  items.value.splice(i, 1)
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

function readBody(): AudioBody {
  return {
    mode: 'merge',
    inputPaths: items.value.map((it) => it.path),
    outputDir: form.outputDir.trim(),
    outputName: form.outputName.trim(),
    format: form.format,
    codec: form.codec,
    bitrate: bitrateIgnored.value ? '' : form.bitrate,
    mergeStrategy: form.strategy,
  }
}

function validate(body: AudioBody) {
  if ((body.inputPaths?.length || 0) < 2) throw new Error('请至少添加 2 个输入文件')
  if (!body.outputDir) throw new Error('请选择输出目录')
  if (!body.outputName) throw new Error('请输入输出文件名')
  if (!body.format) throw new Error('请选择输出格式')
}

function getOutputPath(body: AudioBody): string {
  return Path.join(body.outputDir, `${body.outputName}.${body.format}`)
}

function buildPreview(): string {
  const body = readBody()
  const inputs = body.inputPaths || []
  if (inputs.length < 2 || !body.outputDir || !body.outputName) return ''
  const out = getOutputPath(body)
  if (body.mergeStrategy === 'copy') {
    return `ffmpeg -y -f concat -safe 0 -i <list.txt> -c copy "${out}"`
  }
  const parts = ['ffmpeg -y']
  inputs.forEach((p) => parts.push(`-i "${p}"`))
  const filter = inputs.map((_, i) => `[${i}:a]`).join('') +
    `concat=n=${inputs.length}:v=0:a=1[out]`
  parts.push(`-filter_complex "${filter}"`)
  parts.push(`-map "[out]" -c:a ${body.codec || 'aac'}`)
  if (body.bitrate && body.bitrate !== 'copy') parts.push(`-b:a ${body.bitrate}k`)
  parts.push(`"${out}"`)
  if (body.mergeStrategy === 'auto') parts.push('  # auto：编码一致时降级为快速拼接')
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
      <label class="mb-1 block text-xs text-fg-muted">输入文件（按顺序拼接）</label>
      <ul
        v-if="items.length"
        class="mb-2 flex flex-col gap-1 rounded border border-border-base bg-bg-base p-1 text-xs"
      >
        <li
          v-for="(it, i) in items"
          :key="i"
          class="flex items-center gap-2 rounded px-2 py-1 hover:bg-bg-elevated"
        >
          <span class="w-6 shrink-0 text-fg-muted">{{ i + 1 }}</span>
          <span class="flex-1 truncate font-mono" :title="it.path">{{ Path.basename(it.path) }}</span>
          <span class="text-fg-muted">{{ formatMeta(it.meta) }}</span>
          <button
            class="rounded px-2 hover:bg-bg-panel disabled:opacity-30"
            title="上移"
            :disabled="i === 0"
            @click="move(i, -1)"
          >↑</button>
          <button
            class="rounded px-2 hover:bg-bg-panel disabled:opacity-30"
            title="下移"
            :disabled="i === items.length - 1"
            @click="move(i, +1)"
          >↓</button>
          <button class="rounded px-2 text-danger hover:bg-bg-panel" title="移除" @click="remove(i)">🗑</button>
        </li>
      </ul>
      <button
        class="rounded border border-border-strong bg-bg-elevated px-3 py-1.5 text-xs hover:bg-bg-panel"
        @click="addFile"
      >+ 添加文件</button>
    </div>

    <div>
      <label class="mb-1 block text-xs text-fg-muted">合并策略</label>
      <div class="flex gap-4 text-xs">
        <label class="flex items-center gap-1">
          <input v-model="form.strategy" type="radio" value="auto" /> 自动判断
        </label>
        <label class="flex items-center gap-1">
          <input v-model="form.strategy" type="radio" value="copy" /> 快速拼接（编码一致）
        </label>
        <label class="flex items-center gap-1">
          <input v-model="form.strategy" type="radio" value="reencode" /> 重编码拼接
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

    <div>
      <label class="mb-1 block text-xs text-fg-muted">
        格式 / 编码器 / 码率（仅重编码分支用到；copy 分支仅用格式作后缀）
      </label>
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
  </div>
</template>
