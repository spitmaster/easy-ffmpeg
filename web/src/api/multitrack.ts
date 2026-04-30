import type { Clip as TimelineClip, ExportSettings } from '@/types/timeline'
import { fetchJson, getJson, postJson } from './client'

/**
 * Multitrack editor wire types — mirror easy-ffmpeg/multitrack/domain on
 * the Go side. Source of truth for both the JSON file format
 * (~/.easy-ffmpeg/multitrack/<id>.json) and the API.
 *
 * M5 surface is intentionally minimal: project CRUD only. Sources /
 * export endpoints arrive in M6 and M8.
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

export interface MultitrackVideoTrack {
  id: string
  locked?: boolean
  hidden?: boolean
  clips: TimelineClip[]
}

export interface MultitrackAudioTrack {
  id: string
  locked?: boolean
  muted?: boolean
  volume: number  // 0–2.0
  clips: TimelineClip[]
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

  // M6+: importSources, removeSource, sourceUrl
  // M8+: exportPreview, startExport, cancelExport
}
