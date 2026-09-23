import { postJson, postJsonRaw } from './client'

export interface ConvertBody {
  inputPath: string
  outputDir: string
  outputName: string
  videoEncoder: string
  audioEncoder: string
  format: string
  overwrite?: boolean
  dryRun?: boolean
}

export interface ConvertStartResponse {
  command?: string
  existing?: boolean
  path?: string
  error?: string
}

export const convertApi = {
  /** Build the ffmpeg command without starting it (preview/confirm step). */
  preview: async (body: ConvertBody): Promise<string> => {
    const data = await postJson<ConvertStartResponse>('/api/convert/start', {
      ...body,
      dryRun: true,
    })
    return data.command || ''
  },

  /**
   * Start the real job. Returns the started command line on 200.
   * On 409 with `existing: true`, the caller is expected to ask the user
   * to overwrite and retry with `overwrite: true`.
   */
  start: (body: ConvertBody) => postJsonRaw('/api/convert/start', body),

  cancel: () => postJson<void>('/api/convert/cancel'),
}
