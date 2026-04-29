<script setup lang="ts">
import { computed } from 'vue'
import { useEditorStore } from '@/stores/editor'
import { totalDuration } from '@/utils/timeline'
import { formatTime } from '@/utils/time'

const store = useEditorStore()

const emit = defineEmits<{
  (e: 'prev'): void
  (e: 'next'): void
  (e: 'toggle'): void
}>()

const tc = computed(() => `${formatTime(store.playhead)} / ${formatTime(totalDuration(store.project))}`)
</script>

<template>
  <div class="flex items-center gap-2 border-b border-border-base bg-bg-panel px-3 py-1.5">
    <button
      class="rounded border border-border-strong bg-bg-base px-2 py-1 text-xs hover:bg-bg-elevated"
      title="上一片段 (←)"
      @click="emit('prev')"
    >⏮</button>
    <button
      class="rounded border border-border-strong bg-bg-base px-3 py-1 text-xs hover:bg-bg-elevated"
      title="播放 / 暂停 (Space)"
      @click="emit('toggle')"
    >{{ store.playing ? '⏸' : '▶' }}</button>
    <button
      class="rounded border border-border-strong bg-bg-base px-2 py-1 text-xs hover:bg-bg-elevated"
      title="下一片段 (→)"
      @click="emit('next')"
    >⏭</button>
    <span class="ml-2 font-mono text-xs text-fg-muted">{{ tc }}</span>
  </div>
</template>
