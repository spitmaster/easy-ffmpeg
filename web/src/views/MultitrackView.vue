<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useMultitrackStore } from '@/stores/multitrack'
import type { ProjectsModalItem } from '@/types/timeline'
import ProjectsModal from '@/components/timeline-shared/ProjectsModal.vue'

/**
 * Multitrack editor — M5 scaffold view. UI surface is intentionally a
 * shell: create / open / delete projects. Library, timeline, preview, and
 * export arrive in later milestones (M6 / M7 / M8) and slot into the
 * grid placeholder below without restructuring this file.
 */

const store = useMultitrackStore()

const projectsOpen = ref(false)

const hasProject = computed(() => !!store.project)

async function onCreate() {
  // M5 uses the browser prompt() for the name. M6+ replaces this with
  // the project's standard PickerModal-style input dialog when sources
  // start landing here too.
  const name = window.prompt('新建多轨工程名称(留空则使用默认)', '')
  if (name === null) return
  try {
    await store.createNew(name)
  } catch (e) {
    alert('新建失败: ' + (e instanceof Error ? e.message : String(e)))
  }
}

async function listForModal(): Promise<ProjectsModalItem[]> {
  const rows = await store.fetchList()
  return rows.map((r) => ({
    id: r.id,
    name: r.name,
    updatedAt: r.updatedAt,
    detail: `${r.sourceCount} 个素材`,
  }))
}

async function removeFromModal(id: string): Promise<void> {
  await store.deleteProject(id)
}

async function onLoad(id: string) {
  try {
    await store.openProject(id)
  } catch (e) {
    alert('打开失败: ' + (e instanceof Error ? e.message : String(e)))
  }
}

onMounted(() => {
  // Quietly prefetch the list so the picker opens fast on first click.
  // Errors here are non-fatal — the modal will surface them on open.
  store.fetchList().catch(() => {})
})
</script>

<template>
  <div class="flex h-full flex-col">
    <!-- Top bar: project actions only in M5; library / timeline tools
         appear in M6+. -->
    <div class="flex shrink-0 items-center gap-2 border-b border-border-base bg-bg-panel px-4 py-2 text-sm">
      <button
        class="rounded border border-border-strong px-3 py-1 hover:bg-bg-elevated"
        @click="onCreate"
      >新建工程</button>
      <button
        class="rounded border border-border-strong px-3 py-1 hover:bg-bg-elevated"
        @click="projectsOpen = true"
      >工程列表</button>
      <div class="ml-2 truncate text-xs text-fg-muted">
        <template v-if="hasProject">
          当前工程:<span class="text-fg-base">{{ store.project!.name }}</span>
          <span v-if="store.dirty" class="ml-2 text-warning">●</span>
        </template>
        <template v-else>暂无打开的工程</template>
      </div>
    </div>

    <!-- Body: M6 will replace the placeholder with
         <MultitrackLibrary /> | <MultitrackPreview/Timeline/PlayBar />. -->
    <div class="grid h-full grid-cols-[240px_1fr]">
      <aside class="border-r border-border-base bg-bg-panel p-4 text-xs text-fg-muted">
        <div class="mb-2 font-medium text-fg-base">素材库</div>
        <div>M6 起,从这里导入视频 / 音频文件并拖入时间轴。</div>
      </aside>
      <main class="flex flex-col items-center justify-center gap-3 p-6 text-fg-muted">
        <div class="text-sm">
          多轨剪辑 Tab 处于 v0.5.0 M5 骨架阶段。
        </div>
        <div class="text-xs">
          可执行操作:新建空工程、打开 / 删除既有工程。<br />
          素材导入、轨道渲染、预览与导出会随 M6–M8 陆续启用。
        </div>
        <div v-if="hasProject" class="rounded border border-border-base bg-bg-base px-4 py-3 text-xs text-fg-base">
          已打开 <span class="font-medium">{{ store.project!.name }}</span>
          (id: {{ store.project!.id }},创建于 {{ store.project!.createdAt.replace('T', ' ').slice(0, 16) }})
        </div>
      </main>
    </div>

    <ProjectsModal
      :open="projectsOpen"
      title="多轨工程列表"
      :list="listForModal"
      :remove="removeFromModal"
      :empty-text='`暂无多轨工程,点上方"新建工程"创建`'
      @close="projectsOpen = false"
      @load="onLoad"
    />
  </div>
</template>
