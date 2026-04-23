# 模块设计

逐目录/包拆解项目组成，列出关键函数、类型、协作关系。

## 1. `cmd/`

**职责**：程序入口。

### `main.go`

- 解析 `EASY_FFMPEG_ADDR` 环境变量（默认 `127.0.0.1:0`）
- `server.New()` → `Listen(addr)` 返回实际绑定地址
- 打印 banner + 访问地址 + 退出提示
- **关键决策**：`go service.Prepare()` 放进 goroutine 异步解压，不阻塞浏览器打开
- `browser.Open(url)` 拉起系统默认浏览器
- `signal.Notify` 监听 SIGINT/SIGTERM → `srv.RequestShutdown()`
- `srv.Wait()` 阻塞直到 `/api/quit` 或信号触发

依赖：`server`, `service`, `internal/browser`

## 2. `server/`

**职责**：HTTP 服务层——路由、中间件、静态资源、API 处理。

### 2.1 `server.go`

| 符号 | 说明 |
|------|------|
| `Server` struct | 持有 `http.Server`、`job.Manager`、关停信号 chan |
| `New() *Server` | 构造 + 注册路由 |
| `Listen(addr) (string, error)` | 绑定端口，启动 `http.Serve`，返回实际地址 |
| `Wait()` | 阻塞直到收到关停信号，再优雅关停 |
| `RequestShutdown()` | 触发 `Wait` 返回；`sync.Once` 风格幂等 |
| `logMiddleware` | 记录 API 请求日志；排除 `silentPaths` 中的高频轮询端点 |
| `silentPaths` | `map[string]bool{"/api/prepare/status": true}`，避免控制台被解压轮询刷屏 |

### 2.2 API 路由总览

HTTP handler 已经按功能拆分到 3 个文件，职责如下：

| 文件 | 承接的 API | 备注 |
|------|-----------|------|
| `handlers.go` | 共享基础设施 + convert | fs / config / ffmpeg / prepare / quit + convert |
| `handlers_audio.go` | `/api/audio/*` | probe / start / cancel，外加 `scheduleCleanup` 帮助 merge 清理临时列表文件 |
| `handlers_trim.go` | `/api/trim/*` | probe / start / cancel |

| 路由 | Handler | 作用 |
|------|---------|------|
| `GET /` → 静态文件服务器 | （fileserver） | `//go:embed web` 映射 |
| `GET /api/ffmpeg/status` | `handleFFmpegStatus` | 返回 `{available, embedded, version}` |
| `POST /api/ffmpeg/reveal` | `handleFFmpegReveal` | 调 `service.GetFFmpegDir` → `browser.Open` 打开缓存目录 |
| `GET /api/prepare/status` | `handlePrepareStatus` | 返回 `embedded.GetProgress()`（state, percent, current, error） |
| `GET /api/fs/list?path=` | `handleFsList` | 列目录；Windows 附带盘符；按类型+名排序 |
| `GET /api/fs/home` | `handleFsHome` | 返回 `os.UserHomeDir()` |
| `POST /api/fs/reveal` | `handleFsReveal` | 通用：打开任意路径（文件则打开其父目录） |
| `GET /api/config/dirs` | `handleConfigDirs` | 返回保存的 inputDir/outputDir |
| `POST /api/config/dirs` | `handleConfigDirs` | 写入 inputDir/outputDir |
| `POST /api/convert/start` | `handleConvertStart` | 校验 → `buildFFmpegArgs` → `jobs.Start` |
| `POST /api/convert/cancel` | `handleConvertCancel` | `jobs.Cancel()` |
| `GET /api/convert/stream` | `handleConvertStream` | SSE；订阅 `jobs.Subscribe` → 写 `data: <json>\n\n` + Flush（**所有 Tab 共享**） |
| `POST /api/audio/probe` | `handleAudioProbe` | `service.ProbeAudio` → JSON |
| `POST /api/audio/start` | `handleAudioStart` | `BuildAudioArgs`（convert / extract / merge；merge 的 `auto` 策略在此通过 `resolveMergeStrategy` 用 ffprobe 解析）|
| `POST /api/audio/cancel` | `handleAudioCancel` | `jobs.Cancel()` |
| `POST /api/trim/probe` | `handleTrimProbe` | `service.ProbeVideo` → JSON |
| `POST /api/trim/start` | `handleTrimStart` | `BuildTrimArgs`（trim/crop/scale 可组合） |
| `POST /api/trim/cancel` | `handleTrimCancel` | `jobs.Cancel()` |
| `POST /api/quit` | `handleQuit` | 返回 200 后 `RequestShutdown()` |

