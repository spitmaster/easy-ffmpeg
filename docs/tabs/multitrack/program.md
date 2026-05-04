# 多轨剪辑器 — 程序设计

> 多轨 Tab 的代码组织、共享层抽取方案、数据模型、API 契约、预览策略、导出 filter graph、测试策略。
> 对应产品设计:[product.md](product.md)。底层共享模块见 [core/modules.md](../../core/modules.md);前端架构见 [core/frontend.md](../../core/frontend.md);单视频剪辑器见 [tabs/editor/program.md](../editor/program.md)。

---

## 1. 设计目标(按优先级)

1. **零回归**:单视频 Tab 的视觉 / 交互 / 性能不能因引入多轨而退化。共享层抽取**必须**保证 `editor/` 包对外行为字节级不变,前端 `EditorView` 视觉零回归。
2. **共享层先行**:在写多轨之前,先把单视频中可复用的部分提到内部共享库(后端 `editor/common/`,前端 `components/timeline-shared/` + `composables/timeline/`)。让多轨**站在共享层之上**,而不是 fork 一份单视频代码再改。
3. **架构延续 SOLID 分层**:多轨 `multitrack/` 包按 [editor/program.md §3](../editor/program.md) 同样的 `domain/ports/storage/api/module.go` 分层,只换数据模型与导出 filter。
4. **可独立测试**:`multitrack/domain/` 全纯函数,导出 filter graph 用表驱动测试覆盖单轨 / 多轨 / 短轨 pad / 跨源 / 全空各类边界。
5. **后端零分支 / 前端宿主无感**:沿用项目级不变量(见 [CLAUDE.md](../../../CLAUDE.md));多轨不引入 cgo、不 import Wails。

---

## 2. 共享层抽取方案(关键!M3 + M4 的产物)

### 2.1 后端共享层 `editor/common/`

#### 2.1.1 抽取边界

读 [editor/domain/](../../../editor/domain/) 共 ~700 行(project / timeline / export 三块)。**通用部分**:

| 抽取到 `editor/common/domain/` | 留在 `editor/domain/` | 进 `multitrack/domain/` |
|---|---|---|
| `Clip { sourceStart, sourceEnd, programStart }` | `Source`(单视频专属:单一源) | `Source` 多源(带 `id` / `kind`) |
| `Clip.Duration() / ProgramEnd()` | `Project`(单视频版的容器) | `Project`(多轨版的容器) |
| `Validate(clips []Clip)` 通用 clip 不变量校验 | `Project.Validate()`(单视频特有规则) | `Project.Validate()`(多轨特有规则) |
| `planSegments(clips, totalDur)` 切段 + gap 填充算法 | `Migrate()`(单视频 schema v1→v3) | `Migrate()`(多轨 schema 演进) |
| `buildVideoTrackFilter(clips, srcLabel, totalDur)` 单条视频轨 filter graph(trim/concat + black pad) | `BuildExportArgs(*Project)`(单视频整体导出装配) | `BuildExportArgs(*Project)`(多轨整体导出 + overlay + amix) |
| `buildAudioTrackFilter(clips, srcLabel, volume, totalDur)` 单条音频轨 filter graph(atrim/concat + anullsrc pad + volume) | | |
| `Split(clips, t, newID) / DeleteClip / Reorder / TrimLeft / TrimRight` 时间轴纯函数 | | |
| `CarveRange(clips, start, end)` 范围删除 | | |

#### 2.1.2 共享 ports(从 `editor/ports/` 提到 `editor/common/ports/`)

`Clock` / `JobRunner` / `PathResolver` 三个接口对单视频和多轨都通用,直接共享:

```go
// editor/common/ports/clock.go    Clock
// editor/common/ports/runner.go   JobRunner
// editor/common/ports/paths.go    PathResolver

// editor/ports/repository.go      ProjectRepository(单视频特有 — Project 类型不同)
// multitrack/ports/repository.go  ProjectRepository(多轨特有)

// editor/ports/prober.go     VideoProber     单视频沿用
// multitrack/ports/prober.go MediaProber     多轨需要支持纯音频探测,接口 supersets video
```

