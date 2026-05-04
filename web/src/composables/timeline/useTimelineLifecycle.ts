import { onActivated, onBeforeUnmount, onDeactivated, watch } from 'vue'

export interface TimelineLifecycleOptions {
  /** From useTimelinePlayback. */
  attach: () => void
  detach: () => void
  /** Best-effort autosave flush; called on tab leave + unmount. */
  flushSave: () => Promise<void>
  /** Ref-or-getter for the current project id; triggers re-fit + onProjectChange. */
  projectId: () => string | undefined
  /** Fit zoom to viewport — usually `() => zoom.applyFit()`. */
  applyFit: () => void
  /** Optional extra side effect on project change (e.g. warnIfSourcesMissing). */
  onProjectChange?: () => void
}

/**
 * Wires the lifecycle plumbing both editor and multitrack views need:
 *   - keyboard shortcut listeners attach on activate, detach on deactivate
 *     and unmount (so Space / Delete / S / Ctrl+Z don't fire from the
 *     wrong tab when KeepAlive is in play)
 *   - autosave flushes on tab leave + unmount so unsaved drops persist
 *     across tab switches and app close
 *   - applyFit re-runs when the project id changes so a freshly-loaded
 *     project starts with content fitted to the viewport
 *
 * This was duplicated near-verbatim in EditorView.vue and MultitrackView.vue;
 * differences (multitrack also calls warnIfSourcesMissing on change) are
 * fed in via onProjectChange so the pattern stays uniform.
 */
export function useTimelineLifecycle(opts: TimelineLifecycleOptions) {
  onActivated(() => opts.attach())
  onDeactivated(() => {
    opts.detach()
    opts.flushSave().catch(() => {})
  })
  onBeforeUnmount(() => {
    opts.detach()
    opts.flushSave().catch(() => {})
  })

  watch(
    () => opts.projectId(),
    (id) => {
      if (!id) return
      requestAnimationFrame(() => opts.applyFit())
      opts.onProjectChange?.()
    },
  )
}
