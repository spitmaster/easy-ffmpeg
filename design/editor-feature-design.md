# 单视频剪辑器 PRD

> 本文档定义"单视频剪辑器"Tab 的产品形态、交互细节、数据模型、后端 API 与 ffmpeg 导出规则。
> 目标读者：开发、评审、后续维护者。
>
> **实现状态**：✅ MVP 已实现（v0.3.0）。本版本**替换**了旧的"视频裁剪" Tab（`design/trim-feature-design.md` 已删除）。预览精度为"关键帧对齐"级别；proxy / WebCodecs 两种精度提升方案记录在 §7，尚未落地。
>
> **配套文档**：代码模块架构见 [editor-module-design.md](editor-module-design.md)。

---

## 1. 目标与非目标

### 1.1 目标（MVP）

提供一个**类似 Premiere Pro 的简易剪辑器**，能对**单个视频**执行：

- 导入一个视频，自动解析成一条视频轨 + 一条音频轨
- 在时间轴上**分割**（split）成多个 clip
- 对 clip 进行**删除 / 重排 / 拖拽端点修改 in-out**
- 实时**预览**（精度 MVP 可接受 100-300ms；见 §7 "预览实现方案"）
- 以**单次 ffmpeg 执行**导出为指定格式
- 把剪辑过程作为**工程（Project）**持久化，支持打开历史工程

### 1.2 非目标（本版本不做，留给后续专用 Tab）

- 多素材剪辑（素材库、多段视频拼接）
- 多轨道叠加 / PiP / 画中画 / 绿幕
- 转场（淡入淡出、擦除等）
- 音量包络 / 关键帧动画 / 调色
- 滤镜（模糊、锐化、色彩调整）
- 文字 / 贴纸 / 水印
- 帧级精度（先用原生 seek，接受关键帧对齐误差，见 §7）
- 云同步 / 账号 / 协作

这些都放到未来的**"高级剪辑"Tab**（独立需求、独立设计），本 Tab 保持"一个视频、一条轨、单次导出"的简洁语义。

---

## 2. 总体设计决策

### 2.1 替换旧 trim Tab

旧的"视频裁剪"Tab（`server/handlers_trim.go` + `server/trim_args.go` + `panel-trim`）整体删除。crop / scale 能力在本 Tab **不提供**（业务上 90% 的剪辑需求是"切掉不想要的段"，crop/scale 属于"预处理"，未来可在"视频转换"Tab 或独立"滤镜"Tab 里提供）。

`service.ProbeVideo` **保留**，剪辑器会复用。

### 2.2 单视频 · 单工程 · 双轨（视频 / 音频独立编辑）

- 一个工程恰好关联一个源视频文件（`source.path`）
- 时间轴上展示两条**独立**轨道：视频轨 + 音频轨（源无音轨则音频轨为空）
- 每条轨道有自己的 `clips[]`（`videoClips` / `audioClips`），split / 删除 / 重排 / 修剪均**独立作用于单轨**
- 导出时两轨各自 concat 到 `[v]` / `[a]` 再 mux（详见 §8）

**分割范围（splitScope）**：

| 用户操作 | 当前 splitScope | 按 `S` / 分割按钮后 |
|---------|-----------------|--------------------|
| 点击刻度栏 | `both` | 两轨同时在播放头分割 |
| 点击视频轨空白 | `video` | 仅视频轨分割 |
| 点击音频轨空白 | `audio` | 仅音频轨分割 |
| 点击视频轨内的 clip | `video` | 同上 |
| 点击音频轨内的 clip | `audio` | 同上 |

这个设计保留了"有两条轨道"的直观，同时让剪辑操作真正解耦 —— 用户可以保留背景音乐的完整节奏，只切视频画面。**预览窗** 跟随**视频轨**为主时间线，音频编辑仅在导出阶段体现（单个 `<video>` 元素的限制；若未来引入独立 `<audio>` 元素可以消除这一差异）。

### 2.3 工程持久化：每工程一个 JSON 文件

**放弃 sqlite 的理由**：

- 当前 Go 二进制 `CGO_ENABLED=0`，引入 `mattn/go-sqlite3` 需要 cgo；改用纯 Go 的 `modernc.org/sqlite` 会让二进制膨胀 4-6 MB（当前产物仅 35 MB）
- 剪辑工程的典型查询是"按时间倒序列出所有工程" + "按 id 取一个工程"，这两类查询用**文件系统原生机制**已足够

**存储方案**：

```
~/.easy-ffmpeg/
├── bin-<hash>/                    已有：解压的 ffmpeg
└── projects/                      新增
    ├── index.json                 轻量索引（列表页面用）
    ├── 2026-04-23_14-32-10_a1b2c3d4.json
    ├── 2026-04-23_09-15-00_e5f6a7b8.json
    └── ...
```

