# 多轨剪辑器 — 产品设计

> 类 Premiere Pro 的多源 / 多轨 / 自由组合剪辑器,作为新 Tab 与单视频剪辑 Tab **共存**,共享底层时间轴交互与导出体验。
>
> **实现状态**:
> - v0.5.0 已发布 — 多轨基线(多源 / 多轨 / 跨轨拖动 / overlay 全屏 + amix);
> - v0.5.1 已发布 — **工程画布 + clip 变换**(自定义画布分辨率、每个 clip 的 (x, y, w, h),走真合成 overlay 链)。增量在 §12,基线章节(§1–§11)保留为 v0.5.0 表述,**v0.5.1 与基线冲突的条目已就地划线 + 注释**。
>
> **设计源头**:UI 风格、时间轴交互、播控、撤销栈、导出体验**全部**沿用 [单视频剪辑器 product.md](../editor/product.md);本文只描述**与单视频不同的部分**,公共部分跳转引用。
>
> 对应程序设计:[program.md](program.md)。共享导出体验见 [core/ui-system.md §6](../../core/ui-system.md)。

---

## 1. 目标与非目标

### 1.1 目标(MVP)

提供一个**自由组合的多轨剪辑器**:

- 用户**自建工程**(不再"一个视频 = 一个工程"),工程是空的容器
- 通过**素材库(Library)**导入任意数量的视频 / 音频文件作为 `Source[]`
- 视频素材进时间轴 → 自动建**视频轨 + 音频轨各一条**(若该 source 有音轨)
- 纯音频素材进时间轴 → 自动建**音频轨一条**
- 用户可**自由新增空轨道**(视频轨 / 音频轨)
- Clip **可跨轨道拖动**(视频↔视频,音频↔音频;不可跨类型)
- 时间轴上的 split / delete / trim / 范围选区 / 撤销重做 **沿用单视频的全部交互**
- 单次 ffmpeg 导出:视频轨之间 `overlay` 合成(z-order 按轨号)、音频轨之间 `amix` 混合

### 1.2 非目标(本版本不做)

- 转场(淡入淡出、擦除)、关键帧动画、调色、滤镜
- 文字 / 贴纸 / 水印
- ~~视频 PiP 的 `position` / `scale` / `opacity` 字段(占位:轨号代表 z-order,所有视频轨默认全屏覆盖;PiP 留给 v2)~~ — **v0.5.1 已提前,见 §12**(只做 position + scale,opacity / 旋转仍留 v2)
- 帧级精度预览(沿用单视频 MVP 的关键帧对齐方案)
- 轨道编组、嵌套序列、嵌套工程
- 跨工程素材引用(每个工程的 Source 列表是独立的)
- 工程导入 / 导出(分享给他人)

### 1.3 与单视频 Tab 的关键差异

| 维度 | 单视频剪辑 | 多轨剪辑 |
|------|-----------|----------|
| 工程入口 | 选一个视频文件 → 自动建工程 | 用户**显式新建**空工程 → 后续向工程导入素材 |
| 素材模型 | `source: Source`(单一) | `sources: Source[]`(N 个) |
| 时间轴轨道 | 固定 1 视频轨 + 1 音频轨 | 自由 N 视频轨 + M 音频轨,可增减 |
| Clip 来源 | 全部来自唯一 source | 每个 clip 带 `sourceId` 指向 N 个 source 之一 |
| 跨轨拖动 | 不允许(只能在自己轨内重排) | **允许**(但限同类型轨道间) |
| 导出 filter | 视频轨 concat → `[v]`;音频轨 concat → `[a]` | 每条视频轨独立 concat → 顶层 `overlay` 链;每条音频轨独立 concat → `amix` |
| 工程持久化 | `~/.easy-ffmpeg/projects/` | `~/.easy-ffmpeg/multitrack/`(独立目录) |
| 预览策略 | 单 `<video>` + 间隙时钟 | 见 §6;多源切换 + 多视频轨叠加靠 CSS 层叠 |

UI 整体配色、控件家族、模态约定、播控外观**与单视频完全一致**(见 [ui-system.md](../../core/ui-system.md))。

---

## 2. 核心概念

### 2.1 工程(Project)

```text
Project
├── id           uuid8
├── name         "My Vacation Cut"
├── createdAt / updatedAt
├── sources[]    导入的素材列表(每条 = 一个本地文件 + ffprobe 出来的元信息)
├── videoTracks[] 一条或多条视频轨
├── audioTracks[] 一条或多条音频轨
├── audioVolume  全局音量(沿用单视频)
└── export       导出设置
```

工程独立于素材文件:用户可以建一个空工程、稍后导入素材;也可以删掉工程不影响磁盘上的源文件。

### 2.2 素材(Source)

```text
Source
├── id           uuid8(工程内唯一)
├── path         "D:/videos/clip01.mp4"
├── kind         "video" | "audio"  (有视频流→video,纯音频→audio)
├── duration / width / height / videoCodec / audioCodec / frameRate / hasAudio
└── thumbnail    可选:首帧缩略图(M5+ 才考虑)
```

素材**只是元信息引用**,文件本身不复制到工程目录。源文件丢失 → 该素材标"⚠ 文件未找到",依赖它的 clip 全部禁用。

### 2.3 轨道(Track)

