import { defineStore } from 'pinia'
import { ref } from 'vue'

/**
 * Centralised modal state for the globally-shared dialogs:
 *   - command preview (with copy)
 *   - overwrite confirm
 *   - file/dir picker
 *   - generic confirm (replaces window.confirm() — Wails WebView2 silently
 *     suppresses native dialogs and the styling is jarring on Web too)
 *
 * Modal components mount once at the App root and watch this store; views
 * trigger them through the imperative `show*` functions below, which
 * resolve to the user's choice.
 */

type Resolver<T> = (v: T) => void

interface PickerRequest {
  mode: 'file' | 'dir'
  title: string
  startPath?: string
  resolve: Resolver<string | null>
}

interface OverwriteRequest {
  resolve: Resolver<boolean>
}

interface CommandRequest {
  command: string
  resolve: Resolver<boolean>
}

export interface ConfirmOptions {
  title: string
  message: string
  /** Optional secondary detail rendered as monospace block (e.g. id, path). */
  detail?: string
  okText?: string
  cancelText?: string
  /** Style the OK button as danger (red); default false (accent / blue). */
  danger?: boolean
  /** Hide the cancel button (info-only dialog with a single OK). ESC and
   *  backdrop click still dismiss; the resolved value is irrelevant. */
  hideCancel?: boolean
}

interface ConfirmRequest extends ConfirmOptions {
  resolve: Resolver<boolean>
}

export const useModalsStore = defineStore('modals', () => {
  const overwrite = ref<{ path: string; req: OverwriteRequest } | null>(null)
  const command = ref<CommandRequest | null>(null)
  const picker = ref<PickerRequest | null>(null)
  const confirm = ref<ConfirmRequest | null>(null)

  function showOverwrite(path: string): Promise<boolean> {
    return new Promise((resolve) => {
      overwrite.value = { path, req: { resolve } }
    })
  }

  function showCommand(cmd: string): Promise<boolean> {
    return new Promise((resolve) => {
      command.value = { command: cmd, resolve }
    })
  }

  function showPicker(req: Omit<PickerRequest, 'resolve'>): Promise<string | null> {
    return new Promise((resolve) => {
      picker.value = { ...req, resolve }
    })
  }

  function showConfirm(opts: ConfirmOptions): Promise<boolean> {
    return new Promise((resolve) => {
      confirm.value = { ...opts, resolve }
    })
  }

  function settleOverwrite(v: boolean) {
    if (!overwrite.value) return
    const r = overwrite.value.req.resolve
    overwrite.value = null
    r(v)
  }

  function settleCommand(v: boolean) {
    if (!command.value) return
    const r = command.value.resolve
    command.value = null
    r(v)
  }

  function settlePicker(v: string | null) {
    if (!picker.value) return
    const r = picker.value.resolve
    picker.value = null
    r(v)
  }

  function settleConfirm(v: boolean) {
    if (!confirm.value) return
    const r = confirm.value.resolve
    confirm.value = null
    r(v)
  }

  return {
    overwrite,
    command,
    picker,
    confirm,
    showOverwrite,
    showCommand,
    showPicker,
    showConfirm,
    settleOverwrite,
    settleCommand,
    settlePicker,
    settleConfirm,
  }
})
