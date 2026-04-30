import { defineStore } from 'pinia'
import { ref } from 'vue'
import {
  multitrackApi,
  type MultitrackProject,
  type MultitrackProjectSummary,
} from '@/api/multitrack'
import { useAutosave } from '@/composables/timeline/useAutosave'

/**
 * Multitrack store — M5 surface only. Holds the currently-open project,
 * the project list (lazy-loaded for the picker modal), and a debounced
 * autosave channel. Editing state (selection, playhead, range, undo
 * stack) lands in M6+ once the timeline actually renders.
 *
 * Kept structurally parallel to stores/editor.ts so M6 can pull in the
 * shared timeline composables (useUndoStack, applyProjectPatch, …) with
 * minimal divergence — multitrack just snapshots VideoTracks + AudioTracks
 * instead of VideoClips + AudioClips.
 */
export const useMultitrackStore = defineStore('multitrack', () => {
  const project = ref<MultitrackProject | null>(null)
  const list = ref<MultitrackProjectSummary[]>([])
  const dirty = ref(false)
  const loading = ref(false)
  const error = ref('')

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
    dirty.value = false
    // Refresh list so the new project appears in the picker.
    list.value = [
      { id: created.id, name: created.name, sourceCount: 0, createdAt: created.createdAt, updatedAt: created.updatedAt },
      ...list.value.filter((p) => p.id !== created.id),
    ]
    return created
  }

  async function openProject(id: string): Promise<MultitrackProject> {
    const p = await multitrackApi.getProject(id)
    project.value = p
    dirty.value = false
    return p
  }

  async function deleteProject(id: string): Promise<void> {
    await multitrackApi.deleteProject(id)
    list.value = list.value.filter((p) => p.id !== id)
    if (project.value?.id === id) {
      project.value = null
      dirty.value = false
    }
  }

  function closeProject() {
    project.value = null
    dirty.value = false
    autosave.cancel()
  }

  return {
    // state
    project,
    list,
    dirty,
    loading,
    error,
    // actions
    fetchList,
    createNew,
    openProject,
    deleteProject,
    closeProject,
    flushSave: autosave.flush,
    scheduleSave: autosave.schedule,
  }
})
