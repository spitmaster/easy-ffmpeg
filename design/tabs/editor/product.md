# 单视频剪辑器 — 产品设计

> 时间轴式单视频剪辑器(替代旧裁剪 Tab)。**实现状态**:✅ MVP 已实现(v0.3.0)。预览精度为"关键帧对齐"级别;proxy / WebCodecs 两种精度提升方案记录在 §7,尚未落地。
>
> 对应程序设计:[program.md](program.md)。共享导出体验见 [core/ui-system.md §6](../../core/ui-system.md)。

---

## 1. 目标与非目标

### 1.1 目标(MVP)

提供一个**类似 Premiere Pro 的简易剪辑器**,能对**单个视频**执行:

- 导入一个视频,自动解析成一条视频轨 + 一条音频轨
- 在时间轴上**分割**(split)成多个 clip
- 对 clip 进行**删除 / 重排 / 拖拽端点修改 in-out**
- 实时**预览**(精度 MVP 可接受 100-300ms;见 §7)
- 以**单次 ffmpeg 执行**导出为指定格式
- 把剪辑过程作为**工程(Project)**持久化,支持打开历史工程

### 1.2 非目标(本版本不做,留给后续专用 Tab)

- 多素材剪辑(素材库、多段视频拼接)
- 多轨道叠加 / PiP / 画中画 / 绿幕
- 转场(淡入淡出、擦除等)
- 音量包络 / 关键帧动画 / 调色
- 滤镜(模糊、锐化、色彩调整)
- 文字 / 贴纸 / 水印
- 帧级精度(先用原生 seek,接受关键帧对齐误差)
- 云同步 / 账号 / 协作

---

## 2. 总体设计决策

### 2.1 替换旧 trim Tab

旧的"视频裁剪"Tab 整体删除。crop / scale 能力在本 Tab **不提供**(业务上 90% 的剪辑需求是"切掉不想要的段",crop/scale 属于"预处理")。

`service.ProbeVideo` **保留**,剪辑器复用。

### 2.2 单视频 · 单工程 · 双轨(视频 / 音频独立编辑)

- 一个工程恰好关联一个源视频文件(`source.path`)
- 时间轴上展示两条**独立**轨道:视频轨 + 音频轨(源无音轨则音频轨为空)
- 每条轨道有自己的 `clips[]`(`videoClips` / `audioClips`),split / 删除 / 重排 / 修剪均**独立作用于单轨**
- 导出时两轨各自 concat 到 `[v]` / `[a]` 再 mux

**分割范围(splitScope)**:

| 用户操作 | 当前 splitScope | 按 `S` / 分割按钮后 |
|---------|-----------------|--------------------|
| 点击刻度栏 | `both` | 两轨同时在播放头分割 |
| 点击视频轨空白 | `video` | 仅视频轨分割 |
| 点击音频轨空白 | `audio` | 仅音频轨分割 |
| 点击视频轨内的 clip | `video` | 同上 |
| 点击音频轨内的 clip | `audio` | 同上 |

这个设计保留了"有两条轨道"的直观,同时让剪辑操作真正解耦 —— 用户可以保留背景音乐的完整节奏,只切视频画面。**预览窗** 跟随**视频轨**为主时间线,音频编辑仅在导出阶段体现(单个 `<video>` 元素的限制)。

### 2.3 工程持久化:每工程一个 JSON 文件

**放弃 sqlite 的理由**:

- 当前 Go 二进制 `CGO_ENABLED=0`,引入 `mattn/go-sqlite3` 需要 cgo;改用纯 Go 的 `modernc.org/sqlite` 会让二进制膨胀 4-6 MB(当前产物仅 35 MB)
- 剪辑工程的典型查询是"按时间倒序列出所有工程" + "按 id 取一个工程",这两类查询用**文件系统原生机制**已足够

**存储方案**:

```text
~/.easy-ffmpeg/
├── bin-<hash>/                    已有:解压的 ffmpeg
└── projects/                      新增
    ├── index.json                 轻量索引(列表页面用)
    ├── 2026-04-23_14-32-10_a1b2c3d4.json
    ├── 2026-04-23_09-15-00_e5f6a7b8.json
    └── ...
```