- 每个工程一个独立 JSON 文件，**文件大小与工程总数无关**（用户担心的"文件越来越大"不会发生）
- 文件名：`YYYY-MM-DD_HH-MM-SS_<uuid8>.json`（时间前缀让目录按名字排序 = 按创建时间排序，方便调试；`uuid8` 防同秒冲突）
- `index.json` 缓存 `[{id, name, source.path, updatedAt, thumbnail?}]`，打开"剪辑记录"面板时直接读，不用打开每个工程
- 索引损坏 / 缺失 → 按扫描 `projects/*.json` 重建（自愈）
- **单工程 JSON 典型大小**：< 10 KB（几十个 clip × 每 clip < 100 字节）；100 个工程总计 1 MB 级，不会成负担

### 2.4 预览实现：三阶段演进

见 §7 "预览实现方案"。**MVP 用原生 `<video>` + `currentTime` 跳转**，接受关键帧对齐误差；后续再引入 **proxy file** 和 **WebCodecs**。

### 2.5 编辑器代码独立成模块

`editor/` 顶级包自成一体（详见 [editor-module-design.md](editor-module-design.md)），不依赖 `server/handlers*.go` 的具体实现，通过接口向主程序索取能力（`VideoProber`、`JobRunner`、`PathResolver`）。未来可以把 `editor/` 单独编译成 `cmd/easy-editor/` 出一个 exe。

---

## 3. UI 布局

### 3.1 整体结构

```
┌─ 单视频剪辑 Tab ──────────────────────────────────────────────────────┐
│  [📂 打开视频]  [📋 剪辑记录]  工程名[My Edit______]   [导出 ▼]    │ ← 顶栏
├───────────────────────────────────────────────────────────────────┤
│                                                                   │
│                   ┌─────────────────────────────┐                 │
│                   │                             │                 │
│                   │        预览窗口             │                 │
│                   │        <video> 元素         │                 │ ← 预览区
│                   │                             │                 │
│                   │                             │                 │
│                   └─────────────────────────────┘                 │
│                                                                   │
│   ⏮ ⏸ ▶ ⏭   00:12.340 / 01:23.456        [ 🔊 ━━●━━ ]          │ ← 播控
├───────────────────────────────────────────────────────────────────┤
│  0:00      0:15      0:30      0:45      1:00      1:15           │ ← 时间刻度
│    │         │         │         │         │         │            │
│  ┌──────┐  ┌────────────────┐  ┌──────────┐                       │ ← 视频轨
│  │ clip0│  │     clip1      │  │  clip2   │                       │
│  └──────┘  └────────────────┘  └──────────┘                       │
│  ▂▂▂▂▂▂▂  ▂▂▂▂▂▂▂▂▂▂▂▂▂▂▂▂  ▂▂▂▂▂▂▂▂▂▂                          │ ← 音频轨
│                   ▲ (播放头)                                       │
│                                                                   │
│  [✂ 分割]  [🗑 删除选中]  [↶撤销]  [↷重做]   缩放 [━━●━━]         │ ← 工具条
└───────────────────────────────────────────────────────────────────┘
```

### 3.2 顶栏

| 组件 | 交互 |
|------|------|
| `📂 打开视频` | 打开文件浏览器模态框（复用现有 `Picker`）→ 选中后自动新建工程 |
| `📋 剪辑记录` | 弹出"历史工程"模态框（见 §3.6） |
| 工程名输入框 | 右侧显示当前工程名，用户可改；失焦自动保存 |
| `导出 ▼` | 下拉菜单：格式 mp4/mkv/mov/webm；点"开始导出"弹确认 |

### 3.3 预览区

- `<video>` 元素，`preload="auto"`，不显示原生控制条（用自定义播控）
- 默认最大宽 960px，居中；保持源视频宽高比
- 双击预览 = 切换全屏

### 3.4 播控条

| 按钮 | 快捷键 | 行为 |
|------|--------|------|
| ⏮ 上一 clip | `←` | 播放头跳到当前/上一 clip 起点 |
| ⏸/▶ | `Space` | 播放 / 暂停（基于"节目时间"的播放器，见 §7） |
| ⏭ 下一 clip | `→` | 播放头跳到下一 clip 起点 |
| 时间码 | — | 显示 `节目时间 / 节目总长`，如 `00:12.340 / 01:23.456` |
| 音量 | — | **已移除**——音量改为音频轨级属性，详见 §3.5 时间轴的"音频轨音量"控件 |

### 3.5 时间轴

#### 3.5.1 视觉组成

