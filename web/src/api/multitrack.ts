import type { Clip as TimelineClip, ExportSettings } from '@/types/timeline'
import { fetchJson, getJson, postJson, postJsonRaw } from './client'

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

/**
 * POST /api/multitrack/export body. Mirrors editor.ExportBody so the
 * frontend's modals.showOverwrite + dryRun flow can be wrapped uniformly.
 */
export interface MultitrackExportBody {
  projectId: string
  export?: ExportSettings
  overwrite?: boolean
  dryRun?: boolean
}

/**
 * Shape of the start/dryRun response. Errors and the 409-overwrite reply
 * share keys so the same union type covers all branches the frontend
 * needs to inspect.
 */
export interface MultitrackExportStartResponse {
  command?: string
  outputPath?: string
  existing?: boolean
  path?: string
  error?: string
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

  /**
   * Dry-run an export: returns the would-be ffmpeg command and resolved
   * output path without launching the encoder or checking for an
   * existing file. The frontend surfaces this in modals.showCommand
   * before committing to the real run.
   */
  exportPreview: async (
    body: MultitrackExportBody,
  ): Promise<{ command: string; outputPath: string }> => {
    const data = await postJson<MultitrackExportStartResponse>(
      '/api/multitrack/export',
      { ...body, dryRun: true },
    )
    return { command: data.command || '', outputPath: data.outputPath || '' }
  },

  /**
   * Start a real export. postJsonRaw so the caller can see HTTP 409 with
   * `existing: true` and prompt the user before re-submitting with
   * `overwrite: true`. Same shape as editor.startExport.
   */
  startExport: (body: MultitrackExportBody) =>
    postJsonRaw('/api/multitrack/export', body),

  /**
   * Best-effort cancel. The job runner is the global single-job runner
   * shared with editor / convert / audio, so a cancel here also kills any
   * other currently-running job — but the global single-job invariant
   * means there can't be more than one anyway.
   */
  cancelExport: () => postJson<void>('/api/multitrack/export/cancel'),
}