```text
VideoTrack { id, label?, locked, hidden, clips[] }
AudioTrack { id, label?, locked, muted, volume, clips[] }
```

- `locked`:锁定后该轨所有 clip 不响应 split / delete / drag(占位字段,M9+)
- `hidden`:视频轨隐藏时预览不显示该轨,导出**仍参与 overlay**(占位字段,M9+)
- `muted`:音频轨静音时预览静音,导出**仍参与 amix**(占位字段,M9+)
- `volume`:音频轨独立音量(沿用单视频"轨音量"模型,默认 1.0)

MVP 阶段 `locked / hidden / muted` 在 UI 上预留按钮但**功能可缺省占位**,保证后续不破坏 schema。

### 2.4 Clip

```text
Clip { id, sourceId, sourceStart, sourceEnd, programStart }
```

完全沿用单视频 Clip 模型,只多一个 `sourceId` 指向 `sources[]` 中的某条。

---

## 3. UI 布局

### 3.1 整体布局

#### 3.1.1 布局区域(栅格)

按"1 行顶栏 + 15 行剪辑区"的栅格描述区域(注意:这里只是**比例**,项目并未引入 Bootstrap;实现仍走 Tailwind flex,见 [program.md](program.md)):

```text
┌──────────────────────────────────────────────────────────────────────────┐
│  Region: TOP-BAR                                       (rows 1, cols 15) │
│  [新建工程] [工程列表] [关闭工程] ………………………………………………………… [导出]    │
├──────────────────┬───────────────────────────────────────────────────────┤
│ Region: LIBRARY  │  Region: PREVIEW                  (rows 2-8, cols 12) │
│  (rows 2-16,     │  ┌─────────────────────────────────────────────────┐ │
│   cols 3)        │  │ MultitrackPreview(<video> + 多 <audio>)         │ │
│                  │  │                                                 │ │
│  - 素材列表      │  │                                                 │ │
│  - + 导入        │  └─────────────────────────────────────────────────┘ │
│  - 每条素材点    │  PlayBar:⏮ ⏸ ▶ ⏭   00:14 / 00:48                   │
│    "+ 添加"      ├───────────────────────────────────────────────────────┤
│    建轨         │  Region: TRACKS                   (rows 9-16, cols 12) │
│                  │  ┌─────────────────────────────────────────────────┐ │
│                  │  │ TimelineRuler                                   │ │
│                  │  │ V1 / V2 / … 视频轨                               │ │
│                  │  │ A1 / A2 / … 音频轨                               │ │
│                  │  └─────────────────────────────────────────────────┘ │
│                  │  MultitrackToolbar:[✂分割][🗑删除][↶撤销][↷重做] … │
└──────────────────┴───────────────────────────────────────────────────────┘
```

| Region | 行 | 列 | 内容 | 实现位置 |
|--------|----|----|------|---------|
| **TOP-BAR** | 1 | 1–15 | 工程生命周期 / 导出入口 | [MultitrackView.vue](../../../web/src/views/MultitrackView.vue) `<!-- Region: TOP-BAR -->` |
| **LIBRARY** | 2–16 | 1–3 | 素材库栏(可折叠) | [MultitrackLibrary.vue](../../../web/src/components/multitrack/MultitrackLibrary.vue) |
| **PREVIEW** | 2–8 | 4–15 | 预览区 + PlayBar(剪辑区上半,占 0.5) | [MultitrackPreview.vue](../../../web/src/components/multitrack/MultitrackPreview.vue) + [PlayBar.vue](../../../web/src/components/timeline-shared/PlayBar.vue) |
| **TRACKS** | 9–16 | 4–15 | 时间轴(标尺 + 多轨) + Toolbar(剪辑区下半,占 0.5) | [MultitrackView.vue](../../../web/src/views/MultitrackView.vue) timeline 容器 + [MultitrackToolbar.vue](../../../web/src/components/multitrack/MultitrackToolbar.vue) |

**TOP-BAR 内排布**:

| 位置 | 控件 | 行为 |
|------|------|------|
| 左 | `新建工程` | 弹出名称输入,新建空工程 |
| 左 | `工程列表` | 弹出工程列表模态 |
| 左 | `关闭工程`(`v-if="hasProject"`) | flush 保存后关闭 |
| 右 | `●` 脏标(`v-if="hasProject && dirty"`) | 提示有未保存改动 |
| 右 | `导出`(`v-if="hasProject"`) | 走 dryRun → 命令预览 → 真实导出 |

**剪辑区内部比例**:LIBRARY : (PREVIEW + TRACKS) = **3 : 12**(20% / 80%);PREVIEW : TRACKS = **0.5 : 0.5**(各占剪辑区竖向一半,内部 PlayBar / Toolbar 取自然高度,挤占归属区域)。

**ExportSidebar**(导出期右栏)是**条件性叠加层**,不在固定栅格内;`exportSidebarOpen` 时从 `Region: TRACKS` 右侧切出一列(沿用单视频导出体验)。

#### 3.1.2 低保真示意图

