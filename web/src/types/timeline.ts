/**
 * Shared timeline view types — consumed by components/timeline-shared/ and
 * composables/timeline/ for both single-video (editor) and multitrack.
 *
 * Mirrors editor/common/domain/clip.go (Go) on the time-only fields. Source
 * attribution is resolved at the editor-specific layer: single-video has one
 * project-level Source; multitrack carries it on the track or in a per-clip
 * extension. The shared Clip stays source-agnostic on purpose so it can flow
 * unchanged through both code paths.
 */

export interface Clip {
  id: string
  sourceStart: number  // seconds into source, inclusive
  sourceEnd: number    // seconds into source, exclusive
  programStart: number // seconds on track timeline
}

export type TrackKind = 'video' | 'audio'

/**
 * Visual tone for a track's clips and handles. Single-video maps
 * video → 'accent' and audio → 'success'. Multitrack will assign per
 * source; additional tones can be added by extending tokens.css and the
 * tone map in TimelineClip.vue.
 */
export type TrackTone = 'accent' | 'success' | 'danger'

export interface TrackData<C extends Clip = Clip> {
  id: string
  kind: TrackKind
  clips: C[]
  volume?: number   // audio-only; 0–2.0
  tone?: TrackTone  // visual tone for clip body and handles
  label?: string    // left column label override; falls back to kind icon
}

export interface ClipSelection {
  trackId: string
  clipId: string
}

export interface RangeSelection {
  start: number
  end: number
}

/**
 * Export settings shared by single-video and multitrack views — mirrors
 * editor/common/domain/ExportSettings on the Go side. Single-video and
 * multitrack ship with different defaults but the schema is the same.
 */
export interface ExportSettings {
  format: string
  videoCodec: string
  audioCodec: string
  outputDir: string
  outputName: string
}

/**
 * Minimum shape ProjectsModal needs to render a project list row,
 * regardless of which store backs the list.
 */
export interface ProjectsModalItem {
  id: string
  name: string
  updatedAt: string
  /** Sub-line, e.g. source path for single-video; source count for multitrack. */
  detail?: string
}
