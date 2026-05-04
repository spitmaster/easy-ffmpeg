# 共享 UI 设计系统(产品设计)

> 本文档定义跨 Tab 共用的视觉规范、设计 token、控件家族、对话框约定、导出体验流。各 Tab 的具体布局见对应的 `tabs/<tab>/product.md`。前端工程结构、Pinia store、composable 实现细节见 [frontend.md](frontend.md)。
>
> v0.5.0 起,样式从手写 CSS + CSS 变量整体迁到 **TailwindCSS + tokens.css**:配色 / 字体 / 圆角 / 阴影通过 `:root` CSS 变量集中,Tailwind 在 `tailwind.config.js` 用 `rgb(var(--…) / <alpha-value>)` 把它们暴露成 utility class。

## 1. 整体布局

```text
┌──────────────────────────────────────────────────────────────┐
│  🎬  Easy FFmpeg     v0.5.0     FFmpeg 8.1 · 嵌入    退出     │  ← TopBar
├──────────────────────────────────────────────────────────────┤
│  [视频转换][音频处理][单视频剪辑][媒体信息*][设置*]              │  ← TabNav (* 占位)
├──────────────────────────────────────────────────────────────┤
│                                                              │
│                  RouterView(按当前路由渲染对应 View)            │
│                                                              │
└──────────────────────────────────────────────────────────────┘
```

DOM 结构(`App.vue`):

```html
<div class="flex h-full flex-col">
  <TopBar />     <!-- shrink-0 h-12 -->
  <TabNav />     <!-- shrink-0 -->
  <main class="flex-1 overflow-auto bg-bg-base">
    <RouterView />
  </main>
</div>
```

四个全局对话框(Picker / ConfirmCommand / ConfirmOverwrite / PrepareOverlay)在 App 根 mount 一次,通过 `useModalsStore` 命令式触发并 Promise 等待。

## 2. 设计 token(`web/src/styles/tokens.css`)

颜色全部存为 RGB 三元组(无 `rgb()` 包裹),让 Tailwind 的 `<alpha-value>` 插值生效:

```css
:root {
  /* 背景层 — base < panel < elevated */
  --color-bg-base:     18 18 20;     /* 页面底色 */
  --color-bg-panel:    28 28 32;     /* TopBar / TabNav / 子 panel */
  --color-bg-elevated: 38 38 44;     /* 卡片、模态、chip */

  /* 前景层 — base 正文,muted 标签,subtle 提示 */
  --color-fg-base:    224 224 228;
  --color-fg-muted:   168 168 176;
  --color-fg-subtle:  120 120 128;

  /* 边框 */
  --color-border-base:   56 56 62;
  --color-border-strong: 92 92 100;

  /* 强调色 — 主行动 + hover */
  --color-accent:       96 165 250;  /* sky-400 */
  --color-accent-hover: 129 184 255;

  /* 状态色 */
  --color-danger:  248 113 113;      /* red-400 */
  --color-success: 74 222 128;       /* green-400 */
}
```

Tailwind 通过 `tailwind.config.js` 把这些 token 映射成 utility:

```js
// tailwind.config.js (节选)
colors: {
  bg:     { base: 'rgb(var(--color-bg-base) / <alpha-value>)', panel: ..., elevated: ... },
  fg:     { base: '...', muted: '...', subtle: '...' },
  border: { base: '...', strong: '...' },
  accent: { DEFAULT: '...', hover: '...' },
  danger:  '...',
  success: '...',
}
```

约定:**所有视图代码通过 utility 用色**(`bg-bg-panel` / `text-fg-muted` / `border-border-strong` / `text-accent` / `text-danger` / `text-success`),不直接写 `#abc` 或 hex。换主题时只改 `tokens.css`,utilities 自动跟随。

字体:`tailwind.config.js` 注册 `font-sans`(`-apple-system, BlinkMacSystemFont, Segoe UI, Microsoft YaHei, sans-serif`)和 `font-mono`(`ui-monospace, Menlo, Consolas`)。

