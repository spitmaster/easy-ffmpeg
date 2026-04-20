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

### 2.2 `handlers.go` — 所有 API 处理器

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
| `GET /api/convert/stream` | `handleConvertStream` | SSE；订阅 `jobs.Subscribe` → 写 `data: <json>\n\n` + Flush |
| `POST /api/quit` | `handleQuit` | 返回 200 后 `RequestShutdown()` |

关键辅助：
- `buildFFmpegArgs(req convertRequest) []string`：构造命令参数数组
- `normalizeVideoCodec(name) string`：`h264 → libx264`，`h265 → libx265` 等
- `normalizeAudioCodec(name) string`：空字符串默认 `aac`

### 2.3 `web/` — 前端资源（go:embed）

通过 `//go:embed web` + `fs.Sub` 映射到 `GET /`。内容详见 [ui-design.md](ui-design.md)。

## 3. `service/`

**职责**：业务层门面，对 `server` 屏蔽底层 `embedded` 细节。

### `ffmpeg.go`

| 函数 | 行为 |
|------|------|
| `GetFFmpegPath() string` | 嵌入优先；失败返回 `"ffmpeg"`（系统 PATH 降级） |
| `GetFFprobePath() string` | 同上，for ffprobe |
| `CheckFFmpeg() bool` | 嵌入失败时再 `exec.Command("ffmpeg","-version")` |
| `GetFFmpegVersion() string` | 运行 `ffmpeg -version`，取第一行 |
| `IsEmbedded() bool` | 探测嵌入二进制是否可用 |
| `Prepare() error` | 触发 `embedded.GetFFmpegBinary()` 解压；供 `main.go` 在 goroutine 里预热 |
| `GetFFmpegDir() (string, error)` | 返回 ffmpeg 所在目录（用于"在文件管理器打开"功能） |

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

### 5.4 平台分片

| 文件 | 构建标签 | 作用 |
|------|----------|------|
| `hide_windows.go` | `//go:build windows` | `cmd.SysProcAttr.CreationFlags = CREATE_NO_WINDOW` 防止弹黑窗 |
| `hide_other.go`   | `//go:build !windows` | 空实现 |

## 6. `internal/browser/`

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

## 7. `config/`

**职责**：用户偏好持久化。

### `config.go`

| 函数 | 存储文件 |
|------|----------|
| `GetInputDir` / `SaveInputDir` | `<UserConfigDir>/easy-ffmpeg/input_dir.txt` |
| `GetOutputDir` / `SaveOutputDir` | `<UserConfigDir>/easy-ffmpeg/output_dir.txt` |

纯文本单行。后续要加更多配置项可以升级为单个 JSON/TOML。

## 8. 根级文件

| 文件 | 作用 |
|------|------|
| `build.bat` | Windows cmd/PowerShell 一键编译四平台 |
| `build.sh`  | bash 一键编译四平台（macOS/Linux/Git Bash） |
| `go.mod` / `go.sum` | Go 依赖描述 |
| `tools/download_windows.go` | 开发期：从 gyan.dev 下载 Windows FFmpeg；当前已被 7z 方案取代，保留为历史工具 |

## 9. 依赖清单（go.mod 间接 + 直接）

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