- **时间刻度**：顶部水平尺，刻度数随缩放变化（`1px/s → 20px/s`）
- **视频轨**：一条 40px 高的 DOM 容器，上面是一个或多个 clip 块
- **音频轨**：30px 高，显示波形 SVG（MVP 可以只显示纯色块，v2 再画波形）
- **音频轨音量**：时间轴布局加了一列**轨道动作列**（`.timeline-track-actions`），列宽 88px，行结构与标签列一致一一对齐——音频行放一个**文字按钮**直接显示形如 `音量: 100%` / `音量: 125%` 的实时读数，刻度尺 / 视频行放空占位。前缀"音量:"消除歧义（光是数字让人猜不到这是音量），按钮本身就是不打开浮窗也能看到的当前值，同时不会再挤"🔊 音频"标签让"音频"换行。
  - 点击按钮在按钮**附近**弹一个 124px 宽浮窗——顶端是"音频音量"标题，主体是 160px 高的垂直滑块，左侧并排 200%/100%/0% 三段刻度（绝对定位锚到滑块行高的顶/中/底，刻度后接 4px 短线视觉连到滑块），底下一条分割线 + 大号 accent 色读数实时跟随。**0–200%** 范围（boost 上限 2.0）。绑定 `project.audioVolume`（线性增益，缺省 1.0）：
  - 浮窗用 `position: fixed` 渲染在 `<main>` 内、视口顶层（z-index 60），躲开 `.editor-timeline { overflow: hidden }` 的裁切；打开时 JS 计算坐标——优先贴到按钮**下方**，下方不够高度时自动**翻到上方**（音频按钮就处在编辑区底部，下方常常装不下）。水平方向溢出右沿则向左滑入视口
  - 关闭：再次点按钮 / 点浮窗外任何位置 / 按 `Esc`
  - 滑块拖动 → `EditorStore.commit({ audioVolume })` → 自动保存
  - 预览路由：**WebAudio `GainNode`**（`<audio>.volume` 上限是 1.0，做不了 boost）。`Preview.applyAudioVolume` 懒初始化 `AudioContext + MediaElementSource → GainNode → destination`，从此 `audio.volume` 不再生效，`gain.value = audioVolume` 是单一真源。`createMediaElementSource` 抛错（同元素重复创建等）则降级到 `audio.volume = min(1, v)`，预览悄悄封顶 100%
  - 导出：`buildAudioTrackFilter` 在音频 concat 之后接 `[a_pre]volume=X[a]`；`X == 1.0` 时不接 volume filter（保持旧 filter graph 字节级稳定）
  - **预览播放器原本的音量按钮 / 滑块 / `M` 静音键已删除**——音量是工程持久化属性，不是预览的瞬态控件
- **播放头**：垂直红线，覆盖整条时间轴；拖动 = scrubbing
- **缩放滑块**：控制 `pxPerSecond`

#### 3.5.2 Clip 块

每个 clip = 一个矩形 DOM 块。

视觉：
```
┌─ clip0 ─────────┐
│◀◀ 00:05 - 00:12│        ← 起止时间（鼠标悬停时显示）
│                 │
└─────────────────┘
```

属性：
- 宽度 = `(sourceEnd - sourceStart) * pxPerSecond`
- 左边缘 = 前面所有 clip 长度之和 × pxPerSecond
- 选中时外描边高亮（蓝 2px）
- 多选（shift-click 或框选） —— MVP 可以只做单选

#### 3.5.3 交互清单

| 动作 | 操作 | 行为 |
|------|------|------|
| 播放头 seek | 单击时间轴空白 / 刻度尺 | 播放头跳到该节目时间，鼠标按住可继续拖动 scrub |
| 播放头拖拽 | 鼠标按住播放头菱形头 / 命中区拖动 | 全程 scrubbing（连续 seek `<video>` 与 `<audio>`），拖动期间自动暂停，松开后若原本在播放则恢复 |
| 播放头形态 | — | `splitScope=both` → 跨双轨的"大游标"；`splitScope=video/audio` → 该轨内的"小游标"。**播放一次即把 splitScope 提升为 `both` 并永驻**——大游标不会因为暂停就退回单轨；要恢复单轨指示，必须显式点击单条轨道空白 |
| 选中 clip | 单击 clip | 蓝色高亮；右侧工具条"删除选中"可用 |
| **范围选区** | **右键在刻度尺上按住拖动** | 选定一段 [start, end] 区间，跨刻度+两轨显示半透明黄色覆盖；同时**清空 clip 选中**（与 clip 选中互斥）并**强制 splitScope = both**（覆盖层视觉跨双轨，操作语义必须双轨同步）。覆盖时长 < 0.05s（轻点右键）则自动取消 |
| 分割 | 快捷键 `S` 或工具条 `✂` | 有范围选区时：按当前 splitScope 在 `start` 与 `end` 各切一刀（两次分割）后清除选区；无选区时：在播放头位置切一刀 |
| 删除 | 快捷键 `Delete` 或工具条 | 有范围选区时按钮**亮起可用**：按 splitScope 把 [start, end] 从轨道里挖空（clip 完整在内→丢弃；跨左/右边界→修剪；跨整段→拆成两段），**保留空隙不左移**；无选区时：删除选中 clip |
| 取消范围选区 | `Esc` 键 / 再次右击不拖动 / 加载新工程 / **任意时间轴左键点击**（刻度尺、空白轨道、clip、播放头） | 选区清除（不影响 split/delete 之后的自动清除） |
| 右键菜单抑制 | 编辑器面板内 | `panel-editor` 全局拦截 `contextmenu`：右键已被范围选区占用，浏览器原生菜单永不弹出（其他 Tab 不受影响） |
| 拖动 clip | 鼠标按住 clip 中间拖动 | 改变 clip 在时间轴上的**顺序**（不允许重叠）；松开 snap 到网格或相邻 clip 边 |
| 修剪左端 | 鼠标按住 clip 左边缘拖动 | 改 `sourceStart`，不改 `sourceEnd`；clip 变短或变长 |
| 修剪右端 | 鼠标按住 clip 右边缘拖动 | 改 `sourceEnd`，不改 `sourceStart` |
| 右键菜单 | 右键 clip | `分割 / 删除 / 重置为全段 / 复制` |
| 撤销 / 重做 | `Ctrl+Z` / `Ctrl+Y` | 见 §5.3 |

