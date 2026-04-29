<script setup lang="ts">
import { RouterLink } from 'vue-router'

const tabs = [
  { to: '/convert', label: '视频转换' },
  { to: '/audio', label: '音频处理' },
  { to: '/editor', label: '单视频剪辑' },
]
</script>

<template>
  <nav class="flex shrink-0 gap-1 border-b border-border-base bg-bg-panel px-4">
    <RouterLink
      v-for="t in tabs"
      :key="t.to"
      :to="t.to"
      class="border-b-2 border-transparent px-3 py-2 text-sm text-fg-muted hover:text-fg-base"
      active-class="!border-accent !text-fg-base"
    >
      {{ t.label }}
    </RouterLink>
  </nav>
</template>