#### 2.1.3 抽取后单视频侧的迁移

```go
// editor/domain/timeline.go (改后)
package domain

import "easy-ffmpeg/editor/common/domain"

// 直接 re-export 共享类型,保持 editor/domain.Clip 不变,API 契约稳
type Clip = common.Clip

func Split(clips []Clip, t float64, id string) ([]Clip, error) {
    return common.Split(clips, t, id)
}
```

或者更激进:把 `editor/api/dto.go` 直接改成 import `editor/common/domain`,domain re-export 套层去掉。两种方案 M3 各做小验证。**关键:`/api/editor/*` 字节级不变**,前端 `editor.ts` 不动一行。

#### 2.1.4 共享层 Smoke Test

`editor/common/domain/*_test.go` 复用 `editor/domain/*_test.go` 中的相关测例(把数据无关 of `Project` 的部分迁移过来),**保证抽取后 100% 测例通过**。`go test ./...` + `CGO_ENABLED=0 go test ./...` 双绿才算 M3 完成。

### 2.2 前端共享层

#### 2.2.1 抽取目录

```text
web/src/
├── components/
│   ├── editor/             单视频专属外壳(变薄)
│   ├── multitrack/         多轨专属外壳(新建)
│   └── timeline-shared/    ★ 共享:时间轴 / 播控 / 工程列表 / 导出对话框 / 侧栏 / 音量浮窗
│       ├── TimelineRuler.vue        刻度尺(纯 props 驱动,不依赖 store)
│       ├── TimelineTrackRow.vue     单条轨道渲染(label / actions / clips 容器)
│       ├── TimelineClip.vue         单个 clip DOM(选中态、source 色条、悬停时间码)
│       ├── TimelinePlayhead.vue     播放头(大游标 / 小游标 / 拖拽)
│       ├── TimelineRangeSelection.vue 黄色半透明覆盖 + 拖动逻辑
│       ├── PlayBar.vue              ⏮⏸▶⏭ + 时间码显示
│       ├── ProjectsModal.vue        工程列表(参数化:数据源 + 路由动作)
│       ├── ExportDialog.vue         导出对话框(参数化:格式枚举、文件名默认值、提交动作)
│       ├── ExportSidebar.vue        导出期右侧栏(JobLog 容器 + 取消按钮)
│       └── AudioVolumePopover.vue   0–200% 音量浮窗
├── composables/
│   ├── timeline/           ★ 共享:时间轴交互 composable
│   │   ├── useTimelineZoom.ts        缩放滑块状态机
│   │   ├── useTimelinePlayback.ts    Space / 左右键 / 播放头跳转
│   │   ├── useTimelineRangeSelect.ts 右键拖出范围选区
│   │   ├── useTimelineDrag.ts        clip 拖拽(reorder / trim 通用部分)
│   │   ├── useUndoStack.ts           撤销栈(参数化:快照取值函数)
│   │   └── useAutosave.ts            debounce 1.5s 自动保存(参数化:save 动作)
│   ├── useEditorPreview.ts (留在单视频)
│   ├── useMultitrackPreview.ts (新建,见 §6)
│   └── useJobPanel.ts (已是共享)
└── utils/
    ├── timeline.ts (已是纯函数,共享)
    └── time.ts (已是纯函数,共享)
```

#### 2.2.2 抽取标准:**props/emit driven,不直接 import store**

共享组件**禁止** `import { useEditorStore }`。所有数据靠 `props` 注入,所有改动靠 `emit` 上抛。这样:

- 单视频 `EditorView` 把 `useEditorStore` 的字段铺到共享组件 props 上,emit 时调 `useEditorStore.commit()`
- 多轨 `MultitrackView` 把 `useMultitrackStore` 的字段铺到**同一组**共享组件 props 上,emit 时调 `useMultitrackStore.commit()`

