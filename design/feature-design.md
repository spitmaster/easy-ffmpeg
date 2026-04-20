# 功能设计

## 1. 功能全景

| Tab | 状态 | 说明 |
|-----|------|------|
| 视频转换 | ✅ 已实现 | 核心功能，格式 / 编解码转换 |
| 视频裁剪 | 🚧 占位 | HTML 里 disabled，未实现 |
| 音频处理 | 🚧 占位 | 同上 |
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

## 8. 约束与规则

- 所有功能都走 `service` 层调用 FFmpeg/FFprobe，HTTP handler 不直接 `os/exec`
- 长耗时任务都遵循「goroutine + 非阻塞 broadcast + SSE 订阅 + fyne.Do 对等物（`fetch` + DOM 异步更新）」范式
- 新增 Tab 的入口：
  1. `web/index.html` 的 `<nav class="tabs">` 加一个 button
  2. `web/index.html` 的 main 区域加一个 `<section class="panel" id="panel-xxx">`
  3. `web/app.js` 里加 tab 切换逻辑（目前还没实现，因为只有一个 tab）
  4. 后端加对应 API endpoint
