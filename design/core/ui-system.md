# 共享 UI 设计系统(产品设计)

> 本文档定义跨 Tab 共用的视觉规范、控件家族、对话框约定、导出体验流。各 Tab 的具体布局见对应的 `tabs/<tab>/product.md`。前端 JS 架构(IIFE 模块、SSE、`createJobPanel` 实现细节)见 [frontend.md](frontend.md)。

## 1. 整体布局

```text
┌──────────────────────────────────────────────────────────────┐
│  🎬  Easy FFmpeg     程序版本    FFmpeg 8.1 · 嵌入    退出     │  ← topbar
├──────────────────────────────────────────────────────────────┤
│  [视频转换][音频处理][单视频剪辑][媒体信息*][设置*]             │  ← tabs (*disabled)
├──────────────────────────────────────────────────────────────┤
│                                                              │
│                  主要内容区域(按 active tab 切换 panel)       │
│                                                              │
└──────────────────────────────────────────────────────────────┘
```

- Header 固定顶部、tabs 紧随其后、main 区域自然流式(CSS flex column)。
- 已启用 Tab:视频转换 / 音频处理 / 单视频剪辑;占位 disabled:媒体信息 / 设置。

## 2. 配色系统

使用 CSS 自定义属性集中管理,便于后续主题化:

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

整体风格:**深色高对比、柔和圆角、绿色强调 CTA**。

只有一套深色主题。颜色全部用 CSS 变量,切换主题只需换 `:root` 变量值。若未来要加浅色主题:`prefers-color-scheme: light { :root { --bg: #fff; ... } }`。

## 3. 顶栏(topbar)

| 元素 | 位置 | 说明 |
|------|------|------|
| 🎬 Logo + "Easy FFmpeg" | 左 | |
| `<span class="version-chip">` | 右 | 程序版本号(`-ldflags -X main.Version=...` 注入,默认 `dev`) |
| `<span class="status-chip">` | 右 | FFmpeg 版本状态 + 点击可打开缓存目录 |
| 退出按钮 | 最右 | `.btn-ghost` 样式 |

状态 chip 的三种状态:

- `.ok.clickable` → 绿色边框 + cursor pointer + hover 淡绿背景
- `.err` → 红色边框,不可点击
- 加载中 → 灰色边框,"检测中..."

后端 `GetFFmpegVersion()` 返回完整首行;前端 `parseFFmpegVersion` 提取主版本号(`8.1` / `6.1.1` / `N-119999-g1234`),最终 chip 显示:`FFmpeg 8.1 · 嵌入`,tooltip 保留完整版本串。

## 4. 控件家族

### 4.1 `.btn` 按钮

- `.btn`:默认,surface 背景 + 弱边框
- `.btn-primary`:绿色主行动按钮(开始转码)
- `.btn-danger`:红色边框 + 透明,hover 填红(取消)
- `.btn-ghost`:透明,低调辅助(关闭、退出、上一级)
- `.btn-icon`:紧凑 padding,适合 emoji 图标按钮(📂)

### 4.2 `.status-chip`

圆角 999px 丸形徽章,`ok` 和 `err` 两种语义色,可选 `.clickable` 启用交互。

### 4.3 `.command-preview`

等宽字体,`var(--info)` 蓝色,深色代码块背景。

### 4.4 `.log` 日志区

- 黑底(`#000`)、白字(`#d4d4d4`)、等宽字体
- `flex: 1 + min-height: 0 + overflow: auto`:在 convert / audio 两个 Tab 里自动填充剩余垂直空间
- 在单视频剪辑 Tab 用 `.editor-export-status .log { max-height: 200px }` 约束高度,保证导出期间顶部的预览 / 时间轴仍可见
- 子元素 `.log-line` 修饰类:
  - `.progress`(暖黄)→ FFmpeg 进度行
  - `.success`(绿色)→ "✓ 完成"
  - `.error`(红色)→ "✗ 失败"
  - `.info`(蓝色)→ 命令预览回显
  - `.cancelled`(黄色)→ "! 已取消"

### 4.5 `.segmented`(音频 Tab 模式切换)

行内 flex 容器,盛三个 `.seg` 按钮;活动按钮有 `.active` 类(surface-2 背景 + 主色文字)。`.seg:disabled` 半透明 + not-allowed。

### 4.6 `.progress-wrap`(任务进度条,三 Tab 共用)

```text
[━━━━━━━━━━━━━●━━━━━━━━━━━━━━━━━━━━]   42.5%
```

