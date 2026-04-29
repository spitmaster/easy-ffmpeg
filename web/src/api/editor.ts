import { fetchJson, getJson, postJson, postJsonRaw } from './client'

/**
 * Editor domain types — mirror easy-ffmpeg/editor/domain on the Go side.
 * The shape is the source of truth for both the JSON file format
 * (~/.easy-ffmpeg/projects/<id>.json) and the API.
 */

export const TRACK_VIDEO = 'video' as const
export const TRACK_AUDIO = 'audio' as const
export type Track = typeof TRACK_VIDEO | typeof TRACK_AUDIO

export interface Source {
  path: string
  duration: number
  width: number
  height: number
  videoCodec: string
  audioCodec: string
  frameRate: number
  hasAudio: boolean
}

export interface Clip {
  id: string
  sourceStart: number
  sourceEnd: number
  programStart: number
}

export interface ExportSettings {
  format: string
  videoCodec: string
  audioCodec: string
  outputDir: string
  outputName: string
}

export interface Project {
  schemaVersion: number
  id: string
  name: string
  createdAt: string
  updatedAt: string
  source: Source
  videoClips?: Clip[]
  audioClips?: Clip[]
  audioVolume?: number
  export: ExportSettings
}

export interface ProbeResponse {
  duration: number
  width: number
  height: number
  videoCodec: string
  audioCodec: string
  frameRate: number
  hasAudio: boolean
}

export interface ExportBody {
  projectId: string
  export?: ExportSettings
  overwrite?: boolean
  dryRun?: boolean
}

export interface ExportStartResponse {
  command?: string
  outputPath?: string
  existing?: boolean
  path?: string
  error?: string
}

export const editorApi = {
  listProjects: () => getJson<Project[]>('/api/editor/projects'),

  createProject: (sourcePath: string, name?: string) =>
    postJson<Project>('/api/editor/projects', { sourcePath, name }),

  getProject: (id: string) =>
    getJson<Project>(`/api/editor/projects/${encodeURIComponent(id)}`),

  saveProject: (p: Project) =>
    fetchJson<Project>(`/api/editor/projects/${encodeURIComponent(p.id)}`, {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(p),
    }),

  deleteProject: (id: string) =>
    fetchJson<void>(`/api/editor/projects/${encodeURIComponent(id)}`, {
      method: 'DELETE',
    }),

  probe: (path: string) => postJson<ProbeResponse>('/api/editor/probe', { path }),

  exportPreview: async (body: ExportBody): Promise<{ command: string; outputPath: string }> => {
    const data = await postJson<ExportStartResponse>('/api/editor/export', { ...body, dryRun: true })
    return { command: data.command || '', outputPath: data.outputPath || '' }
  },

  startExport: (body: ExportBody) => postJsonRaw('/api/editor/export', body),

  cancelExport: () => postJson<void>('/api/editor/export/cancel'),

  sourceUrl: (projectId: string) => `/api/editor/source?id=${encodeURIComponent(projectId)}`,
}