关键辅助（handlers.go 内）：
- `buildFFmpegArgs(req convertRequest) []string`：convert Tab 的命令参数数组
- `normalizeVideoCodec(name) string`：`h264 → libx264`，`h265 → libx265` 等
- `normalizeAudioCodec(name) string`：空字符串默认 `aac`

### 2.3 `audio_args.go` — 音频命令构建器

纯函数，无 I/O（merge 的 copy 策略涉及临时文件，但封装在 `writeConcatList` + 返回 `Cleanup` 闭包里，便于测试）。

| 符号 | 说明 |
|------|------|
| `AudioRequest` struct | 三模式的请求体联合（convert/extract/merge 各取所需字段） |
| `AudioBuildResult` struct | `{Args, OutputPath, Cleanup}` |
| `BuildAudioArgs(req)` | 分派到各模式构建器 |
| `buildConvertAudioArgs` | 音频格式转换 / 压缩 |
| `buildExtractAudioArgs` | 从视频提取音轨（`-vn -map 0:a:<idx>`，copy 或 transcode） |
| `buildMergeAudioArgs` | 合并：`copy` 走 concat demuxer + 临时列表文件；`reencode` 走 `-filter_complex concat` |
| `formatConcatList(paths)` | 生成 `-f concat` 列表文件内容；单引号转义 |
| `bitrateApplies(spec, codec, bitrate)` | 判定是否加 `-b:a`（lossless 容器 / PCM / copy 都抑制）|
| `audioFormatTable` | 容器 → 合法编码器白名单（mp3/m4a/flac/wav/ogg/opus） |

详见 [audio-feature-design.md](audio-feature-design.md)。

### 2.4 `trim_args.go` — 视频裁剪命令构建器

纯函数。支持时间裁剪 / 空间裁剪 / 分辨率缩放任意组合。

| 符号 | 说明 |
|------|------|
| `TrimRequest` struct | 主字段 + 三个可启用的操作块：`Trim` / `Crop` / `Scale` |
| `TrimBuildResult` struct | `{Args, OutputPath}` |
| `BuildTrimArgs(req)` | 按启用项叠加参数：`-ss/-to`、`-vf crop=...,scale=...`、`-c:v/-c:a` |
| `validateTrim(t)` | 校验 `HH:MM:SS[.mmm]` + `start < end` |
| `parseTimeSeconds(s)` | 时间字符串 → 秒（浮点） |
| `validateCrop(c)` | 宽高 > 0；X/Y 非负 |
| `resolveScale(s)` | 处理 `KeepRatio` → 把空的一维变 `-2`（ffmpeg 自动等比） |
| `trimTimeRE` | `^(\d{1,2}):(\d{1,2}):(\d{1,2})(?:\.(\d{1,3}))?$` |

详见 [trim-feature-design.md](trim-feature-design.md)。

### 2.5 测试覆盖

| 文件 | 覆盖 |
|------|------|
| `audio_args_test.go` | convert / extract / merge 三种模式的正反路径，formatConcatList 单引号转义，bitrateApplies 矩阵 |
| `trim_args_test.go` | trim+crop+scale 组合矩阵，parseTimeSeconds，resolveScale keep-ratio，错误分支 |

目前 `server` 是唯一有测试的包。其余包暂无 `_test.go`（参考 [roadmap.md](roadmap.md) §2.6）。

### 2.6 `web/` — 前端资源（go:embed）

通过 `//go:embed web` + `fs.Sub` 映射到 `GET /`。内容详见 [ui-design.md](ui-design.md)。

## 3. `service/`

**职责**：业务层门面，对 `server` 屏蔽底层 `embedded` 细节。

### 3.1 `ffmpeg.go`

| 函数 | 行为 |
|------|------|
| `GetFFmpegPath() string` | 嵌入优先；失败返回 `"ffmpeg"`（系统 PATH 降级） |
| `GetFFprobePath() string` | 同上，for ffprobe |
| `CheckFFmpeg() bool` | 嵌入失败时再 `exec.Command("ffmpeg","-version")` |
| `GetFFmpegVersion() string` | 运行 `ffmpeg -version`，取第一行 |
| `IsEmbedded() bool` | 探测嵌入二进制是否可用 |
| `Prepare() error` | 触发 `embedded.GetFFmpegBinary()` 解压；供 `main.go` 在 goroutine 里预热 |
| `GetFFmpegDir() (string, error)` | 返回 ffmpeg 所在目录（用于"在文件管理器打开"功能） |

### 3.2 `probe.go` — ffprobe 封装

统一类型（便于复用）：

| 类型 | 字段 |
|------|------|
| `MediaFormat` | `Duration / BitRate / Size` — 音频视频通用 |
| `AudioStream` | `Index / CodecName / Channels / SampleRate / BitRate / Language / Title`；Index 是**音频流内部** 0-based 位置，供 `-map 0:a:<Index>` |
| `VideoStream` | `CodecName / Width / Height / FrameRate` |
| `ProbeResult` | `Format + Streams []AudioStream`（`ProbeAudio` 返回） |
| `VideoProbeResult` | `Format + Video + Audio *AudioStream`（`ProbeVideo` 返回，取首条音视频流） |

