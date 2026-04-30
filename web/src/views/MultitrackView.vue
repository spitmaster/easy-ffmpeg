<script setup lang="ts">
import { computed, onDeactivated, onMounted, ref, useTemplateRef } from 'vue'
import {
  SOURCE_VIDEO,
  type MultitrackClip,
  type MultitrackSource,
} from '@/api/multitrack'
import type { ProjectsModalItem, TrackData, TrackTone } from '@/types/timeline'
import { useDirsStore } from '@/stores/dirs'
import { useMultitrackStore } from '@/stores/multitrack'
import { useModalsStore } from '@/stores/modals'
import { Path } from '@/utils/path'
import MultitrackLibrary from '@/components/multitrack/MultitrackLibrary.vue'
import MultitrackPreview from '@/components/multitrack/MultitrackPreview.vue'
import PlayBar from '@/components/timeline-shared/PlayBar.vue'
import ProjectsModal from '@/components/timeline-shared/ProjectsModal.vue'
import TimelinePlayhead from '@/components/timeline-shared/TimelinePlayhead.vue'
import TimelineRuler from '@/components/timeline-shared/TimelineRuler.vue'
import TimelineTrackRow from '@/components/timeline-shared/TimelineTrackRow.vue'

/**
 * Multitrack editor view — M6.
 *
 * Layout:
 *   topbar (project actions + add-track buttons)
 *   library | preview / playbar / timeline (video tracks ↑, audio tracks ↓)
 *
 * Drag/drop:
 *   - Library item dragged out carries `application/x-easy-ffmpeg-source`
 *     with `{ sourceId }`.
 *   - Drop on the timeline area: video source → +video track + +audio track
 *     (when hasAudio); audio source → +audio track. Existing track of the
 *     right kind also accepts a drop, appending the clip at the playhead.
 *
 * M6 keeps interactions minimal: no clip selection, drag-trim, split, or
 * cross-track drag. Those land in M7.
 */

const store = useMultitrackStore()
const dirs = useDirsStore()
const modals = useModalsStore()

const projectsOpen = ref(false)
const importing = ref(false)
const libraryRef = useTemplateRef<{ setError: (msg: string) => void }>('libraryRef')
const previewRef = useTemplateRef<{ play: () => void; pause: () => void; toggle: () => void; seek: (t: number) => void }>('previewRef')

const hasProject = computed(() => !!store.project)

// ---- Derived timeline data ----

const total = computed(() => Math.max(store.programDuration, 0))
const trackWidth = computed(() => Math.max(total.value * store.pxPerSecond + 40, 400))
const playheadX = computed(() => store.playhead * store.pxPerSecond)

// Color tones cycle so adjacent tracks don't blend together visually.
const VIDEO_TONES: TrackTone[] = ['accent', 'danger']
const AUDIO_TONES: TrackTone[] = ['success', 'accent']

const videoTracksData = computed<TrackData[]>(() =>
  (store.project?.videoTracks ?? []).map((t, i) => ({
    id: t.id,
    kind: 'video',
    clips: t.clips,
    tone: VIDEO_TONES[i % VIDEO_TONES.length],
    label: `视频 ${i + 1}`,
  })),
)
const audioTracksData = computed<TrackData[]>(() =>
  (store.project?.audioTracks ?? []).map((t, i) => ({
    id: t.id,
    kind: 'audio',
    clips: t.clips,
    tone: AUDIO_TONES[i % AUDIO_TONES.length],
    volume: t.volume,
    label: `音频 ${i + 1}`,
  })),
)

// ---- Project lifecycle ----

