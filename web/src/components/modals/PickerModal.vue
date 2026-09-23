<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import { useModalsStore } from '@/stores/modals'
import { fsApi, type FsEntry } from '@/api/fs'
import { Path } from '@/utils/path'
import { Fmt } from '@/utils/fmt'

const modals = useModalsStore()

const visible = computed(() => modals.picker !== null)
const mode = computed(() => modals.picker?.mode || 'dir')
const title = computed(() => modals.picker?.title || '选择')

const currentPath = ref('')
const drives = ref<string[]>([])
const entries = ref<FsEntry[]>([])
const selected = ref<FsEntry | null>(null)
const pathInput = ref('')
const hint = ref('')
const errorMsg = ref('')

async function loadPath(path: string) {
  errorMsg.value = ''
  try {
    const data = await fsApi.list(path)
    currentPath.value = data.path
    pathInput.value = data.path
    drives.value = data.drives || []
    entries.value = data.entries || []
    selected.value = null
  } catch (e) {
    errorMsg.value = '加载失败: ' + (e instanceof Error ? e.message : String(e))
  }
}

const driveValue = computed({
  get() {
    const cur = currentPath.value.toUpperCase()
    return drives.value.find((d) => cur.startsWith(d.toUpperCase())) || ''
  },
  set(v: string) {
    if (v) loadPath(v)
  },
})

function onEntryClick(e: FsEntry) {
  selected.value = e
}

function onEntryDblClick(e: FsEntry) {
  const full = Path.join(currentPath.value, e.name)
  if (e.isDir) {
    loadPath(full)
  } else if (mode.value === 'file') {
    settle(full)
  }
}

/**
 * Parent of a slash-separated path. Returns the input itself when already
 * at a root, so callers can detect "can't go higher" via `parent === p`.
 *
 *   "E:/foo/bar" → "E:/foo"
 *   "E:/foo"     → "E:/"        (NOT "E:" — backend would Stat the drive's
 *                                 cwd, which on Windows is process-dependent
 *                                 and produces nonsense paths like "E:g")
 *   "E:/"        → "E:/"        (drive root — stop)
 *   "/foo/bar"   → "/foo"
 *   "/foo"       → "/"
 *   "/"          → "/"
 */
function parentPath(p: string): string {
  if (!p) return ''
  const idx = p.lastIndexOf('/')
  if (idx < 0) return p
  if (idx === 0) return '/'
  // Windows drive root form "X:/..." — going up from "X:/foo" must keep
  // the slash, and "X:/" itself has no parent.
  if (idx === 2 && p[1] === ':') return p.slice(0, 3)
  return p.slice(0, idx)
}

function goUp() {
  const cur = currentPath.value
  if (!cur) return
  const parent = parentPath(cur)
  if (parent === cur) return
  loadPath(parent)
}

function onPathKey(e: KeyboardEvent) {
  if (e.key === 'Enter') loadPath(pathInput.value)
}

function confirm() {
  if (mode.value === 'dir') {
    settle(currentPath.value)
  } else if (selected.value && !selected.value.isDir) {
    settle(Path.join(currentPath.value, selected.value.name))
  } else {
    hint.value = '请先选中一个文件'
  }
}

function settle(v: string | null) {
  modals.settlePicker(v)
}

function onKeydown(e: KeyboardEvent) {
  if (!visible.value) return
  if (e.key === 'Escape') {
    e.preventDefault()
    settle(null)
  }
}

/**
 * Walk a missing startPath up to the first ancestor that actually exists.
 * The saved "last input directory" survives across sessions, so by the
 * time we reopen the picker the user may have moved/deleted that folder
 * — without a fallback, the modal would render only an error and the
 * user couldn't even switch drives. Last resorts: home, then empty
 * (backend defaults to home).
 */
async function loadStartPath(start: string) {
  if (!start) {
    await loadPath('')
    return
  }
  let cur = start
  for (let i = 0; i < 16; i++) {
    await loadPath(cur)
    if (!errorMsg.value) return
    const next = parentPath(cur)
    if (next === cur) break
    cur = next
  }
  try {
    const h = (await fsApi.home()).home
    await loadPath(h)
    if (!errorMsg.value) return
  } catch {
    /* fall through */
  }
  await loadPath('')
}

watch(
  () => modals.picker,
  async (req) => {
    if (!req) return
    selected.value = null
    hint.value = req.mode === 'file' ? '选中一个文件后点击确认' : ''
    await loadStartPath(req.startPath || '')
  },
)

onMounted(() => document.addEventListener('keydown', onKeydown))
onUnmounted(() => document.removeEventListener('keydown', onKeydown))
</script>

<template>
  <Teleport to="body">
    <div
      v-if="visible"
      class="fixed inset-0 z-50 flex items-center justify-center bg-black/60 backdrop-blur-sm"
    >
      <div
        class="flex h-[560px] w-[720px] max-w-[95vw] flex-col rounded-md border border-border-base bg-bg-elevated shadow-xl"
      >
        <div class="flex items-center justify-between border-b border-border-base px-4 py-3">
          <h3 class="text-sm font-medium">{{ title }}</h3>
          <button class="text-fg-muted hover:text-fg-base" aria-label="关闭" @click="settle(null)">
            ×
          </button>
        </div>

        <div class="flex items-center gap-2 border-b border-border-base px-4 py-2">
          <select
            v-if="drives.length"
            v-model="driveValue"
            class="rounded border border-border-strong bg-bg-base px-2 py-1 text-xs"
          >
            <option v-for="d in drives" :key="d" :value="d">{{ d }}</option>
          </select>
          <input
            v-model="pathInput"
            type="text"
            spellcheck="false"
            class="flex-1 rounded border border-border-strong bg-bg-base px-2 py-1 font-mono text-xs"
            @keydown="onPathKey"
          />
          <button
            class="rounded border border-border-strong px-2 py-1 text-xs hover:bg-bg-base"
            title="上一级"
            @click="goUp"
          >
            ↑
          </button>
        </div>

        <div class="flex-1 overflow-auto">
          <div v-if="errorMsg" class="px-4 py-3 text-xs text-danger">{{ errorMsg }}</div>
          <ul v-else-if="entries.length" class="divide-y divide-border-base/50">
            <li
              v-for="e in entries"
              :key="e.name"
              class="flex cursor-pointer items-center gap-3 px-4 py-1.5 text-sm hover:bg-bg-panel"
              :class="selected === e ? 'bg-bg-panel' : ''"
              @click="onEntryClick(e)"
              @dblclick="onEntryDblClick(e)"
            >
              <span class="w-5 text-center">{{ e.isDir ? '📁' : '📄' }}</span>
              <span class="flex-1 truncate">{{ e.name }}</span>
              <span v-if="!e.isDir" class="text-xs text-fg-subtle">{{ Fmt.human(e.size) }}</span>
            </li>
          </ul>
          <div v-else class="px-4 py-3 text-xs text-fg-subtle">空目录</div>
        </div>

        <div class="flex items-center gap-2 border-t border-border-base px-4 py-3">
          <span class="text-xs text-fg-subtle">{{ hint }}</span>
          <div class="flex-1"></div>
          <button
            class="rounded border border-border-strong px-3 py-1.5 text-xs hover:bg-bg-base"
            @click="settle(null)"
          >
            取消
          </button>
          <button
            class="rounded bg-accent px-3 py-1.5 text-xs text-bg-base hover:bg-accent-hover"
            @click="confirm"
          >
            {{ mode === 'dir' ? '选择此目录' : '选择文件' }}
          </button>
        </div>
      </div>
    </div>
  </Teleport>
</template>
