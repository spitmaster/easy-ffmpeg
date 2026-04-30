<script setup lang="ts">
import { computed } from 'vue'
import { formatTime } from '@/utils/time'

const props = defineProps<{
  playhead: number
  totalSec: number
  playing: boolean
}>()

defineEmits<{
  (e: 'prev'): void
  (e: 'next'): void
  (e: 'toggle'): void
}>()

const tc = computed(() => `${formatTime(props.playhead)} / ${formatTime(props.totalSec)}`)
</script>

<template>
  <div class="flex items-center gap-2 border-b border-border-base bg-bg-panel px-3 py-1.5">
    <button
      class="rounded border border-border-strong bg-bg-base px-2 py-1 text-xs hover:bg-bg-elevated"
      title="上一片段 (←)"
      @click="$emit('prev')"
    >⏮</button>
    <button
      class="rounded border border-border-strong bg-bg-base px-3 py-1 text-xs hover:bg-bg-elevated"
      title="播放 / 暂停 (Space)"
      @click="$emit('toggle')"
    >{{ playing ? '⏸' : '▶' }}</button>
    <button
      class="rounded border border-border-strong bg-bg-base px-2 py-1 text-xs hover:bg-bg-elevated"
      title="下一片段 (→)"
      @click="$emit('next')"
    >⏭</button>
    <span class="ml-2 font-mono text-xs text-fg-muted">{{ tc }}</span>
  </div>
</template>
