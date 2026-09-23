<script setup lang="ts">
import { computed, onMounted, onUnmounted, watch, nextTick, ref } from 'vue'
import { useModalsStore } from '@/stores/modals'

const modals = useModalsStore()
const okBtn = ref<HTMLButtonElement | null>(null)
const preEl = ref<HTMLPreElement | null>(null)
const hint = ref('点击命令框可复制')
const copied = ref(false)

const visible = computed(() => modals.command !== null)
const cmd = computed(() => modals.command?.command || '')

function settle(v: boolean) {
  modals.settleCommand(v)
}

async function copyCommand() {
  const text = cmd.value
  let ok = false
  try {
    if (navigator.clipboard?.writeText) {
      await navigator.clipboard.writeText(text)
      ok = true
    }
  } catch {
    /* fallthrough */
  }
  if (!ok) {
    // Fallback for older runtimes / non-secure contexts (legacy parity).
    try {
      const ta = document.createElement('textarea')
      ta.value = text
      ta.style.position = 'fixed'
      ta.style.left = '-9999px'
      document.body.appendChild(ta)
      ta.select()
      document.execCommand('copy')
      document.body.removeChild(ta)
      ok = true
    } catch {
      /* clipboard not available */
    }
  }
  hint.value = ok ? '✓ 已复制' : '✗ 复制失败（请手动选择）'
  copied.value = ok
}

function onKeydown(e: KeyboardEvent) {
  if (!visible.value) return
  if (e.key === 'Escape') {
    e.preventDefault()
    settle(false)
  } else if (e.key === 'Enter' && e.target !== preEl.value) {
    // Enter on the <pre> would otherwise hijack the OK button — match
    // legacy behavior of letting Enter inside the command focused area
    // do nothing special.
    e.preventDefault()
    settle(true)
  }
}

watch(visible, async (v) => {
  if (v) {
    hint.value = '点击命令框可复制'
    copied.value = false
    await nextTick()
    okBtn.value?.focus()
  }
})

onMounted(() => document.addEventListener('keydown', onKeydown))
onUnmounted(() => document.removeEventListener('keydown', onKeydown))
</script>

<template>
  <Teleport to="body">
    <div
      v-if="visible"
      class="fixed inset-0 z-50 flex items-center justify-center bg-black/60 backdrop-blur-sm"
    >
      <div class="w-[640px] max-w-[92vw] rounded-md border border-border-base bg-bg-elevated shadow-xl">
        <div class="flex items-center justify-between border-b border-border-base px-4 py-3">
          <h3 class="text-sm font-medium">即将执行</h3>
          <button class="text-fg-muted hover:text-fg-base" aria-label="关闭" @click="settle(false)">
            ×
          </button>
        </div>
        <div class="px-4 py-4">
          <p class="text-sm text-fg-base">下列 ffmpeg 命令将被执行,确认后开始:</p>
          <pre
            ref="preEl"
            tabindex="0"
            class="mt-2 cursor-pointer overflow-auto rounded bg-bg-base p-3 font-mono text-xs leading-relaxed text-fg-base whitespace-pre-wrap break-all hover:border-border-strong"
            title="点击复制"
            @click="copyCommand"
          >{{ cmd }}</pre>
          <div class="mt-1 text-xs" :class="copied ? 'text-success' : 'text-fg-subtle'">
            {{ hint }}
          </div>
        </div>
        <div class="flex items-center gap-2 border-t border-border-base px-4 py-3">
          <button
            class="rounded border border-border-strong px-3 py-1.5 text-xs hover:bg-bg-base"
            @click="copyCommand"
          >
            📋 复制
          </button>
          <div class="flex-1"></div>
          <button
            class="rounded border border-border-strong px-3 py-1.5 text-xs hover:bg-bg-base"
            @click="settle(false)"
          >
            取消
          </button>
          <button
            ref="okBtn"
            class="rounded bg-accent px-3 py-1.5 text-xs text-bg-base hover:bg-accent-hover"
            @click="settle(true)"
          >
            开始执行
          </button>
        </div>
      </div>
    </div>
  </Teleport>
</template>
