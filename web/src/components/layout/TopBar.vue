<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useVersionStore } from '@/stores/version'
import { useFfmpegStore } from '@/stores/ffmpeg'
import { quitApi } from '@/api/quit'

const version = useVersionStore()
const ffmpeg = useFfmpegStore()
const exited = ref(false)

onMounted(() => {
  version.load()
  ffmpeg.load()
})

async function onFfmpegClick() {
  if (!ffmpeg.clickable) return
  try {
    await ffmpeg.reveal()
  } catch (e) {
    alert('打开失败: ' + (e instanceof Error ? e.message : String(e)))
  }
}

async function onQuit() {
  // No native confirm() — Wails WebView2 silently suppresses browser
  // dialogs in production. Clicking 退出 is already explicit intent.
  await quitApi.quit()
  // The goodbye overlay visually covers everything via Teleport, but the
  // <audio>/<video> elements under it (e.g. the editor preview) stay
  // mounted and keep playing — so the user hears phantom audio after
  // "exit". Detach every media element on the page before showing it.
  // pause() alone isn't enough; we also clear src + call load() to
  // release the underlying decoder.
  document.querySelectorAll('audio, video').forEach((el) => {
    const m = el as HTMLMediaElement
    try {
      m.pause()
      m.removeAttribute('src')
      m.load()
    } catch {
      /* ignore — element may already be detached */
    }
  })
  exited.value = true
}
</script>

<template>
  <header
    class="flex h-12 shrink-0 items-center justify-between border-b border-border-base bg-bg-panel px-4"
  >
    <div class="flex items-center gap-2">
      <span class="text-xl">🎬</span>
      <span class="text-sm font-medium">Easy FFmpeg</span>
    </div>

    <div class="flex items-center gap-2 text-xs">
      <span
        v-if="version.version"
        class="rounded bg-bg-elevated px-2 py-1 font-mono text-fg-muted"
        title="程序版本"
      >
        v{{ version.version }}
      </span>

      <span
        class="rounded px-2 py-1 transition-colors"
        :class="{
          'bg-bg-elevated text-fg-muted': ffmpeg.tone === 'pending',
          'bg-success/15 text-success': ffmpeg.tone === 'ok',
          'bg-danger/15 text-danger': ffmpeg.tone === 'err',
          'cursor-pointer hover:bg-success/25': ffmpeg.clickable,
        }"
        :title="ffmpeg.clickable ? ffmpeg.status?.version + '\n\n点击打开 FFmpeg 所在文件夹' : ''"
        @click="onFfmpegClick"
      >
        {{ ffmpeg.label }}
      </span>

      <button
        class="rounded border border-border-strong px-3 py-1 text-fg-base hover:bg-bg-elevated"
        title="退出程序"
        @click="onQuit"
      >
        退出
      </button>
    </div>
  </header>

  <Teleport to="body">
    <div
      v-if="exited"
      class="fixed inset-0 z-[200] flex flex-col items-center justify-center gap-3 bg-bg-base text-fg-muted"
    >
      <div class="text-5xl">👋</div>
      <div class="text-sm">Easy FFmpeg 已退出,可关闭此页面。</div>
    </div>
  </Teleport>
</template>
