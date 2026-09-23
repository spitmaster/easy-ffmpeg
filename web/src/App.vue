<script setup lang="ts">
import { onMounted } from 'vue'
import { RouterView } from 'vue-router'
import TopBar from './components/layout/TopBar.vue'
import TabNav from './components/layout/TabNav.vue'
import ConfirmOverwriteModal from './components/modals/ConfirmOverwriteModal.vue'
import ConfirmCommandModal from './components/modals/ConfirmCommandModal.vue'
import ConfirmModal from './components/modals/ConfirmModal.vue'
import PickerModal from './components/modals/PickerModal.vue'
import PrepareOverlay from './components/modals/PrepareOverlay.vue'
import { jobBus } from './api/jobs'
import { useDirsStore } from './stores/dirs'

const dirs = useDirsStore()

onMounted(async () => {
  // Single SSE channel for the whole app — connect once at boot, the bus
  // fans events out to whichever tab is currently running a job.
  jobBus.connect()
  // Best-effort load of last-used input/output dirs so picker start-paths
  // are pre-populated. Non-blocking: failure leaves them empty.
  await dirs.load()
})
</script>

<template>
  <div class="flex h-full flex-col">
    <TopBar />
    <TabNav />
    <main class="flex-1 overflow-auto bg-bg-base">
      <!-- KeepAlive preserves view state across tab switches. Without this,
           leaving the editor (or any tab) while a job is running unmounts
           the view: useJobPanel's `owning` flag and the export sidebar
           visibility are local state, so on return the SSE events for the
           still-running job are no longer claimed by anyone and the panel
           appears empty. KeepAlive keeps the instance alive and only
           toggles activation; the EditorView migrates its document-level
           keydown listener to onActivated/onDeactivated so its shortcuts
           don't fire while another tab is in front. -->
      <RouterView v-slot="{ Component }">
        <KeepAlive>
          <component :is="Component" />
        </KeepAlive>
      </RouterView>
    </main>
  </div>

  <!-- Globally-shared dialogs. They mount once here and listen on the
       modals store, so any view can trigger them imperatively via
       useModalsStore().showCommand / showOverwrite / showPicker. -->
  <PrepareOverlay />
  <PickerModal />
  <ConfirmCommandModal />
  <ConfirmOverwriteModal />
  <ConfirmModal />
</template>
