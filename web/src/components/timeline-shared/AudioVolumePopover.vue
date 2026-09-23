<script setup lang="ts">
import { computed, onBeforeUnmount, ref } from 'vue'

/**
 * 0–200% audio volume slider in a teleported popover. Pure presentation —
 * the parent owns the volume value via v-model.
 */
const props = defineProps<{
  /** Current volume; 0–2.0 typical (200% cap). */
  modelValue: number
}>()

const emit = defineEmits<{
  (e: 'update:modelValue', v: number): void
}>()

const open = ref(false)
const btnRef = ref<HTMLButtonElement | null>(null)
const popoverPos = ref<{ left: number; top: number }>({ left: 0, top: 0 })

const volume = computed({
  get: () => props.modelValue,
  set: (v: number) => emit('update:modelValue', v),
})

const pct = computed(() => Math.round(volume.value * 100) + '%')

function toggleOpen() {
  if (open.value) close()
  else openIt()
}

function openIt() {
  open.value = true
  requestAnimationFrame(() => {
    const btn = btnRef.value
    if (!btn) return
    const rect = btn.getBoundingClientRect()
    const POP_W = 96
    const POP_H = 200
    let left = rect.left
    if (left + POP_W > window.innerWidth - 8) left = window.innerWidth - POP_W - 8
    if (left < 8) left = 8
    const fitsBelow = rect.bottom + 6 + POP_H <= window.innerHeight - 8
    const top = fitsBelow ? rect.bottom + 6 : Math.max(8, rect.top - POP_H - 6)
    popoverPos.value = { left, top }
  })
  document.addEventListener('mousedown', onOutsideClick)
}

function close() {
  open.value = false
  document.removeEventListener('mousedown', onOutsideClick)
}

function onOutsideClick(ev: MouseEvent) {
  const target = ev.target as Node | null
  if (!target) return
  if (btnRef.value?.contains(target)) return
  close()
}

onBeforeUnmount(() => {
  document.removeEventListener('mousedown', onOutsideClick)
})

defineExpose({ close })
</script>

<template>
  <button
    ref="btnRef"
    type="button"
    class="rounded border border-border-strong bg-bg-base px-2 py-1 text-[10px] text-fg-base hover:bg-bg-elevated"
    :class="{ 'border-accent': open }"
    title="音频音量（点击调节）"
    aria-haspopup="true"
    :aria-expanded="open"
    @click.stop="toggleOpen"
  >音量: {{ pct }}</button>

  <Teleport to="body">
    <div
      v-if="open"
      class="fixed z-50 w-24 rounded border border-border-strong bg-bg-elevated p-3 shadow-lg"
      :style="{ left: popoverPos.left + 'px', top: popoverPos.top + 'px' }"
      @mousedown.stop
    >
      <div class="mb-2 text-center text-[10px] text-fg-muted">音频音量</div>
      <div class="flex items-stretch gap-2">
        <div class="flex flex-col justify-between text-[10px] text-fg-muted">
          <span>200%</span>
          <span>100%</span>
          <span>0%</span>
        </div>
        <input
          v-model.number="volume"
          type="range"
          min="0"
          max="2"
          step="0.01"
          class="h-32 w-4"
          style="writing-mode: vertical-lr; direction: rtl"
          aria-label="音频音量（拖动调节，0–200%）"
        />
      </div>
      <div class="mt-2 text-center text-xs text-accent">{{ pct }}</div>
    </div>
  </Teleport>
</template>