`useUndoStack` / `useAutosave` 这种**有内部状态的 composable** 通过参数化的 getter/setter/saveFn 复用,而非 hard-code 某个 store。

#### 2.2.3 跨 store 类型对齐

共享组件接收的 `Clip` 类型定义在 `web/src/types/timeline.ts`(新建),单视频 `Project` 与多轨 `Project` 都用同一 `Clip` 形状,只差 `sourceId`(单视频隐式 `"main"`)。

```ts
// web/src/types/timeline.ts
export interface Clip {
  id: string
  sourceId: string             // 单视频用 "main",多轨用具体 source id
  sourceStart: number
  sourceEnd: number
  programStart: number
}

export interface TrackData<C extends Clip = Clip> {
  id: string
  kind: "video" | "audio"
  clips: C[]
  volume?: number              // audio 专属,可选
}
```

单视频侧的 `editor` store 把现有 `videoClips: Clip[]` 适配成 `TrackData[2]`(一条 video 一条 audio)的 view 给共享组件;不动持久化 schema(SchemaVersion=3 不变)。

#### 2.2.4 抽取后单视频回归基线

M4 完结判定:打开 `EditorView`,**手测**:

- [ ] 打开历史工程
- [ ] 拖动 clip 重排
- [ ] 拖动 clip 左 / 右 端修剪
- [ ] 在播放头分割(`S`)+ 范围选区分割
- [ ] 删除选中 + 删除范围
- [ ] 撤销 / 重做
- [ ] 修改音量浮窗
- [ ] 自动保存触发(改名 → 等 1.5s → 看 disk 上 mtime)
- [ ] 导出 dryRun / 命令预览 / 真实导出 / 取消导出 / 覆盖确认
- [ ] 导出期间侧栏占据右侧、阻断编辑

**全程视觉无差异**(与 M3 之前对比录屏 / 截图)。

---

## 3. 多轨独立部分

### 3.1 后端 `multitrack/` 包目录

```text
multitrack/
├── module.go                    NewModule(deps) + Module.Register(mux)
├── config.go                    存储路径常量
│
├── domain/                      纯业务逻辑
│   ├── project.go               Project / Source / VideoTrack / AudioTrack 结构 + 不变量
│   ├── project_test.go
│   ├── timeline.go              多轨场景下的纯函数(addTrack/deleteTrack/moveClipAcrossTracks/...)
│   ├── timeline_test.go
│   ├── export.go                BuildExportArgs(*Project) — overlay + amix 装配
│   └── export_test.go
│
├── ports/
│   ├── repository.go            ProjectRepository(多轨工程类型)
│   └── prober.go                MediaProber(超集:支持纯音频文件)
│
├── storage/
│   ├── jsonrepo.go              JSON 落 ~/.easy-ffmpeg/multitrack/
│   └── jsonrepo_test.go
│
└── api/
    ├── handlers_projects.go     CRUD
    ├── handlers_sources.go      POST /api/multitrack/projects/<id>/sources(导入素材)
    ├── handlers_export.go       export / cancel
    ├── handlers_source_serve.go GET /api/multitrack/source?projectId=&sourceId= (Range 文件服务)
    ├── dto.go
    └── routes.go
```

依赖方向严格单向(同 `editor/`):`api → ports`、`api → common.domain` + `multitrack.domain`、`storage → ports`、`domain → common.domain`(只读),domain 之间不互引。

### 3.2 数据模型(Go)