```text
┌─ 多轨剪辑 Tab ───────────────────────────────────────────────────────────────┐
│ [📁 新建工程] [📋 工程列表] 工程名[My Cut___]                  [导出 ▼]      │ ← 顶栏(沿用单视频风格)
├──────────┬──────────────────────────────────────────────────────────────────┤
│ 📦素材库  │                                                                  │
│ ━━━━━━━ │              ┌────────────────────────────────┐                   │
│ + 导入   │              │                                │                   │
│          │              │         预览窗口                │                   │
│ ▶ 🎬 vid1│              │   多轨叠加显示:V1(轨号小、 │ ← 预览区(中央)
│   1080p  │              │   时间轴顶行)→ 顶层 z       │   (单 / 多视频元素)
│   12.3 s │              │                                │
│          │              └────────────────────────────────┘                   │
│ ▶ 🎬 vid2│                                                                  │
│   720p   │       ⏮ ⏸ ▶ ⏭   00:14.500 / 00:48.200                            │ ← 播控(沿用单视频)
│   8.0 s  │                                                                  │
├──────────┼──────────────────────────────────────────────────────────────────┤
│ ▶ 🔊 aud1│ 标签 │ 操作 │ 0:00      0:15      0:30      0:45      1:00        │ ← 时间轴标尺
│   stereo │──────┼──────┼─────────────────────────────────────────────────────│
│   30.0 s │🎬 V1 │ [+轨]│ ┌─vid1──┐ ┌──vid1──┐  ┌──vid2──┐                  │ ← 视频轨 V1(轨号小,顶层 z;默认)
│          │🎬 V2 │ [+轨]│            ┌──vid2──┐                              │ ← 视频轨 V2(轨号大,底层 z)
│ (空白)   │🔊 A1 │ 1.0  │ ▂▂▂▂vid1▂▂▂▂vid1▂▂  ▂▂▂▂▂vid2▂▂▂▂                │ ← 音频轨 A1(随 V1 自动建)
│          │🔊 A2 │ 1.0  │              ▂▂aud1▂▂▂▂▂▂▂▂▂                      │ ← 音频轨 A2(单独导入纯音频)
├──────────┴──────┴──────┴─────────────────────────────────────────────────────┤
│  [✂ 分割]  [🗑 删除选中]  [↶撤销]  [↷重做]   [+🎬视频轨][+🔊音频轨]  缩放 [━●━] │ ← 工具条
└──────────────────────────────────────────────────────────────────────────────┘
```

**与单视频的视觉差异**:

- 左侧多了一个**素材库栏**(类似 VSCode 左侧栏,固定宽 240px,可折叠到 0px)
- 时间轴可以有**任意条**视频轨和音频轨,垂直方向可滚动
- 工具条多了"+视频轨" / "+音频轨"两个按钮
- 顶栏左侧"📂 打开视频"换成"📁 新建工程",名字改一下,行为也变成弹一个简易"工程名 + 确认"
- 其他**全部相同**:配色、按钮风格、播控、范围选区、模态约定

### 3.2 顶栏

| 组件 | 交互 |
|------|------|
| `📁 新建工程` | 弹一个 mini 模态(工程名输入 + 确认),后端 `POST /api/multitrack/projects` 建空工程并切换到它 |
| `📋 工程列表` | 弹"历史工程"模态(沿用单视频的 `EditorProjectsModal` 设计 + skin),按 `updatedAt` 倒序;点条目切换;每条带删除 |
| 工程名输入框 | 同单视频:右侧显示 / 修改 / 失焦自动保存 |
| `导出 ▼` | 同单视频:格式 / 编码 / 输出目录 / 文件名;点"开始导出"走 dryRun → 命令预览 → 真实执行 |

### 3.3 素材库栏

```text
┌─📦 素材库 ───────────┐
│ [+ 导入素材]   [⇆]   │  ← 顶部:导入按钮 + 折叠/展开(⇆ 把整个栏折叠成 36px 窄条)
├─────────────────────┤
│ 🎬 vacation.mp4     │
│    1920×1080 · h264 │  ← 每条:类型图标 / 文件名 / 元信息
│    ▶ 12.345s        │  ← 双击 = 在预览框播试听(独立预览,不进时间轴)
│    [拖入时间轴 ↘]    │
├─────────────────────┤
│ 🎬 b-roll.mp4       │
│    1280×720 · h264  │
│    ▶ 8.0s           │
├─────────────────────┤
│ 🔊 bgm.mp3          │
│    stereo · mp3     │
│    ▶ 30.0s          │
└─────────────────────┘
```

| 交互 | 行为 |
|------|------|
| `+ 导入素材` | 打开 `Picker`(复用现有全局对话框),允许选**多个**文件;后端逐个 `ProbeVideo` 写入 `sources[]`,前端立刻渲染 |
| 双击素材卡 | 在预览框**独立**播试听(不进时间轴,不改播放头);仅作"看清这是什么"用 |
| 拖动素材卡 → 时间轴空白处 | **自动建轨 + 落 clip**:视频素材建 V轨 + A轨各一条,纯音频建 A轨一条;落点时间 = 鼠标释放处的节目时间 |
| 拖动素材卡 → 时间轴现有轨 | 在该轨上落 clip;视频素材落 V轨时**同步**在最近的 A轨建对应音频 clip(若 source.hasAudio);落点时间 = 鼠标释放处 |
| 右键素材卡 | 菜单:`重命名(只改 label,不改文件) / 移除素材(检查无 clip 引用才能删) / 在文件管理器中显示` |
| 源文件丢失 | 卡片标 ⚠ 红边,所有相关 clip 在时间轴标灰禁用;导出时报错 |