async function onCreate() {
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

async function onClose() {
  await store.flushSave()
  store.closeProject()
}

// ---- Source import / remove ----

async function onImport() {
  if (!store.project) return
  const start = dirs.inputDir || ''
  const p = await modals.showPicker({ mode: 'file', title: '选择视频或音频素材', startPath: start })
  if (!p) return
  importing.value = true
  try {
    const { added, errors } = await store.importSources([p])
    const dir = Path.dirname(p)
    if (dir) await dirs.saveInput(dir)
    if (errors.length > 0 && libraryRef.value) {
      libraryRef.value.setError(`导入失败:${errors[0].error}`)
    } else if (added.length === 0 && libraryRef.value) {
      libraryRef.value.setError('未导入任何素材')
    }
  } catch (e) {
    alert('导入失败: ' + (e instanceof Error ? e.message : String(e)))
  } finally {
    importing.value = false
  }
}

async function onRemoveSource(sourceId: string) {
  if (!store.project) return
  if (!confirm('从当前工程移除此素材?')) return
  try {
    await store.removeSource(sourceId)
  } catch (e) {
    const msg = e instanceof Error ? e.message : String(e)
    alert(msg.includes('still referenced') || msg.includes('in use') ? '该素材仍被时间轴上的片段引用，请先删除相关片段' : '移除失败: ' + msg)
  }
}

// ---- Add track buttons ----

function onAddVideoTrack() {
  if (!store.project) return
  store.addVideoTrack()
}

function onAddAudioTrack() {
  if (!store.project) return
  store.addAudioTrack()
}

// ---- Drag/drop from library ----

const SOURCE_MIME = 'application/x-easy-ffmpeg-source'

function readSourcePayload(ev: DragEvent): { sourceId: string } | null {
  const raw = ev.dataTransfer?.getData(SOURCE_MIME)
  if (!raw) return null
  try {
    const parsed = JSON.parse(raw)
    if (typeof parsed?.sourceId === 'string' && parsed.sourceId) return { sourceId: parsed.sourceId }
  } catch {
    return null
  }
  return null
}

function findSource(sid: string): MultitrackSource | null {
  return store.project?.sources.find((s) => s.id === sid) ?? null
}

function onTimelineDragOver(ev: DragEvent) {
  if (!ev.dataTransfer) return
  if (Array.from(ev.dataTransfer.types).includes(SOURCE_MIME)) {
    ev.preventDefault()
    ev.dataTransfer.dropEffect = 'copy'
  }
}

function newClipID(): string {
  return 'c' + Math.random().toString(16).slice(2, 8)
}

function makeClip(src: MultitrackSource, programStart: number): MultitrackClip {
  return {
    id: newClipID(),
    sourceId: src.id,
    sourceStart: 0,
    sourceEnd: src.duration > 0 ? src.duration : 0.001,
    programStart,
  }
}

/**
 * Drop onto blank timeline space → auto-create the right tracks.
 * Video source → 1 video track + 1 audio track if it has audio.
 * Audio source → 1 audio track.
 * Clip program start is 0 — the user can move it later.
 */
function onTimelineDropEmpty(ev: DragEvent) {
  ev.preventDefault()
  if (!store.project) return
  const payload = readSourcePayload(ev)
  if (!payload) return
  const src = findSource(payload.sourceId)
  if (!src) return
  if (src.kind === SOURCE_VIDEO) {
    const vid = store.addVideoTrack()
    store.appendClip('video', vid, makeClip(src, 0))
    if (src.hasAudio) {
      const aid = store.addAudioTrack()
      store.appendClip('audio', aid, makeClip(src, 0))
    }
  } else {
    const aid = store.addAudioTrack()
    store.appendClip('audio', aid, makeClip(src, 0))
  }
}

/**
 * Drop onto a specific track row → append the clip at the current
 * playhead. M6 simplification: precise drop-position math lands in M7.
 * Mismatched kind (e.g. video source on audio track) is silently
 * ignored — the user gets visual feedback by the lack of a clip.
 */
function onTrackDrop(ev: DragEvent, kind: 'video' | 'audio', trackId: string) {
  ev.preventDefault()
  if (!store.project) return
  const payload = readSourcePayload(ev)
  if (!payload) return
  const src = findSource(payload.sourceId)
  if (!src) return
  if (kind === 'video' && src.kind !== SOURCE_VIDEO) return
  store.appendClip(kind, trackId, makeClip(src, store.playhead))
}

// ---- Lifecycle ----

onMounted(() => {
  store.fetchList().catch(() => {})
})

onDeactivated(() => {
  // Best-effort flush on tab switch so unsaved drops persist.
  store.flushSave().catch(() => {})
})
</script>

<template>
  <section class="flex h-full flex-col">
    <!-- Top bar: project actions + add-track buttons -->
    <div class="flex shrink-0 items-center gap-2 border-b border-border-base bg-bg-panel px-4 py-2 text-sm">
      <button
        class="rounded border border-border-strong px-3 py-1 hover:bg-bg-elevated"
        @click="onCreate"
      >新建工程</button>
      <button
        class="rounded border border-border-strong px-3 py-1 hover:bg-bg-elevated"
        @click="projectsOpen = true"
      >工程列表</button>
      <template v-if="hasProject">
        <span class="mx-2 h-4 w-px bg-border-base"></span>
        <button
          class="rounded border border-border-strong px-2 py-1 text-xs hover:bg-bg-elevated"
          @click="onAddVideoTrack"
        >+ 视频轨</button>
        <button
          class="rounded border border-border-strong px-2 py-1 text-xs hover:bg-bg-elevated"
          @click="onAddAudioTrack"
        >+ 音频轨</button>
        <button
          class="rounded px-2 py-1 text-xs text-fg-muted hover:bg-bg-elevated hover:text-fg-base"
          @click="onClose"
        >关闭工程</button>
      </template>
      <div class="ml-auto truncate text-xs text-fg-muted">
        <template v-if="hasProject">
          当前:<span class="text-fg-base">{{ store.project!.name }}</span>
          <span v-if="store.dirty" class="ml-2 text-warning">●</span>
        </template>
        <template v-else>暂无打开的工程</template>
      </div>
    </div>

    <!-- Body -->
    <div v-if="!hasProject" class="flex flex-1 items-center justify-center text-fg-muted">
      <div class="text-center">
        <div class="text-base">尚未打开工程</div>
        <div class="mt-2 text-xs">
          点击顶栏「新建工程」开始,或从「工程列表」打开既有工程
        </div>
      </div>
    </div>

    <div v-else class="flex flex-1 overflow-hidden">
      <MultitrackLibrary
        ref="libraryRef"
        :sources="store.project!.sources"
        :importing="importing"
        @import="onImport"
        @remove="onRemoveSource"
      />

      <div class="flex flex-1 flex-col overflow-hidden">
        <MultitrackPreview ref="previewRef" />

        <PlayBar
          :playhead="store.playhead"
          :total-sec="total"
          :playing="store.playing"
          @prev="previewRef?.seek(0)"
          @next="previewRef?.seek(total)"
          @toggle="previewRef?.toggle()"
        />

        <!-- Timeline area: video tracks (top → highest z) above ruler is
             not how Premiere does it; we put the ruler on top and stack
             video tracks below it in order, so scrolling reads naturally.
             Track index 0 is bottom-most (lowest z); index N-1 is top. -->
        <div
          class="relative flex h-64 shrink-0 overflow-hidden border-t border-border-base bg-bg-base"
          @dragover="onTimelineDragOver"
          @drop="onTimelineDropEmpty"
        >
          <!-- Left labels column: ruler row + per-track labels. -->
          <div class="flex w-24 shrink-0 flex-col overflow-y-auto border-r border-border-base bg-bg-panel text-xs">
            <div class="h-7 shrink-0 border-b border-border-base"></div>
            <div
              v-for="t in videoTracksData"
              :key="'lv-' + t.id"
              class="flex h-12 shrink-0 items-center border-b border-border-base px-2"
            >🎬 {{ t.label }}</div>
            <div
              v-for="t in audioTracksData"
              :key="'la-' + t.id"
              class="flex h-12 shrink-0 items-center border-b border-border-base px-2"
            >🔊 {{ t.label }}</div>
            <div v-if="videoTracksData.length + audioTracksData.length === 0" class="px-2 py-3 text-[11px] text-fg-muted">
              拖入素材即建轨
            </div>
          </div>

          <!-- Right scrolling area -->
          <div class="relative flex-1 overflow-x-auto overflow-y-auto">
            <TimelineRuler
              :px-per-second="store.pxPerSecond"
              :total-sec="total"
              :track-width="trackWidth"
            />

            <TimelineTrackRow
              v-for="t in videoTracksData"
              :key="'v-' + t.id"
              :track="t"
              :px-per-second="store.pxPerSecond"
              :track-width="trackWidth"
              :selected-ids="[]"
              height-class="h-12"
              @dragover="onTimelineDragOver"
              @drop.stop="(ev: DragEvent) => onTrackDrop(ev, 'video', t.id)"
            />

            <TimelineTrackRow
              v-for="t in audioTracksData"
              :key="'a-' + t.id"
              :track="t"
              :px-per-second="store.pxPerSecond"
              :track-width="trackWidth"
              :selected-ids="[]"
              height-class="h-12"
              top-border
              @dragover="onTimelineDragOver"
              @drop.stop="(ev: DragEvent) => onTrackDrop(ev, 'audio', t.id)"
            />

            <!-- Empty-timeline drop hint sits inside the scroll area so
                 the dragover hit-test always lands on something useful. -->
            <div
              v-if="videoTracksData.length + audioTracksData.length === 0"
              class="absolute inset-0 flex items-center justify-center text-[11px] text-fg-muted"
            >拖入素材到此处自动建轨</div>

            <TimelinePlayhead
              v-show="store.project && total > 0"
              :x="playheadX"
            />
          </div>
        </div>
      </div>
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
  </section>
</template>
