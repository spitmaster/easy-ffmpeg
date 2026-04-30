import { defineStore } from 'pinia'
import { ref, watch } from 'vue'
import { editorApi, type Clip, type Project, type Track } from '@/api/editor'
import { useAutosave } from '@/composables/timeline/useAutosave'
import { useUndoStack } from '@/composables/timeline/useUndoStack'
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

/**
 * Editor central store. Holds the project + transient editing state
 * (selection, playhead, zoom, range selection, split scope). The undo/redo
 * stack and the debounced auto-save are delegated to shared composables in
 * composables/timeline/ so the multitrack store can layer the same machinery
 * over its own snapshot/save shape.
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

  function snapshotClips(p: Project): ClipsSnapshot {
    return {
      videoClips: (p.videoClips || []).map((c) => ({ ...c })),
      audioClips: (p.audioClips || []).map((c) => ({ ...c })),
    }
  }

  const undoStack = useUndoStack<ClipsSnapshot>({
    snapshot: () => snapshotClips(project.value!),
    apply: (s) => {
      applyProjectPatch({
        videoClips: s.videoClips.map((c) => ({ ...c })),
        audioClips: s.audioClips.map((c) => ({ ...c })),
      })
    },
  })

  const autosave = useAutosave({
    isDirty: () => dirty.value,
    save: async () => {
      if (!project.value) return
      const saved = await editorApi.saveProject(project.value)
      project.value = saved
      dirty.value = false
    },
    onError: (e) => console.error('editor: save failed', e),
  })

  function applyProjectPatch(patch: Partial<Project>, opts?: { save?: boolean }) {
    if (!project.value) return
    project.value = { ...project.value, ...patch }
    dirty.value = true
    if (!opts || opts.save !== false) autosave.schedule()
  }

  function loadProject(p: Project) {
    project.value = p
    selection.value = []
    splitScope.value = 'both'
    playhead.value = 0
    playing.value = false
    rangeSelection.value = null
    dirty.value = false
    undoStack.reset(snapshotClips(p))
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
    canUndo: undoStack.canUndo,
    canRedo: undoStack.canRedo,
    // actions
    applyProjectPatch,
    pushHistory: undoStack.push,
    undo: undoStack.undo,
    redo: undoStack.redo,
    loadProject,
    setPlayhead,
    flushSave: autosave.flush,
    scheduleSave: autosave.schedule,
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
