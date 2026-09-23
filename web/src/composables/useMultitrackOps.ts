import type { MultitrackClip } from '@/api/multitrack'
import { MultitrackSel, useMultitrackStore } from '@/stores/multitrack'
import { useModalsStore } from '@/stores/modals'
import type { Clip } from '@/types/timeline'
import { carveRange, splitTrack } from '@/utils/timeline'

/**
 * Multitrack editing ops. Mirrors useEditorOps in spirit (split / delete
 * driven by splitScope + selection + range), but generalized over an
 * arbitrary number of video and audio tracks.
 *
 * splitScope semantics:
 *   'all'                  every video + audio track
 *   'video'                every video track
 *   'audio'                every audio track
 *   { kind: 'track', id }  one specific track
 *
 * Both splitTrack() and carveRange() in @/utils/timeline use { ...clip,
 * ... } spread so the multitrack-specific `sourceId` field survives.
 */
type ScopedTrack = { kind: 'video' | 'audio'; id: string }

export function useMultitrackOps() {
  const store = useMultitrackStore()
  const modals = useModalsStore()

  function tracksInScope(): ScopedTrack[] {
    const p = store.project
    if (!p) return []
    const scope = store.splitScope
    if (scope === 'all') {
      return [
        ...p.videoTracks.map((t): ScopedTrack => ({ kind: 'video', id: t.id })),
        ...p.audioTracks.map((t): ScopedTrack => ({ kind: 'audio', id: t.id })),
      ]
    }
    if (scope === 'video') {
      return p.videoTracks.map((t): ScopedTrack => ({ kind: 'video', id: t.id }))
    }
    if (scope === 'audio') {
      return p.audioTracks.map((t): ScopedTrack => ({ kind: 'audio', id: t.id }))
    }
    // single track
    if (p.videoTracks.some((t) => t.id === scope.id)) return [{ kind: 'video', id: scope.id }]
    if (p.audioTracks.some((t) => t.id === scope.id)) return [{ kind: 'audio', id: scope.id }]
    return []
  }

  function getClipsOnTrack(scoped: ScopedTrack): MultitrackClip[] {
    const p = store.project
    if (!p) return []
    if (scoped.kind === 'video') {
      return p.videoTracks.find((t) => t.id === scoped.id)?.clips ?? []
    }
    return p.audioTracks.find((t) => t.id === scoped.id)?.clips ?? []
  }

  function setClipsOnTrack(scoped: ScopedTrack, clips: MultitrackClip[]) {
    const p = store.project
    if (!p) return
    if (scoped.kind === 'video') {
      const next = p.videoTracks.map((t) => (t.id === scoped.id ? { ...t, clips } : t))
      store.applyProjectPatch({ videoTracks: next })
    } else {
      const next = p.audioTracks.map((t) => (t.id === scoped.id ? { ...t, clips } : t))
      store.applyProjectPatch({ audioTracks: next })
    }
  }

  function splitAtPlayhead() {
    if (!store.project) return
    const r = store.rangeSelection
    const cuts = r && r.end - r.start > 0.05 ? [r.start, r.end] : [store.playhead]
    const scopes = tracksInScope()
    let changed = false
    for (const scoped of scopes) {
      let clips = getClipsOnTrack(scoped)
      let trackChanged = false
      for (const t of cuts) {
        // splitTrack only consumes Clip's id/source/program fields — the
        // generic Track param is opaque to the cut math, so passing the
        // track id keeps the new clip's id namespaced ("v…" / "a…" prefix
        // is purely cosmetic; multitrack uses opaque random ids).
        const next = splitTrack(clips as Clip[], scoped.kind === 'video' ? 'video' : 'audio', t)
        if (next) {
          clips = next as MultitrackClip[]
          trackChanged = true
        }
      }
      if (trackChanged) {
        setClipsOnTrack(scoped, clips)
        changed = true
      }
    }
    if (!changed) return
    if (r) store.rangeSelection = null
    store.pushHistory()
  }

  function deleteSelection() {
    if (!store.project) return
    const r = store.rangeSelection
    if (r && r.end - r.start > 0.05) {
      const scopes = tracksInScope()
      let changed = false
      for (const scoped of scopes) {
        const clips = getClipsOnTrack(scoped)
        const next = carveRange(
          clips as Clip[],
          scoped.kind === 'video' ? 'video' : 'audio',
          r.start,
          r.end,
        ) as MultitrackClip[]
        if (next.length !== clips.length || next.some((c, i) => c !== clips[i])) {
          setClipsOnTrack(scoped, next)
          changed = true
        }
      }
      if (!changed) {
        store.rangeSelection = null
        return
      }
      store.rangeSelection = null
      store.selection = []
      store.pushHistory()
      return
    }
    if (!store.selection.length) return
    const p = store.project
    // Group selected clip ids by trackId for one pass per affected track.
    const byTrack = new Map<string, Set<string>>()
    for (const s of store.selection) {
      let bucket = byTrack.get(s.trackId)
      if (!bucket) {
        bucket = new Set<string>()
        byTrack.set(s.trackId, bucket)
      }
      bucket.add(s.clipId)
    }
    let changed = false
    const newVideo = p.videoTracks.map((t) => {
      const ids = byTrack.get(t.id)
      if (!ids) return t
      const filtered = t.clips.filter((c) => !ids.has(c.id))
      if (filtered.length === t.clips.length) return t
      changed = true
      return { ...t, clips: filtered }
    })
    const newAudio = p.audioTracks.map((t) => {
      const ids = byTrack.get(t.id)
      if (!ids) return t
      const filtered = t.clips.filter((c) => !ids.has(c.id))
      if (filtered.length === t.clips.length) return t
      changed = true
      return { ...t, clips: filtered }
    })
    if (!changed) return
    const patch: Partial<typeof p> = {}
    if (newVideo !== p.videoTracks) patch.videoTracks = newVideo
    if (newAudio !== p.audioTracks) patch.audioTracks = newAudio
    store.applyProjectPatch(patch)
    store.selection = []
    store.pushHistory()
  }

  /**
   * Remove a track. Empty tracks vanish silently; tracks with clips go
   * through a styled confirm modal (replaces window.confirm — Wails
   * suppresses native dialogs, and the OS look is jarring on Web too).
   */
  async function removeTrack(kind: 'video' | 'audio', id: string) {
    const p = store.project
    if (!p) return
    const list = kind === 'video' ? p.videoTracks : p.audioTracks
    const idx = list.findIndex((t) => t.id === id)
    if (idx < 0) return
    const track = list[idx]
    if (track.clips.length > 0) {
      const label = `${kind === 'video' ? '视频' : '音频'} ${idx + 1}`
      const ok = await modals.showConfirm({
        title: `删除轨道:${label}`,
        message: `该轨道上有 ${track.clips.length} 个片段,删除后将一并清除。\n此操作可通过 Ctrl+Z 撤销。`,
        okText: '删除',
        danger: true,
      })
      if (!ok) return
    }
    if (kind === 'video') store.removeVideoTrack(id)
    else store.removeAudioTrack(id)
  }

  return {
    splitAtPlayhead,
    deleteSelection,
    tracksInScope,
    removeTrack,
    Sel: MultitrackSel,
  }
}
