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
 * The handler is a no-op when isLocked() returns true (e.g. while an
 * export is running) or when focus is inside an editable element.
 *
 * Not auto-installed — caller wires attach() to onActivated and detach()
 * to onDeactivated / onBeforeUnmount so KeepAlive'd views only get
 * shortcuts while they're the foreground tab.
 */
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
}

function isEditableFocus(): boolean {
  const a = document.activeElement
  if (!a) return false
  const tag = a.tagName
  return tag === 'INPUT' || tag === 'TEXTAREA' || tag === 'SELECT' || (a as HTMLElement).isContentEditable
}

export function useTimelinePlayback(opts: TimelinePlaybackOptions) {
  function onKeyDown(e: KeyboardEvent) {
    if (isEditableFocus()) return
    if (opts.isLocked()) return
    switch (e.key) {
      case ' ':
        e.preventDefault()
        opts.togglePlay()
        break
      case 's':
      case 'S':
        opts.splitAtPlayhead()
        break
      case 'Delete':
      case 'Backspace':
        opts.deleteSelection()
        break
      case 'ArrowLeft':
        opts.seekBackBoundary()
        break
      case 'ArrowRight':
        opts.seekForwardBoundary()
        break
      case 'z':
      case 'Z':
        if (e.ctrlKey || e.metaKey) {
          e.preventDefault()
          if (e.shiftKey) opts.redo()
          else opts.undo()
        }
        break
      case 'y':
      case 'Y':
        if (e.ctrlKey || e.metaKey) {
          e.preventDefault()
          opts.redo()
        }
        break
      case 'Escape':
        opts.clearRangeSelection()
        break
    }
  }

  function attach() {
    document.addEventListener('keydown', onKeyDown)
  }

  function detach() {
    document.removeEventListener('keydown', onKeyDown)
  }

  return { attach, detach }
}