- DOM:`<div class="progress-wrap"><div class="progress-bar"><div class="progress-fill" /></div><span class="progress-text">…</span></div>`
- 进度条 8px 高、圆角;轨道 `--surface-2` 背景,填充 `--accent` 绿色,`width` 用 150ms linear transition(太短抖动、太长跟不上 fast convert)
- 百分比标签 12px、`tabular-nums` 防数字位数变化时位置抖动、`min-width: 38px` 留位
- 由 `createJobPanel` 共用逻辑驱动,从同一行日志同时取数据(详见 §6),空闲态加 `.hidden` 整块折叠

### 4.7 `.merge-list`(音频合并 Tab 的可排序文件列表)

`<ul>`:空态用 `:empty::before` 伪元素显示"尚未添加文件";每项带 `☰` 抓手(装饰)、编号、文件名(ellipsis)、元信息(codec · 声道 · kbps · 时长)、↑/↓/🗑 三个操作按钮。详见 [tabs/audio/product.md](../tabs/audio/product.md)。

### 4.8 `.editor-*`(单视频剪辑 Tab 专用)

样式集中在 `editor/editor.css`,用 `#panel-editor` 前缀避免泄漏到其它 Tab。详见 [tabs/editor/product.md](../tabs/editor/product.md) §UI。

## 5. 对话框约定

### 5.1 自绘 Confirm 对话框(替代浏览器原生 `window.confirm`)

由 `Confirm` IIFE 提供,两种形态共用同一个状态机:

- **覆盖确认(`Confirm.overwrite(path)`)**:460px 宽 `.modal-confirm`,header "目标文件已存在" + × 关闭,body 一句中文提示 + 等宽字体的目标路径(`break-all` 让长路径换行),footer "取消" / "覆盖"。背景半透明 + box-shadow 浮起。Enter=覆盖 / Esc=取消。
- **命令预览(`Confirm.command(cmd)`)**:720px 宽 `.modal-command`,header "即将执行" + × 关闭,body "下列 ffmpeg 命令将被执行" + `<pre class="confirm-command">` 等宽字体 280px 高滚动 + 提示语,footer "📋 复制" / "取消" / "开始执行"。`<pre>` 整块 `cursor: pointer`,点击或点 📋 都会复制全文(优先 `navigator.clipboard.writeText`,失败回退到隐藏 `<textarea> + execCommand("copy")`),提示语短暂变 accent 色"✓ 已复制"。Enter=执行 / Esc=取消(Enter 在 `<pre>` 上不触发,留给文本选择)。

两个对话框都用 `Promise<boolean>` 返回:`createJobPanel.start` 在真正发起 POST 前先 `await Confirm.command(...)`,409 返回时再 `await Confirm.overwrite(...)`,代码线性、无回调地狱。

### 5.2 文件/目录选择模态框

```text
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

- 三层:header / breadcrumb-bar / body / footer
- breadcrumb-bar 三元素:可选的盘符下拉 + 可编辑的路径输入 + 上一级按钮
- body 条目列表:图标 + 名字 + 元信息(文件大小)
- 单击选中(高亮),双击目录进入 / 双击文件直接完成选择
- 排序:目录在前,文件在后,同类按名字字典序(不区分大小写);以 `.` 开头的条目不显示

由于浏览器的 `<input type=file>` 出于安全限制拿不到本地真实路径,而 FFmpeg 需要真路径,所以文件/目录选择走**后端驱动的**模态框:

- `GET /api/fs/home` → 起始路径
- `GET /api/fs/list?path=<dir>` → 返回条目列表 + 父目录 + Windows 盘符
- 模态框支持:面包屑路径输入框(支持回车跳转)、上一级 ↑ 按钮、Windows 盘符下拉框、空目录提示

### 5.3 首次启动加载遮罩

```text
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
- 居中卡片 460px 宽,内含标题 + 副文案 + 进度条 + 百分比 + 当前文件
- 进度条:绿→蓝渐变、0.25s 缓动
- 就绪时 `.fading` 类触发 0.3s 透明度淡出,然后 `display:none`
- 解压失败时不隐藏,副文案变成错误信息,进度条变红

### 5.4 模态弹窗的统一约定

所有自绘 dialog(覆盖确认 / 命令预览 / 编辑器导出配置 / 剪辑记录列表 / 文件选择器):

- **不响应**点击背景空白区域 —— 太容易误触把正在配置的导出操作丢掉
- **× 关闭按钮**位于右上角(`.modal-header` flex + `.spacer { flex: 1 }`),与"取消"按钮等价
- **Esc** 键关闭(等价于"取消"),**Enter** 在确认型 dialog 上等价于"确认"
- 焦点:打开时聚焦主按钮,关闭时还原到打开前的元素(`lastFocused`)

