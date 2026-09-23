<script setup lang="ts">
import { computed, onMounted, onUnmounted, watch, nextTick, ref } from 'vue'
import { useModalsStore } from '@/stores/modals'

const modals = useModalsStore()
const okBtn = ref<HTMLButtonElement | null>(null)

const visible = computed(() => modals.overwrite !== null)
const path = computed(() => modals.overwrite?.path || '')

function settle(v: boolean) {
  modals.settleOverwrite(v)
}

function onKeydown(e: KeyboardEvent) {
  if (!visible.value) return
  if (e.key === 'Escape') {
    e.preventDefault()
    settle(false)
  } else if (e.key === 'Enter') {
    e.preventDefault()
    settle(true)
  }
}

watch(visible, async (v) => {
  if (v) {
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
      <div class="w-[480px] max-w-[90vw] rounded-md border border-border-base bg-bg-elevated shadow-xl">
        <div class="flex items-center justify-between border-b border-border-base px-4 py-3">
          <h3 class="text-sm font-medium">目标文件已存在</h3>
          <button
            class="text-fg-muted hover:text-fg-base"
            aria-label="关闭"
            @click="settle(false)"
          >
            ×
          </button>
        </div>
        <div class="px-4 py-4">
          <p class="text-sm text-fg-base">是否覆盖现有文件？</p>
          <div class="mt-2 break-all rounded bg-bg-base px-2 py-1.5 font-mono text-xs text-fg-muted">
            {{ path }}
          </div>
        </div>
        <div class="flex justify-end gap-2 border-t border-border-base px-4 py-3">
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
            覆盖
          </button>
        </div>
      </div>
    </div>
  </Teleport>
</template>
