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
import { useAutosave } from '@/composables/timeline/useAutosave'

/**
 * Multitrack store. Holds the open project, the project list (lazy for
 * the picker modal), preview state (playhead / playing), and timeline
 * presentation (pxPerSecond). Editing actions (split / delete / undo)
 * land in M7; M6 only needs to seed clips when sources are dropped onto
 * the timeline.
 *
 * Like editor.ts the store is a single source of truth for the UI; every
 * mutation goes through applyProjectPatch so dirty + autosave fire
 * uniformly. SourceID validation lives on the backend (Project.Validate);
 * the frontend trusts whatever the server accepts.
 */

export interface MultitrackTopVideoActive {
  track: MultitrackVideoTrack
  clip: MultitrackClip
  source: MultitrackSource
  /** Time inside the source corresponding to playhead. */
  srcTime: number
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
    project.value = created
    playhead.value = 0
    playing.value = false
    dirty.value = false
    list.value = [
      { id: created.id, name: created.name, sourceCount: 0, createdAt: created.createdAt, updatedAt: created.updatedAt },
      ...list.value.filter((p) => p.id !== created.id),
    ]
    return created
  }

  async function openProject(id: string): Promise<MultitrackProject> {
    const p = await multitrackApi.getProject(id)
    project.value = p
    playhead.value = 0
    playing.value = false
    dirty.value = false
    return p
  }

  async function deleteProject(id: string): Promise<void> {
    await multitrackApi.deleteProject(id)
    list.value = list.value.filter((p) => p.id !== id)
    if (project.value?.id === id) {
      project.value = null
      playhead.value = 0
      playing.value = false
      dirty.value = false
    }
  }

  function closeProject() {
    project.value = null
    dirty.value = false
    playhead.value = 0
    playing.value = false
    autosave.cancel()
  }

  /**
   * Probe + add the given file paths as sources. The backend persists the
   * mutation server-side and returns the updated project; we replace
   * local state in one shot.
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
    return id
  }

  function addAudioTrack(): string {
    if (!project.value) throw new Error('no project open')
    const id = nextTrackId('a', project.value.audioTracks.map((t) => t.id))
    const track: MultitrackAudioTrack = { id, volume: 1, clips: [] }
    applyProjectPatch({ audioTracks: [...project.value.audioTracks, track] })
    return id
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
    // derived
    programDuration,
    sourcesById,
    // actions
    fetchList,
    createNew,
    openProject,
    deleteProject,
    closeProject,
    importSources,
    removeSource,
    addVideoTrack,
    addAudioTrack,
    appendClip,
    applyProjectPatch,
    replaceProject,
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
