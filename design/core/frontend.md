# 前端架构(程序设计)

> 本文档定义前端的技术栈、模块结构、初始化顺序、SSE 处理、`createJobPanel` 工厂、解压进度轮询。视觉规范、控件外观、对话框约定见 [ui-system.md](ui-system.md)。

## 1. 技术栈

- **纯静态三件套**:`index.html` + `app.css` + `app.js`
- **无构建**:没有 Vite/webpack/Tailwind;CSS 变量 + 原生 JS
- **打包方式**:`//go:embed web` 把整个 `server/web/` 目录塞进可执行文件
- **通信协议**:
  - `fetch/JSON` 用于命令式操作
  - `EventSource` (SSE) 用于 FFmpeg 日志实时推送
- **浏览器兼容**:现代浏览器(Chrome/Edge/Firefox 最近 3 年的版本)即可
- **桌面版引擎兼容**:WebView2(Win)/ WKWebView(macOS)/ WebKitGTK(Linux),前端代码不感知宿主,见 [desktop.md §8](desktop.md)

## 2. JS 模块组织

单文件、无框架,全部用 **IIFE 模块**组织,每个模块只导出 init 或少量方法。职责分离清晰(SRP);加新 Tab 不需要动既有模块。

### 2.1 模块一览

| 模块 | 类型 | 职责 |
|------|------|------|
| `$` | helper | `document.getElementById` 缩写 |
| `Http` | helper | `fetchJSON(url, opts)` / `postJSON(url, body)` |
| `Fmt` | helper | `human(size)` 人类可读字节 |
| `Path` | helper | `join` / `basename` / `dirname` / `stripExt` |
| `Time` | helper | `HH:MM:SS[.mmm]` 严格解析与格式化(供 EditorTab 等复用) |
| `Dirs` | IIFE | 输入 / 输出目录缓存与持久化(`/api/config/dirs`) |
| `AppVersion` | IIFE | 顶栏程序版本号 chip(`/api/version`) |
| `FFmpegStatus` | IIFE | 顶栏 FFmpeg 版本 chip 加载与点击跳转缓存目录 |
| `Picker` | IIFE | 共享的文件 / 目录选择模态框(mode=file\|dir,Promise 风格) |
| `JobBus` | IIFE | **全局单 EventSource**(`/api/convert/stream`),广播事件给所有订阅者 |
| `Confirm` | IIFE | 自绘的覆盖确认 / 命令预览两种 dialog,`Confirm.overwrite(path)` / `Confirm.command(cmd)` 都返回 `Promise<boolean>`,共用键盘 + 焦点 + Promise 状态机 |
| `createJobPanel(opts)` | 工厂 | 每个 Tab 独立持有的日志 / 动作行 / 完成条 / 进度条控制器,封装 dryRun preflight + 命令预览 + 覆盖确认 + SSE 订阅 + "owning" 逻辑 |
| `ConvertTab` | IIFE | 视频转换表单 + 预览 + 开始 |
| `AudioCodecs` | IIFE | 共享的容器/编码器/码率知识,供三种音频模式复用(DRY) |
| `AudioConvertMode` / `AudioExtractMode` / `AudioMergeMode` | IIFE | 音频三种模式各自的字段组与命令预览 |
| `AudioTab` | IIFE | 挂载三模式 + segmented 切换 + 调用 `createJobPanel` |
| `EditorApi` / `EditorStore` / `History` / `TL` | IIFE | 剪辑器数据层:fetch wrappers、单一状态源 + 自动保存、撤销/重做栈、节目/源时间换算 |
| `Preview` | IIFE | `<video>` 封装 + 节目时间↔源时间映射、跨 clip 连播、seek、WebAudio gain pipeline |
| `Timeline` | IIFE | 时间轴 DOM 渲染 + 拖拽交互(分割/删除/重排/修剪/范围选区) |
| `ProjectsModal` | IIFE | 剪辑记录模态框(列表 / 打开 / 删除) |
| `ExportModal` | IIFE | 导出对话框 + 复用 `createJobPanel` 走 SSE |
| `EditorTab` | IIFE | 剪辑器顶层:绑定 DOM、键盘快捷键、调度 render(子模块在 `editor/editor.js` 里,与主 `app.js` 分文件) |
| `Tabs` | IIFE | 点击 `.tab` 切换 `.panel` 的显隐 |
| `Quit` | IIFE | 右上角退出按钮 |
| `Prepare` | IIFE | 首次启动解压轮询与遮罩 |

### 2.2 初始化顺序

```js
(async () => {
  await Prepare.wait();      // 解压遮罩;ready 之后继续
  AppVersion.init();         // 程序版本号 chip
  FFmpegStatus.init();       // FFmpeg 版本 chip
  await Dirs.load();         // 预取目录配置
  Picker.init();             // 挂载 picker 模态框事件
  ConvertTab.init();         // Tab 初始化顺序无依赖
  AudioTab.init();
  if (typeof EditorTab !== "undefined") EditorTab.init();  // editor.js 在 app.js 之后加载
  Tabs.init();               // 绑定 tab 切换
  Quit.init();
  Confirm.init();            // 共享 dialog(overwrite / command preview)
  JobBus.connect();          // 开 SSE,事件开始流入所有 panel
})();
```

