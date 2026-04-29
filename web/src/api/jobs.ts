/**
 * SSE event channel shared by all job-running tabs (convert/audio/editor).
 * The legacy app had one EventSource for /api/convert/stream that fans
 * out to every panel via JobBus. Here we keep the same wiring.
 */

export type JobEvent =
  | { type: 'state'; running: boolean }
  | { type: 'log'; line: string }
  | { type: 'done' }
  | { type: 'error'; message: string }
  | { type: 'cancelled' }

type Listener = (ev: JobEvent) => void

let es: EventSource | null = null
const listeners = new Set<Listener>()

function connect() {
  if (es) es.close()
  es = new EventSource('/api/convert/stream')
  es.onmessage = (msg) => {
    let ev: JobEvent
    try {
      ev = JSON.parse(msg.data)
    } catch {
      return
    }
    listeners.forEach((fn) => {
      try {
        fn(ev)
      } catch {
        /* swallow listener errors so one bad subscriber doesn't kill others */
      }
    })
  }
  // Auto-reconnect on transport errors. The legacy bus did the same with
  // a 1.5s backoff — short enough that a Wails WebView pause/resume blip
  // doesn't lose events for long.
  es.onerror = () => {
    setTimeout(connect, 1500)
  }
}

export const jobBus = {
  connect,
  subscribe(fn: Listener): () => void {
    listeners.add(fn)
    return () => listeners.delete(fn)
  },
}
