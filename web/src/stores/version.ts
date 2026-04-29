import { defineStore } from 'pinia'
import { ref } from 'vue'
import { versionApi } from '@/api/version'

export const useVersionStore = defineStore('version', () => {
  const version = ref('')

  async function load() {
    try {
      const r = await versionApi.get()
      version.value = r.version || ''
    } catch {
      // network failure → leave empty (TopBar hides the chip when empty)
    }
  }

  return { version, load }
})
