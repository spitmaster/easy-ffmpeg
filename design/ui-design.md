# UI 设计

## 1. 技术栈

- **纯静态三件套**：`index.html` + `app.css` + `app.js`
- **无构建**：没有 Vite/webpack/Tailwind；CSS 变量 + 原生 JS
- **打包方式**：`//go:embed web` 把整个 `server/web/` 目录塞进可执行文件
- **通信协议**：
  - fetch/JSON 用于命令式操作
  - EventSource (SSE) 用于 FFmpeg 日志实时推送
- **浏览器兼容**：现代浏览器（Chrome/Edge/Firefox 最近 3 年的版本）即可

## 2. 整体布局

```
┌──────────────────────────────────────────────────────────────┐
│  🎬  Easy FFmpeg                 FFmpeg 8.1 · 嵌入    退出    │  ← topbar
├──────────────────────────────────────────────────────────────┤
│  [视频转换][音频处理][视频剪辑][媒体信息*][设置*]             │  ← tabs (*disabled)
├──────────────────────────────────────────────────────────────┤
│                                                              │
│                  主要内容区域（按 active tab 切换 panel）      │
│                                                              │
└──────────────────────────────────────────────────────────────┘
```

- Header 固定顶部、tabs 紧随其后、main 区域自然流式（CSS flex column）。
- Tab 切换由 `Tabs` IIFE 负责：点击 `.tab` → 给对应 `[data-tab]` 加 `.active`，并把所有 `.panel` 按 id 是否匹配切换 `.hidden`。
- 已启用 Tab：视频转换 / 音频处理 / 视频剪辑；占位 disabled：媒体信息 / 设置。

## 3. Tab 布局

### 3.1 视频转换 Tab（convert）

```
┌──────────────────────────────────────────────────────────────────┐
│ 输入文件                                                          │
│ [选择文件] <输入路径>                                             │
│                                                                  │
│ 输出目录 / 文件名                                                  │
│ [选择目录] <输出目录> [📂] <文件名>                                │
│                                                                  │
│ 编码器 / 格式                                                      │
│ [视频编码器 ▼] [音频编码器 ▼] [容器格式 ▼]                        │
│                                                                  │
│ 命令预览                                                          │
│ ffmpeg -y -i "..." -c:v libx264 -c:a aac "out.mp4"               │
│                                                                  │
│ [开始转码] [取消]  空闲                                            │
│                                                                  │
│ 转码日志（黑底等宽字体，进度行原地覆盖）                           │
└──────────────────────────────────────────────────────────────────┘
```

### 3.2 音频处理 Tab（audio，三模式）

顶部是 segmented control（格式转换 / 从视频提取 / 音频合并），下面是各模式独立字段区，底部共用命令预览 / 动作行 / 日志。参见 [audio-feature-design.md](audio-feature-design.md) §3。

```
┌─ 音频处理 Tab ─────────────────────────────────────────┐
│ [格式转换] [从视频提取] [音频合并]   ← segmented       │
│                                                        │
│ … 当前模式对应的字段区（参数集按模式切换） …            │
│                                                        │
│ 命令预览 / 开始·取消 / 日志（三模式共用底部区）        │
└────────────────────────────────────────────────────────┘
```

### 3.3 视频剪辑 Tab（editor）

时间轴式单视频剪辑器（取代旧的"视频裁剪"）。详细设计见 [editor-feature-design.md](editor-feature-design.md) + [editor-module-design.md](editor-module-design.md)。

