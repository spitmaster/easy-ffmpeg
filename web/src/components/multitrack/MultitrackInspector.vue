<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import type { MultitrackCanvas, MultitrackClip, MultitrackTransform } from '@/api/multitrack'
import { useMultitrackStore } from '@/stores/multitrack'

/**
 * Right-side Inspector panel (v0.5.1 / M5). Two stacked sections:
 *   1. 画布 (always shown when a project is open) — read-only summary
 *      with a button to open CanvasSettingsDialog. The dialog is owned
 *      by MultitrackView; we just emit 'open-canvas-dialog'.
 *   2. 选中片段 (only when selectedVideoClip is non-null) — four numeric
 *      inputs (X / Y / W / H), a "复原原始比例" button, and a hint when
 *      the transform is fully outside the canvas. The reset button does
 *      a contain-fit using the source's original aspect ratio (centred
 *      in the canvas); when source dimensions are unknown it falls back
 *      to filling the whole canvas.
 *
 * Numeric inputs use a local draft (so typing "12" mid-stream doesn't
 * commit "1" then "12"). Commit on blur or Enter; Escape reverts to the
 * last committed value. While typing, we emit 'preview' on every change
 * so the overlay box mirrors the value live (mirroring TransformOverlay's
 * preview/commit split).
 *
 * The Inspector stays mounted alongside the export sidebar — but the
 * parent hides it during export by setting v-show off, so a busy export
 * never gets typed into.
 */

const props = defineProps<{
  /** When true the parent renders the panel collapsed to a 36px rail. */
  collapsed: boolean
}>()

const emit = defineEmits<{
  (e: 'open-canvas-dialog'): void
  (e: 'toggle'): void
}>()

const store = useMultitrackStore()

const canvas = computed<MultitrackCanvas | null>(() => store.project?.canvas ?? null)
const selected = computed(() => store.selectedVideoClip)

// Pretty label for the canvas summary line — "1920×1080 @ 30fps" (drop
// trailing zeros so 30.000 doesn't crowd the column).
const canvasLabel = computed(() => {
  const c = canvas.value
  if (!c) return ''
  const fr = Number.isInteger(c.frameRate) ? `${c.frameRate}` : `${c.frameRate.toFixed(2)}`
  return `${c.width}×${c.height} @ ${fr}fps`
})

// ---- Selected-clip transform editing ----

interface TransformDraft {
  x: number | null
  y: number | null
  w: number | null
  h: number | null
}

const draft = ref<TransformDraft>({ x: null, y: null, w: null, h: null })

// Re-seed the draft whenever the selection (or its transform) changes
// from outside the panel — overlay drag, undo, redo, etc.
watch(
  () => selected.value?.clip.transform,
  (t) => {
    if (!t) {
      draft.value = { x: null, y: null, w: null, h: null }
      return
    }
    draft.value = { x: t.x, y: t.y, w: t.w, h: t.h }
  },
  { immediate: true, deep: true },
)

function readDraft(): MultitrackTransform | null {
  if (!selected.value) return null
  const t = selected.value.clip.transform
  const x = Number.isFinite(draft.value.x) ? Math.round(draft.value.x as number) : t.x
  const y = Number.isFinite(draft.value.y) ? Math.round(draft.value.y as number) : t.y
  const w = Number.isFinite(draft.value.w) ? Math.max(1, Math.round(draft.value.w as number)) : t.w
  const h = Number.isFinite(draft.value.h) ? Math.max(1, Math.round(draft.value.h as number)) : t.h
  return { x, y, w, h }
}

function previewDraft() {
  const sel = selected.value
  const t = readDraft()
  if (!sel || !t) return
  store.previewClipTransform(sel.trackId, sel.clipId, t)
}

function commitDraft() {
  const sel = selected.value
  const t = readDraft()
  if (!sel || !t) return
  // Only commit if the value actually moved — typing the same number then
  // tabbing out shouldn't pollute the undo stack.
  const cur = sel.clip.transform
  if (t.x === cur.x && t.y === cur.y && t.w === cur.w && t.h === cur.h) return
  store.commitClipTransform(sel.trackId, sel.clipId, t)
}

function resetTransform() {
  const sel = selected.value
  const c = canvas.value
  if (!sel || !c) return
  const target = computeResetTransform(sel.clip.sourceId, c)
  const cur = sel.clip.transform
  if (target.x === cur.x && target.y === cur.y && target.w === cur.w && target.h === cur.h) return
  store.commitClipTransform(sel.trackId, sel.clipId, target)
}

// Contain-fit centred in the canvas using source dimensions; when the
// source's width/height aren't known (older projects, edge probes), fall
// back to filling the canvas so the button still does something useful.
function computeResetTransform(sourceId: string, c: MultitrackCanvas): MultitrackTransform {
  const src = store.sourcesById[sourceId]
  if (!src || !src.width || !src.height || src.width <= 0 || src.height <= 0) {
    return { x: 0, y: 0, w: c.width, h: c.height }
  }
  const scale = Math.min(c.width / src.width, c.height / src.height)
  const w = Math.max(1, Math.round(src.width * scale))
  const h = Math.max(1, Math.round(src.height * scale))
  return {
    x: Math.round((c.width - w) / 2),
    y: Math.round((c.height - h) / 2),
    w,
    h,
  }
}

function fullyOutOfBounds(clip: MultitrackClip, c: MultitrackCanvas): boolean {
  const t = clip.transform
  if (t.x + t.w <= 0) return true
  if (t.x >= c.width) return true
  if (t.y + t.h <= 0) return true
  if (t.y >= c.height) return true
  return false
}

