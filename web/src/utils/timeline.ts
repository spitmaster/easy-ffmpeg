/**
 * Pure timeline math — operates on Clip arrays for one track. No DOM, no
 * Vue, no I/O. Mirrors the `TL` module + parts of `TimelineOps` in the
 * legacy server/web/editor/editor.js so semantics stay identical to the
 * Go domain (editor/domain/timeline.go).
 */

import type { Clip, Project, Track } from '@/api/editor'
import { TRACK_AUDIO, TRACK_VIDEO } from '@/api/editor'

export function clipDuration(c: Clip): number {
  return c.sourceEnd - c.sourceStart
}

export function clipProgramEnd(c: Clip): number {
  return c.programStart + clipDuration(c)
}

/** Returns { i, src } when t falls within a clip; null in a gap or past end. */
export function programToSource(
  clips: Clip[] | undefined,
  t: number,
): { i: number; src: number } | null {
  if (!clips) return null
  for (let i = 0; i < clips.length; i++) {
    const c = clips[i]
    if (t >= c.programStart && t < clipProgramEnd(c)) {
      return { i, src: c.sourceStart + (t - c.programStart) }
    }
  }
  return null
}

export function programDuration(clips: Clip[] | undefined): number {
  if (!clips || !clips.length) return 0
  let max = 0
  for (const c of clips) {
    const e = clipProgramEnd(c)
    if (e > max) max = e
  }
  return max
}

export function totalDuration(project: Project | null | undefined): number {
  if (!project) return 0
  return Math.max(programDuration(project.videoClips), programDuration(project.audioClips))
}

export function genClipId(track: Track): string {
  const prefix = track === TRACK_AUDIO ? 'a' : 'v'
  return prefix + Math.random().toString(36).slice(2, 6)
}

/** Sorted unique boundaries on a track (includes 0). Used by ⏮ / ⏭ seek. */
export function collectBoundaries(clips: Clip[] | undefined): number[] {
  const xs: number[] = [0]
  if (!clips) return xs
  for (const c of clips) {
    xs.push(c.programStart)
    xs.push(clipProgramEnd(c))
  }
  return Array.from(new Set(xs)).sort((a, b) => a - b)
}

export function trackClipsKey(track: Track): 'videoClips' | 'audioClips' {
  return track === TRACK_VIDEO ? 'videoClips' : 'audioClips'
}

// ---------- Split ----------

/**
 * Split the clip occupying program time `t` into two halves.
 * Returns the new clips array, or null if t lies in a gap or too close
 * (≤50 ms) to a clip boundary.
 */
export function splitTrack(clips: Clip[], track: Track, t: number): Clip[] | null {
  const pos = programToSource(clips, t)
  if (!pos) return null
  const clip = clips[pos.i]
  if (pos.src - clip.sourceStart < 0.05 || clip.sourceEnd - pos.src < 0.05) return null
  const next = clips.slice()
  const leftDur = pos.src - clip.sourceStart
  const left: Clip = { ...clip, sourceEnd: pos.src }
  const right: Clip = {
    ...clip,
    id: genClipId(track),
    sourceStart: pos.src,
    programStart: clip.programStart + leftDur,
  }
  next.splice(pos.i, 1, left, right)
  return next
}

// ---------- Carve range ----------

/**
 * Trim track clips so [rangeStart, rangeEnd] becomes empty space. Gaps
 * are first-class — clips are never reflowed. Mirrors carveRange() in the
 * legacy editor.js.
 */
export function carveRange(
  clips: Clip[],
  track: Track,
  rangeStart: number,
  rangeEnd: number,
): Clip[] {
  const out: Clip[] = []
  for (const c of clips) {
    const ps = c.programStart
    const pe = clipProgramEnd(c)
    if (pe <= rangeStart + 1e-6 || ps >= rangeEnd - 1e-6) {
      out.push(c)
      continue
    }
    if (ps >= rangeStart - 1e-6 && pe <= rangeEnd + 1e-6) continue
    if (ps < rangeStart && pe > rangeEnd) {
      const leftDur = rangeStart - ps
      out.push({ ...c, sourceEnd: c.sourceStart + leftDur })
      out.push({
        ...c,
        id: genClipId(track),
        sourceStart: c.sourceStart + (rangeEnd - ps),
        programStart: rangeEnd,
      })
      continue
    }
    if (ps < rangeStart) {
      out.push({ ...c, sourceEnd: c.sourceStart + (rangeStart - ps) })
      continue
    }
    out.push({
      ...c,
      sourceStart: c.sourceStart + (rangeEnd - ps),
      programStart: rangeEnd,
    })
  }
  return out
}