```
┌─ 视频剪辑 Tab ──────────────────────────────────────────────┐
│ [📂 打开视频][📋 剪辑记录] <工程名>            [导出]      │ ← 顶栏
├────────────────────────────────────────────────────────────┤
│                                                            │
│              ┌─────────────────────────┐                   │
│              │      预览 <video>        │                   │
│              └─────────────────────────┘                   │
│  ⏮ ⏸/▶ ⏭   00:12.340 / 01:23.456        🔊 ━━●━━         │
├────────────────────────────────────────────────────────────┤
│ 0:00   0:15   0:30   0:45   1:00   1:15                   │ ← 时间刻度
│ ┌─────┐ ┌─────────┐ ┌───────┐                             │ ← 视频轨
│ │clip0│ │  clip1  │ │ clip2 │      ▼ (播放头)              │
│ └─────┘ └─────────┘ └───────┘                             │
│ ▂▂▂▂▂ ▂▂▂▂▂▂▂▂▂ ▂▂▂▂▂▂▂                                  │ ← 音频轨
│                                                            │
│ [✂ 分割][🗑 删除][↶撤销][↷重做]       缩放 [━━●━━]         │ ← 工具条
└────────────────────────────────────────────────────────────┘
```

- **静态资源位置**：`server/web/editor/editor.css` + `server/web/editor/editor.js`（按 `<link>` / `<script>` 标签从主 `index.html` 引入），为未来剥离独立 exe 做铺垫。
- **DOM 前缀**：所有元素 id 以 `ed` 开头（`edVideo`、`edRuler`、`edTimeline` 等），避免与其它 Tab 命名冲突。
- **两个模态框**：`edProjectsBackdrop`（剪辑记录列表）+ `edExportBackdrop`（导出对话框）。

## 4. 配色系统

使用 CSS 自定义属性集中管理，便于后续主题化：

```css
--bg:              #0f1419   /* 最底层 */
--surface:         #1a1f26   /* 卡片/表单背景 */
--surface-2:       #232831   /* hover / 次要元素 */
--border:          #2a3038
--border-strong:   #3a4048
--text:            #e5e7eb
--muted:           #9ca3af
--accent:          #10b981   /* 主行动绿 */
--accent-hover:    #059669
--danger:          #ef4444
--warning:         #f59e0b
--info:            #3b82f6   /* 命令预览的蓝 */
```

整体风格：**深色高对比、柔和圆角、绿色强调 CTA**。

## 5. 顶栏（topbar）

| 元素 | 位置 | 说明 |
|------|------|------|
| 🎬 Logo + "Easy FFmpeg" | 左 | |
| `<span class="status-chip ok clickable">` | 右 | 版本状态 + 点击可打开缓存目录 |
| 退出按钮 | 最右 | `.btn-ghost` 样式 |

状态 chip 的三种状态：
- `.ok.clickable` → 绿色边框 + cursor pointer + hover 淡绿背景
- `.err` → 红色边框，不可点击
- 加载中 → 灰色边框，"检测中..."

## 6. 自定义控件

### 6.1 `.btn` 按钮家族

- `.btn`：默认，surface 背景 + 弱边框
- `.btn-primary`：绿色主行动按钮（开始转码）
- `.btn-danger`：红色边框 + 透明，hover 填红（取消）
- `.btn-ghost`：透明，低调辅助（关闭、退出、上一级）
- `.btn-icon`：紧凑 padding，适合 emoji 图标按钮（📂）

### 6.2 `.status-chip` 小圆圈

圆角 999px 丸形徽章，`ok` 和 `err` 两种语义色，可选 `.clickable` 启用交互。

### 6.3 `.command-preview`

等宽字体，`var(--info)` 蓝色，深色代码块背景。

### 6.4 `.log` 日志区

- 黑底（`#000`）、白字（`#d4d4d4`）、等宽字体
- `flex: 1 + min-height: 0 + overflow: auto`：在 convert / audio 两个 Tab 里自动填充剩余垂直空间
- 在视频剪辑 Tab 用 `.editor-export-status .log { max-height: 200px }` 约束高度，保证导出期间顶部的预览 / 时间轴仍可见
- 子元素 `.log-line` 可加修饰类：
  - `.progress`（暖黄）→ FFmpeg 进度行
  - `.success`（绿色）→ "✓ 完成"
  - `.error`（红色）→ "✗ 失败"
  - `.info`（蓝色）→ 命令预览回显
  - `.cancelled`（黄色）→ "! 已取消"

### 6.5 文件/目录选择模态框

