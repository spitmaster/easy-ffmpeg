/**
 * Debounced auto-save. The caller supplies isDirty + save; the composable
 * owns the timer. schedule() resets the debounce, flush() runs immediately
 * (clearing any pending timer), cancel() drops the pending timer without
 * saving. Errors are logged but not re-thrown — auto-save must not break
 * the UI.
 */
export interface AutosaveOptions {
  isDirty: () => boolean
  save: () => Promise<void>
  /** Debounce window in milliseconds. */
  debounceMs?: number
  /** Called when save() throws. Defaults to console.error. */
  onError?: (err: unknown) => void
}

const DEFAULT_DEBOUNCE_MS = 1500

export function useAutosave(opts: AutosaveOptions) {
  const debounceMs = opts.debounceMs ?? DEFAULT_DEBOUNCE_MS
  const onError = opts.onError ?? ((e) => console.error('autosave: save failed', e))

  let timer: ReturnType<typeof setTimeout> | null = null

  function clearTimer() {
    if (timer) {
      clearTimeout(timer)
      timer = null
    }
  }

  function schedule() {
    clearTimer()
    timer = setTimeout(flush, debounceMs)
  }

  async function flush() {
    clearTimer()
    if (!opts.isDirty()) return
    try {
      await opts.save()
    } catch (e) {
      onError(e)
    }
  }

  function cancel() {
    clearTimer()
  }

  return { schedule, flush, cancel }
}