```go
// multitrack/domain/project.go
package domain

import (
    "time"

    common "easy-ffmpeg/editor/common/domain"
)

const SchemaVersion = 1

type Project struct {
    SchemaVersion int
    Kind          string         // "multitrack"
    ID            string
    Name          string
    CreatedAt     time.Time
    UpdatedAt     time.Time
    Sources       []Source
    AudioVolume   float64        // 全局,0–2.0
    VideoTracks   []VideoTrack
    AudioTracks   []AudioTrack
    Export        common.ExportSettings
}

type Source struct {
    ID         string
    Path       string
    Kind       string             // "video" | "audio"
    Duration   float64
    Width      int
    Height     int
    VideoCodec string
    AudioCodec string
    FrameRate  float64
    HasAudio   bool
}

type VideoTrack struct {
    ID     string
    Locked bool
    Hidden bool
    Clips  []common.Clip          // ★ 共享 Clip 类型
}

type AudioTrack struct {
    ID     string
    Locked bool
    Muted  bool
    Volume float64                // 0–2.0
    Clips  []common.Clip
}

func (p *Project) ProgramDuration() float64
func (p *Project) Validate() []error
func (p *Project) Migrate()       // schema v0(用户手编)→ v1 兜底
```

### 3.3 API 路由

| 方法 | 路径 | 作用 |
|------|------|------|
| `GET` | `/api/multitrack/projects` | 列表 |
| `POST` | `/api/multitrack/projects` | 新建空工程 `{name}` |
| `GET` | `/api/multitrack/projects/:id` | 读单个工程 |
| `PUT` | `/api/multitrack/projects/:id` | 全量替换(前端自动保存) |
| `DELETE` | `/api/multitrack/projects/:id` | 删除工程 |
| `POST` | `/api/multitrack/projects/:id/sources` | 导入素材;body `{paths: ["D:/...", ...]}`;后端 ffprobe 每个文件,返回 `Source[]` 追加 |
| `DELETE` | `/api/multitrack/projects/:id/sources/:sid` | 移除素材(前端先确认无 clip 引用) |
| `POST` | `/api/multitrack/export` | 开始导出,body 同单视频:`{projectId, export?, overwrite, dryRun}` |
| `POST` | `/api/multitrack/export/cancel` | 取消 |
| `GET` | `/api/multitrack/source?projectId=&sourceId=` | Range 文件服务给 `<video>` / `<audio>` |

`POST /api/convert/stream`(共享 SSE)在导出时复用,不需要新通道。

---

## 4. 前端 `MultitrackView.vue` 结构

```ts
// stores/multitrack.ts (新建,与 stores/editor.ts 同形,数据类型变)
export const useMultitrackStore = defineStore('multitrack', () => { ... })
```

```vue
<!-- views/MultitrackView.vue 草图 -->
<script setup lang="ts">
import { useMultitrackStore } from '@/stores/multitrack'
import MultitrackTopBar from '@/components/multitrack/MultitrackTopBar.vue'
import MultitrackLibrary from '@/components/multitrack/MultitrackLibrary.vue'

// ★ 全部从共享层来
import TimelineRuler   from '@/components/timeline-shared/TimelineRuler.vue'
import TimelineTrackRow from '@/components/timeline-shared/TimelineTrackRow.vue'
import PlayBar         from '@/components/timeline-shared/PlayBar.vue'
import ExportDialog    from '@/components/timeline-shared/ExportDialog.vue'
import ExportSidebar   from '@/components/timeline-shared/ExportSidebar.vue'
import ProjectsModal   from '@/components/timeline-shared/ProjectsModal.vue'

import { useTimelineZoom }       from '@/composables/timeline/useTimelineZoom'
import { useTimelinePlayback }   from '@/composables/timeline/useTimelinePlayback'
import { useTimelineRangeSelect } from '@/composables/timeline/useTimelineRangeSelect'
import { useTimelineDrag }       from '@/composables/timeline/useTimelineDrag'
import { useUndoStack }          from '@/composables/timeline/useUndoStack'
import { useAutosave }           from '@/composables/timeline/useAutosave'
import { useMultitrackPreview }  from '@/composables/useMultitrackPreview'  // 多轨独有
import { useMultitrackOps }      from '@/composables/useMultitrackOps'      // 多轨独有(跨轨拖动)

const store = useMultitrackStore()
// ... 把共享 composable / 共享组件按 store 状态接通
</script>

<template>
  <div class="grid h-full grid-cols-[240px_1fr]">
    <MultitrackLibrary />
    <div class="flex flex-col">
      <MultitrackTopBar />
      <!-- 预览区:多轨独有 DOM -->
      <MultitrackPreview />
      <!-- 时间轴:共享组件 v-for 多条轨道 -->
      <div class="editor-timeline">
        <TimelineRuler ... />
        <TimelineTrackRow v-for="track in store.videoTracks" :track="track" ... />
        <TimelineTrackRow v-for="track in store.audioTracks" :track="track" ... />
      </div>
      <PlayBar ... />
    </div>
    <!-- 导出期 sidebar 通过 v-if 挂在父 grid 右侧 -->
    <ExportSidebar v-if="exporting" ... />
    <!-- 模态 -->
    <ExportDialog v-if="exportDialogOpen" ... />
    <ProjectsModal v-if="projectsModalOpen" ... />
  </div>
</template>
```

