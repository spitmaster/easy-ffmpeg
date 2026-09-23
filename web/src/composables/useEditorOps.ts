import type { Track } from '@/api/editor'
import { TRACK_AUDIO, TRACK_VIDEO } from '@/api/editor'
import { useEditorStore, Sel } from '@/stores/editor'
import { carveRange, splitTrack, trackClipsKey } from '@/utils/timeline'

/**
 * Editor commands wired to the store. Pulled out of components so the
 * keyboard handler and the toolbar buttons share one implementation.
 */
export function useEditorOps() {
  const store = useEditorStore()

  function tracksInScope(): Track[] {
    const ts: Track[] = []
    if (store.splitScope === 'both' || store.splitScope === TRACK_VIDEO) ts.push(TRACK_VIDEO)
    if (store.splitScope === 'both' || store.splitScope === TRACK_AUDIO) ts.push(TRACK_AUDIO)
    return ts
  }

  function splitAtPlayhead() {
    if (!store.project) return
    const r = store.rangeSelection
    const cuts = r && r.end - r.start > 0.05 ? [r.start, r.end] : [store.playhead]
    const tracks = tracksInScope()
    let cur = store.project
    let changed = false
    for (const trackId of tracks) {
      const key = trackClipsKey(trackId)
      for (const t of cuts) {
        const next = splitTrack(cur[key] || [], trackId, t)
        if (next) {
          cur = { ...cur, [key]: next }
          changed = true
        }
      }
    }
    if (!changed) return
    const patch: Record<string, unknown> = {}
    if (cur.videoClips !== store.project.videoClips) patch.videoClips = cur.videoClips
    if (cur.audioClips !== store.project.audioClips) patch.audioClips = cur.audioClips
    store.applyProjectPatch(patch as Partial<typeof store.project>)
    if (r) store.rangeSelection = null
    store.pushHistory()
  }

  function deleteSelection() {
    if (!store.project) return
    const r = store.rangeSelection
    if (r && r.end - r.start > 0.05) {
      const tracks = tracksInScope()
      const patch: Record<string, unknown> = {}
      for (const trackId of tracks) {
        const key = trackClipsKey(trackId)
        const clips = store.project[key] || []
        const next = carveRange(clips, trackId, r.start, r.end)
        if (next.length !== clips.length || next.some((c, i) => c !== clips[i])) {
          patch[key] = next
        }
      }
      if (!Object.keys(patch).length) {
        store.rangeSelection = null
        return
      }
      store.applyProjectPatch(patch as Partial<typeof store.project>)
      store.rangeSelection = null
      store.selection = []
      store.pushHistory()
      return
    }
    if (!store.selection.length) return
    const vIds = new Set(Sel.inTrack(store.selection, TRACK_VIDEO))
    const aIds = new Set(Sel.inTrack(store.selection, TRACK_AUDIO))
    const patch: Record<string, unknown> = {}
    if (vIds.size) patch.videoClips = (store.project.videoClips || []).filter((c) => !vIds.has(c.id))
    if (aIds.size) patch.audioClips = (store.project.audioClips || []).filter((c) => !aIds.has(c.id))
    if (!Object.keys(patch).length) return
    store.applyProjectPatch(patch as Partial<typeof store.project>)
    store.selection = []
    store.pushHistory()
  }

  return { splitAtPlayhead, deleteSelection }
}
