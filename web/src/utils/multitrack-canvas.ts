import type { MultitrackCanvas, MultitrackClip, MultitrackProject, MultitrackSource, MultitrackTransform } from '@/api/multitrack'

/**
 * Frontend mirror of multitrack/domain/export.go::deriveDefaultCanvas. Used
 * by the Canvas Settings dialog's "use max source" preset so the user can
 * recover the v0.5.0-style auto-derived canvas without a server round-trip.
 *
 * Returns 1920×1080@30 when no video sources are referenced (matches the
 * Go fallback exactly).
 */
export function deriveCanvasFromSources(p: MultitrackProject): MultitrackCanvas {
  const usedByVideo = new Set<string>()
  for (const t of p.videoTracks) {
    for (const c of t.clips) usedByVideo.add(c.sourceId)
  }
  let w = 0
  let h = 0
  let fr = 0
  for (const s of p.sources) {
    if (!usedByVideo.has(s.id)) continue
    if (s.width && s.width > w) w = s.width
    if (s.height && s.height > h) h = s.height
    if (s.frameRate && s.frameRate > fr) fr = s.frameRate
  }
  if (w <= 0) w = 1920
  if (h <= 0) h = 1080
  if (fr <= 0) fr = 30
  return { width: w, height: h, frameRate: fr }
}

/**
 * Description of a clip that would become entirely invisible (transform
 * placed completely off the canvas) after a candidate canvas change.
 * Used to drive the OOB second-confirmation in CanvasSettingsDialog.
 */
export interface OutOfBoundsClip {
  trackIdx: number
  clipIdx: number
  clipId: string
}

/**
 * List the video clips whose transform falls fully outside the candidate
 * canvas. A transform is "fully outside" when its rectangle has zero
 * intersection with [0..W) × [0..H). Transforms that hang off an edge
 * (partial overflow) still show pixels and are NOT flagged.
 */
export function clipsOutOfBounds(p: MultitrackProject, canvas: MultitrackCanvas): OutOfBoundsClip[] {
  const out: OutOfBoundsClip[] = []
  const cw = canvas.width
  const ch = canvas.height
  p.videoTracks.forEach((track, ti) => {
    track.clips.forEach((clip, ci) => {
      if (transformFullyOutside(clip.transform, cw, ch)) {
        out.push({ trackIdx: ti, clipIdx: ci, clipId: clip.id })
      }
    })
  })
  return out
}

function transformFullyOutside(t: MultitrackTransform, cw: number, ch: number): boolean {
  // Right edge ≤ 0  → entirely left of canvas.
  // Left edge  ≥ cw → entirely right of canvas.
  // Same logic vertically.
  if (t.x + t.w <= 0) return true
  if (t.x >= cw) return true
  if (t.y + t.h <= 0) return true
  if (t.y >= ch) return true
  return false
}

/** Type-guard that an unknown is a well-formed canvas. UI can use it
 * to validate inputs before dispatching to the store. */
export function isValidCanvas(c: { width: number; height: number; frameRate: number }): boolean {
  if (!Number.isFinite(c.width) || !Number.isFinite(c.height) || !Number.isFinite(c.frameRate)) return false
  if (c.width < 16 || c.height < 16) return false
  if (c.frameRate <= 0 || c.frameRate > 240) return false
  return true
}

// Re-export the type so callers get the canonical name from one place.
export type { MultitrackCanvas, MultitrackClip, MultitrackTransform, MultitrackSource }