```
┌─────────────────────────────────────────────────────┐
│ 选择输入视频                                     ×   │
├─────────────────────────────────────────────────────┤
│ [C:/ ▾] [/ Users / zhouyijin                    ] ↑ │
├─────────────────────────────────────────────────────┤
│ 📁 Desktop                                           │
│ 📁 Documents                                         │
│ 📄 video.mp4                             12.3 MB    │
│ 📄 audio.wav                              3.1 MB    │
│    ...                                               │
├─────────────────────────────────────────────────────┤
│ 选中一个文件后点击确认      [取消]  [选择文件]       │
└─────────────────────────────────────────────────────┘
```

- 三层：header / breadcrumb-bar / body / footer
- breadcrumb-bar 三元素：可选的盘符下拉 + 可编辑的路径输入 + 上一级按钮
- body 条目列表：图标 + 名字 + 元信息（文件大小）
- 单击选中（高亮），双击目录进入 / 双击文件直接完成选择
- backdrop 半透明黑，点击背景也关闭

### 6.6 首次启动加载遮罩

```
┌─────────────────────────────────────────┐
│           正在准备 FFmpeg                │
│                                         │
│ 首次启动需要解压内嵌的 FFmpeg 组件…      │
│                                         │
│ ████████████████░░░░░░░░░░░░░░           │
│ 53%                    ffprobe.exe      │
└─────────────────────────────────────────┘
```

- 全屏毛玻璃 `backdrop-filter: blur(4px)`
- 居中卡片 460px 宽，内含标题 + 副文案 + 进度条 + 百分比 + 当前文件
- 进度条：绿→蓝渐变、0.25s 缓动
- 就绪时 `.fading` 类触发 0.3s 透明度淡出，然后 `display:none`
- 解压失败时不隐藏，副文案变成错误信息，进度条变红

### 6.7 `.segmented`（音频 Tab 模式切换）

行内 flex 容器，盛三个 `.seg` 按钮；活动按钮有 `.active` 类（surface-2 背景 + 主色文字）。
`.seg:disabled` 半透明 + not-allowed。

### 6.8 `.editor-*`（视频剪辑 Tab 专用组件）

样式集中在 `editor/editor.css`，用 `#panel-editor` 前缀避免泄漏到其它 Tab：

- `.editor-topbar` / `.editor-empty` / `.editor-workspace`：整体布局
- `.editor-preview` video + `.editor-playbar`：预览窗与播控
- `.editor-timeline`：时间轴容器（`.timeline-ruler` + `.timeline-track` × 2 + `.timeline-playhead`）
- `.clip` + `.clip-handle`：时间轴上的片段与拖拽手柄，`.clip.selected` 表示选中状态
- `.editor-project-list .row-item`：剪辑记录列表条目
- `.editor-export-status`：导出日志浮层（仅有工程后显示）

### 6.9 `.merge-list`（音频合并 Tab 的可排序文件列表）

`<ul>`：空态用 `:empty::before` 伪元素显示"尚未添加文件"；每项带 `☰` 抓手（装饰）、编号、文件名（ellipsis）、元信息（codec · 声道 · kbps · 时长）、↑/↓/🗑 三个操作按钮。

## 7. JS 架构（app.js）

单文件、无框架，全部用 **IIFE 模块**组织，每个模块只导出 init 或少量方法。职责分离清晰（SRP）；加新 Tab 不需要动既有模块。

### 7.1 模块一览