**边界**：
- `sourceStart` 不能小于 0，不能大于等于 `sourceEnd`
- `sourceEnd` 不能大于源视频 `duration`
- 修剪 / 拖动时有 2px 的 snap 容忍度（贴到相邻 clip 边缘 / 时间轴起点）
- 删除到 0 个 clip 时，时间轴空态显示"没有 clip，点击此处还原全段"

### 3.6 剪辑记录模态框

点击顶栏"📋 剪辑记录"弹出：

```
┌─ 剪辑记录 ─────────────────────────────────────────┐
│  [+ 新建工程]                            [✕ 关闭] │
│                                                    │
│  🎬 My Vacation Edit                               │
│     源: D:/videos/vacation.mp4                     │
│     更新于 2026-04-23 14:32        [打开] [🗑]    │
│  ─────────────────────────────────────────        │
│  🎬 Demo Cut                                       │
│     源: C:/work/demo.mp4                           │
│     更新于 2026-04-22 09:15        [打开] [🗑]    │
│  ─────────────────────────────────────────        │
│  (空态：暂无剪辑工程)                              │
└────────────────────────────────────────────────────┘
```

- 列表从 `GET /api/editor/projects` 拉取（后端读 `index.json`）
- 按 `updatedAt` 倒序
- `[打开]` → 加载工程到当前 Tab，覆盖当前未保存改动前先 confirm
- `[🗑]` → confirm 后删除 `<id>.json` 和 index 条目
- 源文件已不存在的工程：用灰色 + "⚠ 文件缺失" 标记，仍可打开（用户可以手动指向新路径）——此为 v2，MVP 只显示警告不支持重定位

### 3.7 导出对话框

点"导出" → 下拉格式 + 确认：

```
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

点"开始导出" → `POST /api/editor/export` → 进入共享的 SSE 日志视图（与其他 Tab 一致）。导出期间 Tab 切换安全（后端 job 继续跑）。

**导出期间 UI 布局**：

- DOM 结构：`#panel-editor` 用 `flex-direction: row`，左侧 `.editor-content`（顶栏 + 空态 + 工作区列）和右侧 `.editor-export-status`（日志面板 380px）平级；日志面板沿整个剪辑功能区的右边垂直贯穿
- **不挤压工作区**：导出启动时给 `<body>` 挂 `editor-export-active` 类，`<main>` 的 `max-width: 1200px` 直接撤为 `none`，main 占满视口宽度。日志侧栏贴屏幕右沿，工作区获得"原宽 + 原本被居中浪费掉的左侧空白"——超宽屏 / 4K 上空间全部被利用
- **阻断编辑**：`.editor-content.exporting::after` 全屏覆盖一层 `cursor:not-allowed` 的半透明黑（rgba 0.35）；侧栏不在覆盖范围内，"取消" / "打开文件夹" 始终可点
- **生命周期**：启动时 `setExporting(true)` + 侧栏显示 + body 类 + 执行 `panel.start`；POST 启动失败兜底检查 `cancelBtn.disabled` 立即解除阻断；JobBus `done`/`error`/`cancelled` 任一终态 `setExporting(false)`。侧栏右上 `×` 关闭键 → 若任务仍在跑，先 `confirm` 二次确认再 `POST /api/editor/export/cancel` 取消（避免遗留孤儿 ffmpeg 进程），随后隐藏侧栏 + 摘掉 body 类 → main 收回 1200px 居中
- **进度条**：侧栏内置 `.progress-wrap`（轨 + 百分比），由 `createJobPanel` 共用。导出启动时 `panel.start` 接收 `totalDurationSec = TL.totalDuration(project)`（节目时间轴总长，准确比 ffprobe 源时长更贴合实际剪辑结果），随后从每行 `time=HH:MM:SS.ms` 取当前时间 / 总长 = 进度。`done` 时停留 100% 短暂 600ms 后隐藏，`error/cancelled` 立即隐藏
- **覆盖确认**：`POST /api/editor/export` 在调起 `runner.Start` 之前 `os.Stat(outPath)`，文件已存在且 `req.Overwrite=false` 时返回 409 + `existing:true` + 输出路径。前端 `createJobPanel.start` 见 409 → `await Confirm.overwrite(path)` 弹出**自绘 dialog**（不再用浏览器原生 `confirm`，UI 一致、Esc/点背景=取消、Enter=确认、显示路径用等宽字体清晰可读）→ 同意则带 `overwrite:true` 重发；拒绝则隐藏进度条结束。和 `/api/convert/start`、`/api/audio/start` 同形协议（共享同一个 `Confirm` 模块）
- **执行前命令预览**：所有三个 export endpoint（convert / audio / editor）均接受 `dryRun: true`，server 走完参数校验和 `BuildExportArgs` 等步骤但**不**调用 `runner.Start`、**不** mkdir、**不**做 overwrite 检查，仅返回 200 + `command` 字符串。前端 `createJobPanel.start` 在真正发起任务前先 `dryRun:true` 拉一次命令 → `await Confirm.command(cmd)` 弹自绘命令预览 dialog（720px 宽、等宽 pre 块、Click-to-copy + 「📋 复制」按钮、Esc/取消=不执行）→ 用户确认 → 才发真正的 POST。命令是 server 真正会跑的命令，不是客户端凑出来的近似版本——尤其编辑器导出 filter_complex 链很长，服务端构造保证 1:1 一致