### 4.1 多轨独有组件清单

```text
web/src/components/multitrack/
├── MultitrackTopBar.vue       新建工程 / 工程列表 / 工程名 / 导出
├── MultitrackLibrary.vue      左侧素材库栏(导入 / 列表 / 拖出 / 折叠)
├── MultitrackLibraryItem.vue  单个素材卡(双击试听、拖出、右键菜单)
├── MultitrackPreview.vue      多 <video> + 多 <audio> 容器(由 useMultitrackPreview 接管)
└── MultitrackToolbar.vue      工具条(沿用单视频 + 加 +视频轨/+音频轨)
```

`MultitrackToolbar` 与单视频的 `EditorToolbar` **可以**也提到共享(参数化加轨按钮的 visibility),M4 时评估是否值得抽。MVP 原则:**先复制后抽取**,等明确两份都长一样再合并。

---

## 5. 导出 filter graph 详解

### 5.1 装配规则

```text
1. 输入:每个 source.path 一个 -i,得到 [0:v/0:a]、[1:v/1:a]、...
2. 视频轨 V_k:遍历该轨 clips,按 sourceStart/sourceEnd 切段 trim,concat 成 [V_k]
   - 短轨用 color=c=black:s=<W>x<H>:d=<gap> 填到 programDur
   - 主分辨率取所有视频 source 中的 max(W) × max(H);非主分辨率 source 用 scale=W:H:force_original_aspect_ratio=decrease,pad=W:H:(W-iw)/2:(H-ih)/2 适配
3. 视频叠加:[V_1][V_2]overlay=0:0[V_12]; [V_12][V_3]overlay=0:0[V_123]; ... [V]
   - 轨号小→底,轨号大→顶
4. 音频轨 A_j:同样 atrim/concat,加 [A_j_pre]volume=trackVolume[A_j]
   - 短轨用 anullsrc=cl=stereo:r=44100,atrim=0:<gap>
5. 多音频混合:[A_1][A_2]...amix=inputs=N:duration=longest:dropout_transition=0[A_pre]
6. 全局音量:[A_pre]volume=globalVolume[A]
7. -map "[V]" -map "[A]"
```

### 5.2 边界(沿用单视频规则 + 多轨扩展)

| 情形 | 规则 |
|------|------|
| 视频轨开头留空(任意一条) | 报错"视频轨 V<n> 开头不可留空" |
| 音频轨开头留空 | 允许,`anullsrc` 自动 leading-pad |
| 所有视频轨为空(VideoTracks 全无 clip) | 跳过 video filter chain,不 `-map [V]`,不传 `-c:v` |
| 所有音频轨为空 | 同上,只输出视频 |
| 全空 | 报错 |
| 单条视频轨 + 单条音频轨 | 不进 overlay / amix,直接 `[V_1]→[V]`、`[A_1]→[A_pre]→[A]`(等价于单视频导出) |
| 两条视频轨完全相同 | 合法,overlay 输出 = 顶层(等价于只有顶层) |
| 视频 source 分辨率不一致 | scale + pad 适配主分辨率(M5 时锁定具体公式) |
| 视频轨之间帧率不同 | 主帧率取最高,scale 不动帧率,filter graph 不强制 fps;ffmpeg 会自然处理 |
| 音频 source 采样率不同 | amix 会自动 resample;不显式传 `aresample` |
| 视频轨 hidden(M9+) | 当前 MVP 阶段忽略字段;v2 时跳过该轨参与 overlay |