**折叠**:按 ⇆ 折叠到 36px 窄条只显示图标列,按一下还原;状态记到 `localStorage`(MVP 不必落工程文件)。

### 3.4 预览区

沿用单视频:`<video>` 元素 + 自定义播控 + 双击全屏。**唯一不同**是预览策略(见 §6),DOM 层面可能不只一个 `<video>`。

### 3.5 时间轴

#### 3.5.1 视觉与单视频差异

- **轨道列**:从两条固定(V/A)→ 任意 N 条;垂直方向滚动条;轨道顺序**视频在上、音频在下**(分两组),每组内**轨号小的在上面**(z-order 高,导出时遮挡轨号大的);用户对"列表顶端 = 主轨"的直觉与 Premiere 风格的"V2 在 V1 上方"相反,这里用前者(简化心智)
- **轨道标签列**:每条轨显示 `🎬 V<n>` / `🔊 A<n>`,以及 `锁定 / 隐藏 / 静音 / 删除轨` 的小图标(MVP 阶段先只放"删除轨";其余按钮 v2 再加)
- **轨道动作列**:视频轨为空(占位);音频轨沿用单视频的"音量按钮"(`音量: 100%` / 浮窗滑块,仍是 0–200% gain)
- **Clip 块**:多了一个 `sourceId` 来源色标(每个 source 分配一个固定色,clip 左边缘画一条 4px 色条,让用户一眼识别"这是 vid1 还是 vid2")
- **+ 加轨按钮**:轨道标签列底部空白处常驻 `+视频轨` / `+音频轨` 两个按钮

#### 3.5.2 交互(沿用单视频 + 跨轨拖动)

**沿用单视频的全部交互清单**(见 [editor/product.md §3.5.3](../editor/product.md))。**新增**或**变化**:

| 动作 | 操作 | 行为 |
|------|------|------|
| 拖动 clip(同轨内) | 鼠标按住 clip 中间拖动 | 沿用单视频:不允许重叠,松开 snap |
| **拖动 clip(跨轨)** | 鼠标按住 clip 拖动到**另一条同类型轨道** | 离开当前轨进入"自由飞行"态;放下时 clip 移动到新轨同时间位置;**视频→音频禁止**(用 `cursor:not-allowed` 提示,松开恢复原位) |
| **拖动素材→时间轴** | 见 §3.3 | 落 clip + 必要时建轨 |
| 新增空视频轨 | 工具条 `+🎬视频轨` | `videoTracks.push({})`;轨号 = 现有最大 + 1 |
| 新增空音频轨 | 工具条 `+🔊音频轨` | 同上 |
| 删除轨 | 轨道标签列 🗑 | 该轨非空 → 二次确认;空 → 直接删除 |
| split / delete / range / undo / redo | 同单视频 | **跨多轨**:`splitScope = "all"`(刻度尺命中)在所有视频轨 + 所有音频轨同时切;`splitScope = "video"`(命中视频区)只切所有视频轨;`splitScope = "audio"` 同理只切音频区;`splitScope = "track:<id>"`(命中具体某轨)只切该轨 |

**splitScope 升级**:从单视频的 `"both" / "video" / "audio"` 扩展到 `"all" / "video" / "audio" / "track:<id>"`。播放一次后永久回到 `"all"`(沿用单视频"播放后回到 both"的语义)。

### 3.6 工具条

沿用单视频 + 两个新增按钮(`+🎬视频轨` / `+🔊音频轨`)。

### 3.7 导出对话框 / 导出期 UI

**完全沿用单视频**:格式 / 编码 / 输出目录 / 文件名;dryRun → 命令预览 → 真实执行;期间侧栏 `JobLog` + 进度条;阻断编辑遮罩。差异仅在后端 filter graph(见 [program.md](program.md))。

---

## 4. 数据模型

### 4.1 Project JSON schema(MultitrackSchemaVersion=1)

```jsonc
{
  "schemaVersion": 1,
  "kind": "multitrack",         // 与单视频工程区分
  "id": "a1b2c3d4",
  "name": "My Multi Cut",
  "createdAt": "2026-04-30T10:00:00+08:00",
  "updatedAt": "2026-04-30T10:30:00+08:00",

  "sources": [
    {
      "id": "s1",
      "path": "D:/videos/vacation.mp4",
      "kind": "video",
      "duration": 12.345, "width": 1920, "height": 1080,
      "videoCodec": "h264", "audioCodec": "aac",
      "frameRate": 29.97, "hasAudio": true
    },
    {
      "id": "s2",
      "path": "D:/audio/bgm.mp3",
      "kind": "audio",
      "duration": 30.0,
      "audioCodec": "mp3", "hasAudio": true
    }
  ],

  "audioVolume": 1.0,            // 全局音量(沿用单视频)

  "videoTracks": [
    {
      "id": "vt1",
      "clips": [
        { "id": "c1", "sourceId": "s1", "sourceStart": 0.0, "sourceEnd": 8.0,  "programStart": 0.0 },
        { "id": "c2", "sourceId": "s1", "sourceStart": 4.0, "sourceEnd": 12.0, "programStart": 12.0 }
      ]
    }
  ],
  "audioTracks": [
    {
      "id": "at1",
      "volume": 1.0,
      "clips": [
        { "id": "c3", "sourceId": "s1", "sourceStart": 0.0, "sourceEnd": 8.0,  "programStart": 0.0 }
      ]
    },
    {
      "id": "at2",
      "volume": 0.6,
      "clips": [
        { "id": "c4", "sourceId": "s2", "sourceStart": 0.0, "sourceEnd": 30.0, "programStart": 0.0 }
      ]
    }
  ],

  "export": {
    "format": "mp4",
    "videoCodec": "h264",
    "audioCodec": "aac",
    "outputDir": "D:/output",
    "outputName": "my_multi_cut"
  }
}
```

