import { defineStore } from 'pinia'
import { computed, ref } from 'vue'
import { ffmpegApi, type FfmpegStatus } from '@/api/ffmpeg'

function parseVersion(s: string): string {
  if (!s) return ''
  // Mirror legacy parser: prefer dotted semver, fall back to first non-space token.
  const semver = s.match(/ffmpeg version (\d+(?:\.\d+)*)/i)
  if (semver) return semver[1]
  const any = s.match(/ffmpeg version (\S+)/i)
  return any ? any[1] : ''
}

export const useFfmpegStore = defineStore('ffmpeg', () => {
  const status = ref<FfmpegStatus | null>(null)
  const errored = ref(false)

  const label = computed(() => {
    if (errored.value) return '状态检测失败'
    if (!status.value) return '检测中...'
    if (!status.value.available) return 'FFmpeg 未安装'
    const v = parseVersion(status.value.version)
    const where = status.value.embedded ? '嵌入' : '系统'
    return v ? `FFmpeg ${v} · ${where}` : `FFmpeg 可用（${where}）`
  })

  const tone = computed<'ok' | 'err' | 'pending'>(() => {
    if (errored.value) return 'err'
    if (!status.value) return 'pending'
    return status.value.available ? 'ok' : 'err'
  })

  const clickable = computed(() => tone.value === 'ok')

  async function load() {
    try {
      status.value = await ffmpegApi.status()
      errored.value = false
    } catch {
      errored.value = true
      status.value = null
    }
  }

  async function reveal() {
    if (!clickable.value) return
    await ffmpegApi.reveal()
  }

  return { status, errored, label, tone, clickable, load, reveal }
})