- 每个工程一个独立 JSON 文件,**文件大小与工程总数无关**
- 文件名:`YYYY-MM-DD_HH-MM-SS_<uuid8>.json`(时间前缀让目录按名字排序 = 按创建时间排序)
- `index.json` 缓存 `[{id, name, source.path, updatedAt, thumbnail?}]`,打开"剪辑记录"面板时直接读
- 索引损坏 / 缺失 → 按扫描 `projects/*.json` 重建(自愈)
- 单工程 JSON 典型大小:< 10 KB(几十个 clip × 每 clip < 100 字节)

### 2.4 预览实现:三阶段演进

见 §7。**MVP 用原生 `<video>` + `currentTime` 跳转**,接受关键帧对齐误差;后续再引入 **proxy file** 和 **WebCodecs**。

### 2.5 编辑器代码独立成模块

`editor/` 顶级包自成一体(详见 [program.md](program.md)),不依赖 `server/handlers*.go` 的具体实现,通过接口向主程序索取能力。未来可以把 `editor/` 单独编译成 `cmd/easy-editor/` 出一个 exe。

---

## 3. UI 布局

### 3.1 整体结构

```text
┌─ 单视频剪辑 Tab ──────────────────────────────────────────────────────┐
│  [📂 打开视频]  [📋 剪辑记录]  工程名[My Edit______]   [导出 ▼]    │ ← 顶栏
├───────────────────────────────────────────────────────────────────┤
│                                                                   │
│                   ┌─────────────────────────────┐                 │
│                   │                             │                 │
│                   │        预览窗口             │                 │
│                   │        <video> 元素         │                 │ ← 预览区
│                   │                             │                 │
│                   └─────────────────────────────┘                 │
│                                                                   │
│   ⏮ ⏸ ▶ ⏭   00:12.340 / 01:23.456                               │ ← 播控
├───────────────────────────────────────────────────────────────────┤
│ 标签 | 操作 |  0:00   0:15   0:30   0:45   1:00   1:15            │ ← 三列:标签 / 动作 / 滚动区
├──────┼──────┼─────────────────────────────────────────────┤
│🎬视频│      │ ┌─────┐ ┌─────────┐ ┌───────┐               │ ← 视频轨
│      │      │ │clip0│ │  clip1  │ │ clip2 │  ◆ ━━━ (大游标) │
│      │      │ └─────┘ └─────────┘ └───────┘               │
│🔊音频│音量: │ ▂▂▂▂▂ ▂▂▂▂▂▂▂▂▂ ▂▂▂▂▂▂▂                   │ ← 音频轨
│      │125% ▾│                                              │
├──────┴──────┴─────────────────────────────────────────────┤
│  [✂ 分割]  [🗑 删除选中]  [↶撤销]  [↷重做]   缩放 [━━●━━]         │ ← 工具条
└───────────────────────────────────────────────────────────┘
```

### 3.2 顶栏

| 组件 | 交互 |
|------|------|
| `📂 打开视频` | 打开文件浏览器模态框(复用现有 `Picker`)→ 选中后自动新建工程 |
| `📋 剪辑记录` | 弹出"历史工程"模态框(见 §3.6) |
| 工程名输入框 | 右侧显示当前工程名,用户可改;失焦自动保存 |
| `导出 ▼` | 下拉菜单:格式 mp4/mkv/mov/webm;点"开始导出"弹确认 |

### 3.3 预览区

- `<video>` 元素,`preload="auto"`,不显示原生控制条(用自定义播控)
- 默认最大宽 960px,居中;保持源视频宽高比
- 双击预览 = 切换全屏

### 3.4 播控条

| 按钮 | 快捷键 | 行为 |
|------|--------|------|
| ⏮ 上一 clip | `←` | 播放头跳到当前/上一 clip 起点 |
| ⏸/▶ | `Space` | 播放 / 暂停(基于"节目时间"的播放器) |
| ⏭ 下一 clip | `→` | 播放头跳到下一 clip 起点 |
| 时间码 | — | 显示 `节目时间 / 节目总长`,如 `00:12.340 / 01:23.456` |

> 音量按钮**已移除** —— 音量改为音频轨级属性,详见 §3.5 时间轴的"音频轨音量"控件。

### 3.5 时间轴

#### 3.5.1 视觉组成

