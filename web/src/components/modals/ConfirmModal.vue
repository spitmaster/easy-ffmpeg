<script setup lang="ts">
import { computed, onMounted, onUnmounted, watch, nextTick, ref } from 'vue'
import { useModalsStore } from '@/stores/modals'

/**
 * Generic confirm dialog. Triggered by `useModalsStore().showConfirm({...})`.
 * Replaces window.confirm() — Wails WebView2 silently suppresses native
 * dialogs and the OS look is inconsistent with the rest of the app. The OK
 * button can be styled as `danger` (red) for destructive actions like
 * deleting a track / source.
 */

const modals = useModalsStore()
const okBtn = ref<HTMLButtonElement | null>(null)

const visible = computed(() => modals.confirm !== null)
const title = computed(() => modals.confirm?.title ?? '确认')
const message = computed(() => modals.confirm?.message ?? '')
const detail = computed(() => modals.confirm?.detail ?? '')
const okText = computed(() => modals.confirm?.okText ?? '确认')
const cancelText = computed(() => modals.confirm?.cancelText ?? '取消')
const danger = computed(() => modals.confirm?.danger === true)
const hideCancel = computed(() => modals.confirm?.hideCancel === true)

function settle(v: boolean) {
  modals.settleConfirm(v)
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
      @click.self="settle(false)"
    >
      <div class="w-[460px] max-w-[90vw] rounded-md border border-border-base bg-bg-elevated shadow-xl">
        <div class="flex items-center justify-between border-b border-border-base px-4 py-3">
          <h3 class="text-sm font-medium">{{ title }}</h3>
          <button
            class="text-fg-muted hover:text-fg-base"
            aria-label="关闭"
            @click="settle(false)"
          >×</button>
        </div>
        <div class="px-4 py-4">
          <p class="whitespace-pre-line text-sm text-fg-base">{{ message }}</p>
          <div
            v-if="detail"
            class="mt-2 break-all rounded bg-bg-base px-2 py-1.5 font-mono text-xs text-fg-muted"
          >{{ detail }}</div>
        </div>
        <div class="flex justify-end gap-2 border-t border-border-base px-4 py-3">
          <button
            v-if="!hideCancel"
            class="rounded border border-border-strong px-3 py-1.5 text-xs hover:bg-bg-base"
            @click="settle(false)"
          >{{ cancelText }}</button>
          <button
            ref="okBtn"
            class="rounded px-3 py-1.5 text-xs"
            :class="danger
              ? 'bg-danger text-bg-base hover:bg-danger/90'
              : 'bg-accent text-bg-base hover:bg-accent-hover'"
            @click="settle(true)"
          >{{ okText }}</button>
        </div>
      </div>
    </div>
  </Teleport>
</template>
