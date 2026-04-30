<script setup lang="ts">
import { ref, watch } from 'vue'
import type { ProjectsModalItem } from '@/types/timeline'

/**
 * Project picker modal. Parameterized over data source so single-video
 * (editorApi.listProjects) and multitrack (multitrackApi.listProjects)
 * share the same UI shell. Caller owns the side effects (fetch, delete);
 * this component just renders rows + emits load/close.
 */
const props = defineProps<{
  open: boolean
  title?: string
  /** Async lister called whenever `open` flips to true. */
  list: () => Promise<ProjectsModalItem[]>
  /**
   * Async deleter; the component handles confirm + optimistic removal.
   * Returns a Promise so the component can revert on failure if needed.
   */
  remove: (id: string) => Promise<void>
  /** Confirmation prompt builder; defaults to "删除工程 \"<name>\"？". */
  confirmDelete?: (item: ProjectsModalItem) => string
  /** Empty-state message; default: "暂无工程". */
  emptyText?: string
}>()

const emit = defineEmits<{
  (e: 'close'): void
  (e: 'load', id: string): void
}>()

const items = ref<ProjectsModalItem[]>([])
const loading = ref(false)
const error = ref('')

watch(
  () => props.open,
  async (v) => {
    if (!v) return
    loading.value = true
    error.value = ''
    try {
      items.value = (await props.list()) || []
    } catch (e) {
      error.value = e instanceof Error ? e.message : String(e)
    } finally {
      loading.value = false
    }
  },
)

function fmtDate(s: string): string {
  if (!s) return ''
  return s.replace('T', ' ').slice(0, 16)
}

async function onDelete(p: ProjectsModalItem) {
  const msg = props.confirmDelete ? props.confirmDelete(p) : `删除工程 "${p.name}"？`
  if (!confirm(msg)) return
  try {
    await props.remove(p.id)
    items.value = items.value.filter((x) => x.id !== p.id)
  } catch (e) {
    alert('删除失败: ' + (e instanceof Error ? e.message : String(e)))
  }
}

function onLoad(p: ProjectsModalItem) {
  emit('load', p.id)
  emit('close')
}
</script>

<template>
  <div
    v-if="open"
    class="fixed inset-0 z-40 flex items-center justify-center bg-black/60"
  >
    <div class="flex max-h-[80vh] w-[640px] flex-col rounded border border-border-strong bg-bg-panel">
      <div class="flex items-center justify-between border-b border-border-base px-4 py-2">
        <h3 class="text-sm font-medium">{{ title ?? '工程列表' }}</h3>
        <button class="text-fg-muted hover:text-fg-base" @click="emit('close')">×</button>
      </div>
      <div class="flex-1 overflow-y-auto p-3">
        <div v-if="loading" class="text-center text-xs text-fg-muted">加载中...</div>
        <div v-else-if="error" class="text-xs text-danger">{{ error }}</div>
        <div v-else-if="!items.length" class="text-center text-xs text-fg-muted">{{ emptyText ?? '暂无工程' }}</div>
        <ul v-else class="flex flex-col gap-1 text-xs">
          <li
            v-for="p in items"
            :key="p.id"
            class="flex items-center gap-2 rounded border border-border-base bg-bg-base px-3 py-2 hover:bg-bg-elevated"
          >
            <div class="flex-1 overflow-hidden">
              <div class="truncate text-fg-base">{{ p.name || '(未命名)' }}</div>
              <div class="truncate text-[10px] text-fg-muted">
                <template v-if="p.detail">{{ p.detail }} · </template>更新于 {{ fmtDate(p.updatedAt) }}
              </div>
            </div>
            <button
              class="rounded border border-border-strong px-2 py-1 hover:bg-bg-elevated"
              @click="onLoad(p)"
            >打开</button>
            <button
              class="rounded border border-danger px-2 py-1 text-danger hover:bg-danger/10"
              @click="onDelete(p)"
            >🗑</button>
          </li>
        </ul>
      </div>
    </div>
  </div>
</template>