## 6. 跨 Tab 共用的导出体验

视频转换 / 音频处理 / 单视频剪辑三个 Tab 都通过 `createJobPanel`(见 [frontend.md §3](frontend.md))触发任务,共享同一套交互流:

```text
点击"开始"按钮
    │
    ▼
① 后端 dryRun POST     {…params, dryRun: true}
   后端:构建参数 + 构造命令字符串,但不 mkdir、不查 overwrite、不启 ffmpeg
   返回 200 + {command: "ffmpeg -y -i ... <out>"}
    │
    ▼
② 自绘"命令预览"dialog(replaces window.confirm)
   ┌─ 即将执行 ──────────────────────────────────┐
   │ 下列 ffmpeg 命令将被执行,确认后开始:        │
   │ ┌──────────────────────────────────────┐  │
   │ │ ffmpeg -y -i "..." -filter_complex   │  │ ← click-to-copy 整块
   │ │   "..." -map [v] -map [a] ...        │  │
   │ └──────────────────────────────────────┘  │
   │ 点击命令框可复制                            │
   │ ─────────────────────────────────────────  │
   │ [📋 复制]                  [取消] [开始执行] │
   └────────────────────────────────────────────┘
   关闭路径:取消 / × / Esc / 点 [开始执行]
    │
    ▼ 用户确认
③ 后端真实 POST       {…params}
    │
    ├─ 409 + {existing:true, path}
    │     ▼
    │   自绘"覆盖确认"dialog → 同意带 overwrite:true 重发;拒绝中止
    │
    └─ 200 → SSE 开始推日志 + 解析 `time=` 算进度条百分比
        │
        ▼
       ④ 终态:done / error / cancelled → 完成条 + 进度条短暂停 100% 再隐藏
```

### 6.1 进度条

- **位置**:动作行下方一条独立的轨 + 百分比标签(`.progress-wrap`),三 Tab 各一份,由 `createJobPanel` 公共逻辑驱动
- **数据源**:解析 ffmpeg stderr 里的 `time=HH:MM:SS.ms`(当前进度)和首次出现的 `Duration: HH:MM:SS.ms`(总时长)。编辑器导出时 `panel.start({ totalDurationSec })` 显式传节目时间总长,比 `Duration:`(源文件长度)更准
- **生命周期**:启动 → 0% → 跟随 `time=` 实时增长 → `done` 停 100% 600ms 后隐藏 → `error/cancelled/409 取消` 立即隐藏

### 6.2 命令预览 dialog(dryRun 协议)

- 协议:所有三个 endpoint 都接受 `dryRun: true`,返回 `{ok, dryRun, command}` 不动文件不启进程;merge mode 的临时 list 文件在 dryRun 路径上立即 cleanup
- UI:720px 宽 `.modal-command`,`<pre class="confirm-command">` 用等宽字体最高 280px 高滚动;click-to-copy 用 `navigator.clipboard.writeText`,失败回退到隐藏 `<textarea> + execCommand("copy")`
- 接管 Enter / Esc 全局键

### 6.3 覆盖确认 dialog

- 协议:未带 `overwrite:true` 时,`os.Stat(outPath)` 命中则返回 `409 + {existing:true, path}`
- UI:460px 宽 `.modal-confirm`,等宽字体显示路径(`break-all`),Enter=覆盖 / Esc=取消
- 三 endpoint 协议统一,`createJobPanel.start` 一份代码处理所有 Tab

## 7. 已知视觉问题

- **字体**:用的是 system-ui 堆栈 `-apple-system, BlinkMacSystemFont, Segoe UI, PingFang SC, Hiragino Sans GB, Microsoft YaHei, sans-serif`。在部分 Linux 发行版上可能 fallback 到难看的 DejaVu Sans。
- **模态框尺寸在小屏**:760px 固定宽,移动端会溢出 `max-width: 90vw`。移动端 UX 未认真设计。
- **编辑 Tab 在矮屏**:时间轴高度固定 140px,预览用 `minmax(0, 1fr)` 弹性占用;窄屏下预览会变得较小,后续可加"预览全屏"按钮。
- **merge 排序手势**:目前只用 ↑/↓ 按钮,没做拖拽排序。列表很长时效率偏低。

## 8. 国际化

全中文硬编码,未做 i18n 基础设施。后续要做:

- 提取所有中文文案到 `i18n/zh.json`、`en.json`
- 前端按 `navigator.language` 或用户设置选择
- 后端的错误消息也需要国际化(目前返回英文 `error: ...`)

见 [roadmap.md](roadmap.md) §3。