- **时间刻度**:顶部水平尺,刻度数随缩放变化(`1px/s → 20px/s`)
- **视频轨**:一条 40px 高的 DOM 容器,上面是一个或多个 clip 块
- **音频轨**:30px 高,显示波形 SVG(MVP 可以只显示纯色块)
- **音频轨音量**:时间轴布局加了一列**轨道动作列**(`.timeline-track-actions`),列宽 88px。音频行放一个**文字按钮**直接显示形如 `音量: 100%` / `音量: 125%` 的实时读数:
  - 点击按钮在按钮**附近**弹一个 124px 宽浮窗 —— 顶端是"音频音量"标题,主体是 160px 高的垂直滑块,左侧并排 200%/100%/0% 三段刻度,底下一条分割线 + 大号 accent 色读数实时跟随。**0–200%** 范围(boost 上限 2.0)
  - 浮窗用 `position: fixed`(z-index 60)躲开 `.editor-timeline { overflow: hidden }` 的裁切;打开时按按钮 `getBoundingClientRect()` 算坐标,优先贴下方,下方不够高时自动翻到上方
  - 关闭:再次点按钮 / 点浮窗外任何位置 / 按 `Esc`
  - 滑块拖动 → `EditorStore.commit({ audioVolume })` → 自动保存
  - 预览路由:**WebAudio `GainNode`**(`<audio>.volume` 上限是 1.0,做不了 boost)
  - 导出:`buildAudioTrackFilter` 在音频 concat 之后接 `[a_pre]volume=X[a]`;`X == 1.0` 时不接 volume filter
- **播放头**:垂直红线,覆盖整条时间轴;拖动 = scrubbing
- **缩放滑块**:控制 `pxPerSecond`

#### 3.5.2 Clip 块

每个 clip = 一个矩形 DOM 块。

视觉:

```text
┌─ clip0 ─────────┐
│◀◀ 00:05 - 00:12│        ← 起止时间(鼠标悬停时显示)
│                 │
└─────────────────┘
```

属性:

- 宽度 = `(sourceEnd - sourceStart) * pxPerSecond`
- 左边缘 = `programStart * pxPerSecond`
- 选中时外描边高亮(蓝 2px)

#### 3.5.3 交互清单

| 动作 | 操作 | 行为 |
|------|------|------|
| 播放头 seek | 单击时间轴空白 / 刻度尺 | 播放头跳到该节目时间,鼠标按住可继续拖动 scrub |
| 播放头拖拽 | 鼠标按住播放头菱形头 / 命中区拖动 | 全程 scrubbing(连续 seek `<video>` 与 `<audio>`),拖动期间自动暂停,松开后若原本在播放则恢复 |
| 播放头形态 | — | `splitScope=both` → 跨双轨的"大游标";`splitScope=video/audio` → 该轨内的"小游标"。**播放一次即把 splitScope 提升为 `both` 并永驻** |
| 选中 clip | 单击 clip | 蓝色高亮;右侧工具条"删除选中"可用 |
| **范围选区** | **右键在刻度尺上按住拖动** | 选定一段 [start, end] 区间,跨刻度+两轨显示半透明黄色覆盖;同时**清空 clip 选中**(与 clip 选中互斥)并**强制 splitScope = both** |
| 分割 | 快捷键 `S` 或工具条 `✂` | 有范围选区时:按当前 splitScope 在 `start` 与 `end` 各切一刀(两次分割)后清除选区;无选区时:在播放头位置切一刀 |
| 删除 | 快捷键 `Delete` 或工具条 | 有范围选区时按钮**亮起可用**:按 splitScope 把 [start, end] 从轨道里挖空(clip 完整在内→丢弃;跨左/右边界→修剪;跨整段→拆成两段),**保留空隙不左移**;无选区时:删除选中 clip |
| 取消范围选区 | `Esc` 键 / 再次右击不拖动 / 加载新工程 / **任意时间轴左键点击**(刻度尺、空白轨道、clip、播放头) | 选区清除 |
| 右键菜单抑制 | 编辑器面板内 | `panel-editor` 全局拦截 `contextmenu`:右键已被范围选区占用,浏览器原生菜单永不弹出 |
| 拖动 clip | 鼠标按住 clip 中间拖动 | 改变 clip 在时间轴上的**顺序**(不允许重叠);松开 snap 到网格或相邻 clip 边 |
| 修剪左端 | 鼠标按住 clip 左边缘拖动 | 改 `sourceStart`,不改 `sourceEnd`;clip 变短或变长 |
| 修剪右端 | 鼠标按住 clip 右边缘拖动 | 改 `sourceEnd`,不改 `sourceStart` |
| 右键菜单 | 右键 clip | `分割 / 删除 / 重置为全段 / 复制` |
| 撤销 / 重做 | `Ctrl+Z` / `Ctrl+Y` | 见 §4.3 |

