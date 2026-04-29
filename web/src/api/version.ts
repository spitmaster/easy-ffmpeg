import { getJson } from './client'

export interface VersionResponse {
  version: string
}

export const versionApi = {
  get: () => getJson<VersionResponse>('/api/version'),
}
