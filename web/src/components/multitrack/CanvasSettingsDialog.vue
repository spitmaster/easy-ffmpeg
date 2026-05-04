<script setup lang="ts">
import { computed, reactive, watch } from 'vue'
import type { MultitrackCanvas, MultitrackProject } from '@/api/multitrack'
import {
  clipsOutOfBounds,
  deriveCanvasFromSources,
  isValidCanvas,
} from '@/utils/multitrack-canvas'
import { useModalsStore } from '@/stores/modals'

/**
 * Project canvas settings dialog (v0.5.1 / M4). The dialog is fully
 * controlled — the parent owns `open`, passes the current canvas as
 * `defaults`, and the project (for the "use max source" preset and the
 * out-of-bounds calculation) as `project`. On submit it emits the new
 * canvas; the parent dispatches store.setCanvas.
 *
 * Out-of-bounds confirmation: if the candidate canvas would push any clip
 * fully outside the frame, the user gets a second-chance confirm
 * (`modals.showConfirm`) listing the affected clips before we emit. We
 * deliberately do NOT auto-clamp transforms — clipping data on a layout
 * change would silently destroy work; the user picks "continue" or the
 * Inspector "reset to full canvas" path in M5.
 */

const props = defineProps<{
  open: boolean
  defaults: MultitrackCanvas
  project: MultitrackProject
}>()

const emit = defineEmits<{
  (e: 'close'): void
  (e: 'submit', canvas: MultitrackCanvas): void
}>()

const modals = useModalsStore()

const form = reactive<MultitrackCanvas>({
  width: props.defaults.width,
  height: props.defaults.height,
  frameRate: props.defaults.frameRate,
})

// Reset the form whenever the dialog re-opens so the user always sees
// the current project canvas (not whatever they typed last time and
// abandoned).
watch(
  () => props.open,
  (v) => {
    if (!v) return
    form.width = props.defaults.width
    form.height = props.defaults.height
    form.frameRate = props.defaults.frameRate
  },
)

interface Preset {
  label: string
  build: () => MultitrackCanvas
}

const presets = computed<Preset[]>(() => [
  {
    label: '1920×1080@30 (1080p)',
    build: () => ({ width: 1920, height: 1080, frameRate: 30 }),
  },
  {
    label: '3840×2160@30 (4K UHD)',
    build: () => ({ width: 3840, height: 2160, frameRate: 30 }),
  },
  {
    label: '1080×1920@30 (竖屏 9:16)',
    build: () => ({ width: 1080, height: 1920, frameRate: 30 }),
  },
  {
    label: '使用源最大值',
    build: () => deriveCanvasFromSources(props.project),
  },
])

function applyPreset(p: Preset) {
  const c = p.build()
  form.width = c.width
  form.height = c.height
  form.frameRate = c.frameRate
}

const candidate = computed<MultitrackCanvas>(() => ({
  width: Math.round(form.width),
  height: Math.round(form.height),
  frameRate: Number(form.frameRate),
}))

const formValid = computed(() => isValidCanvas(candidate.value))

const oobClips = computed(() => clipsOutOfBounds(props.project, candidate.value))

async function submit() {
  if (!formValid.value) {
    await modals.showConfirm({
      title: '画布设置无效',
      message: '宽 / 高最少 16 像素;帧率必须在 (0, 240] 之间。',
      okText: '我知道了',
      hideCancel: true,
    })
    return
  }
  // Out-of-bounds second-chance: the user might be moving from a wide
  // canvas to a tall one and forget that some clips are placed off the
  // new edges. List the affected clip ids so they can match it back to
  // the timeline.
  if (oobClips.value.length > 0) {
    const lines = oobClips.value
      .slice(0, 6)
      .map((c) => `视频 ${c.trackIdx + 1} - clip ${c.clipId}`)
      .join('\n')
    const more = oobClips.value.length > 6 ? `\n... 还有 ${oobClips.value.length - 6} 条` : ''
    const ok = await modals.showConfirm({
      title: '部分 clip 将不可见',
      message: `新画布会让 ${oobClips.value.length} 个 clip 完全出画布,变得不可见。可以稍后在 Inspector 中重置为全画布。继续吗?`,
      detail: lines + more,
      okText: '继续修改',
      cancelText: '取消',
      danger: false,
    })
    if (!ok) return
  }
  emit('submit', candidate.value)
}
</script>

<template>
  <div
    v-if="open"
    class="fixed inset-0 z-40 flex items-center justify-center bg-black/60"
    @click.self="emit('close')"
  >
    <div class="w-[420px] rounded border border-border-strong bg-bg-panel">
      <div class="flex items-center justify-between border-b border-border-base px-4 py-2">
        <h3 class="text-sm font-medium">画布设置</h3>
        <button class="text-fg-muted hover:text-fg-base" @click="emit('close')">×</button>
      </div>
      <div class="flex flex-col gap-3 p-4 text-xs">
        <div class="grid grid-cols-3 gap-2">
          <div>
            <label class="mb-1 block text-fg-muted">宽 (像素)</label>
            <input
              v-model.number="form.width"
              type="number"
              min="16"
              step="2"
              class="w-full rounded border border-border-strong bg-bg-base px-2 py-1.5 font-mono"
            />
          </div>
          <div>
            <label class="mb-1 block text-fg-muted">高 (像素)</label>
            <input
              v-model.number="form.height"
              type="number"
              min="16"
              step="2"
              class="w-full rounded border border-border-strong bg-bg-base px-2 py-1.5 font-mono"
            />
          </div>
          <div>
            <label class="mb-1 block text-fg-muted">帧率 (fps)</label>
            <input
              v-model.number="form.frameRate"
              type="number"
              min="1"
              max="240"
              step="1"
              class="w-full rounded border border-border-strong bg-bg-base px-2 py-1.5 font-mono"
            />
          </div>
        </div>

        <div>
          <div class="mb-1 text-fg-muted">预设</div>
          <div class="flex flex-wrap gap-1.5">
            <button
              v-for="p in presets"
              :key="p.label"
              class="rounded border border-border-strong bg-bg-base px-2 py-1 hover:bg-bg-elevated"
              @click="applyPreset(p)"
            >{{ p.label }}</button>
          </div>
        </div>

        <div
          v-if="oobClips.length > 0"
          class="rounded border border-warning/60 bg-warning/10 px-2 py-1.5 text-warning"
        >
          ⚠ 当前候选画布会让 {{ oobClips.length }} 个 clip 完全不可见,提交时会再次确认。
        </div>

        <div class="flex justify-end gap-2 pt-2">
          <button
            class="rounded border border-border-strong bg-bg-base px-3 py-1.5 hover:bg-bg-elevated"
            @click="emit('close')"
          >取消</button>
          <button
            class="rounded bg-accent px-4 py-1.5 text-bg-base hover:bg-accent-hover disabled:opacity-50"
            :disabled="!formValid"
            @click="submit"
          >应用</button>
        </div>
      </div>
    </div>
  </div>
</template>
