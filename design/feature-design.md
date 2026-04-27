# 功能设计

## 1. 功能全景

| Tab | 状态 | 说明 |
|-----|------|------|
| 视频转换 | ✅ 已实现 | 核心功能，格式 / 编解码转换 |
| 单视频剪辑 | ✅ 已实现 (MVP) | 时间轴式单视频剪辑器，替代旧裁剪功能（详见 [editor-feature-design.md](editor-feature-design.md) + [editor-module-design.md](editor-module-design.md)） |
| 音频处理 | ✅ 已实现 | 三模式：格式转换 / 从视频提取 / 合并（详见 [audio-feature-design.md](audio-feature-design.md)） |
| 媒体信息 | 🚧 占位 | 同上；已经嵌入 ffprobe 为未来做准备 |
| 设置 | 🚧 占位 | 同上 |

全局功能：
- ✅ FFmpeg 版本显示（右上角 chip，含"嵌入"/"系统"标识）
- ✅ 点击 chip 在文件管理器打开 FFmpeg 缓存目录
- ✅ 输出目录"打开"按钮（📂）
- ✅ 首次启动解压进度条（浏览器遮罩 + 控制台 \r 重绘）
- ✅ 优雅关停（右上角"退出"按钮 / Ctrl+C）
- ✅ 输入输出目录记忆

## 2. 视频转换（核心功能）

### 2.1 用户流程

```
进入"视频转换"Tab
    │
    ▼
"选择文件"按钮 → 打开后端驱动的文件浏览器模态框
    └─ 上次输入目录自动作为起始路径
    │
    ▼ 选中文件
inputPath 填入
输出文件名自动补为 "<原名>_converted"
输入目录保存到配置
    │
    ▼
"选择目录"按钮 → 打开后端驱动的目录浏览器模态框
    └─ 上次输出目录自动作为起始路径
    │
    ▼
编码器（视频/音频）+ 容器格式默认 H.264/AAC/MP4，可自选
    │
    ▼
命令预览区实时显示最终 ffmpeg 命令（任一字段变化都触发刷新）
    │
    ▼
"开始转码"（绿色按钮）→ POST /api/convert/start
    │
    ├─ 400（字段缺失） → dialog/alert
    └─ 200 → 订阅 SSE 开始拉取日志
          │
          ├─ 用户点"取消"→ POST /api/convert/cancel → Kill ffmpeg
          │
          ├─ type=log 进度行    → 原地覆盖上一条
          ├─ type=log 其他      → 新行追加
          ├─ type=done          → 日志尾"✓ 转码完成"；按钮恢复空闲
          ├─ type=error         → 日志尾"✗ 转码失败：…"；按钮恢复；alert
          └─ type=cancelled     → 日志尾"! 转码已取消"；按钮恢复
```

### 2.2 表单字段

| 字段 | 控件 | 默认 | 说明 |
|------|------|------|------|
| 输入文件 | Browse 按钮 + 只读文本框 | — | 路径由后端文件浏览器返回 |
| 输出目录 | Browse 按钮 + 📂 打开按钮 + 文本框 | 上次选择 | 📂 按钮在目录非空时启用 |
| 输出文件名 | 文本框 | `<输入名>_converted` | 不含后缀 |
| 视频编码器 | select | h264 | 见下表 |
| 音频编码器 | select | aac | 见下表 |
| 输出格式 | select | mp4 | 作为文件后缀 |

### 2.3 视频编码器

| 前端 value | FFmpeg 实际参数 | 备注 |
|------------|-----------------|------|
| `h264` | `libx264` | 默认 |
| `h265` | `libx265` | |
| `vp9`  | `vp9` | |
| `av1`  | `av1` | |
| `mpeg4` | `mpeg4` | |
| `copy` | `copy` | 快速拷贝，不重新编码 |

### 2.4 音频编码器

