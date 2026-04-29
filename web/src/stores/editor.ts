import { defineStore } from 'pinia'
import { computed, ref, watch } from 'vue'
import { editorApi, type Clip, type Project, type Track } from '@/api/editor'
import { totalDuration } from '@/utils/timeline'

export interface ClipSelection {
  track: Track
  clipId: string
}

export interface RangeSelection {
  start: number
  end: number
}

export type SplitScope = 'both' | Track

export interface ClipsSnapshot {
  videoClips: Clip[]
  audioClips: Clip[]
}

const HISTORY_MAX = 100
const SAVE_DEBOUNCE_MS = 1500

/**
 * Editor central store. Holds the project + transient editing state
 * (selection, playhead, zoom, range selection, split scope). A debounced
 * auto-save flushes changes back to the backend; an undo/redo stack of
 * (videoClips, audioClips) snapshots is layered on top.
 */
export const useEditorStore = defineStore('editor', () => {
  const project = ref<Project | null>(null)
  const dirty = ref(false)
  const selection = ref<ClipSelection[]>([])
  const splitScope = ref<SplitScope>('both')
  const playhead = ref(0)
  const playing = ref(false)
  const pxPerSecond = ref(8)
  const rangeSelection = ref<RangeSelection | null>(null)

  // History: array of snapshots; cursor points at the snapshot that
  // matches the current state. Push truncates redos beyond cursor.
  const history = ref<ClipsSnapshot[]>([])
  const historyCursor = ref(-1)
  const canUndo = computed(() => historyCursor.value > 0)
  const canRedo = computed(() => historyCursor.value < history.value.length - 1)

  // Debounced auto-save
  let saveTimer: ReturnType<typeof setTimeout> | null = null
  function scheduleSave() {
    if (saveTimer) clearTimeout(saveTimer)
    saveTimer = setTimeout(flushSave, SAVE_DEBOUNCE_MS)
  }
  async function flushSave() {
    if (saveTimer) {
      clearTimeout(saveTimer)
      saveTimer = null
    }
    if (!project.value || !dirty.value) return
    try {
      const saved = await editorApi.saveProject(project.value)
      project.value = saved
      dirty.value = false
    } catch (e) {
      console.error('editor: save failed', e)
    }
  }

  function applyProjectPatch(patch: Partial<Project>, opts?: { save?: boolean }) {
    if (!project.value) return
    project.value = { ...project.value, ...patch }
    dirty.value = true
    if (!opts || opts.save !== false) scheduleSave()
  }

  function snapshot(p: Project): ClipsSnapshot {
    return {
      videoClips: (p.videoClips || []).map((c) => ({ ...c })),
      audioClips: (p.audioClips || []).map((c) => ({ ...c })),
    }
  }

  function pushHistory() {
    if (!project.value) return
    const next = history.value.slice(0, historyCursor.value + 1)
    next.push(snapshot(project.value))
    if (next.length > HISTORY_MAX) next.splice(0, next.length - HISTORY_MAX)
    history.value = next
    historyCursor.value = next.length - 1
  }

  function resetHistory(p: Project) {
    history.value = [snapshot(p)]
    historyCursor.value = 0
  }

  function applySnapshot(s: ClipsSnapshot) {
    applyProjectPatch({
      videoClips: s.videoClips.map((c) => ({ ...c })),
      audioClips: s.audioClips.map((c) => ({ ...c })),
    })
  }

  function undo() {
    if (!canUndo.value) return
    historyCursor.value--
    applySnapshot(history.value[historyCursor.value])
  }

  function redo() {
    if (!canRedo.value) return
    historyCursor.value++
    applySnapshot(history.value[historyCursor.value])
  }

  function loadProject(p: Project) {
    project.value = p
    selection.value = []
    splitScope.value = 'both'
    playhead.value = 0
    playing.value = false
    rangeSelection.value = null
    dirty.value = false
    resetHistory(p)
  }

  function clampPlayhead(t: number): number {
    const total = totalDuration(project.value)
    return Math.max(0, Math.min(t, total))
  }

  function setPlayhead(t: number) {
    playhead.value = clampPlayhead(t)
  }

  // Auto-clamp the playhead when total duration shrinks (after delete).
  watch(
    () => totalDuration(project.value),
    (total) => {
      if (playhead.value > total) playhead.value = total
    },
  )

  return {
    // state
    project,
    dirty,
    selection,
    splitScope,
    playhead,
    playing,
    pxPerSecond,
    rangeSelection,
    history,
    historyCursor,
    canUndo,
    canRedo,
    // actions
    applyProjectPatch,
    pushHistory,
    resetHistory,
    undo,
    redo,
    loadProject,
    setPlayhead,
    flushSave,
    scheduleSave,
  }
})

// ---- Selection helpers (pure; not stored on the store) ----

export const Sel = {
  has(sel: ClipSelection[], track: Track, clipId: string): boolean {
    return sel.some((s) => s.track === track && s.clipId === clipId)
  },
  toggle(sel: ClipSelection[], track: Track, clipId: string): ClipSelection[] {
    const i = sel.findIndex((s) => s.track === track && s.clipId === clipId)
    if (i >= 0) {
      const out = sel.slice()
      out.splice(i, 1)
      return out
    }
    return sel.concat([{ track, clipId }])
  },
  replace(track: Track, clipId: string): ClipSelection[] {
    return [{ track, clipId }]
  },
  inTrack(sel: ClipSelection[], track: Track): string[] {
    return sel.filter((s) => s.track === track).map((s) => s.clipId)
  },
}