**边界**:

- `sourceStart` 不能小于 0,不能大于等于 `sourceEnd`
- `sourceEnd` 不能大于源视频 `duration`
- 修剪 / 拖动时有 2px 的 snap 容忍度(贴到相邻 clip 边缘 / 时间轴起点)
- 删除到 0 个 clip 时,时间轴空态显示"没有 clip,点击此处还原全段"

### 3.6 剪辑记录模态框

点击顶栏"📋 剪辑记录"弹出列表 + 删除/打开。从 `GET /api/editor/projects` 拉取(后端读 `index.json`),按 `updatedAt` 倒序。

### 3.7 导出对话框

点"导出" → 下拉格式 + 编码 + 输出目录 + 文件名:

```text
┌─ 导出 ───────────────────────────────────────┐
│  格式  [ mp4 ▼ ]                            │
│  视频编码 [ h264 ▼ ]                        │
│  音频编码 [ aac ▼ ]                         │
│  输出目录 [选择] D:/output   📂             │
│  文件名   [my_edit_1]                       │
│                                              │
│  [取消]                      [ 开始导出 ]   │
└──────────────────────────────────────────────┘
```

点"开始导出" → `POST /api/editor/export` → 进入共享的 SSE 日志视图。导出期间 Tab 切换安全(后端 job 继续跑)。

**导出期间 UI 布局**:

- DOM 结构:`#panel-editor` 用 `flex-direction: row`,左侧 `.editor-content`(顶栏 + 空态 + 工作区列)和右侧 `.editor-export-status`(日志面板 380px)平级
- **不挤压工作区**:导出启动时给 `<body>` 挂 `editor-export-active` 类,`<main>` 的 `max-width: 1200px` 直接撤为 `none`
- **阻断编辑**:`.editor-content.exporting::after` 全屏覆盖一层 `cursor:not-allowed` 的半透明黑(rgba 0.35);侧栏不在覆盖范围内,"取消" / "打开文件夹" 始终可点
- **生命周期**:启动时 `setExporting(true)` + 侧栏显示 + body 类 + 执行 `panel.start`;终态 `setExporting(false)`。侧栏右上 `×` 关闭键 → 若任务仍在跑,先 `confirm` 二次确认再 `POST /api/editor/export/cancel` 取消
- **进度条**:侧栏内置 `.progress-wrap`,导出启动时 `panel.start` 接收 `totalDurationSec = TL.totalDuration(project)`(节目时间轴总长,准确比 ffprobe 源时长更贴合实际剪辑结果)

---

## 4. 数据模型

### 4.1 Project JSON schema(SchemaVersion=3)

```jsonc
{
  "schemaVersion": 3,
  "id": "a1b2c3d4",
  "name": "My Vacation Edit",
  "createdAt": "2026-04-23T14:32:10+08:00",
  "updatedAt": "2026-04-23T15:02:40+08:00",

  "source": {
    "path": "D:/videos/vacation.mp4",
    "duration": 123.456,
    "width": 1920,
    "height": 1080,
    "videoCodec": "h264",
    "audioCodec": "aac",
    "frameRate": 29.97,
    "hasAudio": true
  },

  "audioVolume": 1.0,            // 0–2.0 线性增益,缺省 1.0

  "videoClips": [
    { "id": "v1", "sourceStart": 0.0,  "sourceEnd": 12.3, "programStart": 0.0 },
    { "id": "v2", "sourceStart": 45.0, "sourceEnd": 60.0, "programStart": 12.3 }
  ],
  "audioClips": [
    { "id": "a1", "sourceStart": 0.0,  "sourceEnd": 123.456, "programStart": 0.0 }
  ],

  "export": {
    "format": "mp4",
    "videoCodec": "h264",
    "audioCodec": "aac",
    "outputDir": "D:/output",
    "outputName": "my_edit_1"
  }
}
```

