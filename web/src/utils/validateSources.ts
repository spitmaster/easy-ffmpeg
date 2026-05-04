/**
 * After a project loads, the source files it references may be gone from
 * disk (user moved/deleted them between sessions). The backend serves
 * 404 for missing files, which surfaces only when the <video>/<audio>
 * elements try to fetch — silently, leaving the user staring at a black
 * preview with no clue. This util probes each source URL with HEAD up
 * front so the view can warn explicitly.
 *
 * HEAD on http.ServeContent is cheap (no body read), so checking many
 * sources in parallel is fine. Network errors are treated as "available"
 * — they'll surface naturally on play; we only flag 404 as missing.
 */

export interface SourceCheck {
  /** Display path shown to the user when this source is missing. */
  path: string
  /** URL the backend serves the bytes from. HEAD is fired against it. */
  url: string
}

export async function findMissingSources(checks: SourceCheck[]): Promise<string[]> {
  if (checks.length === 0) return []
  const results = await Promise.all(
    checks.map(async (c) => {
      try {
        const r = await fetch(c.url, { method: 'HEAD' })
        return r.status === 404 ? c.path : null
      } catch {
        return null
      }
    }),
  )
  return results.filter((p): p is string => p !== null)
}
