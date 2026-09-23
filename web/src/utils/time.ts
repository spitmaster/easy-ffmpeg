/**
 * HH:MM:SS[.ms] parser / formatter. Mirrors the legacy `Time` module in
 * server/web/app.js and matches the format ffmpeg accepts on the command line.
 */

const RE = /^(\d{1,2}):(\d{1,2}):(\d{1,2})(?:\.(\d{1,3}))?$/

export function parseTime(s: string): number {
  const m = String(s || '').trim().match(RE)
  if (!m) throw new Error(`时间格式不合法，应为 HH:MM:SS 或 HH:MM:SS.mmm: "${s}"`)
  const h = parseInt(m[1], 10)
  const min = parseInt(m[2], 10)
  const sec = parseInt(m[3], 10)
  const ms = m[4] ? parseInt(m[4].padEnd(3, '0'), 10) : 0
  if (min >= 60 || sec >= 60) throw new Error(`分/秒必须 < 60: "${s}"`)
  return h * 3600 + min * 60 + sec + ms / 1000
}

export function formatTime(totalSec: number): string {
  if (!isFinite(totalSec) || totalSec < 0) totalSec = 0
  const h = Math.floor(totalSec / 3600)
  const m = Math.floor((totalSec % 3600) / 60)
  const s = Math.floor(totalSec % 60)
  const ms = Math.round((totalSec - Math.floor(totalSec)) * 1000)
  const pad = (n: number, w = 2) => String(n).padStart(w, '0')
  return `${pad(h)}:${pad(m)}:${pad(s)}.${pad(ms, 3)}`
}
