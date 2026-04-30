import type { Clip as TimelineClip, ExportSettings } from '@/types/timeline'
import { fetchJson, getJson, postJson } from './client'

/**
 * Multitrack editor wire types — mirror easy-ffmpeg/multitrack/domain on
 * the Go side. Source of truth for both the JSON file format
 * (~/.easy-ffmpeg/multitrack/<id>.json) and the API.
 *
 * M6 adds source import / removal + a streaming endpoint for <video> /
 * <audio>. Export endpoints arrive in M8.
 */

export const SOURCE_VIDEO = 'video' as const
export const SOURCE_AUDIO = 'audio' as const
export type SourceKind = typeof SOURCE_VIDEO | typeof SOURCE_AUDIO

export interface MultitrackSource {
  id: string
  path: string
  kind: SourceKind
  duration: number
  width?: number
  height?: number
  videoCodec?: string
  audioCodec?: string
  frameRate?: number
  hasAudio: boolean
}

/**
 * Multitrack-specific clip — extends the shared TimelineClip with the id
 * of the source it slices. Mirrors multitrack/domain.Clip on the Go side
 * (an embedded common.Clip + a SourceID field).
 */
export type MultitrackClip = TimelineClip & { sourceId: string }

export interface MultitrackVideoTrack {
  id: string
  locked?: boolean
  hidden?: boolean
  clips: MultitrackClip[]
}

export interface MultitrackAudioTrack {
  id: string
  locked?: boolean
  muted?: boolean
  volume: number  // 0–2.0
  clips: MultitrackClip[]
}

export interface MultitrackProject {
  schemaVersion: number
  kind: 'multitrack'
  id: string
  name: string
  createdAt: string
  updatedAt: string
  sources: MultitrackSource[]
  audioVolume?: number
  videoTracks: MultitrackVideoTrack[]
  audioTracks: MultitrackAudioTrack[]
  export: ExportSettings
}

/**
 * Lightweight summary returned by GET /projects. Mirrors
 * multitrack/ports.ProjectSummary; primary detail is the source count.
 */
export interface MultitrackProjectSummary {
  id: string
  name: string
  sourceCount: number
  createdAt: string
  updatedAt: string
}

export interface MultitrackImportError {
  path: string
  error: string
}

export interface MultitrackImportResponse {
  sources: MultitrackSource[]
  project: MultitrackProject
  errors?: MultitrackImportError[]
}

export const multitrackApi = {
  listProjects: () =>
    getJson<MultitrackProjectSummary[]>('/api/multitrack/projects'),

  createProject: (name?: string) =>
    postJson<MultitrackProject>('/api/multitrack/projects', { name }),

  getProject: (id: string) =>
    getJson<MultitrackProject>(`/api/multitrack/projects/${encodeURIComponent(id)}`),

  saveProject: (p: MultitrackProject) =>
    fetchJson<MultitrackProject>(`/api/multitrack/projects/${encodeURIComponent(p.id)}`, {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(p),
    }),

  deleteProject: (id: string) =>
    fetchJson<void>(`/api/multitrack/projects/${encodeURIComponent(id)}`, {
      method: 'DELETE',
    }),

  importSources: (projectId: string, paths: string[]) =>
    postJson<MultitrackImportResponse>(
      `/api/multitrack/projects/${encodeURIComponent(projectId)}/sources`,
      { paths },
    ),

  removeSource: (projectId: string, sourceId: string) =>
    fetchJson<MultitrackProject>(
      `/api/multitrack/projects/${encodeURIComponent(projectId)}/sources/${encodeURIComponent(sourceId)}`,
      { method: 'DELETE' },
    ),

  /**
   * URL the browser hits for a Range-served source file. Used by both
   * <video src=…> in MultitrackPreview and <audio src=…> per audio track.
   */
  sourceUrl: (projectId: string, sourceId: string) =>
    `/api/multitrack/source?projectId=${encodeURIComponent(projectId)}&sourceId=${encodeURIComponent(sourceId)}`,

  // M8+: exportPreview, startExport, cancelExport
}