| 函数 | 行为 |
|------|------|
| `ProbeAudio(path) (*ProbeResult, error)` | `ffprobe -select_streams a` 只看音频流 |
| `ProbeVideo(path) (*VideoProbeResult, error)` | 不选流，取首条 video + 首条 audio |
| `runFFprobe(path, extra...) []byte` | 内部 helper；`procutil.HideWindow` 防 Windows 弹黑窗 |
| `parseRational(candidates...)` | 把 ffprobe 的 `"30000/1001"` 这类 rational 串转 float（按顺序 fallback） |

## 4. `internal/embedded/`

**职责**：平台相关二进制嵌入 + 首次启动解压 + 进度追踪。

### 4.1 平台分片（构建标签）

| 文件 | 构建标签 | 嵌入 |
|------|----------|------|
| `embedded_windows.go` | `//go:build windows` | `windows/windows.7z` |
| `embedded_darwin.go`  | `//go:build darwin`  | `darwin/darwin.7z` |
| `embedded_linux.go`   | `//go:build linux`   | `linux/linux.7z` |

每个文件只导出三个符号：`archiveData []byte`、`ffmpegBinaryName`、`ffprobeBinaryName`。

### 4.2 `common.go` — 公共逻辑

| 符号 | 说明 |
|------|------|
| `Progress` struct | `{State, Percent, Current, Error}`，JSON 暴露给前端 |
| `GetProgress() Progress` | 线程安全快照读取 |
| `setProgress(fn)` | 线程安全更新 |
| `GetFFmpegBinary() (string, error)` | 走 `ensureExtracted()` → 返回 `<cacheDir>/ffmpeg[.exe]` |
| `GetFFprobeBinary() (string, error)` | 同上 |
| `CheckEmbeddedFFmpeg() bool` | 抽出后运行 `-version` |
| `ensureExtracted()` | `sync.Once` 包裹的单次执行 |
| `extractArchive()` | 主流程：检查 `.ok` → 解压 → 写标记 |
| `cacheDir()` | `~/.easy-ffmpeg/bin-<sha256[:4].hex>/` |
| `extractOne(f, destDir)` | 写一个文件并 chmod 755 |
| `progressWriter` | 包装 `io.Writer`，在每次 `Write` 更新全局 doneBytes/percent |
| `startProgressPrinter()` | 返回可 `Stop()` 的控制台进度条渲染器（每 200ms `\r` 重绘） |

### 4.3 解压流程

```
ensureExtracted:
  cacheDir := ~/.easy-ffmpeg/bin-<hash>/
  if exists(cacheDir/.ok):
      setProgress(ready, 100)
      return cacheDir

  MkdirAll(cacheDir)
  print "首次启动：正在解压 FFmpeg 到 ..."

  reader := sevenzip.NewReader(archiveData)
  totalBytes = sum(file.UncompressedSize for file in reader.File)
  setProgress(extracting, 0)

  printer := startProgressPrinter()  # 控制台 \r 重绘

  for f in reader.File:
      setProgress(current=f.Name)
      extractOne(f, cacheDir)   # 内部 progressWriter 更新 percent

  WriteFile(cacheDir/.ok, [])   # 标记成功
  printer.Stop()                # 最终行 + \n
  setProgress(ready, 100)
  print "解压完成 (%.1fs)"
```

## 5. `internal/job/`

**职责**：FFmpeg 任务状态管理 + 事件广播。

### 5.1 `manager.go`

| 符号 | 说明 |
|------|------|
| `Event` struct | `{Type, Line, Message, Running}`；Type ∈ `state\|log\|done\|error\|cancelled` |
| `Manager` struct | 持有 `cmd`、`subscribers map[chan Event]struct{}`、running 标志、cancelled 标志 |
| `New() *Manager` | 构造 |
| `Running() bool` | 线程安全读 |
| `Start(binary, args) error` | 构造 `exec.Cmd` → `StderrPipe` → `Start` → `go pump` |
| `Cancel()` | Kill 当前进程，设 cancelled 标志 |
| `Subscribe() (<-chan Event, func())` | 注册 → 立即发送 `state` 事件 → 返回 unsubscribe 闭包 |
| `pump(cmd, stderr)` | 核心：scanner + 进度行节流 + 广播 + Wait + 最终事件 |
| `broadcast(ev)` | 遍历订阅者，**非阻塞**发送（`select default`） |

### 5.2 进度行节流（`pump` 内）