**与单视频工程的区别**:

- 加 `kind: "multitrack"` 字段(以及单视频隐式 `kind: "single"`,迁移时补默认值)
- `source: Source` → `sources: Source[]`,每个 Source 加 `id` / `kind`
- `videoClips / audioClips` → `videoTracks[].clips / audioTracks[].clips`
- 单视频的 `audioVolume`(全局)沿用;**新增**音频轨级 `volume`(独立于全局)

### 4.2 存储路径

| 内容 | 路径 |
|------|------|
| 索引 | `~/.easy-ffmpeg/multitrack/index.json` |
| 工程文件 | `~/.easy-ffmpeg/multitrack/<timestamp>_<id>.json` |

**与单视频独立**(`~/.easy-ffmpeg/projects/`),避免混在同一个目录里、避免 schema 冲突、避免误操作。两个 Tab 各管各的工程列表。

---

## 5. 前端状态管理

### 5.1 状态模型(`useMultitrackStore`)

```ts
state = {
  project: { ... } | null,    // 见 §4.1
  dirty: false,
  selection: ["c2"],          // 选中的 clip id
  rangeSelection: { start, end } | null,
  splitScope: "all" | "video" | "audio" | `track:${string}`,
  playhead: 0,
  playing: false,
  pxPerSecond: 8,
  libraryCollapsed: false,
  draggingSource: null | { sourceId, mouseStart }, // 素材库拖动时的瞬态
}
```

### 5.2 撤销 / 重做、自动保存

完全沿用单视频:

- 历史栈深度 100;拖动结束 / split / delete / 改名等操作 push 一次
- 每次 commit 后 `dirty=true` + debounce 1.5s 触发 `PUT /api/multitrack/projects/<id>`

---

## 6. 预览实现

**关键技术决策**(M2 锁定 v1 单 `<video>` 顶层近似;v0.5.1 推进到多 `<video>` CSS 层叠真合成,即原 v2 方案落地):

### 6.1 v0.5.1 方案:**多 `<video>` CSS 层叠 + 独立 `<audio>` 通道**