| 模块 | 类型 | 职责 |
|------|------|------|
| `$` | helper | `document.getElementById` 缩写 |
| `Http` | helper | `fetchJSON(url, opts)` / `postJSON(url, body)` |
| `Fmt` | helper | `human(size)` 人类可读字节 |
| `Path` | helper | `join` / `basename` / `dirname` / `stripExt` |
| `Time` | helper | `HH:MM:SS[.mmm]` 严格解析与格式化（供 EditorTab 等复用） |
| `Dirs` | IIFE | 输入 / 输出目录缓存与持久化（`/api/config/dirs`） |
| `FFmpegStatus` | IIFE | 顶栏版本 chip 加载与点击跳转缓存目录 |
| `Picker` | IIFE | 共享的文件 / 目录选择模态框（mode=file\|dir，Promise 风格） |
| `JobBus` | IIFE | **全局单 EventSource**（`/api/convert/stream`），广播事件给所有订阅者 |
| `createJobPanel(opts)` | 工厂 | 每个 Tab 独立持有的日志 / 动作行 / 完成条控制器，包括 SSE 订阅与 "owning" 逻辑 |
| `ConvertTab` | IIFE | 视频转换表单 + 预览 + 开始 |
| `AudioCodecs` | IIFE | 共享的容器/编码器/码率知识，供三种音频模式复用（DRY）|
| `AudioConvertMode` / `AudioExtractMode` / `AudioMergeMode` | IIFE | 音频三种模式各自的字段组与命令预览 |
| `AudioTab` | IIFE | 挂载三模式 + segmented 切换 + 调用 `createJobPanel` |
| `EditorApi` / `EditorStore` / `History` / `TL` | IIFE | 剪辑器数据层：fetch wrappers、单一状态源 + 自动保存、撤销/重做栈、节目/源时间换算 |
| `Preview` | IIFE | `<video>` 封装 + 节目时间↔源时间映射、跨 clip 连播、seek |
| `Timeline` | IIFE | 时间轴 DOM 渲染 + 拖拽交互（分割/删除/重排/修剪） |
| `ProjectsModal` | IIFE | 剪辑记录模态框（列表 / 打开 / 删除） |
| `ExportModal` | IIFE | 导出对话框 + 复用 `createJobPanel` 走 SSE |
| `EditorTab` | IIFE | 顶层：绑定 DOM、键盘快捷键、调度 render；这些子模块都在 `editor/editor.js` 里，与主 `app.js` 分文件 |
| `Tabs` | IIFE | 点击 `.tab` 切换 `.panel` 的显隐 |
| `Quit` | IIFE | 右上角退出按钮 |
| `Prepare` | IIFE | 首次启动解压轮询与遮罩 |

### 7.2 初始化顺序

```js
(async () => {
  await Prepare.wait();      // 解压遮罩；ready 之后继续
  FFmpegStatus.init();       // 版本 chip
  await Dirs.load();         // 预取目录配置
  Picker.init();             // 挂载 picker 模态框事件
  ConvertTab.init();         // Tab 初始化顺序无依赖
  AudioTab.init();
  if (typeof EditorTab !== "undefined") EditorTab.init();  // editor.js 在 app.js 之后加载
  Tabs.init();               // 绑定 tab 切换
  Quit.init();
  JobBus.connect();          // 开 SSE，事件开始流入所有 panel
})();
```

### 7.3 任务面板模式（`createJobPanel`）

每个 Tab 有自己的日志区 / 开始按钮 / 取消按钮 / 完成条。`createJobPanel` 是工厂函数：

- 订阅 `JobBus`（单例 SSE）
- 内部维护 `owning` 标志：只有"从自己发起任务"的 panel 才响应 log/done/error/cancelled 事件；其他 panel 收到后忽略
- 暴露 `start({url, body, outputPath})`：打 POST、处理 409 overwrite 确认、写入 "> ffmpeg …" 回显、置 `owning=true`、`setRunning(true)`
- cancel 按钮绑定到构造参数中的 `cancelUrl`（各 Tab 传自己的 `/api/*/cancel`）

这样三个 Tab 共享一条 SSE，但只有发起方看到 log / finish bar。

### 7.4 进度行原地覆盖（`createJobPanel.appendLog`）

```js
const PROGRESS_RE = /^(frame=|size=|video:|Lsize=)/;
if (isProgress && lastLine.classList.contains("progress")) {
  lastLine.textContent = text;   // 原地覆盖
} else {
  /* append new span */
}
```

前端视觉上像终端实时刷新；DOM 节点数量不随时间增长。

### 7.5 SSE 自动重连

```js
es.onerror = () => setTimeout(connect, 1500);
```