### 5.3 测试矩阵 `multitrack/domain/export_test.go`

至少覆盖:

- [ ] 单视频轨单 clip(等价于单视频导出)
- [ ] 双视频轨各一 clip,长度相等(纯 overlay)
- [ ] 双视频轨长度不等 → 短轨 black pad
- [ ] 视频轨开头留空 → 期望 error
- [ ] 三视频轨链式 overlay(z-order 正确)
- [ ] 单音频轨 + 全局 volume
- [ ] 双音频轨 amix + 各轨独立 volume
- [ ] 跨 source 的 clip 序列(`[0:v]trim` + `[1:v]trim` 都出现)
- [ ] 多源分辨率不一致 → scale + pad 出现
- [ ] 全空 → error
- [ ] 只有视频轨没音频轨 → 不出现 `-map [A]`
- [ ] 只有音频轨没视频轨 → 不出现 `-map [V]`

---

## 6. 预览策略(关键技术决策)

### 6.1 v1 默认方案:单 `<video>` 切源 + 多 `<audio>` 同步

**结构**:

```text
MultitrackPreview.vue
├── <video ref="vMain" muted preload="auto">  唯一视频元素;预览顶层视频轨
└── for each audioTrack in store.audioTracks
    └── <audio ref="aTrack[i]" preload="auto"> 每条音频轨独立 element
        └── WebAudio MediaElementSource → GainNode(轨级) → Master Gain → destination
```

**算法**(`useMultitrackPreview.ts`):

```ts
// 每帧 onTimeUpdate / 每 rAF 调用
function tick(now) {
  // 1. 找当前节目时间下"轨号最高的、当前正命中视频 clip"的 (track, clip)
  const topActive = findTopVideoActive(playhead, store.videoTracks)
  if (topActive) {
    const { source, srcTime } = topActive
    if (vMain.src !== sourceUrl(source.id)) {
      vMain.src = sourceUrl(source.id)
      await vMain.load()
    }
    if (Math.abs(vMain.currentTime - srcTime) > 0.05) vMain.currentTime = srcTime
    if (playing && vMain.paused) vMain.play()
  } else {
    // 全空 / 全部被空隙 → 黑屏 + gap clock
    vMain.classList.add('in-gap')
    vMain.pause()
  }

  // 2. 每条音频轨独立同步
  for (const [i, track] of audioTracks.entries()) {
    const active = findActive(playhead, track.clips)
    syncAudioElement(aTrack[i], track, active, playhead, store.playing)
  }
}
```

**代价**:

- 多视频轨叠加在预览端**不真合成**,只看顶层(产品已接受,见 [product.md §6](product.md))
- `vMain.src` 切换有 `loading` 闪屏 → 用 `<canvas>` 截上一帧覆盖在切换间隙(M5 评估)

### 6.2 v2 方案:多 `<video>` CSS 层叠

```html
<div class="preview-stack relative">
  <video v-for="track in videoTracks" :key="track.id"
         :style="{ zIndex: track.order }"
         class="absolute inset-0" />
</div>
```

每条视频轨一个 `<video>`,`requestVideoFrameCallback` 同步播放头。复杂度:多个元素的 seek 异步、解码资源占用、音视频同步漂移。**留给 v2**,v1 不引入。

### 6.3 v3:Canvas + WebCodecs

帧精确,不在多轨 v1 范围。