### 3.8 空态设计

- **未导入视频**：中间大号提示 "拖入视频文件或点击「📂 打开视频」开始"（拖入 MVP 可后延）
- **已导入但 clip 清空**：时间轴上显示"没有 clip，点击此处还原全段"

---

## 4. 数据模型

### 4.1 Project JSON schema（v2 双轨）

```jsonc
{
  "schemaVersion": 2,
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

  "videoClips": [
    { "id": "v1", "sourceStart": 0.0,  "sourceEnd": 12.3 },
    { "id": "v2", "sourceStart": 45.0, "sourceEnd": 60.0 }
  ],
  "audioClips": [
    { "id": "a1", "sourceStart": 0.0,  "sourceEnd": 123.456 }
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

**设计备注**：

- 两条轨道各自的位置由数组顺序隐式决定（第 i 个 clip 开始于前 i-1 个的总时长），**不存绝对 position** —— 拖动 clip 改变顺序 = 数组 reorder
- `clips[].id` 用短随机字符串（video 以 `v` 开头、audio 以 `a` 开头便于调试），撤销/重做依赖稳定 id
- `source.*` 元数据用 ffprobe 一次性抓全，避免每次导出再探测
- 两轨长度**允许不一致** —— 程序时长取两者的 max，ffmpeg 导出时容器长度由两个流的自然结束点决定
- 源无音轨（`source.hasAudio=false`）时 `audioClips` 省略

**v1 → v2 迁移**：旧版本的单 `clips` 字段被 `domain.Project.Migrate()` 透明拷贝到 `videoClips` 和 `audioClips`（如果源有音轨）。迁移在 `editor/storage.JSONRepo.Get` 里隐式调用，对 UI 和 API 无感。

### 4.2 Index JSON schema

```jsonc
[
  {
    "id": "a1b2c3d4",
    "name": "My Vacation Edit",
    "sourcePath": "D:/videos/vacation.mp4",
    "createdAt": "...",
    "updatedAt": "..."
  },
  ...
]
```

- 仅用于列表展示
- Save 项目时同步更新；若 index 与文件不一致（偶发 / 外部手动改过），后端启动时扫描 rebuild

### 4.3 存储路径

| 内容 | 路径 |
|------|------|
| 索引 | `~/.easy-ffmpeg/projects/index.json` |
| 工程文件 | `~/.easy-ffmpeg/projects/<timestamp>_<id>.json` |
| 代理文件（v2） | `~/.easy-ffmpeg/proxies/<source-sha8>.mp4` |

---

## 5. 前端状态管理

### 5.1 模块结构

沿用 `app.js` 的 IIFE 模式，新增：

- `EditorTab` — 顶层模块，`init()` 绑定 DOM，管理整体状态
- `EditorStore` — 工程数据的**单一可信源**，发布-订阅模式，任何改动调用 `commit(patch)`
- `Timeline` — 时间轴 DOM 渲染、拖拽交互
- `Preview` — 预览播放器（封装 `<video>` + 节目时间映射）
- `HistoryStack` — 撤销 / 重做栈

### 5.2 状态模型（前端）

```js
EditorStore.state = {
  project: { ... },          // 见 §4.1，null 表示未导入
  dirty: false,              // 有未保存改动
  selection: ["c2"],         // 选中的 clip id 列表
  playhead: 12.34,           // 节目时间秒
  playing: false,
  pxPerSecond: 8,            // 缩放
}
```

### 5.3 撤销 / 重做

- 每次**用户可感知的**操作（split / delete / reorder / trim / 改工程名）→ push 当前 `project.clips` 快照到 `HistoryStack`
- 拖动过程中不 push，松开鼠标后 push 一次（防连续事件爆栈）
- 栈深度上限 100；溢出丢最老
- Ctrl+Z / Ctrl+Y 从栈里取；操作后播放头保持不变

### 5.4 自动保存策略

- 每次 `commit()` 后：
  - 标 `dirty = true`
  - 启动 debounce 1.5s 的定时器 → `PUT /api/editor/projects/<id>`
- 切换工程 / 关闭 Tab 前若 `dirty` → 立即保存
- 导出成功不自动保存；导出失败不影响保存（工程是工程，导出是运行时）

### 5.5 节目时间 ↔ 源时间映射

```js
// 节目时间 → {clipIndex, sourceTime}
function programToSource(t) {
  let acc = 0;
  for (let i = 0; i < clips.length; i++) {
    const len = clips[i].sourceEnd - clips[i].sourceStart;
    if (t < acc + len) return { i, src: clips[i].sourceStart + (t - acc) };
    acc += len;
  }
  return null;  // 超过总长
}
```

播放器核心逻辑：监听 `<video>.timeupdate`，若接近当前 clip 的 `sourceEnd` 则 seek 到下一 clip 的 `sourceStart`；若是最后一段则 pause。

---

## 6. 后端 API

全部前缀 `/api/editor/`，以便与其他 Tab 隔离、未来能整体剥离。

| 方法 | 路径 | 作用 |
|------|------|------|
| `GET` | `/api/editor/projects` | 列出工程（读 index.json） |
| `POST` | `/api/editor/projects` | 新建工程（需 source path；后端调 ffprobe 填 metadata） |
| `GET` | `/api/editor/projects/:id` | 读单个工程 |
| `PUT` | `/api/editor/projects/:id` | 全量替换工程（前端保存） |
| `DELETE` | `/api/editor/projects/:id` | 删除工程文件 + 更新 index |
| `POST` | `/api/editor/probe` | 探测视频（`service.ProbeVideo` 壳） |
| `POST` | `/api/editor/export` | 开始导出；body = 工程 + export settings；走 `jobs.Start` |
| `POST` | `/api/editor/export/cancel` | 取消当前导出 |

`GET /api/convert/stream`（现有 SSE）在导出时复用 —— 所有 Tab 的 Job 共享一条 SSE。

### 6.1 新建工程请求

```jsonc
POST /api/editor/projects
{
  "sourcePath": "D:/videos/vacation.mp4",
  "name": "My Vacation Edit"        // 可选，默认 "未命名工程 <timestamp>"
}
```

**后端处理**：
1. `ProbeVideo(sourcePath)` 拿 duration/w/h/codec
2. 生成 `id` (8-hex)、`createdAt`、`updatedAt = createdAt`
3. 初始 `clips = [{id:"c1", sourceStart:0, sourceEnd:duration}]`（整段作为一个 clip）
4. 保存到 `projects/<timestamp>_<id>.json` + 更新 index
5. 返回完整 Project JSON

### 6.2 导出请求

```jsonc
POST /api/editor/export
{
  "projectId": "a1b2c3d4",
  "export": {
    "format": "mp4",
    "videoCodec": "h264",
    "audioCodec": "aac",
    "outputDir": "D:/output",
    "outputName": "my_edit_1"
  }
}
```

后端流程：
1. 读工程 JSON
2. 合并 `req.export` 进工程（覆盖，**不持久化**——用户改导出设置不污染工程）
3. `BuildExportArgs(project)` 构造 ffmpeg 参数（§8）
4. `jobs.Start(ffmpegPath, args)`
5. 前端订阅现有 `/api/convert/stream`

---

## 7. 预览实现方案（分三阶段）

| 阶段 | 方案 | 精度 | 效果 |
|------|------|------|------|
| **v1 (MVP)** | 原生 `<video>` + `currentTime` seek | 100-300ms，对齐到最近关键帧 | 简单可用 |
| **v2** | 后台生成 Proxy 文件（低分辨率 + GOP=1）| 每帧都是关键帧 → 16-33ms | 接近 PR |
| **v3** | WebCodecs + MP4Box.js | 帧精确 | 与专业软件无异 |

### 7.1 MVP 方案（v1）

前端：
- `<video>` 的 `src` = `/api/editor/source?path=<sourcePath>`（后端返回 byte range 支持的 file server，详见 §10）
- "节目时间 ↔ 源时间"映射在 JS 里完成（§5.5）
- 播放时监听 `timeupdate`：clip 末尾若紧邻下一 clip（< 0.01s 间距）则直接 seek 过去；否则进入"gap 时钟"模式
- **Gap 时钟**：播放头落在视频轨空隙时，`<video>` 暂停 + `.in-gap` 类隐藏（容器 `#0b0b0b` 透出 → 黑屏），改用 `requestAnimationFrame` 按真实时间推进 `playhead`；当 playhead 跨入下一段视频 clip 时把 `<video>.currentTime` 设到对应源时间并恢复播放，gap 时钟交还给原生 timeupdate；到达节目总长则统一 pause
- 音频独立：处于音频轨空隙时 `<audio>` 暂停（静音），不影响视频侧；`keepAudioInSync` 跨边界时硬 seek
- **黑屏与导出一致性**：预览的视频空隙 = 黑屏；导出时 `buildVideoTrackFilter` 用 `color=c=black` 填空隙、`buildAudioTrackFilter` 用 `anullsrc` 填空隙，所见即所得

