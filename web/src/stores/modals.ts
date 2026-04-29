import { defineStore } from 'pinia'
import { ref } from 'vue'

/**
 * Centralised modal state for the three globally-shared dialogs:
 *   - command preview (with copy)
 *   - overwrite confirm
 *   - file/dir picker
 *
 * Modal components mount once at the App root and watch this store; views
 * trigger them through the imperative `show*` functions below, which
 * resolve to the user's choice. This mirrors the legacy `Confirm.command`
 * / `Confirm.overwrite` / `Picker.open` Promise-returning APIs.
 */

type Resolver<T> = (v: T) => void

interface PickerRequest {
  mode: 'file' | 'dir'
  title: string
  startPath?: string
  resolve: Resolver<string | null>
}

interface ConfirmRequest {
  resolve: Resolver<boolean>
}

interface CommandRequest {
  command: string
  resolve: Resolver<boolean>
}

export const useModalsStore = defineStore('modals', () => {
  const overwrite = ref<{ path: string; req: ConfirmRequest } | null>(null)
  const command = ref<CommandRequest | null>(null)
  const picker = ref<PickerRequest | null>(null)

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

  return {
    overwrite,
    command,
    picker,
    showOverwrite,
    showCommand,
    showPicker,
    settleOverwrite,
    settleCommand,
    settlePicker,
  }
})