**SchemaVersion 演进**:

- v1:单一 `clips` 数组,覆盖音视频两轨
- v2:拆成 `videoClips` + `audioClips` 独立编辑
- v3:Clip 加 `programStart` 字段支持任意位置 + 空隙;Project 加 `audioVolume`

迁移在 `editor/storage.JSONRepo.Get` 里隐式调用,对 UI 和 API 无感。

### 4.2 Index JSON schema

```jsonc
[
  {
    "id": "a1b2c3d4",
    "name": "My Vacation Edit",
    "sourcePath": "D:/videos/vacation.mp4",
    "createdAt": "...",
    "updatedAt": "..."
  }
]
```

仅用于列表展示。Save 项目时同步更新;若 index 与文件不一致,后端启动时扫描 rebuild。

### 4.3 存储路径

| 内容 | 路径 |
|------|------|
| 索引 | `~/.easy-ffmpeg/projects/index.json` |
| 工程文件 | `~/.easy-ffmpeg/projects/<timestamp>_<id>.json` |
| 代理文件(v2 计划) | `~/.easy-ffmpeg/proxies/<source-sha8>.mp4` |

---

## 5. 前端状态管理

### 5.1 状态模型

```js
EditorStore.state = {
  project: { ... },          // 见 §4.1,null 表示未导入
  dirty: false,              // 有未保存改动
  selection: ["c2"],         // 选中的 clip id 列表
  rangeSelection: { start, end } | null, // 右键拖出的范围选区
  splitScope: "both" | "video" | "audio",
  playhead: 12.34,           // 节目时间秒
  playing: false,
  pxPerSecond: 8,            // 缩放
}
```

### 5.2 撤销 / 重做

- 每次**用户可感知的**操作(split / delete / reorder / trim / 改工程名)→ push 当前 `project` 快照到 `HistoryStack`
- 拖动过程中不 push,松开鼠标后 push 一次(防连续事件爆栈)
- 栈深度上限 100;溢出丢最老
- Ctrl+Z / Ctrl+Y 从栈里取;操作后播放头保持不变

### 5.3 自动保存策略

- 每次 `commit()` 后:
  - 标 `dirty = true`
  - 启动 debounce 1.5s 的定时器 → `PUT /api/editor/projects/<id>`
- 切换工程 / 关闭 Tab 前若 `dirty` → 立即保存
- 导出成功不自动保存;导出失败不影响保存

---

## 6. 预览实现方案(分三阶段)

| 阶段 | 方案 | 精度 | 效果 |
|------|------|------|------|
| **v1 (MVP,已实现)** | 原生 `<video>` + `currentTime` seek | 100-300ms,对齐到最近关键帧 | 简单可用 |
| **v2** | 后台生成 Proxy 文件(低分辨率 + GOP=1) | 每帧都是关键帧 → 16-33ms | 接近 PR |
| **v3** | WebCodecs + MP4Box.js | 帧精确 | 与专业软件无异 |

### 6.1 MVP 方案细节

- `<video>` 的 `src` = `/api/editor/source?id=<projectId>`(后端 byte range 文件服务)
- "节目时间 ↔ 源时间"映射在 JS 里完成
- 播放时监听 `timeupdate`:clip 末尾若紧邻下一 clip(< 0.01s 间距)则直接 seek 过去;否则进入"gap 时钟"模式
- **Gap 时钟**:播放头落在视频轨空隙时,`<video>` 暂停 + `.in-gap` 类隐藏(容器 `#0b0b0b` 透出 → 黑屏),改用 `requestAnimationFrame` 按真实时间推进 `playhead`;当 playhead 跨入下一段视频 clip 时把 `<video>.currentTime` 设到对应源时间并恢复播放
- 音频独立:处于音频轨空隙时 `<audio>` 暂停(静音)
- **黑屏与导出一致性**:预览的视频空隙 = 黑屏;导出时 `buildVideoTrackFilter` 用 `color=c=black` 填空隙、`buildAudioTrackFilter` 用 `anullsrc` 填空隙,所见即所得

