import { getJson, postJson } from './client'

export interface FfmpegStatus {
  available: boolean
  embedded: boolean
  version: string
}

export const ffmpegApi = {
  status: () => getJson<FfmpegStatus>('/api/ffmpeg/status'),
  reveal: () => postJson<void>('/api/ffmpeg/reveal'),
}