浏览器刷新 / 服务重启 / 网络故障都自愈。

### 7.6 解压进度轮询

```js
async function wait() {
  while (true) {
    const p = await Http.fetchJSON("/api/prepare/status");
    if (p.state === "ready") { fade backdrop; return; }
    if (p.state === "error") { show error; return; }
    update progress bar;
    await sleep(300ms);
  }
}
```

## 8. 交互细节

### 8.1 视频转换 Tab
- **输入文件变化 → 输出文件名自动填充**：`<原文件名>_converted`，自动保存输入目录到 `/api/config/dirs`
- **输出目录变化 → 保存到配置 + 启用 📂 按钮**
- **任一表单字段变化 → 命令预览实时刷新**（`input` / `change` 统一绑定）
- **转码过程中"开始"按钮 disabled**，"取消"启用；右侧状态在"转码中…"/"空闲"间切换

### 8.2 音频 Tab
- **模式切换时**：命令预览清空重建；运行中禁止切换（看 start 按钮是否 disabled）
- **从视频提取** 在选完输入后自动 ffprobe 音轨；单音轨时下拉隐藏
- **合并** 列表里每项展示 codec · 声道 · kbps · 时长；↑↓ 排序，🗑 移除；添加按钮触发 Picker

### 8.3 视频剪辑 Tab
- **打开视频** → `POST /api/editor/projects` 自动 probe 填 source 元数据、生成整段为一 clip 的默认工程、保存到 `~/.easy-ffmpeg/projects/`
- **EditorStore 单一状态源**：UI 所有改动走 `commit(patch)` / `set(patch)`；`commit` 标 `dirty=true` + 1.5s debounce 自动 `PUT`
- **时间轴交互**：点击空白 seek；点击 clip 选中；边缘拖拽修剪；中央拖拽改顺序（不允许重叠）；`S` 分割，`Del` 删除
- **撤销/重做**：每次可感知改动（拖拽松开后）push 到 `History` 栈；`Ctrl+Z` / `Ctrl+Y`
- **预览**：单 `<video>` 放原始文件，按节目时间映射到源时间；跨 clip 边界时自动 seek 到下一 clip 的 `sourceStart`
- **导出**：弹对话框选格式/编码/输出目录 → `flushSave` 后 `POST /api/editor/export` → 共享 `createJobPanel` + SSE

### 8.4 所有 Tab 共用
- **日志自动滚动到底部**：`requestAnimationFrame` 后设 `scrollTop = scrollHeight`
- **完成条** 成功时可"📂 打开文件夹"（用记录的 outputPath）
- **覆盖确认**：后端 409 + `existing:true` → 前端 `confirm()` 再带 `overwrite:true` 重试

## 9. 国际化

全中文硬编码，未做 i18n 基础设施。后续要做：
- 提取所有中文文案到 `i18n/zh.json`、`en.json`
- 前端按 `navigator.language` 或用户设置选择
- 后端的错误消息也需要国际化（目前返回英文 `error: ...`）

## 10. 主题

- 只有一套深色主题
- 颜色全部用 CSS 变量，切换主题只需换 `:root` 变量值
- 若未来要加浅色主题：`prefers-color-scheme: light { :root { --bg: #fff; ... } }`

## 11. 已知视觉问题

- **字体**：用的是 system-ui 堆栈 `-apple-system, BlinkMacSystemFont, Segoe UI, PingFang SC, Hiragino Sans GB, Microsoft YaHei, sans-serif`。在部分 Linux 发行版上可能 fallback 到难看的 DejaVu Sans。
- **模态框尺寸在小屏**：760px 固定宽，移动端会溢出 `max-width: 90vw`。移动端 UX 未认真设计。
- **编辑 Tab 在矮屏**：时间轴高度固定 140px，预览用 `minmax(0, 1fr)` 弹性占用；窄屏下预览会变得较小，后续可加"预览全屏"按钮。
- **merge 排序手势**：目前只用 ↑/↓ 按钮，没做拖拽排序。列表很长时效率偏低。
