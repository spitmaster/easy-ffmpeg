import { postJson, postJsonRaw } from './client'

/**
 * AudioRequest is the union of fields for all three audio-processing modes
 * (convert / extract / merge). Each mode uses only a subset; the backend
 * builder in server/audio_args.go enforces which fields are required.
 */
export interface AudioBody {
  mode: 'convert' | 'extract' | 'merge'
  // common
  outputDir: string
  outputName: string
  format: string
  codec?: string
  bitrate?: string // "192" (kbps) or "copy" or ""
  sampleRate?: number
  channels?: number
  overwrite?: boolean
  dryRun?: boolean
  // convert + extract
  inputPath?: string
  // extract
  audioStreamIndex?: number
  extractMethod?: 'copy' | 'transcode'
  // merge
  inputPaths?: string[]
  mergeStrategy?: 'auto' | 'copy' | 'reencode'
}

export interface AudioStartResponse {
  command?: string
  existing?: boolean
  path?: string
  error?: string
}

export interface AudioStream {
  index: number
  codecName: string
  channels: number
  sampleRate: number
  bitRate: number
  lang?: string
  title?: string
}

export interface AudioProbeResult {
  format: { duration: number; bitrate: number; size: number }
  streams: AudioStream[]
}

export const audioApi = {
  probe: (path: string) => postJson<AudioProbeResult>('/api/audio/probe', { path }),

  preview: async (body: AudioBody): Promise<string> => {
    const data = await postJson<AudioStartResponse>('/api/audio/start', { ...body, dryRun: true })
    return data.command || ''
  },

  start: (body: AudioBody) => postJsonRaw('/api/audio/start', body),

  cancel: () => postJson<void>('/api/audio/cancel'),
}

// ---------- Shared codec/format knowledge (mirrors AudioCodecs in legacy app.js) ----------

export interface CodecOption {
  v: string
  t: string
}

const FORMAT_CODECS: Record<string, CodecOption[]> = {
  mp3: [
    { v: 'libmp3lame', t: 'libmp3lame (MP3)' },
    { v: 'copy', t: 'copy' },
  ],
  m4a: [
    { v: 'aac', t: 'aac (AAC)' },
    { v: 'copy', t: 'copy' },
  ],
  flac: [
    { v: 'flac', t: 'flac (FLAC)' },
    { v: 'copy', t: 'copy' },
  ],
  wav: [
    { v: 'pcm_s16le', t: 'pcm_s16le (16-bit)' },
    { v: 'pcm_s24le', t: 'pcm_s24le (24-bit)' },
    { v: 'copy', t: 'copy' },
  ],
  ogg: [
    { v: 'libvorbis', t: 'libvorbis (Vorbis)' },
    { v: 'libopus', t: 'libopus (Opus)' },
    { v: 'copy', t: 'copy' },
  ],
  opus: [
    { v: 'libopus', t: 'libopus (Opus)' },
    { v: 'copy', t: 'copy' },
  ],
}

const LOSSLESS_CONTAINERS = new Set(['flac', 'wav'])
const CODEC_TO_CONTAINER: Record<string, string> = {
  aac: 'm4a',
  mp3: 'mp3',
  opus: 'opus',
  vorbis: 'ogg',
  flac: 'flac',
}

export const AudioCodecs = {
  formats: ['mp3', 'm4a', 'flac', 'wav', 'ogg', 'opus'] as const,
  codecsFor: (fmt: string): CodecOption[] => FORMAT_CODECS[fmt] || [],
  isBitrateIgnored: (fmt: string, codec: string): boolean =>
    LOSSLESS_CONTAINERS.has(fmt) || codec.startsWith('pcm_') || codec === 'copy',
  containerForCodec: (codec: string): string =>
    CODEC_TO_CONTAINER[(codec || '').toLowerCase()] || 'mka',
}

export const BITRATE_OPTIONS = ['64', '96', '128', '160', '192', '256', '320', 'copy'] as const
export const SAMPLE_RATE_OPTIONS: { v: number; t: string }[] = [
  { v: 0, t: '原始采样率' },
  { v: 48000, t: '48000 Hz' },
  { v: 44100, t: '44100 Hz' },
  { v: 22050, t: '22050 Hz' },
  { v: 8000, t: '8000 Hz' },
]
export const CHANNEL_OPTIONS: { v: number; t: string }[] = [
  { v: 0, t: '原始声道' },
  { v: 2, t: '立体声 (2)' },
  { v: 1, t: '单声道 (1)' },
]