| 前端 value | FFmpeg 参数 |
|------------|-------------|
| `aac`       | `aac` 默认 |
| `mp3`       | `mp3` |
| `libopus`   | `libopus` |
| `libvorbis` | `libvorbis` |
| `copy`      | `copy` |

### 2.5 容器格式

`mp4` · `mkv` · `avi` · `mov` · `flv` · `webm` · `m3u8`

### 2.6 命令构建规则（`buildFFmpegArgs`）

```
args := ["-y", "-i", inputPath]

if videoCodec == "copy" AND audioCodec == "copy":
    args += ["-c", "copy"]
else:
    args += ["-c:v", videoCodec, "-c:a", audioCodec]

args += [outputDir/outputName.format]
```

**与旧 Fyne 版的区别**：原版当视频选 `copy` 时强制音频也 `copy`（忽略音频选择）；新版仅在**两者都 copy** 时才走 `-c copy` 快捷路径，其余情形独立处理——更灵活。

### 2.7 进度展示

- 日志区实时滚动显示 FFmpeg stderr
- 进度行（以 `frame=` / `size=` 开头）在前端原地覆盖上一条，营造"终端刷新"观感
- 服务端节流：每 100ms 最多广播一条进度事件，降低 SSE / DOM 压力
- 其他消息（错误、总结、filter 信息）一条不丢，即时推送

### 2.8 取消与退出

- **用户点"取消"**：`POST /api/convert/cancel` → `cmd.Process.Kill()`；半成品文件不清理
- **浏览器关闭**：SSE 连接断，但 ffmpeg 进程**不会**被 kill（服务端还在运行）
- **用户点"退出"** 或 Ctrl+C：服务端优雅关停；没显式 kill ffmpeg，但进程组通常会随之终止

## 3. 首次启动解压（进度可视化）

### 3.1 用户体验

```
双击 easy-ffmpeg.exe
    │
    ▼
控制台：
  Easy FFmpeg 已启动
  访问地址: http://127.0.0.1:38472/
  关闭服务: Ctrl+C  或  在网页右上角点击「退出」
  首次启动：正在解压 FFmpeg 到 C:\Users\...\.easy-ffmpeg\bin-b9b48d4f ...
  [████████████████░░░░░░░░░░░░░░]  55%  ffprobe.exe     ← \r 原地刷新
    │
    ▼（同时 0.5s 内）
浏览器打开 → 显示全屏遮罩
  大标题："正在准备 FFmpeg"
  副文案："首次启动需要解压内嵌的 FFmpeg 组件，请稍候…"
  进度条（绿→蓝渐变） + 百分比 + 当前文件名
    │
    ▼（约 30-45 秒后）
控制台：
  [██████████████████████████████] 100%                    
  解压完成 (39.5s)
浏览器：遮罩淡出 300ms → 主界面浮现
    │
    ▼
右上角状态 chip："FFmpeg 8.1 · 嵌入"（绿色边框）
用户开始使用
```

### 3.2 后续启动

- `~/.easy-ffmpeg/bin-<hash>/.ok` 存在 → 直接跳过解压
- `GetProgress()` 立即返回 `{state:"ready"}`
- 前端不显示遮罩（零闪烁）
- 控制台也不打印"首次启动"提示

### 3.3 失败处理

- 解压过程中任何错误（磁盘满、权限、7z 损坏）→ `progress.State = "error"`
- 前端遮罩保留可见，副文案改为"解压失败：<原因>"，进度条变红
- 主界面仍会加载（但 `FFmpeg 未安装` 或降级系统 ffmpeg）

### 3.4 服务端日志静默

`/api/prepare/status` 在解压期间被前端 300ms 轮询一次，容易刷屏。`server.logMiddleware` 的 `silentPaths` 白名单将其排除：

```go
var silentPaths = map[string]bool{
    "/api/prepare/status": true,
}
```

## 4. 文件浏览器模态框