---

## 7. 导出命令构建

`BuildExportArgs(project Project) ([]string, string, error)` 是纯函数。

### 7.1 双轨独立 concat

视频轨和音频轨**各自**构建 trim + concat 子链,分别输出到 `[v]` 和 `[a]` 再 mux。两轨的 clip 数量可以不同、长度也可以不一致。

```text
ffmpeg -y -i <source>
       -filter_complex
       "[0:v]trim=start=0:end=12.3,setpts=PTS-STARTPTS[v0];
        [0:v]trim=start=45:end=60,setpts=PTS-STARTPTS[v1];
        [v0][v1]concat=n=2:v=1:a=0[v];
        [0:a]atrim=start=0:end=123.456,asetpts=PTS-STARTPTS[a0];
        [a0]concat=n=1:v=0:a=1[a]"
       -map "[v]" -map "[a]"
       -c:v libx264 -c:a aac
       <outDir>/<name>.<format>
```

### 7.2 边界处理(关键)

- **轨道时长对齐**:`programDur = max(VideoDuration, AudioDuration)`,两条 filter 链都按这个长度 pad。**没这条规则时**两个流长度不一致,浏览器 `<video>` 元素会在更短流的 EOF 处停止播放,预览看上去就是"视频结束了,剩下的音频没了"
- 源无音轨:`audioClips` 为空,跳过音频链、不 `-map [a]`、不 `-c:a`
- 用户删光视频轨:只输出音频
- 用户删光音频轨:只输出视频(画面无声)
- 两轨都空:报错拒绝导出
- **视频轨开头不能留空**:`BuildExportArgs` 检查 `VideoClips` 最早 `ProgramStart` 是否 ≈ 0;非零 → 返回中文错误。**编辑期允许临时留空**(方便用户先布置后段再补开头),导出期硬性拒绝。**音频轨开头允许留空** —— pre-roll 静音是正当用法,filter graph 用 `anullsrc` 自动填充
- **轨道中间允许留空**:filter graph 自动用 `color=c=black` / `anullsrc` 填补,预览端 gap clock 也保持黑屏 + 静音
- clip 数量 = 1 且覆盖全段 → 仍然走 filter_complex(简单;不搞"快速拷贝"特例)

详见 [program.md §5](program.md)。

---

## 8. 交互细节与快捷键

| 快捷键 | 行为 |
|--------|------|
| `Space` | 播放 / 暂停 |
| `←` / `→` | 跳到上 / 下一 clip 起点 |
| `Shift + ← / →` | 播放头 ±1 帧(v1 用 ±0.04s 近似) |
| `S` | 在播放头位置分割 |
| `Delete` / `Backspace` | 删除选中 clip |
| `Ctrl + Z` | 撤销 |
| `Ctrl + Y` / `Ctrl + Shift + Z` | 重做 |
| `Ctrl + S` | 立即保存(也有自动保存,这是保险) |
| `Ctrl + E` | 打开导出对话框 |
| `Esc` | 取消刻度尺右键拖出的范围选区 |
| `+` / `-` | 时间轴缩放 |

**焦点处理**:快捷键只在 Tab 面板 focus 时生效;焦点在输入框时让原生编辑行为优先。

---

## 9. 边界情况与错误处理

| 情形 | 处理 |
|------|------|
| 源视频不存在(再次打开工程发现文件丢失) | 预览显示"⚠ 源文件未找到: <path>";所有编辑操作禁用;"导出"按钮 disabled |
| 浏览器不支持该视频编码(如 h265 播放) | 预览黑屏;后端仍可导出;提示"浏览器无法预览该编码,但导出仍可工作" |
| 源视频无音轨 | 音频轨渲染为虚线空轨;clip 仍正常;导出走 §7.2 分支 |
| 源视频是 .mov/.mkv 等浏览器不原生支持的容器 | 用 `<video>` 的 `canPlayType` 检测;不能播则显示黑屏 + 提示;v2 靠 proxy 解决 |
| 工程文件损坏 / schemaVersion 不兼容 | 列表里标红;打开时弹错误对话框 |
| 导出过程中切走 Tab | 后端 job 继续;切回来看见最新状态 |
| 删除所有 clip | 允许;时间轴显示空态;导出按钮 disabled |
