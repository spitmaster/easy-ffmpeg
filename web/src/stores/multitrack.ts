import { defineStore } from 'pinia'
import { computed, ref, watch } from 'vue'
import {
  multitrackApi,
  type MultitrackAudioTrack,
  type MultitrackClip,
  type MultitrackImportError,
  type MultitrackProject,
  type MultitrackProjectSummary,
  type MultitrackSource,
  type MultitrackVideoTrack,
} from '@/api/multitrack'
import type { RangeSelection } from '@/types/timeline'
import { useAutosave } from '@/composables/timeline/useAutosave'
import { useUndoStack } from '@/composables/timeline/useUndoStack'

/**
 * Multitrack store. Holds the open project, the project list (lazy for
 * the picker modal), preview state (playhead / playing), timeline
 * presentation (pxPerSecond), and editing state (selection, split scope,
 * range selection, library-collapsed flag).
 *
 * The shape mirrors stores/editor.ts so the shared composables (useUndoStack,
 * useTimelineDrag, useTimelineRangeSelect, useTimelinePlayback) can drive
 * either store interchangeably.
 */

export interface MultitrackTopVideoActive {
  track: MultitrackVideoTrack
  clip: MultitrackClip
  source: MultitrackSource
  /** Time inside the source corresponding to playhead. */
  srcTime: number
}

/** Per-track-and-clip selection — track id (not kind) so two tracks of the
 * same kind don't collide. */
export interface MultitrackClipSelection {
  trackId: string
  clipId: string
}

/**
 * Split scope semantics:
 *   'all'         every track
 *   'video'       all video tracks
 *   'audio'       all audio tracks
 *   { kind, id }  one specific track
 */
export type MultitrackSplitScope =
  | 'all'
  | 'video'
  | 'audio'
  | { kind: 'track'; id: string }

/** Snapshot of the editable timeline state for the undo stack. Sources +
 * AudioVolume are intentionally excluded — undo only retracts edits, not
 * library or mix changes (matches editor.ts ClipsSnapshot). */
export interface MultitrackSnapshot {
  videoTracks: MultitrackVideoTrack[]
  audioTracks: MultitrackAudioTrack[]
}

