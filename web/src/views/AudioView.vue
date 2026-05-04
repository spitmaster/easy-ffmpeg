<script setup lang="ts">
import { computed, ref, useTemplateRef } from 'vue'
import AudioConvertMode from '@/components/audio/AudioConvertMode.vue'
import AudioExtractMode from '@/components/audio/AudioExtractMode.vue'
import AudioMergeMode from '@/components/audio/AudioMergeMode.vue'
import JobLog from '@/components/job/JobLog.vue'
import { audioApi, type AudioBody } from '@/api/audio'
import { useModalsStore } from '@/stores/modals'
import { useJobPanel } from '@/composables/useJobPanel'

type Mode = 'convert' | 'extract' | 'merge'

interface ModeAPI {
  readBody(): AudioBody
  validate(b: AudioBody): void
  getOutputPath(b: AudioBody): string
  buildPreview(): string
}

const modals = useModalsStore()

const activeMode = ref<Mode>('convert')
const tick = ref(0) // bumped on every mode child 'change' to refresh preview

const convertRef = useTemplateRef<ModeAPI>('convertRef')
const extractRef = useTemplateRef<ModeAPI>('extractRef')
const mergeRef = useTemplateRef<ModeAPI>('mergeRef')

function activeImpl(): ModeAPI | null {
  switch (activeMode.value) {
    case 'convert': return convertRef.value
    case 'extract': return extractRef.value
    case 'merge': return mergeRef.value
  }
  return null
}

const commandPreview = computed(() => {
  // depend on tick so this recomputes when child forms emit change
  void tick.value
  try {
    return activeImpl()?.buildPreview() || 'ffmpeg ...'
  } catch {
    return 'ffmpeg ...'
  }
})

function bumpPreview() {
  tick.value++
}

const job = useJobPanel({
  cancelUrl: '/api/audio/cancel',
  runningLabel: '处理中...',
  doneLabel: '✓ 处理完成',
  errorLabel: '✗ 处理失败',
  cancelledLabel: '! 已取消',
})

function switchMode(m: Mode) {
  if (job.running.value) return // legacy: forbid mode swap mid-job
  activeMode.value = m
  bumpPreview()
}

async function start() {
  const impl = activeImpl()
  if (!impl) return
  let body: AudioBody
  try {
    body = impl.readBody()
    impl.validate(body)
  } catch (e) {
    await modals.showConfirm({
      title: '提示',
      message: e instanceof Error ? e.message : String(e),
      okText: '我知道了',
      hideCancel: true,
    })
    return
  }
  const outputPath = impl.getOutputPath(body)

  let preview: string
  try {
    preview = await audioApi.preview(body)
  } catch (e) {
    await modals.showConfirm({
      title: '生成命令失败',
      message: e instanceof Error ? e.message : String(e),
      okText: '我知道了',
      hideCancel: true,
    })
    return
  }
  if (!(await modals.showCommand(preview))) return

  const sendStart = async (b: AudioBody): Promise<string> => {
    const { res, data } = await audioApi.start(b)
    if (res.status === 409 && data.existing) {
      const ok = await modals.showOverwrite(data.path || '')
      if (!ok) throw new Error('已取消覆盖')
      return sendStart({ ...b, overwrite: true })
    }
    if (!res.ok) {
      throw new Error(data.error || `HTTP ${res.status}`)
    }
    return data.command || ''
  }

  await job.startJob({ outputPath, request: () => sendStart(body) })
}
</script>

<template>
  <section class="mx-auto flex max-w-3xl flex-col gap-4 p-6">
    <!-- Mode segmented switch -->
    <div class="inline-flex self-start overflow-hidden rounded border border-border-strong text-xs">
      <button
        v-for="m in (['convert','extract','merge'] as Mode[])"
        :key="m"
        class="px-3 py-1.5 hover:bg-bg-elevated disabled:opacity-50"
        :class="activeMode === m ? 'bg-accent text-bg-base' : 'bg-bg-base'"
        :disabled="job.running.value"
        @click="switchMode(m)"
      >
        {{ m === 'convert' ? '格式转换' : m === 'extract' ? '从视频提取' : '音频合并' }}
      </button>
    </div>

    <AudioConvertMode v-show="activeMode === 'convert'" ref="convertRef" @change="bumpPreview" />
    <AudioExtractMode v-show="activeMode === 'extract'" ref="extractRef" @change="bumpPreview" />
    <AudioMergeMode v-show="activeMode === 'merge'" ref="mergeRef" @change="bumpPreview" />

    <div>
      <label class="mb-1 block text-xs text-fg-muted">命令预览</label>
      <pre
        class="overflow-auto rounded border border-border-base bg-bg-base p-2 font-mono text-xs leading-relaxed text-fg-base whitespace-pre-wrap break-all"
      >{{ commandPreview }}</pre>
    </div>

    <div class="flex items-center gap-2">
      <button
        class="rounded bg-accent px-4 py-1.5 text-xs text-bg-base hover:bg-accent-hover disabled:opacity-50"
        :disabled="job.running.value"
        @click="start"
      >开始处理</button>
      <button
        class="rounded border border-danger px-4 py-1.5 text-xs text-danger hover:bg-danger/10 disabled:opacity-40"
        :disabled="!job.running.value"
        @click="job.cancel"
      >取消</button>
      <span class="text-xs text-fg-muted">{{ job.stateLabel.value }}</span>
    </div>

    <JobLog
      label="处理日志"
      :log="job.log.value"
      :progress="job.progress.value"
      :progress-visible="job.progressVisible.value"
      :finish-visible="job.finishVisible.value"
      :finish-kind="job.finishKind.value"
      :finish-text="job.finishText.value"
      :has-output-path="!!job.lastOutputPath.value"
      @reveal="job.revealOutput"
    />
  </section>
</template>