整体风格:**深色高对比、紧凑 NLE 风(类 Premiere / Resolve)、蓝色强调 CTA**。当前只有一套深色主题,未来加浅色主题可在 `:root` 之外加 `prefers-color-scheme: light` 块。

## 3. TopBar(`components/layout/TopBar.vue`)

`<header class="flex h-12 shrink-0 items-center justify-between border-b border-border-base bg-bg-panel px-4">`,从左到右:

| 元素 | 说明 |
|------|------|
| 🎬 + "Easy FFmpeg" | 文本商标 |
| 程序版本 chip | `v{store.version}`,`bg-bg-elevated text-fg-muted font-mono`,空载时不渲染 |
| FFmpeg 状态 chip | 三态由 `useFfmpegStore.tone` 决定:`pending` / `ok` / `err`;`ok` 态点击调 `fsApi.reveal(cacheDir)` 打开缓存目录,`title` 显示完整版本串 |
| 退出按钮 | `border border-border-strong`,点击 `quitApi.quit()` 后 Teleport 一个 👋 全屏覆盖告知用户可关页 |

退出后会清场所有 `<audio>/<video>`(pause / removeAttribute('src') / load()),避免编辑器预览继续在后台播放。

## 4. 控件家族

控件不再以 BEM 类(`.btn-primary` / `.status-chip` 等)出现,而是直接用 Tailwind utility 表达。下表是高频组合的"模式语言":

### 4.1 按钮

| 模式 | 典型 utility 组合 |
|------|------|
| 主行动(开始转码 / 开始执行) | `rounded bg-accent px-4 py-1.5 text-xs text-bg-base hover:bg-accent-hover disabled:opacity-50` |
| 次要(选择文件 / 选择目录) | `rounded border border-border-strong bg-bg-elevated px-3 py-1.5 text-xs hover:bg-bg-panel` |
| 危险(取消) | `rounded border border-danger px-4 py-1.5 text-xs text-danger hover:bg-danger/10 disabled:opacity-40` |
| 幽灵(关闭 / × / 退出) | `text-fg-muted hover:text-fg-base` 或不加边框的 `px-2 py-1` |
| 图标按钮(📂) | `shrink-0 rounded border border-border-strong bg-bg-elevated px-3 py-1.5 text-xs disabled:opacity-40` |

### 4.2 状态 chip(TopBar)

`rounded px-2 py-1 transition-colors` + 三态 class:

- `bg-bg-elevated text-fg-muted` — pending(检测中)
- `bg-success/15 text-success` — ok(可点击的话再加 `cursor-pointer hover:bg-success/25`)
- `bg-danger/15 text-danger` — err

### 4.3 命令预览块

```html
<pre class="overflow-auto rounded border border-border-base bg-bg-base p-2
            font-mono text-xs leading-relaxed text-fg-base
            whitespace-pre-wrap break-all">{{ command }}</pre>
```

模态版还会启用 `cursor-pointer`(整块点击复制,见 §5.1)。

### 4.4 日志区(`components/job/JobLog.vue`)

三 Tab 共用的子组件,绑定 `useJobPanel` 暴露的 ref:

- 容器:`bg-bg-base text-fg-base font-mono text-xs flex flex-col overflow-auto`(剪辑器导出场景下父容器约束最大高,保持时间轴可见)
- 行 class:基础 `text-fg-base`,修饰类 `text-warning`(进度暖色)/ `text-success`(完成)/ `text-danger`(失败)/ `text-accent`(命令回显)/ `text-fg-muted`(已取消等淡色)
- 进度行原地覆盖在 `useJobPanel.appendLog` 内完成,渲染层只 `v-for` 显示当前快照,DOM 节点不爆

### 4.5 segmented(音频 Tab 模式切换)

`AudioView.vue` 顶部三按钮(转换 / 提取 / 合并),活动按钮 `bg-bg-elevated text-accent`,非活动 `text-fg-muted hover:text-fg-base`,公用一个 `useJobPanel`,切换时只换子组件不换面板。

### 4.6 进度条(`<JobLog>` 内,见 §6.1)