- 视频轨 → **每条轨一个独立的 `<video>` DOM 元素**(`v-for="(t, i) in videoTracks"`),`position: absolute` + `z-index = N - i`(轨号小 → z 顶,与导出 overlay 链顺序、与时间轴 UI 列表顶端一致)
- 每个 `<video>` 由对应轨的 `videoActive(track, t)` 独立驱动:`src` / `currentTime` / `play` 状态都是 per-track。空轨道时 `v-show:false` 隐藏,不画过期帧
- `<video>` 在 canvas-box 内的位置/尺寸由该轨当前 active clip 的 `transform` (X, Y, W, H) 转成百分比驱动,与导出端的 `scale=W:H` + `overlay=x:y` 几何一致(像素级 vs ffmpeg 的不一致只可能来自浏览器解码精度)
- **时钟仲裁**:顶层(轨号最小、有 active clip 的)`<video>` 是 master,其原生 `timeupdate` 推进 playhead;其余 active 轨道是 follower,通过 rAF 校正 currentTime(漂移 > 0.08s 时一次性写回);master 切换(顶层 clip 结束、下一条轨道接管)由 `topVideoActiveIndex` 反应式驱动,core 自动 detach 旧 master + attach 新 master
- 多音频轨:**多个独立 `<audio>` 元素**(每条音频轨一个),用 WebAudio `GainNode` 接到混合输出;每条轨独立 `currentTime` 同步
- gap clock:沿用单视频(预览端遇到所有视频轨都空 → 黑底 canvas-box + rAF 推进 playhead)
- **画布背景**:canvas-box 用 photoshop 风格的灰度棋盘格(#1f1f1f / #2b2b2b 16px 交替)填底,让画布范围在外层黑色 shell 中清晰可辨,clip transform 没占满的区域露出棋盘

**代价**:多 `<video>` 元素之间没有帧精确同步——浏览器各自解码,follower 通过 rAF 校正 currentTime,理论漂移在 50–100ms 量级。**这是预览近似;导出仍由 ffmpeg overlay 帧精确合成**。

### 6.2 v3(远期):Canvas + WebCodecs

- 帧精确预览;不在 v0.5.1 范围。需要离屏解码 + `requestVideoFrameCallback` + 自绘合成;复杂度大,目前不立项

详细方案权衡见 [program.md §6](program.md)。

---

## 7. 导出命令构建

**[program.md §5](program.md) 详述**。这里只给出概念:

```text
ffmpeg -y -i <s1> -i <s2> -i <s3>
       -filter_complex
       "
        # 视频轨 V1 = s1.video concat
        [0:v]trim=...[v1_0]; [0:v]trim=...[v1_1]; [v1_0][v1_1]concat=n=2:v=1[V1];

        # 视频轨 V2 = s1.video 第二段 concat
        [0:v]trim=...[v2_0]; [v2_0]concat=n=1:v=1[V2];

        # 视频叠加:V1 → 底,V2 overlay 上去
        [V1][V2]overlay=0:0[V];

        # 音频轨 A1 = s1.audio concat
        [0:a]atrim=...[a1_0]; [a1_0]concat=n=1:v=0:a=1[A1];

        # 音频轨 A2 = s2.audio concat + 轨级 volume
        [1:a]atrim=...[a2_0]; [a2_0]concat=n=1:v=0:a=1[A2_pre]; [A2_pre]volume=0.6[A2];

        # 多音频轨混合
        [A1][A2]amix=inputs=2:duration=longest:dropout_transition=0[A_pre];
        [A_pre]volume=1.0[A]
       "
       -map "[V]" -map "[A]"
       -c:v libx264 -c:a aac
       <outDir>/<name>.mp4
```

边界条件(短轨自动 pad、leading-gap 处理、单轨退化、所有轨都空报错)沿用单视频规则,扩展到多轨后见 [program.md](program.md)。

---

## 8. 交互细节与快捷键

完全沿用 [editor/product.md §8](../editor/product.md) 的快捷键表 — 只多一个:

| 快捷键 | 行为 |
|--------|------|
| `Ctrl + L` | 折叠 / 展开素材库栏(`libraryCollapsed`) |

---

## 9. 边界情况与错误处理

| 情形 | 处理 |
|------|------|
| 素材文件丢失 | 素材卡 ⚠;时间轴上引用该 source 的 clip 全标灰禁用;导出报错"以下素材未找到:..." |
| 工程没有任何素材 | 空工程合法;时间轴显示空态"导入素材开始剪辑"|
| 视频轨开头留空 | **导出报错**(沿用单视频规则:视频开头不可空);音频轨开头允许 leading-gap |
| 所有视频轨为空 | 允许;导出**只有音频**(`-vn` 或不 `-map [V]`) |
| 所有音频轨为空 | 允许;导出**只有视频** |
| 全空 | 导出按钮 disabled |
| 多视频轨完全重叠且最上面遮挡完全 | 合法(用户故意如此);预览只显示顶层 |
| clip 跨类型拖动尝试(视频→音频) | 鼠标 `cursor: not-allowed`;松开还原原位置 |
| 删除有 clip 引用的 source | 二次确认 + 列出引用清单;确认后**同步删除所有相关 clip** |
| 源视频浏览器不能预览(h265 等) | 同单视频:预览黑屏,导出仍可工作 |
| 导出过程中切走 Tab | 后端 job 继续;切回看见最新状态(沿用 useJobPanel) |

---

## 10. 与单视频 Tab 的能力对照表

| 能力 | 单视频 | 多轨 |
|------|--------|------|
| 配色 / 控件 / 模态 | ✓ | ✓ 相同 |
| 时间轴标尺 / 缩放 / 播放头 | ✓ | ✓ 沿用共享组件 |
| Clip split / delete / trim / 重排 / 范围选区 | ✓ | ✓ 沿用共享 composable |
| 撤销 / 重做(100 步) | ✓ | ✓ 相同模型 |
| 自动保存(debounce 1.5s) | ✓ | ✓ 相同 |
| 工程列表模态 | ✓ | ✓ 同 skin、独立索引 |
| 导出对话框 / dryRun / 命令预览 / 覆盖确认 | ✓ | ✓ 沿用共享对话框 |
| 导出期侧栏 + JobLog + 进度 | ✓ | ✓ 沿用共享组件 |
| 全局音量 (`audioVolume`,0–200%) | ✓ | ✓ 相同 |
| 音频轨独立音量 | ✗ | ✓ **新增**(`audioTracks[].volume`) |
| 多源 / 多轨 / 跨轨拖动 | ✗ | ✓ **核心新增** |
| 素材库 | ✗ | ✓ **核心新增** |
| 视频叠加 / overlay | ✗ | ✓ 导出端真合成 + 预览端 v0.5.1 多 `<video>` 层叠近似真合成 |
| PiP / 位置 / 缩放 / 不透明度 | ✗ | ⏳ v2 |
| 转场 / 调色 / 关键帧 | ✗ | ✗ |

---

## 11. 范围裁剪与未来演进

### 11.1 v1 必须有的

- 多源导入(素材库)
- 多视频轨 / 多音频轨 + 跨轨拖动(同类型)
- 时间轴所有现有交互在多轨模型上跑通
- 导出 v1:`overlay`(z-order 按轨号,默认全屏覆盖) + `amix`
- 工程独立持久化、独立索引

### 11.2 留给后续版本

- ~~v2:视频 PiP(`position` / `scale` / `opacity` / 旋转)+ 多 `<video>` 同步预览~~ — **v0.5.1 提前其中的 position / scale**(见 §12);opacity / 旋转 / 多 `<video>` 同步预览仍留 v2
- v3:转场(crossfade / fade)、关键帧动画、调色
- v4:嵌套序列、轨道编组、模板系统
- 远期:WebCodecs 帧精确预览

---

## 12. v0.5.1 增量 — 工程画布 + clip 变换

### 12.1 动机

v0.5.0 多轨的视频叠加是"全屏 overlay":每个 clip 在导出端被 `scale + 居中 pad` 撑满整个画布(画布 = `max(W) × max(H)`),然后逐轨 `overlay=0:0` 叠。**结果是上层轨道完全遮挡下层** —— 多轨在视觉上等价于"只看最上层轨道",看不到下层。

实际剪辑场景需要**画中画(PiP)**:同一画布上小窗口叠在主画面角落、双机位分屏、两路并排访谈、字幕/角标轨等。这些都需要:
1. **工程级画布**:用户决定输出分辨率(常见 1920×1080 / 3840×2160 / 1080×1920 竖屏 / 自定义),不再被动取 `max(sources)`
2. **每个 clip 在画布上的位置与尺寸**:四个数 `(x, y, w, h)`,表示 clip 渲染到画布的哪个矩形区域

### 12.2 目标(v0.5.1)

- 工程文件携带 `canvas: { width, height, frameRate }`,默认值 = v0.5.0 那套 max() 计算结果(打开旧工程零回归)
- 顶栏新增"画布设置"按钮,弹模态调画布 `W × H × FR`
- Clip 数据结构加 `transform: { x, y, w, h }`,默认值 = 全画布(`0, 0, canvasW, canvasH`,等价 v0.5.0 行为)
- **预览框**变成一个**画布尺寸的容器**(等比缩放进可视区,保持画布纵横比,留黑边):
  - 选中某个 clip → 预览框上叠加**变换框**:8 个手柄(4 角 + 4 边中点)+ 中心拖拽,实时改 `(x, y, w, h)`
  - Shift 锁纵横比拖角;Alt 以中心缩放;方向键微调 1 px,Shift+方向键 10 px
  - 取消选中 → 变换框消失
- **属性栏 / Inspector**(右栏新增 "属性" tab,与 "导出" 共用右栏空间;非导出期默认折叠):
  - 工程级:`画布 W × H × FR`,数字输入 + 预设按钮(`1080p / 4K / 竖屏 9:16 / 当前源最大值`)
  - 选中 clip:`位置 X / Y`、`尺寸 W / H`、"重置为全画布"按钮
- **导出**:每条视频 clip 单独 `scale → overlay=X:Y:enable='between(t, p_start, p_end)'` 平铺到 base 画布;**真合成**(下层在上层未覆盖区域可见)。`overlay` 走 alpha 兼容像素格式以让"出框 / 部分覆盖"区域透出底层
- 撤销栈:画布修改、变换修改各 push 一次(拖手柄期间不 push,松开 push 一次,沿用 clip 拖拽的"操作终结才入栈"约定)

### 12.3 非目标(v0.5.1)

- ❌ 透明度 `opacity` —— v2 再加(本期 alpha 仅来自"出框/未覆盖区域",clip 本身仍 100% 不透明)
- ❌ 旋转 `rotate` —— v2
- ❌ 关键帧动画(变换值随时间变化)—— v3
- ❌ clip 内裁切(crop):本期 `(x, y, w, h)` 描述的是"clip 在画布上的位置与显示尺寸",不是"clip 内取哪一块"。源帧整帧按 (w, h) 缩放到画布 (x, y) 处
- ❌ 真正帧精确的合成预览:多 `<video>` 同步预览仍是 v2 课题。本期预览端走"显示选中 clip 所在轨道顶层激活源"的近似(详见 [program.md §12](program.md))

### 12.4 数据模型变更

```diff
  Project
+ ├── canvas: { width, height, frameRate }   // 默认 = max(referenced video sources)
  ├── ...

  Clip
+ ├── transform: { x, y, w, h }              // 默认 = (0, 0, canvasW, canvasH)
  ├── ...
```

- `canvas`:整数像素 + 浮点帧率;约束 `W ≥ 16 && H ≥ 16 && FR ∈ [1, 240]`(超出报错)
- `transform`:整数像素;**允许出界**(部分/全部 X+W > canvasW 或 X<0,出界部分自然不可见,这是动态摆位的基础);**禁止 W ≤ 0 / H ≤ 0**(报错)
- SchemaVersion v1 → **v2**;Migrate 兜底:无 `canvas` 时按 v0.5.0 规则推算;无 `transform` 时填 `(0, 0, canvasW, canvasH)`

### 12.5 UI 变更详述

#### 12.5.1 顶栏(MultitrackTopBar)

```text
[新建] [列表] [关闭]   工程名[___]   ●        [画布: 1920×1080 30fps ▾]  [导出 ▾]
                                                ↑ 新增按钮(打开"画布设置"模态)
```

- 文字格式:`画布: {W}×{H} {FR}fps`,点击弹模态
- 模态内容:三个数字输入 + 预设按钮 + "确认/取消";点击预设直接填入输入框,确认才落工程
- 改画布**不影响**已有 clip 的 transform 数值(transform 是绝对像素,画布缩小后 clip 可能出界 —— 这是用户可以接受的,UI 提示一次"画布缩小可能让部分 clip 出界")

#### 12.5.2 预览框(MultitrackPreview)

```text
┌─ Region: PREVIEW ──────────────────────────┐
│  ┌────────────画布(等比缩放)─────────┐  │
│  │                                       │  │
│  │   ┌─ 选中 clip 的 transform 框 ─┐    │  │
│  │   │ □──────────□──────────□    │    │  │
│  │   │ │                       │    │    │  │
│  │   │ □         (拖动中心)    □    │    │  │
│  │   │ │                       │    │    │  │
│  │   │ □──────────□──────────□    │    │  │
│  │   └───────────────────────────────┘    │  │
│  │                                       │  │
│  └───────────────────────────────────────┘  │
└─────────────────────────────────────────────┘
```

- 容器外层:`flex` 居中,内层 `aspect-ratio: W/H`,自动等比适配
- 画布内正常播视频(本期仍走 v0.5.0 的"顶层激活源"近似)
- 选中 clip → 上叠绝对定位 `<div>` 表示变换框,边线 `accent` 色,8 个手柄是 12×12px 圆点
- 手柄 / 中心命中 → 进入拖拽状态,实时改 `transform`(本地 state 跟随鼠标),松开 push 一次撤销栈
- 鼠标在变换框外 + 画布内 → 不产生 clip 选中变化(避免误操作);切换选中靠时间轴

#### 12.5.3 Inspector(右栏属性面板)

非导出期默认隐藏 / 折叠到窄条;点击"画布: ..." 顶栏按钮或选中 clip → 自动展开(可手动 pin)。

```text
┌─ 属性 ───────────────┐
│ 画布                 │
│  宽   [1920]         │
│  高   [1080]         │
│  帧率 [30]           │
│  预设 [1080p][4K]... │
├──────────────────────┤
│ 选中 clip(若有)     │
│  X [  0]  Y [  0]    │
│  W [1920] H [1080]   │
│  [重置为全画布]      │
└──────────────────────┘
```

- 数字输入框失焦或回车提交,提交一次 push 一次撤销栈
- 拖手柄期间数字框跟随显示(只读),拖完同步可编辑
- "重置为全画布":一键 `(0, 0, canvasW, canvasH)`(常用于"我做完 PIP 想还原")

导出期(`exportSidebarOpen`)Inspector 让位给 ExportSidebar(沿用 v0.5.0 右栏切换约定)。

#### 12.5.4 时间轴

时间轴**完全不变**。clip 在时间轴上仍然只显示"时间区间 + source 色条",不可视化 (x, y, w, h)(那是空间坐标,不属于时间轴语义)。

### 12.6 边界与默认值

| 情形 | 处理 |
|------|------|
| 新建空工程 | `canvas` = `1920 × 1080 × 30`(无源时的安全默认);导入第一个视频源后**不**自动改画布(用户应该决定) |
| 打开 v0.5.0 工程(无 `canvas`) | Migrate:`canvas` = `max(referenced video sources)`;`transform` 全部填全画布。**视觉零回归** |
| 用户删光所有 clip,留空画布 | 画布保留(用户可能要再导素材) |
| Clip transform W/H ≤ 0 | UI 阻止(数字输入下限 1);后端 Validate 兜底报错 |
| Clip 完全出画布(`X+W ≤ 0` 或 `X ≥ canvasW` 等) | 合法(可能用户在做"飞入"摆位前的中间态);导出渲染零像素,UI 在 clip 上画"⚠ 不可见"角标 |
| Canvas 缩小导致旧 clip 出界 | 模态二次确认("以下 clip 部分/完全出画布");用户取消则不改 |
| 多个 clip 完全重叠且最上层全覆盖 | 合法(等价 v0.5.0 行为);下层不可见但参与 ffmpeg 编码(性能上不优,但语义清晰,用户可手动删 ) |

### 12.7 与单视频 Tab 的能力对照表(增量)

| 能力 | 单视频 | 多轨 v0.5.0 | 多轨 v0.5.1 |
|------|--------|------------|------------|
| 工程画布尺寸自定义 | ✗ | ✗(取 max sources) | ✓ |
| Clip 位置 (X, Y) | ✗ | ✗(全屏) | ✓ |
| Clip 尺寸 (W, H) | ✗ | ✗(撑满画布) | ✓ |
| 真合成(下层透出) | ✗ | ✗(上遮下) | ✓ |
| 透明度 / 旋转 | ✗ | ✗ | ✗(留 v2) |
| 关键帧动画 | ✗ | ✗ | ✗(留 v3) |

### 12.8 快捷键(增量)

| 快捷键 | 行为 |
|--------|------|
| 选中 clip + `←/→/↑/↓` | 变换框 X/Y 微调 1px |
| 选中 clip + `Shift + ←/→/↑/↓` | 微调 10px |
| 选中 clip + `Ctrl + 0` | 重置变换为全画布 |
| 拖手柄 + `Shift` | 锁纵横比 |
| 拖手柄 + `Alt` | 以中心缩放(对边联动) |

时间轴 / 播控 / 选中相关快捷键沿用 v0.5.0 不变。

### 12.9 验收标准

- 打开 v0.5.0 工程,导出结果与 v0.5.0 时**字节相同 / 视觉相同**(零回归)
- 新建工程,设画布 1920×1080,导入两段视频,放到两轨,改第二轨 clip 为 (1440, 810, 480, 270),导出 MP4 在右下角看到 PIP 小窗口
- 改画布到 3840×2160,clip 数值不变(右下角的小窗 480×270 比例变小),导出 4K 文件
- 撤销/重做能恢复变换数值
- 跨轨拖动 clip 不影响其 transform
- 单视频 Tab 视觉与交互零回归
