/**
 * Generic fetch helpers. All API calls go through this so error shape
 * (`{error: "..."}` from the Go backend) is parsed uniformly and HTTP
 * non-2xx becomes a thrown Error with the server message preserved.
 *
 * Mirrors the legacy Http object in server/web/app.js so server-side
 * contracts are unchanged.
 */
export class ApiError extends Error {
  status: number
  constructor(message: string, status: number) {
    super(message)
    this.status = status
  }
}

async function parseJson(res: Response): Promise<any> {
  try {
    return await res.json()
  } catch {
    return {}
  }
}

export async function fetchJson<T = unknown>(url: string, init?: RequestInit): Promise<T> {
  const res = await fetch(url, init)
  const data = await parseJson(res)
  if (!res.ok) {
    throw new ApiError(data?.error || `HTTP ${res.status}`, res.status)
  }
  return data as T
}

export function getJson<T = unknown>(url: string): Promise<T> {
  return fetchJson<T>(url, { method: 'GET' })
}

export function postJson<T = unknown>(url: string, body?: unknown): Promise<T> {
  return fetchJson<T>(url, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: body === undefined ? undefined : JSON.stringify(body),
  })
}

/**
 * Same as postJson but returns the raw Response so callers can inspect
 * the status code (e.g. 409 for "file exists, confirm overwrite") without
 * losing the parsed body. Body is parsed for them.
 */
export async function postJsonRaw(
  url: string,
  body?: unknown,
): Promise<{ res: Response; data: any }> {
  const res = await fetch(url, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: body === undefined ? undefined : JSON.stringify(body),
  })
  const data = await parseJson(res)
  return { res, data }
}