```html
<div class="h-1 w-full overflow-hidden rounded bg-bg-elevated">
  <div class="h-full bg-accent transition-[width] duration-150 ease-linear"
       :style="{ width: progress * 100 + '%' }" />
</div>
<span class="tabular-nums text-fg-muted">{{ percent }}%</span>
```

`tabular-nums` 防数字位数变化时整行抖动;空闲态整块用 `v-if="progressVisible"` 折叠。

### 4.7 音频合并文件列表(`AudioMergeMode.vue`)

可排序列表:`<ul>` 加 `divide-y divide-border-base`,空态用 `v-if`/`v-else` 显示"尚未添加文件";每项带 `☰` 抓手(装饰)、编号、文件名(`truncate`)、元信息(codec · 声道 · kbps · 时长)、↑/↓/🗑 三个操作按钮。

### 4.8 编辑器子组件

样式集中在 `components/editor/*.vue` 各自的 scoped 区(只在必要时);布局用 utility。详见 [tabs/editor/product.md](../tabs/editor/product.md) §UI。

## 5. 对话框约定

四个全局对话框都通过 `useModalsStore` 暴露 Promise API:`showCommand` / `showOverwrite` / `showPicker` 返回各自类型的 Promise。任意视图都可以 `await` 这些函数,不用 prop 透传或事件总线。

### 5.1 命令预览(`ConfirmCommandModal.vue`)

```text
┌─ 即将执行 ─────────────────────────────────┐
│  下列 ffmpeg 命令将被执行,确认后开始:        │
│  ┌──────────────────────────────────────┐  │
│  │ ffmpeg -y -i "..." -filter_complex   │  │ ← 整块 click-to-copy
│  │   "..." -map [v] -map [a] ...        │  │
│  └──────────────────────────────────────┘  │
│  点击命令框可复制                              │
│  ─────────────────────────────────────────  │
│  [📋 复制]                  [取消] [开始执行] │
└────────────────────────────────────────────┘
```

- 720px 宽,`<pre>` 等宽字体 280px 最大高滚动
- 整块点击复制,优先 `navigator.clipboard.writeText`,失败回退 `<textarea> + execCommand("copy")`(WebView2 / 旧 WebKit 兜底);提示语短暂变 accent 色"✓ 已复制"
- Enter=开始执行;Esc=取消;Enter 在 `<pre>` 上不触发(留给文本选择)

### 5.2 覆盖确认(`ConfirmOverwriteModal.vue`)

460px 宽。header "目标文件已存在" + ×;body 一句中文 + 等宽字体路径(`break-all`);footer "取消" / "覆盖"。Enter=覆盖 / Esc=取消。

### 5.3 文件 / 目录选择(`PickerModal.vue`)

```text
┌─────────────────────────────────────────────────────┐
│ 选择输入视频                                     ×   │
├─────────────────────────────────────────────────────┤
│ [C:/ ▾] [/ Users / zhouyijin                    ] ↑ │
├─────────────────────────────────────────────────────┤
│ 📁 Desktop                                           │
│ 📁 Documents                                         │
│ 📄 video.mp4                             12.3 MB    │
├─────────────────────────────────────────────────────┤
│ 选中一个文件后点击确认      [取消]  [选择文件]       │
└─────────────────────────────────────────────────────┘
```

- 三层:header / breadcrumb-bar / body / footer
- breadcrumb-bar:可选盘符下拉(Windows)+ 可编辑路径输入 + 上一级 ↑
- body:目录在前,文件在后,同类按字典序(不区分大小写);以 `.` 开头条目隐藏
- 单击选中(`bg-bg-elevated`),双击目录进入 / 双击文件直接完成
- 数据源:`fsApi.home()` / `fsApi.list(path)` / `fsApi.reveal(path)`(后端驱动,因为浏览器 `<input type=file>` 拿不到真实路径)

### 5.4 解压加载遮罩(`PrepareOverlay.vue`)

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

- 全屏 `backdrop-blur-sm`(用 Tailwind 的 backdrop utility)
- 居中卡片 460px 宽:标题 + 副文案 + 进度条 + 百分比 + 当前文件
- 进度条:`bg-success → bg-accent` 渐变(自定义 background-image),0.25s 缓动
- ready 时叠 `transition-opacity duration-300` 淡出后 `v-if=false` 卸载
- 错误态:不卸载,副文案变红、进度条变红