export const useMultitrackStore = defineStore('multitrack', () => {
  const project = ref<MultitrackProject | null>(null)
  const list = ref<MultitrackProjectSummary[]>([])
  const dirty = ref(false)
  const loading = ref(false)
  const error = ref('')

  const playhead = ref(0)
  const playing = ref(false)
  const pxPerSecond = ref(8)

  // M7 editing state.
  const selection = ref<MultitrackClipSelection[]>([])
  const splitScope = ref<MultitrackSplitScope>('all')
  const rangeSelection = ref<RangeSelection | null>(null)
  const libraryCollapsed = ref(false)

  /**
   * True while a multitrack export is running. Drives the timeline lock:
   * playback toggles, key shortcuts (Space / S / Delete / Ctrl+Z / …),
   * clip drag, and drop accept paths all gate on this. The view flips it
   * around the exportSubmit / closeExportSidebar lifecycle. Resets to
   * false on every loadProject / closeProject so a stale lock can never
   * persist across project switches.
   */
  const exportLocked = ref(false)

  function snapshotTracks(p: MultitrackProject): MultitrackSnapshot {
    return {
      videoTracks: p.videoTracks.map((t) => ({ ...t, clips: t.clips.map((c) => ({ ...c })) })),
      audioTracks: p.audioTracks.map((t) => ({ ...t, clips: t.clips.map((c) => ({ ...c })) })),
    }
  }

  const undoStack = useUndoStack<MultitrackSnapshot>({
    snapshot: () => snapshotTracks(project.value!),
    apply: (s) => {
      applyProjectPatch({
        videoTracks: s.videoTracks.map((t) => ({ ...t, clips: t.clips.map((c) => ({ ...c })) })),
        audioTracks: s.audioTracks.map((t) => ({ ...t, clips: t.clips.map((c) => ({ ...c })) })),
      })
    },
  })

  const autosave = useAutosave({
    isDirty: () => dirty.value,
    save: async () => {
      if (!project.value) return
      const saved = await multitrackApi.saveProject(project.value)
      project.value = saved
      dirty.value = false
    },
    onError: (e) => console.error('multitrack: save failed', e),
  })

  function applyProjectPatch(patch: Partial<MultitrackProject>, opts?: { save?: boolean }) {
    if (!project.value) return
    project.value = { ...project.value, ...patch }
    dirty.value = true
    if (!opts || opts.save !== false) autosave.schedule()
  }

  // Replace the whole project (used after import / track add / autosave).
  function replaceProject(p: MultitrackProject, markDirty = false) {
    project.value = p
    dirty.value = markDirty
    if (markDirty) autosave.schedule()
  }

  /** Single entry point that any open/create/openProject path can use to
   * reset the transient editing state and the undo stack. Mirrors
   * editor.ts loadProject. */
  function loadProject(p: MultitrackProject) {
    project.value = p
    selection.value = []
    splitScope.value = 'all'
    rangeSelection.value = null
    playhead.value = 0
    playing.value = false
    libraryCollapsed.value = false
    exportLocked.value = false
    dirty.value = false
    undoStack.reset(snapshotTracks(p))
  }

  async function fetchList(): Promise<MultitrackProjectSummary[]> {
    loading.value = true
    error.value = ''
    try {
      list.value = (await multitrackApi.listProjects()) || []
      return list.value
    } catch (e) {
      error.value = e instanceof Error ? e.message : String(e)
      throw e
    } finally {
      loading.value = false
    }
  }

  async function createNew(name?: string): Promise<MultitrackProject> {
    const trimmed = (name ?? '').trim() || undefined
    const created = await multitrackApi.createProject(trimmed)
    loadProject(created)
    list.value = [
      { id: created.id, name: created.name, sourceCount: 0, createdAt: created.createdAt, updatedAt: created.updatedAt },
      ...list.value.filter((p) => p.id !== created.id),
    ]
    return created
  }

  async function openProject(id: string): Promise<MultitrackProject> {
    const p = await multitrackApi.getProject(id)
    loadProject(p)
    return p
  }

  async function deleteProject(id: string): Promise<void> {
    await multitrackApi.deleteProject(id)
    list.value = list.value.filter((p) => p.id !== id)
    if (project.value?.id === id) {
      project.value = null
      selection.value = []
      rangeSelection.value = null
      playhead.value = 0
      playing.value = false
      dirty.value = false
    }
  }

  function closeProject() {
    project.value = null
    selection.value = []
    rangeSelection.value = null
    splitScope.value = 'all'
    dirty.value = false
    playhead.value = 0
    playing.value = false
    libraryCollapsed.value = false
    exportLocked.value = false
    autosave.cancel()
  }

  /**
   * Probe + add the given file paths as sources. The backend persists the
   * mutation server-side and returns the updated project; we replace
   * local state in one shot. History snapshot is pushed so the user can
   * undo a wrong import.
   */
  async function importSources(
    paths: string[],
  ): Promise<{ added: MultitrackSource[]; errors: MultitrackImportError[] }> {
    if (!project.value) throw new Error('no project open')
    const resp = await multitrackApi.importSources(project.value.id, paths)
    project.value = resp.project
    dirty.value = false
    return { added: resp.sources, errors: resp.errors ?? [] }
  }

  async function removeSource(sourceId: string): Promise<void> {
    if (!project.value) throw new Error('no project open')
    const updated = await multitrackApi.removeSource(project.value.id, sourceId)
    project.value = updated
    dirty.value = false
  }

  /**
   * Append an empty video track. Returns the new track id so callers
   * (e.g. the drop handler) can immediately drop a clip into it.
   */
  function addVideoTrack(): string {
    if (!project.value) throw new Error('no project open')
    const id = nextTrackId('v', project.value.videoTracks.map((t) => t.id))
    const track: MultitrackVideoTrack = { id, clips: [] }
    applyProjectPatch({ videoTracks: [...project.value.videoTracks, track] })
    undoStack.push()
    return id
  }

  function addAudioTrack(): string {
    if (!project.value) throw new Error('no project open')
    const id = nextTrackId('a', project.value.audioTracks.map((t) => t.id))
    const track: MultitrackAudioTrack = { id, volume: 1, clips: [] }
    applyProjectPatch({ audioTracks: [...project.value.audioTracks, track] })
    undoStack.push()
    return id
  }

  /**
   * Remove a video track and all its clips. Sources referenced by removed
   * clips are not auto-removed — the user purges them via the library
   * "remove source" button (which does its own ref-count check).
   */
  function removeVideoTrack(id: string) {
    if (!project.value) return
    const next = project.value.videoTracks.filter((t) => t.id !== id)
    if (next.length === project.value.videoTracks.length) return
    applyProjectPatch({ videoTracks: next })
    selection.value = selection.value.filter((s) => s.trackId !== id)
    if (typeof splitScope.value === 'object' && splitScope.value.id === id) {
      splitScope.value = 'all'
    }
    undoStack.push()
  }

  function removeAudioTrack(id: string) {
    if (!project.value) return
    const next = project.value.audioTracks.filter((t) => t.id !== id)
    if (next.length === project.value.audioTracks.length) return
    applyProjectPatch({ audioTracks: next })
    selection.value = selection.value.filter((s) => s.trackId !== id)
    if (typeof splitScope.value === 'object' && splitScope.value.id === id) {
      splitScope.value = 'all'
    }
    undoStack.push()
  }

  /**
   * Move a clip from one track to another, both of the given kind. Cross-
   * kind moves (video → audio or vice versa) are silently rejected — the
   * UI prevents them via dropEffect, but the action is double-checked
   * here so a stale drop can never corrupt the model.
   */
  function moveClipAcrossTracks(
    kind: 'video' | 'audio',
    fromTrackId: string,
    toTrackId: string,
    clipId: string,
    newProgramStart: number,
  ) {
    if (!project.value) return
    const clamped = Math.max(0, newProgramStart)
    if (kind === 'video') {
      const tracks = project.value.videoTracks
      const fromIdx = tracks.findIndex((t) => t.id === fromTrackId)
      const toIdx = tracks.findIndex((t) => t.id === toTrackId)
      if (fromIdx < 0 || toIdx < 0) return
      const from = tracks[fromIdx]
      const clipIdx = from.clips.findIndex((c) => c.id === clipId)
      if (clipIdx < 0) return
      const moving: MultitrackClip = { ...from.clips[clipIdx], programStart: clamped }
      const next = tracks.slice()
      const newFromClips = from.clips.slice()
      newFromClips.splice(clipIdx, 1)
      next[fromIdx] = { ...from, clips: newFromClips }
      // Re-read dest from the updated array so same-track moves still
      // see the shortened source.
      const to = next[toIdx]
      next[toIdx] = { ...to, clips: [...to.clips, moving] }
      applyProjectPatch({ videoTracks: next })
    } else {
      const tracks = project.value.audioTracks
      const fromIdx = tracks.findIndex((t) => t.id === fromTrackId)
      const toIdx = tracks.findIndex((t) => t.id === toTrackId)
      if (fromIdx < 0 || toIdx < 0) return
      const from = tracks[fromIdx]
      const clipIdx = from.clips.findIndex((c) => c.id === clipId)
      if (clipIdx < 0) return
      const moving: MultitrackClip = { ...from.clips[clipIdx], programStart: clamped }
      const next = tracks.slice()
      const newFromClips = from.clips.slice()
      newFromClips.splice(clipIdx, 1)
      next[fromIdx] = { ...from, clips: newFromClips }
      const to = next[toIdx]
      next[toIdx] = { ...to, clips: [...to.clips, moving] }
      applyProjectPatch({ audioTracks: next })
    }
    // Selection follows the clip to its new track id.
    selection.value = selection.value.map((s) =>
      s.clipId === clipId && s.trackId === fromTrackId ? { trackId: toTrackId, clipId } : s,
    )
    // History is the caller's responsibility — drag-and-drop pushes on
    // mouseup so a single drag (which may flip tracks) is one history
    // entry, not two.
  }

  /**
   * Append a clip to the named track and bump dirty + schedule save.
   * Track must already exist.
   */
  function appendClip(trackKind: 'video' | 'audio', trackId: string, clip: MultitrackClip) {
    if (!project.value) return
    if (trackKind === 'video') {
      const tracks = project.value.videoTracks.map((t) =>
        t.id === trackId ? { ...t, clips: [...t.clips, clip] } : t,
      )
      applyProjectPatch({ videoTracks: tracks })
    } else {
      const tracks = project.value.audioTracks.map((t) =>
        t.id === trackId ? { ...t, clips: [...t.clips, clip] } : t,
      )
      applyProjectPatch({ audioTracks: tracks })
    }
    undoStack.push()
  }

  // ---- Derived ----

  const programDuration = computed(() => {
    if (!project.value) return 0
    let max = 0
    for (const t of project.value.videoTracks) {
      for (const c of t.clips) {
        const e = c.programStart + (c.sourceEnd - c.sourceStart)
        if (e > max) max = e
      }
    }
    for (const t of project.value.audioTracks) {
      for (const c of t.clips) {
        const e = c.programStart + (c.sourceEnd - c.sourceStart)
        if (e > max) max = e
      }
    }
    return max
  })

  const sourcesById = computed<Record<string, MultitrackSource>>(() => {
    const m: Record<string, MultitrackSource> = {}
    if (project.value) for (const s of project.value.sources) m[s.id] = s
    return m
  })

  /**
   * Find the active clip on the topmost (highest index) video track at the
   * given playhead. Returns null when no video track has a clip there.
   * The preview composable uses this to pick which source to stream.
   */
  function topVideoActive(t: number): MultitrackTopVideoActive | null {
    if (!project.value) return null
    const tracks = project.value.videoTracks
    for (let i = tracks.length - 1; i >= 0; i--) {
      const tr = tracks[i]
      if (tr.hidden) continue
      const clip = clipAt(tr.clips, t)
      if (!clip) continue
      const src = sourcesById.value[clip.sourceId]
      if (!src) continue
      const srcTime = clip.sourceStart + (t - clip.programStart)
      return { track: tr, clip, source: src, srcTime }
    }
    return null
  }

  /**
   * Find the active clip on the given audio track at the playhead. Used
   * by useMultitrackPreview to drive each <audio> element independently.
   */
  function audioActive(track: MultitrackAudioTrack, t: number): { clip: MultitrackClip; source: MultitrackSource; srcTime: number } | null {
    if (!project.value) return null
    const clip = clipAt(track.clips, t)
    if (!clip) return null
    const src = sourcesById.value[clip.sourceId]
    if (!src) return null
    return { clip, source: src, srcTime: clip.sourceStart + (t - clip.programStart) }
  }

  // Auto-clamp the playhead when total duration shrinks (after delete /
  // remove source). Same pattern the editor store uses.
  watch(programDuration, (total) => {
    if (playhead.value > total) playhead.value = total
  })

  return {
    // state
    project,
    list,
    dirty,
    loading,
    error,
    playhead,
    playing,
    pxPerSecond,
    selection,
    splitScope,
    rangeSelection,
    libraryCollapsed,
    exportLocked,
    canUndo: undoStack.canUndo,
    canRedo: undoStack.canRedo,
    // derived
    programDuration,
    sourcesById,
    // actions
    fetchList,
    createNew,
    openProject,
    deleteProject,
    closeProject,
    loadProject,
    importSources,
    removeSource,
    addVideoTrack,
    addAudioTrack,
    removeVideoTrack,
    removeAudioTrack,
    moveClipAcrossTracks,
    appendClip,
    applyProjectPatch,
    replaceProject,
    pushHistory: undoStack.push,
    undo: undoStack.undo,
    redo: undoStack.redo,
    topVideoActive,
    audioActive,
    flushSave: autosave.flush,
    scheduleSave: autosave.schedule,
  }
})

