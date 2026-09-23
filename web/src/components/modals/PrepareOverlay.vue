<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { prepareApi } from '@/api/prepare'

/**
 * First-run extraction overlay. Polls /api/prepare/status; only shows
 * itself if state ever was non-ready, so subsequent boots (with cached
 * ffmpeg) flash by invisibly. Mirrors legacy Prepare module behavior.
 */
const visible = ref(false)
const fading = ref(false)
const percent = ref(0)
const current = ref('')
const errorMsg = ref('')
const hint = ref('首次启动需要解压内嵌的 FFmpeg 组件,请稍候…')

async function poll() {
  while (true) {
    try {
      const p = await prepareApi.status()
      if (p.state === 'ready') {
        if (visible.value) {
          fading.value = true
          setTimeout(() => {
            visible.value = false
            fading.value = false
          }, 300)
        }
        return
      }
      if (p.state === 'error') {
        if (!visible.value) visible.value = true
        errorMsg.value = '解压失败:' + (p.error || '未知错误')
        return
      }
      if (!visible.value) visible.value = true
      percent.value = p.percent || 0
      current.value = p.current || ''
    } catch {
      await new Promise((r) => setTimeout(r, 500))
      continue
    }
    await new Promise((r) => setTimeout(r, 300))
  }
}

onMounted(poll)
</script>

<template>
  <Teleport to="body">
    <div
      v-if="visible"
      class="fixed inset-0 z-[100] flex items-center justify-center bg-black/80 transition-opacity duration-300"
      :class="fading ? 'opacity-0' : 'opacity-100'"
    >
      <div class="w-[420px] max-w-[90vw] rounded-md border border-border-base bg-bg-elevated p-5 shadow-xl">
        <div class="mb-3 text-sm font-medium">正在准备 FFmpeg</div>
        <div class="mb-3 text-xs text-fg-muted">
          {{ errorMsg || hint }}
        </div>
        <div class="h-2 overflow-hidden rounded bg-bg-base">
          <div
            class="h-full transition-[width] duration-300"
            :class="errorMsg ? 'bg-danger' : 'bg-accent'"
            :style="{ width: percent + '%' }"
          ></div>
        </div>
        <div class="mt-2 flex items-center justify-between text-xs text-fg-subtle">
          <span>{{ percent }}%</span>
          <span class="truncate">{{ current }}</span>
        </div>
      </div>
    </div>
  </Teleport>
</template>
