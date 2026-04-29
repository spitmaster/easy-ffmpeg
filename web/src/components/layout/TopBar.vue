<script setup lang="ts">
// M1: static placeholder. M2 will wire the version chip to /api/version
// and the ffmpeg status chip to /api/ffmpeg/status via Pinia stores.
</script>

<template>
  <header
    class="flex h-12 shrink-0 items-center justify-between border-b border-border-base bg-bg-panel px-4"
  >
    <div class="flex items-center gap-2">
      <span class="text-xl">🎬</span>
      <span class="text-sm font-medium">Easy FFmpeg</span>
    </div>
    <div class="flex items-center gap-2 text-xs text-fg-muted">
      <span class="rounded bg-bg-elevated px-2 py-1">脚手架就绪</span>
    </div>
  </header>
</template>