## 3. 任务面板模式(`createJobPanel`)

每个 Tab 有自己的日志区 / 开始按钮 / 取消按钮 / 完成条 / 进度条。`createJobPanel` 是工厂函数,接受所有这些 DOM 引用 + `cancelUrl` + 标签文案。

行为:

- 订阅 `JobBus`(单例 SSE)
- 内部维护 `owning` 标志:只有"从自己发起任务"的 panel 才响应 log/done/error/cancelled 事件;其他 panel 收到后忽略
- `start({url, body, outputPath, totalDurationSec})` 完整流程:
  1. **dryRun preflight**:先 POST `{...body, dryRun: true}` 拿真实命令字符串
  2. `await Confirm.command(cmd)` → 用户拒绝则直接返回不显示进度条
  3. 用户同意 → 显示进度条 → 真实 POST(无 dryRun)
  4. 若 409 → `await Confirm.overwrite(data.path)` → 同意带 `overwrite:true` 重发
  5. 启动成功后写入 "> ffmpeg …" 回显、置 `owning=true`、`setRunning(true)`
- 进度条:解析每行日志的 `time=HH:MM:SS.ms`(进度) 和 `Duration: HH:MM:SS.ms`(总长 — 没显式传 `totalDurationSec` 时的回退);`done` 短暂保留 100% 600ms 后隐藏(避免连发任务时进度条直接归零的视觉跳变),`error/cancelled` 立即隐藏
- cancel 按钮绑定到构造参数中的 `cancelUrl`(各 Tab 传自己的 `/api/*/cancel`)

这样三个 Tab 共享一条 SSE,但只有发起方看到 log / finish bar / 进度条 / 弹窗。

## 4. 进度行原地覆盖(`createJobPanel.appendLog`)

```js
const PROGRESS_RE = /^(frame=|size=|video:|Lsize=)/;
if (isProgress && lastLine.classList.contains("progress")) {
  lastLine.textContent = text;   // 原地覆盖
} else {
  /* append new span */
}
parseForProgress(text);  // 顺带抽 Duration / time= 喂进度条
```

前端视觉上像终端实时刷新;DOM 节点数量不随时间增长。日志文本和进度条共享同一份输入,无需额外事件通道。

## 5. SSE 自动重连

```js
es.onerror = () => setTimeout(connect, 1500);
```

浏览器刷新 / 服务重启 / 网络故障都自愈。

## 6. 解压进度轮询

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

## 7. 跨 Tab 通用交互

- **日志自动滚动到底部**:`requestAnimationFrame` 后设 `scrollTop = scrollHeight`
- **完成条** 成功时可"📂 打开文件夹"(用记录的 outputPath)
- **执行前命令预览**:所有"开始"按钮都先经 dryRun 拿到真实命令 → `Confirm.command` 弹自绘 dialog(一键复制 / 取消 / 开始执行)→ 用户确认才真正执行
- **覆盖确认**:后端 409 + `existing:true` → `Confirm.overwrite` 自绘 dialog(替代浏览器原生 `confirm`,Esc/取消 / Enter/覆盖)→ 同意带 `overwrite:true` 重试
- **模态弹窗约定**:所有自绘 dialog **不再点背景空白处关闭**(误触代价高);统一靠右上角 × / Esc / 取消按钮退出

## 8. 新增 Tab 的入口

1. `web/index.html` 的 `<nav class="tabs">` 去掉对应 button 的 `disabled`
2. `web/index.html` 的 main 区域加一个 `<section class="panel hidden" id="panel-xxx">`
3. `web/app.js` 里新增 `XxxTab` IIFE,并在 init 序列里调用 `XxxTab.init()`(`Tabs.init()` 会自动识别 `[data-tab]` 按钮,不需要改切换逻辑)
4. 后端加对应 API endpoint(`handlers_xxx.go`)+ 纯函数 `xxx_args.go` + 测试

## 9. 文件组织

```text
server/web/
├── index.html           主页,引入 app.css + editor/editor.css + app.js + editor/editor.js
├── app.css              主样式 + 各 Tab 共用控件
├── app.js               所有共享模块 + ConvertTab + AudioTab + Picker + JobBus + Confirm
└── editor/
    ├── editor.css       剪辑器专属样式(`#panel-editor` 前缀)
    └── editor.js        EditorTab + EditorStore + Timeline + Preview + ...
```

剪辑器静态资源放 `editor/` 子目录,为未来剥离独立 exe 做铺垫(独立入口可以只 import `editor` 包并服务这套资源)。

## 10. DOM 命名约定

- 所有剪辑器元素 id 以 `ed` 开头(`edVideo`、`edRuler`、`edTimeline` 等),避免与其它 Tab 命名冲突
- 各 Tab 的 panel id 是 `panel-<tab>`(`panel-convert` / `panel-audio` / `panel-editor`)
- 通用对话框 id 用 `<feature>Backdrop`(`pickerBackdrop` / `confirmBackdrop`)