**一个取舍**：用户点击时间轴 seek 到 clip 中间时，会 seek 到最近关键帧，和时间轴上播放头的位置可能差 0~1.5s。MVP 接受，不处理。

### 7.2 Proxy 方案（v2，留白设计）

- 导入视频时后台异步执行：
  ```
  ffmpeg -i <source> -vf scale=-2:360 -g 1 -c:v libx264 -crf 32 \
         -c:a aac -b:a 96k ~/.easy-ffmpeg/proxies/<sha8>.mp4
  ```
- 生成期间预览用原文件（慢 seek），完成后自动切到 proxy
- Proxy 与源 **时长相同、帧率相同**，每帧 = 关键帧 → 任意点秒级 seek 无卡顿
- 导出**始终用源文件**（proxy 只是预览代理）
- 空间代价：约源文件 5-10%

**v2 加入时改动范围**：
- 后端：新增 `POST /api/editor/proxy/generate`、`GET /api/editor/proxy/status`、`GET /api/editor/proxy/:sha` 静态服务
- 前端：`Preview` 模块感知 `project.source.proxy` 字段并优先用
- 数据模型：`source.proxyPath`, `source.proxyState: "none"|"generating"|"ready"`

### 7.3 WebCodecs 方案（v3，研究中）

如果用户对精度有硬需求（比如要求 "播放头在哪里就显示哪一帧"），用 MP4Box.js 解析 MP4 盒 + WebCodecs 解码到指定 frame。本期不纳入。