```
lastEmit := 零值 time.Time
pendingProgress := ""

for scanner.Scan():
    line := scanner.Text()
    if isProgressLine(line):         # 前缀 frame= / size=
        pendingProgress = line
        if time.Since(lastEmit) < 100ms:
            continue                  # 丢弃本次，只留最新
        broadcast({type:log, line})
        pendingProgress = ""
        lastEmit = Now()
    else:
        broadcast({type:log, line})  # 非进度行一条不丢
        lastEmit = Now()

# 循环结束前补发
if pendingProgress != "":
    broadcast(...)
```

### 5.3 自定义 scanner splitter

`scanLinesOrCR` 同时在 `\r` 和 `\n` 处切分。这是必须的：FFmpeg 每次刷新进度写的是 `\r`（覆盖同一行），标准 `bufio.ScanLines` 只识别 `\n` 会导致所有进度在一整段累积，直到程序结束才吐出来。

### 5.4 子进程窗口抑制

`job.Manager.Start` 在 `exec.Command` 后立刻调 `procutil.HideWindow(cmd)`（见 §6.5）。Windows 下会设 `CREATE_NO_WINDOW` 标志位，防止每次转码/探测都弹黑色控制台；其他平台是空 no-op。同一 helper 被 `service.probe.go` 的 `runFFprobe` 复用。

## 6. `internal/procutil/`

**职责**：抽出 `job` 与 `service/probe` 共用的子进程跨平台适配。避免在两个包里各维护一份 `hide_*.go`。

| 文件 | 构建标签 | 导出 |
|------|----------|------|
| `hide_windows.go` | `//go:build windows`  | `HideWindow(cmd *exec.Cmd)` 设置 `CREATE_NO_WINDOW` |
| `hide_other.go`   | `//go:build !windows` | `HideWindow` no-op |

## 7. `internal/browser/`

**职责**：跨平台打开 URL 或本地路径。

### `open.go`

```go
func Open(url string) error {
    switch runtime.GOOS {
    case "windows": exec.Command("cmd", "/c", "start", "", url)
    case "darwin":  exec.Command("open", url)
    default:        exec.Command("xdg-open", url)
    }
}
```

对 URL 和本地路径都适用（`start` 会派给 URL handler 或 Explorer，取决于参数形式）。

## 8. `config/`

**职责**：用户偏好持久化。

### `config.go`

| 函数 | 存储文件 |
|------|----------|
| `GetInputDir` / `SaveInputDir` | `<UserConfigDir>/easy-ffmpeg/input_dir.txt` |
| `GetOutputDir` / `SaveOutputDir` | `<UserConfigDir>/easy-ffmpeg/output_dir.txt` |

纯文本单行。后续要加更多配置项可以升级为单个 JSON/TOML。

## 9. 根级文件与 `tools/`

| 文件 | 作用 |
|------|------|
| `build.bat` / `build.sh` | 一键编译四平台（Windows / macOS arm64 & amd64 / Linux），并为 macOS 二进制自动封 `.app` Bundle |
| `go.mod` / `go.sum` | Go 依赖描述 |
| `tools/build_icon.go` | 开发期：把 PNG 图标烧成 Windows 资源文件（生成 `cmd/rsrc_windows.syso`） |
| `tools/build_macapp.go` | 把 macOS 纯二进制包成 `.app` Bundle（含 Info.plist + icon.icns），供 `build.{bat,sh}` 的最后一步调用 |
| `tools/download_windows.go` | 历史：从 gyan.dev 下载 Windows FFmpeg；当前已被 7z 嵌入方案取代，保留为参考 |
| `assets/icon.svg` / `icon.icns` | 品牌图标源文件 |

## 10. 依赖清单（go.mod 间接 + 直接）

| 依赖 | 用途 |
|------|------|
| `github.com/bodgit/sevenzip` | 纯 Go 7z 读取，支持 LZMA2+BCJ2 |
| `github.com/bodgit/plumbing` | sevenzip 的工具库 |
| `github.com/bodgit/windows` | Windows 特定实用函数 |
| `github.com/klauspost/compress` | sevenzip 使用的 deflate 等压缩方法 |
| `github.com/ulikunitz/xz` | sevenzip 使用的 LZMA/LZMA2 解码 |
| `github.com/pierrec/lz4/v4` | sevenzip 使用的 LZ4 |
| `github.com/andybalholm/brotli` | sevenzip 使用的 brotli |
| `github.com/hashicorp/golang-lru/v2` | sevenzip 内部缓存 |
| `github.com/spf13/afero` | sevenzip 使用的文件抽象 |
| `golang.org/x/text` | sevenzip 的字符编码支持 |
| `go4.org` | 其他工具 |

所有间接依赖都是纯 Go，无 cgo。前端零依赖（HTML/CSS/JS 原生）。
