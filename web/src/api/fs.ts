import { getJson, postJson } from './client'

export interface FsEntry {
  name: string
  isDir: boolean
  size: number
}

export interface FsListResponse {
  path: string
  entries: FsEntry[]
  drives?: string[]
}

export interface FsHomeResponse {
  home: string
}

export const fsApi = {
  list: (path?: string) => {
    const q = path ? `?path=${encodeURIComponent(path)}` : ''
    return getJson<FsListResponse>(`/api/fs/list${q}`)
  },
  home: () => getJson<FsHomeResponse>('/api/fs/home'),
  reveal: (path: string) => postJson<void>('/api/fs/reveal', { path }),
}