### 6.4 与单视频共用的部分

`useEditorPreview.ts` 中的**间隙时钟**(`startGapClock` / `stopGapClock` / `in-gap` 黑屏)抽到 `composables/timeline/useGapClock.ts` 共享。WebAudio gain pipeline 抽到 `composables/timeline/useAudioGain.ts` 共享。

---

## 7. 状态机(`stores/multitrack.ts`)

```ts
export const useMultitrackStore = defineStore('multitrack', () => {
  const project = ref<MultitrackProject | null>(null)
  const dirty = ref(false)
  const selection = ref<string[]>([])
  const splitScope = ref<'all' | 'video' | 'audio' | `track:${string}`>('all')
  const playhead = ref(0)
  const playing = ref(false)
  const pxPerSecond = ref(8)
  const rangeSelection = ref<{start:number;end:number}|null>(null)
  const libraryCollapsed = ref(false)

  // 共享:撤销栈、自动保存
  const undo = useUndoStack({
    snapshot: () => ({ videoTracks: project.value!.videoTracks, audioTracks: project.value!.audioTracks }),
    apply: (snap) => { project.value!.videoTracks = snap.videoTracks; project.value!.audioTracks = snap.audioTracks },
  })
  const autosave = useAutosave({
    isDirty: () => dirty.value,
    save: () => api.put(project.value!.id, project.value),
  })

  // 操作:applyProjectPatch / pushHistory / addTrack / deleteTrack / moveClipAcrossTracks ...
  // ...
  return { project, dirty, selection, splitScope, playhead, playing, pxPerSecond, ... }
})
```

`useUndoStack` / `useAutosave` 都是**参数化共享 composable**(M4 抽出),被两个 store 复用。

---

## 8. 主程序装配

### 8.1 `server/multitrack_wiring.go`(新增)

复用单视频已有的 `proberAdapter / jobRunnerAdapter / pathResolverAdapter`,只需把 `Prober` 升级成 `MediaProber`(超集,支持 `kind` 字段):

```go
type mediaProberAdapter struct{}

func (mediaProberAdapter) ProbeMedia(_ context.Context, path string) (*mtports.MediaInfo, error) {
    res, err := service.ProbeVideo(path)
    if err != nil { return nil, err }
    info := &mtports.MediaInfo{
        Duration:   res.Format.Duration,
        AudioCodec: ...,
        HasAudio:   res.Audio != nil,
    }
    if res.Video != nil {
        info.Kind = "video"
        info.Width = res.Video.Width
        info.Height = res.Video.Height
        info.VideoCodec = res.Video.CodecName
        info.FrameRate = res.Video.FrameRate
    } else {
        info.Kind = "audio"
    }
    return info, nil
}
```

### 8.2 装配(`server/server.go`)

```go
// 已有:editor.NewModule(...).Register(mux, "/api/editor")

mtMod, err := multitrack.NewModule(multitrack.Deps{
    Prober:  mediaProberAdapter{},
    Runner:  jobRunnerAdapter{m: s.jobs},   // ★ 共享 JobRunner — 全局单 job
    Paths:   pathResolverAdapter{},
    DataDir: multitrackDataDir(),           // ~/.easy-ffmpeg/multitrack/
})
mtMod.Register(mux, "/api/multitrack")
```

**关键**:`s.jobs` 是同一个全局 JobManager。多轨导出与单视频导出与转换/音频导出互斥 — 同时只能跑一个 ffmpeg(沿用项目级不变量,UI 上看到运行态即可)。

---

## 9. 测试策略

| 层 | 测试方式 | 覆盖率目标 |
|----|----------|-----------|
| `editor/common/domain/` | 表驱动单测,数据无关 | ≥ 90% |
| `multitrack/domain/` | 表驱动 + 边界 case | ≥ 90% |
| `multitrack/storage/` | `t.TempDir()` 文件系统 | ≥ 80% |
| `multitrack/api/` | `httptest` + in-memory fakes | ≥ 70% |
| 集成手测 | 单视频 Tab 零回归清单(见 §2.2.4)+ 多轨基本流程清单(M9 / M10 各落 checklist) | 手测每 M 终态 |

