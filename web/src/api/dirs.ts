import { getJson, postJson } from './client'

export interface DirsConfig {
  inputDir?: string
  outputDir?: string
}

export const dirsApi = {
  load: () => getJson<DirsConfig>('/api/config/dirs').catch(() => ({}) as DirsConfig),
  saveInput: (dir: string) => postJson<void>('/api/config/dirs', { inputDir: dir }),
  saveOutput: (dir: string) => postJson<void>('/api/config/dirs', { outputDir: dir }),
}
