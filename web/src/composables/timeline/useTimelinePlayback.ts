/**
 * Document-level keyboard shortcuts for timeline playback and editing.
 * Intentionally callback-based so single-video and multitrack views can
 * each install the same key bindings against their own composables.
 *
 * Bindings:
 *   Space         togglePlay
 *   S             splitAtPlayhead
 *   Delete/Bksp   deleteSelection
 *   Left / Right  seekBackBoundary / seekForwardBoundary
 *   Ctrl+Z        undo
 *   Ctrl+Shift+Z  redo
 *   Ctrl+Y        redo
 *   Escape        clearRangeSelection
 *
 * Callers can layer view-specific bindings via `extraBindings` (e.g.
 * Ctrl+L to collapse the multitrack library). Extra bindings fire after
 * the built-in handler returns without matching, and respect the same
 * isLocked / editable-focus gating.
 *
 * The handler is a no-op when isLocked() returns true (e.g. while an
 * export is running) or when focus is inside an editable element.
 *
 * Not auto-installed — caller wires attach() to onActivated and detach()
 * to onDeactivated / onBeforeUnmount so KeepAlive'd views only get
 * shortcuts while they're the foreground tab.
 */
export interface ExtraKeyBinding {
  /** Keys that match this binding (e.g. ['l', 'L']). Compared case-sensitively
   * — pass both cases to handle Shift. */
  keys: string[]
  /** Required modifier; true means require, false means require absent,
   * undefined means don't care. */
  ctrl?: boolean
  shift?: boolean
  alt?: boolean
  meta?: boolean
  action: (e: KeyboardEvent) => void
}

export interface TimelinePlaybackOptions {
  isLocked: () => boolean
  togglePlay: () => void
  splitAtPlayhead: () => void
  deleteSelection: () => void
  seekBackBoundary: () => void
  seekForwardBoundary: () => void
  undo: () => void
  redo: () => void
  clearRangeSelection: () => void
  extraBindings?: ExtraKeyBinding[]
}

function isEditableFocus(): boolean {
  const a = document.activeElement
  if (!a) return false
  const tag = a.tagName
  return tag === 'INPUT' || tag === 'TEXTAREA' || tag === 'SELECT' || (a as HTMLElement).isContentEditable
}

function modMatch(value: boolean | undefined, actual: boolean): boolean {
  if (value === undefined) return true
  return value === actual
}

export function useTimelinePlayback(opts: TimelinePlaybackOptions) {
  function tryExtra(e: KeyboardEvent): boolean {
    if (!opts.extraBindings) return false
    for (const b of opts.extraBindings) {
      if (!b.keys.includes(e.key)) continue
      if (!modMatch(b.ctrl, e.ctrlKey || e.metaKey)) continue
      if (!modMatch(b.shift, e.shiftKey)) continue
      if (!modMatch(b.alt, e.altKey)) continue
      if (!modMatch(b.meta, e.metaKey)) continue
      b.action(e)
      return true
    }
    return false
  }

  function onKeyDown(e: KeyboardEvent) {
    if (isEditableFocus()) return
    if (opts.isLocked()) return
    switch (e.key) {
      case ' ':
        e.preventDefault()
        opts.togglePlay()
        return
      case 's':
      case 'S':
        if (e.ctrlKey || e.metaKey) break // Ctrl+S falls through to extras (e.g. save) if any
        opts.splitAtPlayhead()
        return
      case 'Delete':
      case 'Backspace':
        opts.deleteSelection()
        return
      case 'ArrowLeft':
        opts.seekBackBoundary()
        return
      case 'ArrowRight':
        opts.seekForwardBoundary()
        return
      case 'z':
      case 'Z':
        if (e.ctrlKey || e.metaKey) {
          e.preventDefault()
          if (e.shiftKey) opts.redo()
          else opts.undo()
          return
        }
        break
      case 'y':
      case 'Y':
        if (e.ctrlKey || e.metaKey) {
          e.preventDefault()
          opts.redo()
          return
        }
        break
      case 'Escape':
        opts.clearRangeSelection()
        return
    }
    tryExtra(e)
  }

  function attach() {
    document.addEventListener('keydown', onKeyDown)
  }

  function detach() {
    document.removeEventListener('keydown', onKeyDown)
  }

  return { attach, detach }
}