由于浏览器的 `<input type=file>` 出于安全限制拿不到本地真实路径，而 FFmpeg 需要真路径，所以文件/目录选择走**后端驱动的**模态框：

- `GET /api/fs/home` → 起始路径
- `GET /api/fs/list?path=<dir>` → 返回条目列表 + 父目录 + Windows 盘符
- 双击目录 → 进入；双击文件（文件模式）→ 选中并关闭
- 模态框支持：
  - 面包屑路径输入框（支持回车跳转）
  - 上一级 ↑ 按钮
  - Windows 盘符下拉框
  - 空目录提示
- 排序：目录在前，文件在后，同类按名字字典序（不区分大小写）
- 隐藏：以 `.` 开头的条目不显示

## 5. FFmpeg 缓存目录访问

右上角的版本 chip 是可点击的：

- 悬停：手型光标 + 暗绿背景 hover + tooltip 显示完整版本号 + "点击打开 FFmpeg 所在文件夹"
- 点击：`POST /api/ffmpeg/reveal`
  - 后端 `service.GetFFmpegDir()` → `browser.Open(dir)` → 系统文件管理器打开
  - 路径形如 `C:\Users\<用户>\.easy-ffmpeg\bin-<hash>\`
- 失败（极少见）：alert

同样的机制驱动"输出目录 📂 按钮"：`POST /api/fs/reveal` + `path`。

## 6. 退出机制

- **网页右上角"退出"按钮**：`confirm()` → `POST /api/quit` → 后端优雅关停
- **Ctrl+C**：`signal.Notify` 触发同样的关停路径
- 关停流程：
  - `http.Server.Shutdown(ctx, 3s timeout)`：等待所有进行中请求完成
  - `srv.Wait()` 返回 → `main()` 执行 `fmt.Println("已退出。")` → 进程结束

## 7. 状态栏版本解析

后端 `GetFFmpegVersion()` 返回完整首行：
```
ffmpeg version 8.1-essentials_build-www.gyan.dev Copyright (c) 2000-2026 the FFmpeg developers
```

前端 `parseFFmpegVersion` 正则：
1. 先试 `ffmpeg version (\d+(?:\.\d+)*)` → 匹配 `8.1` / `6.1.1` 这类纯版本号
2. 回退 `ffmpeg version (\S+)` → 匹配 git 构建如 `N-119999-g1234`

最终 chip 显示：`FFmpeg 8.1 · 嵌入`，tooltip 保留完整版本串。

## 8. 跨 Tab 共用的导出体验

视频转换 / 音频处理 / 单视频剪辑三个 Tab 都通过 `createJobPanel`（[ui-design.md §7.3](ui-design.md)）触发任务，共享同一套交互流：

```
点击"开始"按钮
    │
    ▼
① 后端 dryRun POST     {…params, dryRun: true}
   后端：构建参数 + 构造命令字符串，但不 mkdir、不查 overwrite、不启 ffmpeg
   返回 200 + {command: "ffmpeg -y -i ... <out>"}
    │
    ▼
② 自绘"命令预览"dialog（replaces window.confirm）
   ┌─ 即将执行 ──────────────────────────────────┐
   │ 下列 ffmpeg 命令将被执行，确认后开始：       │
   │ ┌──────────────────────────────────────┐  │
   │ │ ffmpeg -y -i "..." -filter_complex   │  │ ← click-to-copy 整块
   │ │   "..." -map [v] -map [a] ...        │  │
   │ └──────────────────────────────────────┘  │
   │ 点击命令框可复制                            │
   │ ─────────────────────────────────────────  │
   │ [📋 复制]                  [取消] [开始执行] │
   └────────────────────────────────────────────┘
   关闭路径：取消 / × / Esc / 点 [开始执行]
    │
    ▼ 用户确认
