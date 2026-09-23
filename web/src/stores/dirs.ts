import { defineStore } from 'pinia'
import { ref } from 'vue'
import { dirsApi } from '@/api/dirs'

/**
 * Last-used input/output directories. Persisted server-side under the
 * user's data dir so they survive across launches and are shared between
 * the Web and desktop builds (same backend).
 */
export const useDirsStore = defineStore('dirs', () => {
  const inputDir = ref('')
  const outputDir = ref('')

  async function load() {
    const cfg = await dirsApi.load()
    inputDir.value = cfg.inputDir || ''
    outputDir.value = cfg.outputDir || ''
  }

  async function saveInput(dir: string) {
    inputDir.value = dir
    try {
      await dirsApi.saveInput(dir)
    } catch {
      // Persistence is best-effort; the in-memory value still drives
      // start-paths for the next picker open in this session.
    }
  }

  async function saveOutput(dir: string) {
    outputDir.value = dir
    try {
      await dirsApi.saveOutput(dir)
    } catch {
      /* best-effort — see saveInput */
    }
  }

  return { inputDir, outputDir, load, saveInput, saveOutput }
})