const oob = computed(() => {
  const sel = selected.value
  const c = canvas.value
  if (!sel || !c) return false
  return fullyOutOfBounds(sel.clip, c)
})

// Esc inside an input → revert that input to the model and blur. Other
// keys with side effects (Enter to commit) are emitted through @change /
// @blur handlers below so we don't reinvent the form lifecycle.
function onKeyDown(ev: KeyboardEvent) {
  if (ev.key === 'Escape') {
    if (selected.value) {
      const t = selected.value.clip.transform
      draft.value = { x: t.x, y: t.y, w: t.w, h: t.h }
    }
    ;(ev.target as HTMLElement).blur()
  }
}
</script>

<template>
  <aside
    v-if="props.collapsed"
    class="flex h-full w-9 shrink-0 flex-col items-center border-l border-border-base bg-bg-panel pt-2"
    :title="'展开 Inspector'"
  >
    <button
      class="rounded px-1 py-0.5 text-xs text-fg-muted hover:bg-bg-elevated hover:text-fg-base"
      @click="emit('toggle')"
    >‹</button>
    <div class="mt-2 select-none px-1 text-[10px] leading-tight text-fg-muted [writing-mode:vertical-rl]">
      属性
    </div>
  </aside>

  <aside
    v-else
    class="flex h-full w-64 shrink-0 flex-col border-l border-border-base bg-bg-panel text-xs"
  >
    <div class="flex shrink-0 items-center gap-2 border-b border-border-base px-3 py-2">
      <span class="font-medium">属性</span>
      <div class="flex-1"></div>
      <button
        class="rounded px-1 py-0.5 text-fg-muted hover:bg-bg-elevated hover:text-fg-base"
        :title="'折叠'"
        @click="emit('toggle')"
      >›</button>
    </div>

    <div class="flex-1 overflow-y-auto">
      <!-- Canvas section -->
      <section v-if="canvas" class="border-b border-border-base px-3 py-3">
        <div class="mb-1 text-fg-muted">画布</div>
        <div class="font-mono">{{ canvasLabel }}</div>
        <button
          class="mt-2 w-full rounded border border-border-strong bg-bg-base px-2 py-1 hover:bg-bg-elevated disabled:opacity-50"
          :disabled="store.exportLocked"
          @click="emit('open-canvas-dialog')"
        >修改画布…</button>
      </section>

      <!-- Selected clip transform section -->
      <section v-if="selected" class="border-b border-border-base px-3 py-3">
        <div class="mb-2 flex items-center gap-2">
          <span class="text-fg-muted">选中片段</span>
          <span class="ml-auto truncate font-mono text-[10px] text-fg-muted">{{ selected.clipId }}</span>
        </div>

        <div class="grid grid-cols-2 gap-2">
          <label class="block">
            <span class="mb-0.5 block text-fg-muted">X</span>
            <input
              v-model.number="draft.x"
              type="number"
              step="1"
              :disabled="store.exportLocked"
              class="w-full rounded border border-border-strong bg-bg-base px-2 py-1 font-mono"
              @input="previewDraft"
              @change="commitDraft"
              @blur="commitDraft"
              @keydown="onKeyDown"
            />
          </label>
          <label class="block">
            <span class="mb-0.5 block text-fg-muted">Y</span>
            <input
              v-model.number="draft.y"
              type="number"
              step="1"
              :disabled="store.exportLocked"
              class="w-full rounded border border-border-strong bg-bg-base px-2 py-1 font-mono"
              @input="previewDraft"
              @change="commitDraft"
              @blur="commitDraft"
              @keydown="onKeyDown"
            />
          </label>
          <label class="block">
            <span class="mb-0.5 block text-fg-muted">宽 W</span>
            <input
              v-model.number="draft.w"
              type="number"
              min="1"
              step="1"
              :disabled="store.exportLocked"
              class="w-full rounded border border-border-strong bg-bg-base px-2 py-1 font-mono"
              @input="previewDraft"
              @change="commitDraft"
              @blur="commitDraft"
              @keydown="onKeyDown"
            />
          </label>
          <label class="block">
            <span class="mb-0.5 block text-fg-muted">高 H</span>
            <input
              v-model.number="draft.h"
              type="number"
              min="1"
              step="1"
              :disabled="store.exportLocked"
              class="w-full rounded border border-border-strong bg-bg-base px-2 py-1 font-mono"
              @input="previewDraft"
              @change="commitDraft"
              @blur="commitDraft"
              @keydown="onKeyDown"
            />
          </label>
        </div>

        <button
          class="mt-3 w-full rounded border border-border-strong bg-bg-base px-2 py-1 hover:bg-bg-elevated disabled:opacity-50"
          :disabled="store.exportLocked"
          :title="'按源比例居中适配画布 (Ctrl+0)'"
          @click="resetTransform"
        >复原原始比例</button>

        <div
          v-if="oob"
          class="mt-2 rounded border border-warning/60 bg-warning/10 px-2 py-1 text-warning"
        >
          ⚠ 当前 transform 完全在画布外,导出时此片段不可见
        </div>
      </section>

      <section v-else-if="canvas" class="px-3 py-3 text-fg-muted">
        <div class="text-[11px] leading-snug">选中视频轨上的片段以编辑位置 / 尺寸</div>
        <div class="mt-2 text-[10px]">
          快捷键:<br>
          ←/→/↑/↓ 微调 1px<br>
          Shift+← 等 一次 10px<br>
          Ctrl+0 复原原始比例
        </div>
      </section>
    </div>
  </aside>
</template>