### 5.5 模态弹窗的统一约定

- **不响应**点背景空白 — 误触代价高
- × 关闭 / Esc / 取消 三种方式退出,行为等价
- Enter 在确认型 dialog = 主行动
- 焦点:打开聚焦主按钮,关闭还原前一个聚焦元素

## 6. 跨 Tab 共用的导出体验

视频转换 / 音频处理 / 单视频剪辑都通过 `useJobPanel` + `useModalsStore` 触发任务,共享同一套交互流(详见 [frontend.md §6–§7](frontend.md)):

```text
点击"开始"按钮
    │
    ▼
① 后端 dryRun POST     {…params, dryRun: true}
   后端:构建参数 + 构造命令字符串,但不 mkdir、不查 overwrite、不启 ffmpeg
   返回 200 + {command: "ffmpeg -y -i ... <out>"}
    │
    ▼
② await modals.showCommand(cmd)  → ConfirmCommandModal Promise<boolean>
    │
    ▼ 用户确认
③ 后端真实 POST       {…params}
    │
    ├─ 409 + {existing:true, path}
    │     ▼
    │   await modals.showOverwrite(path) → 同意带 overwrite:true 重发;拒绝中止
    │
    └─ 200 → SSE(jobBus)开始推日志 → useJobPanel.appendLog 解析 time= 算进度
        │
        ▼
       ④ 终态:done / error / cancelled
          → 完成条 + 进度条 done 后 100% 停 600ms 再隐藏
```

### 6.1 进度条

- **位置**:动作行下方一条独立的轨 + 百分比标签,三 Tab 各一份(`<JobLog>` 内),由 `useJobPanel` 公共逻辑驱动
- **数据源**:解析 ffmpeg stderr 里的 `time=HH:MM:SS.ms`(当前进度)和首次出现的 `Duration: HH:MM:SS.ms`(总时长)。编辑器导出时 `useJobPanel.startJob({ totalDurationSec })` 显式传节目时间总长,比 `Duration:`(源文件长度)更准
- **生命周期**:启动 → 0% → 跟随 `time=` 实时增长 → `done` 停 100% 600ms 后隐藏 → `error/cancelled` / 启动失败立即隐藏

### 6.2 命令预览(dryRun 协议)

- 协议:所有三个 endpoint 都接受 `dryRun: true`,返回 `{ok, dryRun, command}`,不动文件不启进程;merge mode 的临时 list 文件在 dryRun 路径上立即 cleanup
- UI:见 §5.1
- 接管 Enter / Esc 全局键

### 6.3 覆盖确认

- 协议:未带 `overwrite:true` 时,后端 `os.Stat(outPath)` 命中 → `409 + {existing:true, path}`
- UI:见 §5.2
- 三 endpoint 协议统一,`useJobPanel` 调用方一份代码处理所有 Tab(各 View 自己写一段 sendStart 包覆盖重试,沿用 v0.5.0 起的 ConvertView 模板)

## 7. 已知视觉问题

- **字体**:`-apple-system, BlinkMacSystemFont, Segoe UI, Microsoft YaHei, sans-serif`。在部分 Linux 发行版上仍可能 fallback 到 DejaVu Sans
- **模态在小屏**:固定宽度,移动端会溢出 `max-w-[90vw]`;移动 UX 未认真设计
- **编辑 Tab 在矮屏**:时间轴高度固定,预览区域弹性占用;窄屏下预览偏小,后续可加"预览全屏"按钮
- **merge 拖拽排序**:目前只用 ↑/↓ 按钮,没做原生拖拽;长列表效率偏低

## 8. 国际化

全中文硬编码,未做 i18n 基础设施。后续要做:

- 提取所有中文文案到 `web/src/i18n/{zh,en}.json`,接入 vue-i18n 或类似库
- 前端按 `navigator.language` 或用户设置选择
- 后端的错误消息也需要国际化(目前返回英文 `error: ...`)

见 [roadmap.md](roadmap.md) §3。
