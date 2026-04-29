/**
 * Quit uses raw fetch (not the json helpers) because the server may
 * close the listener before flushing a response — in which case fetch
 * rejects and we should still proceed with the UI dismissal.
 */
export const quitApi = {
  async quit(): Promise<void> {
    try {
      await fetch('/api/quit', { method: 'POST' })
    } catch {
      /* server may have closed before responding */
    }
  },
}
