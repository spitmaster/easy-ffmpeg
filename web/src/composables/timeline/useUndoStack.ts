import { computed, ref } from 'vue'

/**
 * Generic undo/redo stack. The caller supplies how to take and apply a
 * snapshot of the editing state — the stack itself is just a cursor over
 * an array of snapshots. Push truncates redos beyond the cursor.
 *
 * Used by stores/editor (single video) and (future) stores/multitrack to
 * back Ctrl+Z / Ctrl+Shift+Z without each store reinventing the bookkeeping.
 */
export interface UndoStackOptions<S> {
  snapshot: () => S
  apply: (s: S) => void
  /** Maximum stack depth; older entries are dropped from the head. */
  max?: number
}

const DEFAULT_MAX = 100

export function useUndoStack<S>(opts: UndoStackOptions<S>) {
  const max = opts.max ?? DEFAULT_MAX
  const history = ref<S[]>([]) as { value: S[] }
  const cursor = ref(-1)

  const canUndo = computed(() => cursor.value > 0)
  const canRedo = computed(() => cursor.value < history.value.length - 1)

  function push() {
    const next = history.value.slice(0, cursor.value + 1)
    next.push(opts.snapshot())
    if (next.length > max) next.splice(0, next.length - max)
    history.value = next
    cursor.value = next.length - 1
  }

  function reset(initial?: S) {
    history.value = [initial ?? opts.snapshot()]
    cursor.value = 0
  }

  function undo() {
    if (!canUndo.value) return
    cursor.value--
    opts.apply(history.value[cursor.value])
  }

  function redo() {
    if (!canRedo.value) return
    cursor.value++
    opts.apply(history.value[cursor.value])
  }

  return { history, cursor, canUndo, canRedo, push, reset, undo, redo }
}
