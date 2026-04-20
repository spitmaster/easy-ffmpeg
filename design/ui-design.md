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
│  [视频转换][视频裁剪*][音频处理*][媒体信息*][设置*]           │  ← tabs (*disabled)
├──────────────────────────────────────────────────────────────┤
│                                                              │
│                  主要内容区域（视频转换 Tab）                  │
│                                                              │
└──────────────────────────────────────────────────────────────┘
```

`container.NewBorder` 式布局由 CSS flex 实现：header 固定顶部，tabs 紧随其后，main 区域自然流式。

## 3. 视频转换 Tab

```
┌──────────────────────────────────────────────────────────────────┐
│ 输入文件                                                          │
│ ┌────────────┬─────────────────────────────────────────────────┐  │
│ │ 选择文件   │ <输入路径 Entry>                                 │  │
│ └────────────┴─────────────────────────────────────────────────┘  │
│                                                                  │
│ 输出目录 / 文件名                                                  │
│ ┌────────────┬──────────────────────────┬─────┬─────────────────┐ │
│ │ 选择目录   │ <输出目录 Entry>         │ 📂  │ <文件名 Entry>  │  │
│ └────────────┴──────────────────────────┴─────┴─────────────────┘ │
│                                                                  │
│ 编码器 / 格式                                                      │
│ ┌──────────────┬──────────────┬───────────────┐                  │
│ │ <视频编码器>  │ <音频编码器>  │ <容器格式>    │                  │
│ └──────────────┴──────────────┴───────────────┘                  │
│                                                                  │
│ 命令预览                                                          │
│ ┌─────────────────────────────────────────────────────────────┐  │
│ │ ffmpeg -y -i "..." -c:v libx264 -c:a aac "out.mp4"           │  │
│ └─────────────────────────────────────────────────────────────┘  │
│                                                                  │
│ [开始转码]  [取消]  空闲                                           │
│                                                                  │
│ 转码日志                                                          │
│ ┌─────────────────────────────────────────────────────────────┐  │
│ │ > ffmpeg -y -i "input.mkv" -c:v libx264 ...                  │  │
│ │ Input #0, matroska,webm, from 'input.mkv':                   │  │
│ │ frame= 1420 fps=75 q=-1.0 size=...  (原地刷新)               │  │
│ │ ✓ 转码完成                                                    │  │
│ └─────────────────────────────────────────────────────────────┘  │
└──────────────────────────────────────────────────────────────────┘
```

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
- `height: 320px` 固定，超出滚动
- 子元素 `.log-line` 可加修饰类：
  - `.progress`（暖黄）→ FFmpeg 进度行
  - `.success`（绿色）→ "✓ 转码完成"
  - `.error`（红色）→ "✗ 转码失败"
  - `.info`（蓝色）→ 命令预览回显
  - `.cancelled`（黄色）→ "! 转码已取消"

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

## 7. JS 架构（app.js）

单文件、无框架、顶层异步 IIFE。按职能分五段：

1. **helpers**：`$`、`fetchJSON`、`human`、`joinPath`
2. **FFmpeg 状态**：`parseFFmpegVersion`、`loadFFmpegStatus`、chip 点击事件
3. **转码表单**：`readForm`、`updateCommandPreview`、输入监听
4. **文件选择器**：`openPicker`、`loadPickerPath`、`renderDrives`、`renderEntries`
5. **SSE + 转码执行**：`appendLog`、`setRunning`、`connectStream`、开始/取消按钮
6. **首次启动等待**：`waitForPrepare` 轮询 + 遮罩控制
7. **初始化 IIFE**：`waitForPrepare()` → `loadFFmpegStatus()` → 加载配置 → 启动 SSE

### 7.1 进度行原地覆盖

```js
const PROGRESS_RE = /^(frame=|size=|video:|Lsize=)/;

function appendLog(text, cls) {
  const isProgress = !cls && PROGRESS_RE.test(text);
  if (isProgress) {
    const last = logEl.lastElementChild;
    if (last && last.classList.contains("progress")) {
      last.textContent = text;   // 原地覆盖
      logEl.scrollTop = logEl.scrollHeight;
      return;
    }
  }
  // 否则追加新 span
  ...
}
```

### 7.2 SSE 自动重连

```js
es.onerror = () => setTimeout(connectStream, 1500);
```

浏览器刷新 / 服务重启 / 网络故障都能自愈。

### 7.3 解压进度轮询

```js
async function waitForPrepare() {
  while (true) {
    const p = await fetchJSON("/api/prepare/status");
    if (p.state === "ready") { hide overlay; return; }
    if (p.state === "error") { show error; return; }
    update progress bar;
    await sleep(300ms);
  }
}
```

## 8. 交互细节

- **输入文件变化 → 输出文件名自动填充**：`<原文件名>_converted`，且自动保存输入目录到 `/api/config/dirs`
- **输出目录变化 → 保存到配置 + 启用 📂 按钮**
- **任一表单字段变化 → 命令预览实时刷新**（所有表单元素统一绑定 `input` / `change` 事件）
- **转码过程中"开始转码"按钮 disabled**，"取消"按钮启用
- **转码结束**：两个按钮状态互换；右侧"正在转码..."→"空闲"
- **日志自动滚动到底部**：每次 appendLog 后 `logEl.scrollTop = logEl.scrollHeight`

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
- **日志区固定 320px 高**：长转码会让用户频繁滚动。后续可加"全屏日志"按钮。