③ 后端真实 POST       {…params}
    │
    ├─ 409 + {existing:true, path}
    │     ▼
    │   自绘"覆盖确认"dialog → 同意带 overwrite:true 重发；拒绝中止
    │
    └─ 200 → SSE 开始推日志 + 解析 `time=` 算进度条百分比
        │
        ▼
       ④ 终态：done / error / cancelled → 完成条 + 进度条短暂停 100% 再隐藏
```

### 8.1 进度条

- **位置**：动作行下方一条独立的轨 + 百分比标签（`.progress-wrap`），三 Tab 各一份，由 `createJobPanel` 公共逻辑驱动
- **数据源**：解析 ffmpeg stderr 里的 `time=HH:MM:SS.ms`（当前进度）和首次出现的 `Duration: HH:MM:SS.ms`（总时长）。编辑器导出时 `panel.start({ totalDurationSec })` 显式传节目时间总长，比 `Duration:`（源文件长度）更准
- **生命周期**：启动 → 0% → 跟随 `time=` 实时增长 → `done` 停 100% 600ms 后隐藏 → `error/cancelled/409 取消` 立即隐藏

### 8.2 命令预览 dialog（dryRun 协议）

- 协议：所有三个 endpoint 都接受 `dryRun: true`，返回 `{ok, dryRun, command}` 不动文件不启进程；merge mode 的临时 list 文件在 dryRun 路径上立即 cleanup
- UI：720px 宽 `.modal-command`，`<pre class="confirm-command">` 用等宽字体最高 280px 高滚动；click-to-copy 用 `navigator.clipboard.writeText`,失败回退到隐藏 `<textarea> + execCommand("copy")`
- 接管 Enter / Esc 全局键

### 8.3 覆盖确认 dialog

- 协议：未带 `overwrite:true` 时,`os.Stat(outPath)` 命中则返回 `409 + {existing:true, path}`
- UI：460px 宽 `.modal-confirm`,等宽字体显示路径(`break-all`),Enter=覆盖 / Esc=取消
- 三 endpoint 协议统一,`createJobPanel.start` 一份代码处理所有 Tab

### 8.4 模态弹窗的统一约定

所有自绘 dialog（覆盖确认 / 命令预览 / 编辑器导出配置 / 剪辑记录列表 / 文件选择器）：

- **不响应**点击背景空白区域 —— 太容易误触把正在配置的导出操作丢掉
- **× 关闭按钮**位于右上角(`.modal-header` flex + `.spacer { flex: 1 }`),与"取消"按钮等价
- **Esc** 键关闭(等价于"取消"),**Enter** 在确认型 dialog 上等价于"确认"
- 焦点：打开时聚焦主按钮,关闭时还原到打开前的元素(`lastFocused`)

## 9. 约束与规则

- 所有功能都走 `service` 层调用 FFmpeg/FFprobe，HTTP handler 不直接 `os/exec`
- 长耗时任务都遵循「goroutine + 非阻塞 broadcast + SSE 订阅 + `fetch` + DOM 异步更新」范式
- 命令预览 dialog 显示的命令是 server 真实将执行的命令(经过同样的构造函数,dryRun 仅跳过 mkdir / overwrite check / `Start` 调用),客户端不重复构造,避免显示与执行漂移
- 纯函数命令构建器（`server/*_args.go` / `editor/domain/export.go`）必须保持无 I/O，便于表驱动测试
- 新增 Tab 的入口：
  1. `web/index.html` 的 `<nav class="tabs">` 去掉对应 button 的 `disabled`
  2. `web/index.html` 的 main 区域加一个 `<section class="panel hidden" id="panel-xxx">`
  3. `web/app.js` 里新增 `XxxTab` IIFE，并在 init 序列里调用 `XxxTab.init()`
     （`Tabs.init()` 会自动识别 `[data-tab]` 按钮，不需要改切换逻辑）
  4. 后端加对应 API endpoint（`handlers_xxx.go`）+ 纯函数 `xxx_args.go` + 测试
