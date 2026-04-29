<script setup lang="ts">
import { nextTick, ref, watch } from 'vue'
import type { LogLine, FinishKind } from '@/composables/useJobPanel'

const props = defineProps<{
  log: LogLine[]
  progress: number
  progressVisible: boolean
  finishVisible: boolean
  finishKind: FinishKind
  finishText: string
  hasOutputPath: boolean
  label?: string
}>()

const emit = defineEmits<{
  (e: 'reveal'): void
}>()

const logEl = ref<HTMLDivElement | null>(null)

watch(
  () => props.log.length,
  async () => {
    await nextTick()
    if (logEl.value) logEl.value.scrollTop = logEl.value.scrollHeight
  },
)
</script>

<template>
  <div class="flex flex-col gap-2">
    <div
      v-if="progressVisible"
      class="flex items-center gap-2"
    >
      <div class="h-2 flex-1 overflow-hidden rounded bg-bg-base">
        <div
          class="h-full bg-accent transition-[width] duration-200"
          :style="{ width: (progress * 100).toFixed(1) + '%' }"
        ></div>
      </div>
      <span class="w-14 text-right font-mono text-xs text-fg-muted">
        {{ (progress * 100).toFixed(1) }}%
      </span>
    </div>

    <div
      v-if="label"
      class="text-xs text-fg-muted"
    >
      {{ label }}
    </div>
    <div
      ref="logEl"
      class="h-56 overflow-auto rounded border border-border-base bg-bg-base p-2 font-mono text-xs leading-relaxed"
    >
      <span
        v-for="(line, i) in log"
        :key="i"
        class="block whitespace-pre-wrap break-all"
        :class="{
          'text-fg-base': line.cls === '',
          'text-accent': line.cls === 'info',
          'text-danger': line.cls === 'error',
          'text-fg-muted': line.isProgress,
        }"
      >{{ line.text }}</span>
    </div>

    <div
      v-if="finishVisible"
      class="flex items-center gap-2 rounded border px-3 py-2 text-xs"
      :class="{
        'border-success/40 bg-success/10 text-success': finishKind === 'success',
        'border-danger/40 bg-danger/10 text-danger': finishKind === 'error',
        'border-fg-subtle/40 bg-bg-elevated text-fg-muted': finishKind === 'cancelled',
      }"
    >
      <span>{{ finishText }}</span>
      <div class="flex-1"></div>
      <button
        v-if="hasOutputPath"
        class="rounded border border-border-strong px-2 py-1 text-xs text-fg-base hover:bg-bg-base"
        @click="emit('reveal')"
      >
        📂 打开文件夹
      </button>
    </div>
  </div>
</template>
