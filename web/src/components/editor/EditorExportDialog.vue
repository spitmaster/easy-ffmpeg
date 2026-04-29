<script setup lang="ts">
import { reactive, watch } from 'vue'
import type { ExportSettings } from '@/api/editor'
import { useDirsStore } from '@/stores/dirs'
import { useEditorStore } from '@/stores/editor'
import { useModalsStore } from '@/stores/modals'

const props = defineProps<{ open: boolean }>()
const emit = defineEmits<{
  (e: 'close'): void
  (e: 'submit', settings: ExportSettings): void
}>()

const store = useEditorStore()
const dirs = useDirsStore()
const modals = useModalsStore()

const form = reactive<ExportSettings>({
  format: 'mp4',
  videoCodec: 'h264',
  audioCodec: 'aac',
  outputDir: '',
  outputName: '',
})

watch(
  () => props.open,
  (v) => {
    if (!v) return
    const e = store.project?.export
    form.format = e?.format || 'mp4'
    form.videoCodec = e?.videoCodec || 'h264'
    form.audioCodec = e?.audioCodec || 'aac'
    form.outputDir = e?.outputDir || dirs.outputDir || ''
    form.outputName = e?.outputName || store.project?.name || 'edit'
  },
)

async function pickDir() {
  const p = await modals.showPicker({
    mode: 'dir',
    title: '选择输出目录',
    startPath: form.outputDir || dirs.outputDir,
  })
  if (!p) return
  form.outputDir = p
  await dirs.saveOutput(p)
}

function submit() {
  if (!form.outputDir) {
    alert('请选择输出目录')
    return
  }
  if (!form.outputName) {
    alert('请输入文件名')
    return
  }
  emit('submit', { ...form })
}
</script>

<template>
  <div
    v-if="open"
    class="fixed inset-0 z-40 flex items-center justify-center bg-black/60"
  >
    <div class="w-[480px] rounded border border-border-strong bg-bg-panel">
      <div class="flex items-center justify-between border-b border-border-base px-4 py-2">
        <h3 class="text-sm font-medium">导出</h3>
        <button class="text-fg-muted hover:text-fg-base" @click="emit('close')">×</button>
      </div>
      <div class="flex flex-col gap-3 p-4 text-xs">
        <div>
          <label class="mb-1 block text-fg-muted">格式</label>
          <select
            v-model="form.format"
            class="w-full rounded border border-border-strong bg-bg-base px-2 py-1.5"
          >
            <option value="mp4">mp4</option>
            <option value="mkv">mkv</option>
            <option value="mov">mov</option>
            <option value="webm">webm</option>
          </select>
        </div>
        <div>
          <label class="mb-1 block text-fg-muted">视频编码</label>
          <select
            v-model="form.videoCodec"
            class="w-full rounded border border-border-strong bg-bg-base px-2 py-1.5"
          >
            <option value="h264">h264 (H.264/AVC)</option>
            <option value="h265">h265 (H.265/HEVC)</option>
            <option value="vp9">vp9</option>
            <option value="av1">av1</option>
          </select>
        </div>
        <div>
          <label class="mb-1 block text-fg-muted">音频编码</label>
          <select
            v-model="form.audioCodec"
            class="w-full rounded border border-border-strong bg-bg-base px-2 py-1.5"
          >
            <option value="aac">aac (AAC)</option>
            <option value="mp3">mp3 (MP3)</option>
            <option value="libopus">libopus (Opus)</option>
          </select>
        </div>
        <div>
          <label class="mb-1 block text-fg-muted">输出目录</label>
          <div class="flex gap-2">
            <button
              class="rounded border border-border-strong bg-bg-elevated px-3 py-1.5 hover:bg-bg-panel"
              @click="pickDir"
            >选择</button>
            <input
              v-model.trim="form.outputDir"
              readonly
              class="flex-1 rounded border border-border-strong bg-bg-base px-2 py-1.5 font-mono"
            />
          </div>
        </div>
        <div>
          <label class="mb-1 block text-fg-muted">文件名（不含后缀）</label>
          <input
            v-model.trim="form.outputName"
            class="w-full rounded border border-border-strong bg-bg-base px-2 py-1.5"
          />
        </div>
        <div class="flex justify-end gap-2 pt-2">
          <button
            class="rounded border border-border-strong bg-bg-base px-3 py-1.5 hover:bg-bg-elevated"
            @click="emit('close')"
          >取消</button>
          <button
            class="rounded bg-accent px-4 py-1.5 text-bg-base hover:bg-accent-hover"
            @click="submit"
          >开始导出</button>
        </div>
      </div>
    </div>
  </div>
</template>