// ---- Helpers (pure) ----

function nextTrackId(prefix: string, existing: string[]): string {
  const used = new Set(existing)
  for (let n = existing.length + 1; ; n++) {
    const id = `${prefix}${n}`
    if (!used.has(id)) return id
  }
}

function clipAt(clips: MultitrackClip[], t: number): MultitrackClip | null {
  for (const c of clips) {
    const start = c.programStart
    const end = start + (c.sourceEnd - c.sourceStart)
    if (t + 1e-6 >= start && t < end - 1e-6) return c
  }
  return null
}

// ---- Selection helpers (pure; not stored on the store) ----

export const MultitrackSel = {
  has(sel: MultitrackClipSelection[], trackId: string, clipId: string): boolean {
    return sel.some((s) => s.trackId === trackId && s.clipId === clipId)
  },
  toggle(
    sel: MultitrackClipSelection[],
    trackId: string,
    clipId: string,
  ): MultitrackClipSelection[] {
    const i = sel.findIndex((s) => s.trackId === trackId && s.clipId === clipId)
    if (i >= 0) {
      const out = sel.slice()
      out.splice(i, 1)
      return out
    }
    return sel.concat([{ trackId, clipId }])
  },
  replace(trackId: string, clipId: string): MultitrackClipSelection[] {
    return [{ trackId, clipId }]
  },
  inTrack(sel: MultitrackClipSelection[], trackId: string): string[] {
    return sel.filter((s) => s.trackId === trackId).map((s) => s.clipId)
  },
}
