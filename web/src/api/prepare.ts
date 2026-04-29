import { getJson } from './client'

export type PrepareState = 'idle' | 'extracting' | 'ready' | 'error'

export interface PrepareStatus {
  state: PrepareState
  percent?: number
  current?: string
  error?: string
}

export const prepareApi = {
  status: () => getJson<PrepareStatus>('/api/prepare/status'),
}