`go test ./...` + `CGO_ENABLED=0 go test ./...` 双绿 = 后端 M 完结门槛。

---

## 10. 风险与已知妥协

| 风险 | 妥协 / 应对 |
|------|------|
| 共享层抽取破坏单视频 Tab(回归) | M3 / M4 各保留**对比基线**(抽取前的 commit hash);M3 完成后跑 `go test ./...` 全绿 + `editor/api` 字节级响应不变;M4 完成后手测 §2.2.4 清单 |
| 跨 store 共用组件 props 接口飘移 | `web/src/types/timeline.ts` 中的 `Clip / TrackData` 锁定;改这个文件等于改契约,需要双向更新两个 store |
| 多视频轨预览不真合成 | UI 给明确提示;导出始终精确;v2 上多 `<video>` 解决 |
| `<video>.src` 切换闪屏 | M5 评估 `<canvas>` 截帧覆盖;v2 多 `<video>` 同步彻底解决 |
| 多 source 分辨率 / 帧率不一致导致 overlay 失败 | M5 在 export 装配时先 scale+pad 到主分辨率,filter 测例覆盖 |
| 工程内 source 数量大(>50)→ 列表性能 | MVP 直接 v-for;v2 引虚拟列表 |
| 大量 source `<audio>` 同时解码占资源 | 音频轨数限制 ≤ 8(UI 阻断)+ MVP 监测;真实瓶颈出现再上 OfflineAudioContext |
| 多轨工程持久化 schema 后续要演进 | 沿用单视频 `Migrate()` 模式,版本号留出空间;**禁止**手编工程文件(用户应通过 UI 操作) |
| 导出 filter graph 长度过长(命令行限制) | 跨平台命令长度 32K~ 上限;监控 `len(args)` 超 100 source 时切换为 `-filter_complex_script <file>` |

---

## 11. 各 M 交付边界(简版,详见 [milestones/feature-v0.5.0_multitrack-editor.md](../../milestones/feature-v0.5.0_multitrack-editor.md))

| M | 交付 | 进入条件 |
|---|------|---------|
| **M1** PRD | [product.md](product.md) | — |
| **M2** 技术设计 | [program.md](program.md)(本文) | M1 完成 |
| **M3** 后端共享层抽取 | `editor/common/`;`editor/` 改用共享层;`go test ./...` 全绿;API 字节级不变 | M2 完成 |
| **M4** 前端共享层抽取 | `components/timeline-shared/` + `composables/timeline/`;`EditorView` 重写为薄壳;§2.2.4 手测清单全过 | M3 完成 |
| **M5** 多轨工程骨架 | `multitrack/` 后端包 + `MultitrackView` 空壳 + 路由 + Tab 入口;新建空工程能落 disk | M4 完成 |
| **M6** 多源导入 + 多轨渲染 | 素材库 / 拖入时间轴 / 自动建轨 / N 条轨道渲染;预览仅顶层视频轨 | M5 完成 |
| **M7** 多源剪辑操作 | split/delete/trim/range/undo/redo 在多轨模型上跑通;**跨轨拖动**(同类型) | M6 完成 |
| **M8** 导出 v1 | `BuildExportArgs` overlay+amix;dryRun / 命令预览 / 真实导出 / 取消 / 覆盖确认全流程 | M7 完成 |
| **M9** 收尾 | 文档归档 / 主索引更新 / `roadmap.md §4` 加 0.5.0 行 / 版本号 bump 0.5.0 / 单视频 Tab 零回归再确认 | M8 完成 |

> 注:原 multitrack.md 中的"M9 视频叠加 / PiP" 推迟到 v2(0.7.x);v1 视频叠加只到 z-order 全屏 overlay,不含 position/scale/opacity。
