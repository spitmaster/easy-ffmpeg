# 多轨剪辑器 — 程序设计

> 多轨 Tab 的代码组织、共享层抽取方案、数据模型、API 契约、预览策略、导出 filter graph、测试策略。
> 对应产品设计:[product.md](product.md)。底层共享模块见 [core/modules.md](../../core/modules.md);前端架构见 [core/frontend.md](../../core/frontend.md);单视频剪辑器见 [tabs/editor/program.md](../editor/program.md)。
>
> **当前版本对齐**:
> - §1–§11 描述 **v0.5.0** 已实现状态;
> - **v0.5.1 工程画布 + clip 变换**的技术设计在 §12;v0.5.1 与基线冲突的章节(主要是 §3.2 数据模型、§5 导出 filter graph、§6 预览策略)已就地标注 "→ §12 覆盖"。

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

> **v0.5.1 扩展**:`Project` 加 `Canvas`、`Clip` 加 `Transform`,`SchemaVersion` v1 → v2。新增字段、Migrate 兜底、Validate 规则在 [§12.2](#122-数据模型变更v051) 详述,本节保留 v0.5.0 形态。

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

> **v0.5.1 重写**:本节描述的"每轨 concat 全长 → 轨道间 `overlay=0:0`"结构在 v0.5.1 被替换为"base 画布 + 全 clip 平铺 overlay 链",见 [§12.3](#123-导出-filter-graph-真合成版v051)。本节保留作为 v0.5.0 历史描述与"零回归基线"。

### 5.1 装配规则

```text
1. 输入:每个 source.path 一个 -i,得到 [0:v/0:a]、[1:v/1:a]、...
2. 视频轨 V_k:遍历该轨 clips,按 sourceStart/sourceEnd 切段 trim,concat 成 [V_k]
   - 短轨用 color=c=black:s=<W>x<H>:d=<gap> 填到 programDur
   - 主分辨率取所有视频 source 中的 max(W) × max(H);非主分辨率 source 用 scale=W:H:force_original_aspect_ratio=decrease,pad=W:H:(W-iw)/2:(H-ih)/2 适配
3. 视频叠加:[V_1][V_2]overlay=0:0[V_12]; [V_12][V_3]overlay=0:0[V_123]; ... [V]
   - 轨号小→顶,轨号大→底(v0.5.1 与时间轴 UI 顶端一致:列表顶行 = 顶层 z)
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

> **v0.5.1 推进**:从 v0.5.0 的"单 `<video>` 切源、只显示顶层轨"升级到**多 `<video>` 层叠近似真合成**(原 §6.2 v2 方案落地)。每条视频轨一个独立 `<video>` 元素,按 z-index 层叠;顶层 active video 当 master 驱动 playhead,follower 通过 rAF currentTime 校正同步。预览容器是"画布尺寸的等比缩放盒",变换框作为 DOM overlay 叠加(详见 [§12.4](#124-前端预览端的画布与变换框v051))。详细算法见 [§6.2](#62-v051-多-video-css-层叠近似真合成)。

### 6.1 v0.5.0 历史方案(已废弃):单 `<video>` 切源 + 多 `<audio>` 同步

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
  // 1. 找当前节目时间下"轨号最低的(顶层 z)、当前正命中视频 clip"的 (track, clip)
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

**v0.5.0 代价**(v0.5.1 已通过 §6.2 多 video 方案解决):

- 多视频轨叠加在预览端**不真合成**,只看顶层
- `vMain.src` 切换有 `loading` 闪屏

### 6.2 v0.5.1 方案:多 `<video>` CSS 层叠近似真合成

```html
<div class="canvas-box relative">
  <!-- 棋盘格背景 + 每条视频轨一个 <video> -->
  <video v-for="(t, i) in videoTracks"
         :key="t.id"
         v-show="trackVisuals[i].visible"
         :ref="(el) => setVideoRef(i, el)"
         class="absolute object-fill"
         :style="trackVisuals[i].style" />  <!-- left/top/width/height/zIndex -->
  <audio v-for="t in audioTracks" :key="t.id" ... />
  <TransformOverlay v-if="overlayVisible" ... />
</div>
```

**结构**:

- 每条视频轨一个独立 `<video>` 元素,`v-for` 渲染、`videoRefs[i]` 数组化
- z-index = `N - i`(i=0 顶层、i=N-1 底层),与导出 overlay 链顺序一致(轨号小 = 顶层 z)
- 每个 `<video>` 的 `left/top/width/height` 来自该轨当前 active clip 的 `transform` 转成 canvas-box 百分比;`object-fill` 让源帧拉伸到 (W, H),与导出端 `scale=W:H` 等价
- 该轨在当前 playhead 没有 active clip → `v-show:false`(保留 src,避免下次进入预热)

**算法**(`useMultitrackPreview.ts`):

```ts
// applyVideoFor(t):per-track 同步
for each track i {
  const active = videoActive(track, t)
  if (!active) { videoRefs[i].pause(); continue }
  if (videoRefs[i].src !== url(active.source)) videoRefs[i].src = url(...)
  if (|videoRefs[i].currentTime - active.srcTime| > 0.05) videoRefs[i].currentTime = active.srcTime
  if (store.playing && videoRefs[i].paused) videoRefs[i].play()
  if (!store.playing && !videoRefs[i].paused) videoRefs[i].pause()
}

// onVideoTimeUpdate:只 master 推进 playhead,顺便校正 follower
function onVideoTimeUpdate(v) {
  if (v !== masterVideoRef.value) return  // 只信 master
  store.playhead = topActive.clip.programStart + (v.currentTime - topActive.clip.sourceStart)
  syncAudio(...)
  // follower 漂移 > 0.08s 一次性写回 currentTime(rAF 不够稳就 timeupdate 兜底)
  for (followers) if (drift > 0.08) v.currentTime = expected
}
```

**Master 选举**:`masterVideoRef = computed(() => videoRefs[topVideoActiveIndex(playhead)])`。当顶层 active 切换轨道(顶层 clip 结束、下一条顶层接管),computed 重新求值,`usePreviewCore` 的 `watch(videoRef.value)` 自动 detach 旧 master + attach 新 master,无需显式触发。

**代价**:

- 多个 `<video>` 元素之间没有帧精确同步;follower 通过 currentTime 校正,理论漂移 50–100ms 量级。**这是预览近似;导出仍由 ffmpeg overlay 帧精确合成**
- 多 source 同时解码占用 CPU/内存高于 v0.5.0 单 video,但 4K 双轨在主流硬件可接受

### 6.3 v3:Canvas + WebCodecs

帧精确预览;不在 v0.5.x 范围。

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

> **v0.5.1 启动后**(2026-05-04),原 v2 的 `position` / `scale` 部分**提前**到 v0.5.1,见 §12 与 [milestones/feature-v0.5.1_multitrack-scale-video.md](../../milestones/feature-v0.5.1_multitrack-scale-video.md);`opacity` / 旋转 / 多 `<video>` 同步预览仍留 v2。

---

## 12. v0.5.1 设计 — 工程画布 + clip 变换(真合成)

> 对应 PRD:[product.md §12](product.md)。本节给出后端 / 前端 / filter graph 的具体技术方案。

### 12.1 设计目标

1. **真合成**:导出端从"上层全遮下层"切换为"alpha-aware overlay 平铺",上层 clip 不覆盖的区域必须看到下层
2. **零回归**:打开 v0.5.0 旧工程,默认值兜底到 v0.5.0 行为(画布 = max sources、变换 = 全画布),导出**字节相同 / 视觉相同**
3. **后端纯函数**:画布 / 变换全部进 `multitrack/domain/`,filter 装配仍是表驱动可测;不引新 cgo
4. **前端共享层不动**:`components/timeline-shared/` + `composables/timeline/` 零改;变换框 / 画布对话框 / Inspector 全部在 `components/multitrack/` 下新增,不污染单视频
5. **schema 演进可逆**:v2 → v0.5.0 客户端打开是"画布默认值 + 变换全画布",数值字段被解析层忽略,不破坏 JSON 解码(虽然项目不承诺向后兼容,但保持 schema 干净有助于回溯调试)

### 12.2 数据模型变更(v0.5.1)

#### 12.2.1 Go 类型

```go
// multitrack/domain/project.go
const SchemaVersion = 2 // ← 从 1 升到 2

type Canvas struct {
    Width     int     `json:"width"`     // ≥ 16
    Height    int     `json:"height"`    // ≥ 16
    FrameRate float64 `json:"frameRate"` // (0, 240]
}

type Project struct {
    // ...(v0.5.0 字段不变)
    Canvas Canvas `json:"canvas"`        // ← v0.5.1 新增
}

// multitrack/domain/clip.go
type Transform struct {
    X int `json:"x"`
    Y int `json:"y"`
    W int `json:"w"` // > 0
    H int `json:"h"` // > 0
}

type Clip struct {
    common.Clip
    SourceID  string    `json:"sourceId"`
    Transform Transform `json:"transform"` // ← v0.5.1 新增
}
```

> **不放在 `common.Clip` 上**的理由:`common.Clip` 是单视频与多轨的共享基类,单视频没有"画布上的位置"概念(它的画布 = 源分辨率);Transform 是多轨语义。把它挂在多轨自己的 `Clip` 上,共享层零改动。

#### 12.2.2 Migrate(零回归核心)

```go
func (p *Project) Migrate() {
    // ...(v0.5.0 兜底逻辑保留)

    // v1 → v2:画布默认 = max(referenced video sources)
    if p.Canvas.Width <= 0 || p.Canvas.Height <= 0 {
        cw, ch, fr := deriveDefaultCanvas(p) // 复用 v0.5.0 在 export.go 里的算法
        p.Canvas = Canvas{Width: cw, Height: ch, FrameRate: fr}
    }
    if p.Canvas.FrameRate <= 0 {
        p.Canvas.FrameRate = 30
    }

    // v1 → v2:变换默认 = 全画布
    fillDefaultTransform := func(clips []Clip) {
        for i := range clips {
            t := &clips[i].Transform
            if t.W <= 0 || t.H <= 0 {
                t.X, t.Y = 0, 0
                t.W, t.H = p.Canvas.Width, p.Canvas.Height
            }
        }
    }
    for i := range p.VideoTracks {
        fillDefaultTransform(p.VideoTracks[i].Clips)
    }
    // 音频轨的 Transform 永远忽略,不填(序列化时仍写出零值,无害)

    p.SchemaVersion = SchemaVersion
}
```

`deriveDefaultCanvas` 从现 `export.go` [multitrack/domain/export.go:103-135](../../../multitrack/domain/export.go#L103-L135) 抽出,export 路径继续调它(画布字段为零时兜底 — 但 Migrate 应该已经填上了,**这是双保险**)。

#### 12.2.3 Validate(增量)

```go
// Project.Validate() 内追加:
if p.Canvas.Width < 16 || p.Canvas.Height < 16 {
    errs = append(errs, fmt.Errorf("canvas: %dx%d 太小(最小 16×16)", p.Canvas.Width, p.Canvas.Height))
}
if p.Canvas.FrameRate <= 0 || p.Canvas.FrameRate > 240 {
    errs = append(errs, fmt.Errorf("canvas: frameRate %.2f 超出 (0, 240]", p.Canvas.FrameRate))
}

// 视频轨每个 clip 追加:
if c.Transform.W <= 0 || c.Transform.H <= 0 {
    errs = append(errs, fmt.Errorf("videoTracks[%d][%d]: transform W/H 必须 > 0", i, j))
}
// 出界**不报错**(允许动态摆位)
```

### 12.3 导出 filter graph(真合成版,v0.5.1)

#### 12.3.1 总体结构

放弃 v0.5.0 的"每轨 concat 全长 → 轨道间 overlay" 结构,改为**单一 base 画布 + 全部 video clip 按 z-order 平铺 overlay**:

```text
1. 起 base 画布:
   color=c=black:s=CWxCH:r=FR:d=programDur,format=yuv420p [base]

2. 收集所有视频 clip,按 (trackIndex 降序, programStart 升序) 排序得到 z-list:
   - 轨号大的在底(链尾先 emit,处于 base 之上),轨号小的在顶(链头最后 emit)
   - **轨号小 = 顶层** 与时间轴 UI 的视觉顺序一致(列表顶行 = 顶层 z),也是 frontend `topVideoActive` 的同一约定
   - 同轨道内按时间(同一轨道下 clip 不重叠,排序顺序对像素无影响,但保持稳定)

3. 对每个 video clip 生成 segment:
   [iv:v]
     trim=start=sStart:end=sEnd,
     setpts=PTS-STARTPTS+programStart/TB,    // ★ 关键:把 segment 的 PTS 平移到 programStart
     scale=W:H,                              // 缩到 transform.W × transform.H
     setsar=1,
     fps=FR,                                 // 统一帧率,避免下游 overlay 帧对齐异常
     format=yuva420p                         // alpha 兼容
   [seg_k]

4. 平铺 overlay 链:
   [base][seg_0]   overlay=x=X0:y=Y0:enable='between(t,p_start_0,p_end_0)':eof_action=pass [v_0]
   [v_0][seg_1]    overlay=x=X1:y=Y1:enable='between(t,p_start_1,p_end_1)':eof_action=pass [v_1]
   ...
   [v_{N-2}][seg_{N-1}] overlay=...:eof_action=pass [V]

5. 音频路径不变(沿用 v0.5.0):per-track concat → 全局 amix → -map [A]
```

关键 filter 参数:

| 参数 | 取值 | 理由 |
|------|------|------|
| `setpts=PTS-STARTPTS+programStart/TB` | 把 segment 的零点搬到 programStart | overlay 按 PTS 时间合成,segment 必须落在正确的时间窗。常见错误是只用 `setpts=PTS-STARTPTS` 让所有 segment 都从 0 开始,结果全部叠在画布开头 |
| `enable='between(t,start,end)'` | gating | 即便 segment PTS 已正确,enable 仍是显式安全网,避免 overlay 在 segment 末帧后"卡住最后一帧" |
| `eof_action=pass` | 默认 `repeat`(用最后一帧)→ 改 `pass`(透传 base) | 不让单个 segment 完帧后继续盖在底层上;配合 enable 双保险 |
| `format=yuva420p`(segment 端) + `format=yuv420p`(base 端) | alpha 链路 | overlay 默认 alpha-aware,segment 必须有 alpha 通道才能让 base 透出。base 是 yuv420p 不带 alpha 也没关系,overlay 的输出会保留 base 的像素格式 |
| `fps=FR`(segment 端) | 统一帧率 | 避免 25fps 源 + 30fps 画布 overlay 时丢帧/卡帧 |

#### 12.3.2 单轨 / 单 clip 退化

| 情况 | 行为 |
|------|------|
| 0 video clip | 跳过 video filter chain;不 `-map [V]`(沿用 v0.5.0) |
| 1 video clip,且 transform = 全画布 | base + 1 个 overlay → `[V]`;比 v0.5.0 单轨直出多了一个 base。**性能损耗可忽略**(color 滤镜便宜),换来代码路径单一,不再为 N=1 写特例 |
| ≥ 2 clips | 标准平铺 overlay 链 |

如果 v0.5.0 单轨直出的"零 overlay 路径"对回归基线测试很重要,可以保留 N=1 的 fast path(`[V_0]` 直接 alias `[V]`),但**默认走 base + overlay 统一路径**,简化测试矩阵。

#### 12.3.3 边界与错误

| 情形 | 处理 |
|------|------|
| Clip transform 完全出画布 | 合法,filter graph 照常生成,overlay 输出零像素 |
| Clip transform 部分出画布 | 合法,overlay 自动裁剪(ffmpeg 行为) |
| Canvas FR 不是源 FR 的整数倍 | `fps=FR` 在 segment 端处理,接受丢帧/插帧 |
| 旧工程(无 canvas / transform) | Migrate 已填上,export 路径看到的永远是合法值 |
| 视频轨开头留空 | 沿用 v0.5.0:逐条 error 带 `videoTracks[i]` |
| 跨源不同分辨率 | 不再像 v0.5.0 那样统一 scale+pad,**每个 clip 按自己的 transform.W × transform.H scale**,自然适配 |
| 单 source 被多个 clip 引用(同一时间窗交叠引用同一 source 的不同段) | filter graph 用 `[i:v]` 多次切片,filter_complex 支持(单条流多次 trim 在 ffmpeg 是合法的,内部会做必要的 split) |

#### 12.3.4 测试矩阵(`multitrack/domain/export_test.go` 重写 / 增补)

继承 v0.5.0 §5.3 全矩阵,**新增**:

- [ ] 单视频轨单 clip + transform 全画布 → 与 v0.5.0 视觉等价
- [ ] 单视频轨双 clip 时间不重叠 + 各自不同 transform → segment 各自正确摆位
- [ ] 双视频轨各一 clip 时间相同 + 上层小窗(右下角 PIP)→ 下层主画面 + 右下角小窗
- [ ] 双视频轨上层完全覆盖下层 → 等价 v0.5.0 上层全屏(下层不可见)
- [ ] 三视频轨 z-order(`[V0]→[V1]→[V2]`)→ overlay 链顺序正确
- [ ] Clip transform 完全出画布 → segment 仍生成,overlay 输出空
- [ ] Clip transform 部分出画布(X<0 或 X+W>canvasW)→ 不报错,filter 不裁
- [ ] Canvas 非源整数倍帧率 → segment 端 `fps=FR` 出现
- [ ] v0.5.0 工程(`schemaVersion=1`,无 canvas/transform)→ Migrate 后导出 = v0.5.0 字节(回归基线)
- [ ] Canvas W/H/FR 越界 → Validate 报错
- [ ] Transform W/H ≤ 0 → Validate 报错

### 12.4 前端预览端的画布与变换框(v0.5.1)

#### 12.4.1 预览容器结构

```vue
<!-- MultitrackPreview.vue 草图 -->
<template>
  <div class="preview-shell relative h-full flex items-center justify-center bg-black">
    <!-- 画布盒:等比缩放进可视区,保持 aspectRatio = canvasW / canvasH -->
    <div class="preview-canvas relative" :style="canvasBoxStyle">
      <video ref="vMain" class="absolute inset-0 w-full h-full object-fill" />
      <audio v-for="(_, i) in audioTracks" :key="i" :ref="el => aRefs[i] = el" />

      <!-- 选中 clip 的变换框(v0.5.1) -->
      <TransformOverlay
        v-if="selectedClipTransform"
        :canvas="store.project.canvas"
        :transform="selectedClipTransform"
        @update="onTransformUpdate"
        @commit="onTransformCommit"
      />
    </div>
  </div>
</template>
```

`canvasBoxStyle` 计算:

```ts
const canvasBoxStyle = computed(() => {
  const { width: cw, height: ch } = store.project.canvas
  // 父容器尺寸由 ResizeObserver 持续监听 → boxW / boxH
  const sx = boxW.value / cw
  const sy = boxH.value / ch
  const s = Math.min(sx, sy)
  return {
    width: `${cw * s}px`,
    height: `${ch * s}px`,
  }
})
```

预览端的 `<video>` 仍走 v0.5.0 的"显示当前 playhead 下的顶层激活源"近似,但因为现在画布有自定义尺寸,`object-fit` 改为 `object-fill`(把源拉伸到画布盒),不再是 `contain`。**多轨叠加预览仍只显示顶层**,UI 提示"预览仅显示顶层视频轨;PIP 效果以导出为准"(v0.5.0 已有的提示沿用)。

#### 12.4.2 TransformOverlay 组件

新组件 `web/src/components/multitrack/TransformOverlay.vue`:

- 绝对定位在 `.preview-canvas` 内部,坐标按 `transform / canvas` 比例换算到容器像素
- 边线 1px `accent` 色;角手柄 12×12 圆点,边手柄 12×8 矩形
- 拖拽事件:
  - 中心区域 → 平移 (dx, dy);按 Shift 锁单轴
  - 角手柄 → 同时改 (X, Y, W, H);按 Shift 锁纵横比;按 Alt 以中心缩放
  - 边手柄 → 单边改 W 或 H
- 拖拽期间 `@update` 实时发出 transform 草稿(本地 state,不入撤销栈),松开发 `@commit`(push 撤销栈)
- 像素 → canvas 坐标的换算用 `boxW / canvasW` 比例,需要 round 到整数

接到 store:

```ts
// MultitrackView.vue 内
function onTransformUpdate(t: Transform) {
  // 直接改当前 selected clip 的 transform(本地,不 commit)
  store.previewClipTransform(selectedClipId.value, t)
}
function onTransformCommit(t: Transform) {
  // push 撤销栈 + 标 dirty + 触发 autosave
  store.commitClipTransform(selectedClipId.value, t)
}
```

#### 12.4.3 画布设置模态

新组件 `web/src/components/multitrack/CanvasSettingsDialog.vue`:

- 三个数字输入(W / H / FR)+ 预设按钮列表
- 预设:`1920×1080@30 (1080p)`、`3840×2160@30 (4K UHD)`、`1080×1920@30 (竖屏 9:16)`、`使用源最大值`(等价 Migrate 兜底)
- 确认前如果新画布会让任意 clip 变得"完全出画布",二次提示 + 可继续(v0.5.1 不强制 clamp)
- 改画布等价一次 commit(push 撤销栈,因为 canvas 也是工程状态)

#### 12.4.4 Inspector 面板

新组件 `web/src/components/multitrack/MultitrackInspector.vue`:

- 折叠到右栏边缘窄条(36px),点开 240–280px 宽
- 内容分两段:**画布**(始终可见)+ **选中 clip**(条件显示)
- 数字输入失焦或回车提交,提交触发 `commitClipTransform`(同变换框 onCommit)
- 与 `ExportSidebar` 互斥:`exportSidebarOpen` 时整体隐藏

### 12.5 状态机改动(`stores/multitrack.ts`)

```ts
// 新增 actions:
function setCanvas(canvas: Canvas) {
  pushHistory()
  project.value!.canvas = canvas
  dirty.value = true
}

// 草稿模式(拖手柄期间):不入栈,不 dirty
function previewClipTransform(clipId: string, t: Transform) {
  const clip = findClip(clipId)
  if (!clip) return
  clip.transform = t
  // 不 push,不标 dirty(预览态)
}

// 提交模式(松开手柄 / 数字框失焦):入栈 + dirty
function commitClipTransform(clipId: string, t: Transform) {
  pushHistory()
  const clip = findClip(clipId)
  if (!clip) return
  clip.transform = t
  dirty.value = true
}
```

⚠ **草稿与提交分离**是关键模式:拖手柄过程每帧改 50+ 次 transform,如果都进撤销栈会爆;只在松开时入栈一次。这是 v0.5.0 clip 拖拽已经在用的模式([useTimelineDrag](../../../web/src/composables/timeline/useTimelineDrag.ts))。

撤销栈快照(`useUndoStack`)需要把 `canvas` 加进 snapshot 函数:

```ts
const undo = useUndoStack({
  snapshot: () => ({
    canvas: project.value!.canvas,
    videoTracks: project.value!.videoTracks,
    audioTracks: project.value!.audioTracks,
  }),
  apply: (snap) => {
    project.value!.canvas = snap.canvas
    project.value!.videoTracks = snap.videoTracks
    project.value!.audioTracks = snap.audioTracks
  },
})
```

### 12.6 API 契约变更

无新增端点。已有端点的 body / response 自动跟 schema 走:

| 端点 | 变化 |
|------|------|
| `GET /api/multitrack/projects/:id` | 响应 `Project` 多 `canvas` 字段;`videoTracks[].clips[]` 多 `transform` 字段 |
| `PUT /api/multitrack/projects/:id` | 同上,前端发什么后端存什么(经 Migrate + Validate) |
| `POST /api/multitrack/export` | 后端用 `Project.Canvas` 装 filter graph;dryRun 返回的命令也按 v0.5.1 格式 |

向后兼容:打开 v0.5.0 工程文件 → 后端 Migrate 加上 canvas + transform → 第一次保存就升到 v2。

### 12.7 共享层影响评估

| 共享模块 | 受影响 | 说明 |
|---------|--------|------|
| `editor/common/domain/Clip` | ❌ 不动 | Transform 在 multitrack 自己的 Clip 上 |
| `editor/common/domain/BuildVideoTrackFilter` | ❌ 不动 | 单视频不需要画布概念,继续用 |
| `editor/common/domain/PlanSegments` | ❌ 不动 | 时间维度算法,与空间无关 |
| 单视频 `editor/` | ❌ 不动 | 单视频 Tab 视觉与导出零回归(强制约束) |
| `components/timeline-shared/` | ❌ 不动 | 时间轴不可视化空间属性 |
| `composables/timeline/useUndoStack` | ⚠ snapshot 函数变 | 多轨 store 自己管,共享 composable 接口不变 |
| `MultitrackPreview.vue` | ✓ 改造 | §12.4.1 |
| `MultitrackTopBar.vue` | ✓ 加按钮 | "画布: ... " 入口 |
| `multitrack/domain/{project,clip,filter,export}.go` | ✓ 改造 | §12.2 / §12.3 |
| `multitrack/domain/{project,export}_test.go` | ✓ 重写 | §12.3.4 |

### 12.8 风险与已知妥协

| 风险 | 妥协 / 应对 |
|------|------|
| `setpts` PTS 平移在边界条件下可能偏 1 帧 | 测例覆盖 `programStart` 不是整帧的情况(如 0.033s),核对 ffprobe 输出帧数 |
| `format=yuva420p` 编码到 `yuv420p` 的损耗(主流 mp4 不存 alpha)| overlay 在 base 处合成后输出仍是 yuv420p,只是 segment 中间过程带 alpha;最终编码无 alpha,无损耗 |
| `eof_action=pass` 在某些 ffmpeg 版本不存在 | 项目嵌入的 ffmpeg 版本(见 [internal/embedded/](../../../internal/embedded/))支持;首个 M 末做版本验证 |
| 多 clip 平铺 overlay 链长度爆 | 100 clip → 100 节 overlay,filter 长度 ~10KB,远低于命令行长度上限。超 100 时已有 `-filter_complex_script` 路径(沿用 v0.5.0 §10 的应对) |
| 预览端不真合成,与导出不一致 | 沿用 v0.5.0 提示;变换框给出框选辅助 / 数字精确反馈;真预览合成留 v2 多 `<video>` 同步方案 |
| 用户改画布让 clip 出界后困惑 | UI 在 clip 上画"⚠ 不可见"角标 + Inspector "重置为全画布"快捷;不强制自动 clamp(避免数据丢失) |
| transform 整数像素粒度不够精细 | v0.5.1 锁整数(避免浮点累积误差);v2 引入百分比 / 浮点像素时再考虑 |

### 12.9 各 M 交付边界(简版,详见 [milestones/feature-v0.5.1_multitrack-scale-video.md](../../milestones/feature-v0.5.1_multitrack-scale-video.md))

| M | 交付 | 进入条件 |
|---|------|---------|
| **M1** PRD | [product.md §12](product.md) | — |
| **M2** 技术设计 | 本节(§12) | M1 完成 |
| **M3** 后端数据模型 + filter 重写 | `Canvas` / `Transform` 进 `multitrack/domain/`;Migrate v1→v2 + 旧工程零回归;`BuildExportArgs` 切到 base + 平铺 overlay 链;`export_test.go` 矩阵覆盖(含 v0.5.0 兼容回归);`go test ./...` + `CGO_ENABLED=0 go test ./...` 双绿 | M2 完成 |
| **M4** 前端工程画布 UI | `CanvasSettingsDialog.vue` + 顶栏入口;`MultitrackPreview` 容器变成画布盒;新建工程默认 1920×1080;打开旧工程 Migrate 透传 canvas;改画布触发 dirty + autosave + 撤销栈 | M3 完成 |
| **M5** 前端 clip 变换 UI(变换框 + Inspector) | `TransformOverlay.vue`(8 手柄 + 中心拖拽 + Shift/Alt 修饰);`MultitrackInspector.vue`(数字输入 + 重置按钮);store `previewClipTransform` / `commitClipTransform` 草稿/提交模式;键盘箭头微调 | M4 完成 |
| **M6** 收尾 + 归档 | 用户手测清单全过(v0.5.0 旧工程零回归 + 新建 PIP 导出 + 改画布出界提示 + 撤销重做);`web/dist/` 重建;版本号 bump v0.5.1;归档 `git mv` + 主索引切换 + roadmap 加行 | M5 完成 |

> **回归基线**:M3 末跑 v0.5.0 的最后一个测试工程,导出结果与 v0.5.0 commit `6d739a5` 时**视觉相同**(允许字节差异 — Migrate 注入了 canvas/transform 默认值导致 filter 多 base 节点,但视觉结果一致)。如果坚持字节相同,可以为"v1 工程 + 默认值不变"加一条 fast path(N=1 全画布 → 走 v0.5.0 单轨直出),M3 决策。