---

## 8. 导出命令构建

`BuildExportArgs(project Project) ([]string, string)` 是纯函数，位置 `editor/domain/export.go`。

### 8.1 构建规则（双轨独立 concat）

视频轨和音频轨**各自**构建 trim + concat 子链，分别输出到 `[v]` 和 `[a]` 再 mux。两轨的 clip 数量可以不同、长度也可以不一致。

```
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

当视频轨空（`videoClips=[]`）时，`[v]` 链和 `-map [v]`、`-c:v` 都省略；音频同理。源无音轨或音频轨被清空时不输出 `[a]`。

### 8.2 参考实现

真实代码见 `editor/domain/export.go`：对两轨分别调用 `buildTrackFilter`（一个纯函数复用），然后按轨道情况条件拼 `-map` / `-c:v` / `-c:a`。关键点：

- `buildTrackFilter(clips, "v", "trim", "setpts")` → `[0:v]trim=...[v0]; [v0]concat=n=1:v=1:a=0[v]`
- `buildTrackFilter(clips, "a", "atrim", "asetpts")` → `[0:a]atrim=...[a0]; [a0]concat=n=1:v=0:a=1[a]`
- 主函数根据 `hasVideo` / `hasAudio` 组合上述两段，只在对应轨道存在 clip 时才加 `-map`
- 无视频轨 or 无音频轨都允许（例如：用户删光了视频轨只剩音频轨）

### 8.3 边界处理

- **轨道时长对齐**：`BuildExportArgs` 计算 `programDur = max(VideoDuration, AudioDuration)`，`buildVideoTrackFilter` 和 `buildAudioTrackFilter` 都接收该参数；`planSegments` 在轨道末尾如果 `cursor < programDur` 则自动追加一段 trailing gap（视频用 `color=c=black`，音频用 `anullsrc`）。**没这条规则时**两个流长度不一致，浏览器 `<video>` 元素会在更短流的 EOF 处停止播放，预览看上去就是"视频结束了，剩下的音频没了"——一个真实出现过的 bug
- 源无音轨：`audioClips` 为空，跳过音频链、不 `-map [a]`、不 `-c:a`
- 用户删光视频轨：只输出音频（可用于单独抽音场景）
- 用户删光音频轨：只输出视频（画面无声）
- 两轨都空：报错拒绝导出
- **视频轨开头不能留空**：`BuildExportArgs` 检查 `VideoClips` 最早 `ProgramStart` 是否 ≈ 0；非零 → 返回中文错误（"视频轨道开头必须有内容：第一个 clip 从 X.XXs 开始..."）。**编辑期允许临时留空**（方便用户先布置后段再补开头），导出期硬性拒绝。前端 `ExportModal` 也做同样校验，提前以 `alert` 阻止提交。**音频轨开头允许留空**——pre-roll 静音（开头几秒无配音）是正当用法，filter graph 用 `anullsrc` 自动填充
- **轨道中间允许留空**：filter graph 自动用 `color=c=black` / `anullsrc` 填补，预览端 `gap clock` 也保持黑屏 + 静音
- clip 数量 = 1 且覆盖全段 → 仍然走 filter_complex（简单；不搞"快速拷贝"特例）
- clip 数量 > 100 → filter 字符串会很长；经验上 ffmpeg 能处理 100+，>500 要警告；MVP 不设硬限

### 8.4 测试策略

`editor/domain/export_test.go` 表驱动测试：
- 1 clip / 2 clip / 5 clip 正常构造
- 无音轨分支
- 空 clips 数组错误
- 文件名 / 路径转义（含空格、中文）

---

## 9. 交互细节与快捷键

| 快捷键 | 行为 |
|--------|------|
| `Space` | 播放 / 暂停 |
| `←` / `→` | 跳到上 / 下一 clip 起点 |
| `Shift + ← / →` | 播放头 ±1 帧（v1 用 ±0.04s 近似） |
| `S` | 在播放头位置分割 |
| `Delete` / `Backspace` | 删除选中 clip |
| `Ctrl + Z` | 撤销 |
| `Ctrl + Y` / `Ctrl + Shift + Z` | 重做 |
| `Ctrl + S` | 立即保存（也有自动保存，这是保险） |
| `Ctrl + E` | 打开导出对话框 |
| `Esc` | 取消刻度尺右键拖出的范围选区 |
| `+` / `-` | 时间轴缩放 |

**焦点处理**：快捷键只在 Tab 面板 focus 时生效；焦点在输入框时让原生编辑行为优先。

---

## 10. 边界情况与错误处理

| 情形 | 处理 |
|------|------|
| 源视频不存在（再次打开工程发现文件丢失） | 预览显示"⚠ 源文件未找到: <path>"；所有编辑操作禁用；"导出"按钮 disabled |
| 浏览器不支持该视频编码（如 h265 播放） | 预览黑屏；后端仍可导出；提示"浏览器无法预览该编码，但导出仍可工作" |
| 源视频无音轨 | 音频轨渲染为虚线空轨；clip 仍正常；导出 §8.3 分支 |
| 源视频是 .mov/.mkv 等浏览器不原生支持的容器 | 用 `<video>` 的 `canPlayType` 检测；不能播则显示黑屏 + 提示；v2 靠 proxy 解决 |
| 工程文件损坏 / schemaVersion 不兼容 | 列表里标红；打开时弹错误对话框 |
| 两个 Tab 同时写同一工程 | 本 Tab 内串行（前端状态单例）；跨 Tab/窗口 MVP 不处理（单实例假设） |
| 导出过程中切走 Tab | 后端 job 继续；切回来看见最新状态 |
| 删除所有 clip | 允许；时间轴显示空态；导出按钮 disabled |
| 源文件路径含引号 / 中文 | 后端 ffmpeg 参数用 `exec.Command` 原样传不受 shell 影响；前端 URL 用 `encodeURIComponent` |

### 10.1 `/api/editor/source?path=...` 安全

此端点会把本地文件流给 `<video>`。和旧 `/api/fs/reveal` 同样，因为服务只绑 `127.0.0.1`，本机进程才能访问；但应当：

- 只允许 path 指向**当前已加载工程的 source.path**（服务端维护"已授权路径白名单"，用工程加载时注册）
- 支持 HTTP Range（`<video>` seek 需要）

---

## 11. 实现切片（实施顺序）

| # | 范围 | 关键交付 | 估算 |
|---|------|---------|------|
| 1 | 删除旧 trim 功能 | 清除 `handlers_trim.go` / `trim_args.go` / 前端面板，删 `design/trim-feature-design.md` | 0.5 天 |
| 2 | 编辑器模块骨架 | `editor/` 目录结构 + 接口定义 + 空 handler + Module wire-up 到 server | 1 天 |
| 3 | 工程 CRUD | `editor/storage/jsonrepo.go` + `GET/POST/PUT/DELETE /api/editor/projects*` + 单元测试 | 1 天 |
| 4 | 前端骨架 | `panel-editor` + 空顶栏 + 空预览 + 空时间轴 DOM；`EditorStore` + `EditorTab.init()` | 1 天 |
| 5 | 打开视频 + 探测 + 工程生命周期 | 选文件 → POST /projects → 加载；自动保存；剪辑记录模态框（列表+删除+打开） | 1.5 天 |
| 6 | 预览播放器 v1 | `Preview` 模块；节目时间映射；播控条；`<video>` 源 URL 端点 `/api/editor/source` | 1.5 天 |
| 7 | 时间轴渲染 + clip 选中 | Timeline DOM；clip 块；时间刻度；播放头；点击 seek | 1 天 |
| 8 | 时间轴编辑交互 | split / delete / 拖动重排 / 边缘 trim；撤销/重做 | 2 天 |
| 9 | 导出 | `BuildExportArgs` + 导出对话框 + 复用 SSE | 1 天 |
| 10 | 边界与文案 | 空态、错误态、快捷键、文档同步 | 1 天 |

**总估算**：约 11 天。MVP 可交付点是第 9 片（能完整打开视频 → 剪 → 导出）。

---

## 12. 与既有文档的关系

| 文档 | 变更 |
|------|------|
| `design/README.md` | 目录去掉 `trim-feature-design.md`，加 `editor-feature-design.md` + `editor-module-design.md` |
| `design/feature-design.md` | "视频裁剪"行 → "单视频剪辑"；指向本文档 |
| `design/architecture.md` | 目录结构加 `editor/` 子树；分层图加编辑器模块 |
| `design/module-design.md` | 删除 §2.4 `trim_args.go`；新增 `editor/` 模块章 |
| `design/roadmap.md` | 0.2.x 末期的"视频裁剪"里程碑被 0.3.0 单视频剪辑器替换；技术债里的 trim 条目删除 |
| `design/trim-feature-design.md` | **删除** |
