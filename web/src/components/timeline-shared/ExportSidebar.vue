<script setup lang="ts">
import type { LogLine, FinishKind } from '@/composables/useJobPanel'
import JobLog from '@/components/job/JobLog.vue'

/**
 * Export progress sidebar — wraps JobLog + a cancel button + a state label.
 * Pure props/emit; the parent owns the underlying useJobPanel state.
 */
defineProps<{
  open: boolean
  running: boolean
  stateLabel: string
  log: LogLine[]
  progress: number
  progressVisible: boolean
  finishVisible: boolean
  finishKind: FinishKind
  finishText: string
  hasOutputPath: boolean
  /** Sidebar header label; default: '导出日志'. */
  title?: string
}>()

defineEmits<{
  (e: 'close'): void
  (e: 'cancel'): void
  (e: 'reveal'): void
}>()
</script>

<template>
  <aside
    v-if="open"
    class="flex w-96 shrink-0 flex-col border-l border-border-base bg-bg-panel"
  >
    <div class="flex items-center gap-2 border-b border-border-base px-3 py-2">
      <span class="text-xs font-medium">{{ title ?? '导出日志' }}</span>
      <span class="text-xs text-fg-muted">{{ stateLabel }}</span>
      <div class="flex-1"></div>
      <button
        class="text-fg-muted hover:text-fg-base"
        title="关闭日志面板"
        @click="$emit('close')"
      >×</button>
    </div>
    <div class="flex-1 overflow-y-auto p-3">
      <JobLog
        :log="log"
        :progress="progress"
        :progress-visible="progressVisible"
        :finish-visible="finishVisible"
        :finish-kind="finishKind"
        :finish-text="finishText"
        :has-output-path="hasOutputPath"
        @reveal="$emit('reveal')"
      />
    </div>
    <div class="flex items-center gap-2 border-t border-border-base px-3 py-2">
      <button
        class="rounded border border-danger px-3 py-1 text-xs text-danger hover:bg-danger/10 disabled:opacity-40"
        :disabled="!running"
        @click="$emit('cancel')"
      >取消</button>
    </div>
  </aside>
</template>
