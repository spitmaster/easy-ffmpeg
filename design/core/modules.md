# 共享后端模块(程序设计)

> 本文档逐目录拆解共享后端各包的职责、关键函数、协作关系。Tab 专属的命令构建器和 handler 见对应 `tabs/<tab>/program.md`。

## 1. `cmd/`

**职责**:程序入口。

### `main.go`(Web 版)

- 解析 `EASY_FFMPEG_ADDR` 环境变量(默认 `127.0.0.1:0`)
- `server.New()` → `Listen(addr)` 返回实际绑定地址
- 打印 banner + 访问地址 + 退出提示
- **关键决策**:`go service.Prepare()` 放进 goroutine 异步解压,不阻塞浏览器打开
- `browser.Open(url)` 拉起系统默认浏览器
- `signal.Notify` 监听 SIGINT/SIGTERM → `srv.RequestShutdown()`
- `srv.Wait()` 阻塞直到 `/api/quit` 或信号触发

依赖:`server`, `service`, `internal/browser`

### `desktop/main.go` + `desktop/app.go`(桌面版,v0.4.0+)

详见 [desktop.md](desktop.md)。要点:`server.New() + Listen("127.0.0.1:0")` 与 Web 版同构,只是把端口交给 Wails WebView 而非系统浏览器。CGO 必须开启;**不感知** Web 版,反之亦然。

## 2. `server/`

**职责**:HTTP 服务层 —— 路由、中间件、静态资源、API 处理。Tab 专属 handler 在对应 Tab 的 `program.md` 详述。

### 2.1 `server.go`

| 符号 | 说明 |
|------|------|
| `Server` struct | 持有 `http.Server`、`job.Manager`、关停信号 chan |
| `New() *Server` | 构造 + 注册路由 |
| `Listen(addr) (string, error)` | 绑定端口,启动 `http.Serve`,返回实际地址 |
| `Wait()` | 阻塞直到收到关停信号,再优雅关停 |
| `RequestShutdown()` | 触发 `Wait` 返回;`sync.Once` 风格幂等 |
| `logMiddleware` | 记录 API 请求日志;排除 `silentPaths` 中的高频轮询端点 |
| `silentPaths` | `map[string]bool{"/api/prepare/status": true}`,避免控制台被解压轮询刷屏 |

### 2.2 共享路由

| 路由 | Handler | 作用 |
|------|---------|------|
| `GET /` → 静态文件服务器 | (fileserver) | `//go:embed web` 映射 |
| `GET /api/version` | `handleVersion` | 返回程序版本号(由 `-ldflags -X` 注入) |
| `GET /api/ffmpeg/status` | `handleFFmpegStatus` | 返回 `{available, embedded, version}` |
| `POST /api/ffmpeg/reveal` | `handleFFmpegReveal` | 调 `service.GetFFmpegDir` → `browser.Open` 打开缓存目录 |
| `GET /api/prepare/status` | `handlePrepareStatus` | 返回 `embedded.GetProgress()`(state, percent, current, error) |
| `GET /api/fs/list?path=` | `handleFsList` | 列目录;Windows 附带盘符;按类型+名排序 |
| `GET /api/fs/home` | `handleFsHome` | 返回 `os.UserHomeDir()` |
| `POST /api/fs/reveal` | `handleFsReveal` | 通用:打开任意路径(文件则打开其父目录) |
| `GET /api/config/dirs` | `handleConfigDirs` | 返回保存的 inputDir/outputDir |
| `POST /api/config/dirs` | `handleConfigDirs` | 写入 inputDir/outputDir |
| `GET /api/convert/stream` | `handleConvertStream` | SSE;订阅 `jobs.Subscribe` → 写 `data: <json>\n\n` + Flush(**所有 Tab 共享**) |
| `POST /api/quit` | `handleQuit` | 返回 200 后 `RequestShutdown()` |

Tab 专属端点(convert / audio / editor)见各自的 `program.md`。

### 2.3 三 Tab 共用的请求扩展字段

| 字段 | 类型 | 行为 |
|------|------|------|
| `overwrite` | bool | 默认 false。文件已存在 + 该字段为 false → handler 返回 `409 + {existing:true, path}`,前端自绘覆盖确认弹窗。同意后带 `overwrite:true` 重发 |
| `dryRun` | bool | 默认 false。true 时 handler 走完参数构建(含 audio merge 策略解析),返回 `{ok, dryRun, command}` 但**不** mkdir、**不**查 overwrite、**不** `jobs.Start`;merge 的临时 list 文件在该路径上立即 `Cleanup()` 释放。前端用它在真正执行前显示精确命令 |

### 2.4 `editor_wiring.go`

把 `service.*` / `internal/job.Manager` 适配成 `editor/ports` 接口,`buildEditorModule` 在 `server.go` 的路由注册阶段调用,挂载 `/api/editor/*`。详见 [tabs/editor/program.md](../tabs/editor/program.md) §6。

### 2.5 `web/`

通过 `//go:embed web` + `fs.Sub` 映射到 `GET /`。`server/web/editor/` 目录下存放剪辑器专属的 CSS/JS,由 `index.html` 用 `<link>` / `<script>` 引入。前端架构见 [frontend.md](frontend.md)。

## 3. `editor/`

**职责**:单视频剪辑器模块,自成一体。完整模块设计见 [tabs/editor/program.md](../tabs/editor/program.md)。

分层(严格单向依赖):

| 子包 | 职责 | 依赖 |
|------|------|------|
| `editor/domain/` | 业务类型 + 纯函数:`Project`、`Clip`、`Source`、`ExportSettings`、`ProgramDuration`、`Validate`、`Split/Delete/Reorder/TrimLeft/TrimRight`、`BuildExportArgs` | 仅 stdlib |
| `editor/ports/` | DIP 接口:`ProjectRepository`、`VideoProber`、`JobRunner`、`PathResolver`、`Clock` + DTO | `domain` |
| `editor/storage/` | `JSONRepo` 实现 `ProjectRepository`;索引双写 + 原子写 + 损坏自愈 | `domain` + `ports` |
| `editor/api/` | HTTP handler(projects / probe / export / source)+ DTO + `Router.Register(mux, prefix)` | `domain` + `ports`(不依赖具体存储/探测/任务实现) |
| `editor/module.go` | 对外唯一入口:`Deps` / `NewModule(d)` / `Module.Register(mux, prefix)` | 组合 `api` + `storage` |

主程序在 `server/editor_wiring.go` 里以小适配器把 `service.ProbeVideo` / `job.Manager` / `service.GetFFmpeg*Path` 桥接到 `editor/ports` 的接口。

## 4. `service/`

**职责**:业务层门面,对 `server` 屏蔽底层 `embedded` 细节。

### 4.1 `ffmpeg.go`

| 函数 | 行为 |
|------|------|
| `GetFFmpegPath() string` | 嵌入优先;失败返回 `"ffmpeg"`(系统 PATH 降级) |
| `GetFFprobePath() string` | 同上,for ffprobe |
| `CheckFFmpeg() bool` | 嵌入失败时再 `exec.Command("ffmpeg","-version")` |
| `GetFFmpegVersion() string` | 运行 `ffmpeg -version`,取第一行 |
| `IsEmbedded() bool` | 探测嵌入二进制是否可用 |
| `Prepare() error` | 触发 `embedded.GetFFmpegBinary()` 解压;供 `main.go` 在 goroutine 里预热 |
| `GetFFmpegDir() (string, error)` | 返回 ffmpeg 所在目录(用于"在文件管理器打开"功能) |

### 4.2 `probe.go` — ffprobe 封装

统一类型(便于复用):

| 类型 | 字段 |
|------|------|
| `MediaFormat` | `Duration / BitRate / Size` — 音频视频通用 |
| `AudioStream` | `Index / CodecName / Channels / SampleRate / BitRate / Language / Title`;Index 是**音频流内部** 0-based 位置,供 `-map 0:a:<Index>` |
| `VideoStream` | `CodecName / Width / Height / FrameRate` |
| `ProbeResult` | `Format + Streams []AudioStream`(`ProbeAudio` 返回) |
| `VideoProbeResult` | `Format + Video + Audio *AudioStream`(`ProbeVideo` 返回,取首条音视频流) |

| 函数 | 行为 |
|------|------|
| `ProbeAudio(path) (*ProbeResult, error)` | `ffprobe -select_streams a` 只看音频流 |
| `ProbeVideo(path) (*VideoProbeResult, error)` | 不选流,取首条 video + 首条 audio |
| `runFFprobe(path, extra...) []byte` | 内部 helper;`procutil.HideWindow` 防 Windows 弹黑窗 |
| `parseRational(candidates...)` | 把 ffprobe 的 `"30000/1001"` 这类 rational 串转 float(按顺序 fallback) |

## 5. `internal/embedded/`

**职责**:平台相关二进制嵌入 + 首次启动解压 + 进度追踪。

### 5.1 平台分片(构建标签)

| 文件 | 构建标签 | 嵌入 |
|------|----------|------|
| `embedded_windows.go` | `//go:build windows` | `windows/windows.7z` |
| `embedded_darwin.go`  | `//go:build darwin`  | `darwin/darwin.7z` |
| `embedded_linux.go`   | `//go:build linux`   | `linux/linux.7z` |

每个文件只导出三个符号:`archiveData []byte`、`ffmpegBinaryName`、`ffprobeBinaryName`。

### 5.2 `common.go`

| 符号 | 说明 |
|------|------|
| `Progress` struct | `{State, Percent, Current, Error}`,JSON 暴露给前端 |
| `GetProgress() Progress` | 线程安全快照读取 |
| `setProgress(fn)` | 线程安全更新 |
| `GetFFmpegBinary() (string, error)` | 走 `ensureExtracted()` → 返回 `<cacheDir>/ffmpeg[.exe]` |
| `GetFFprobeBinary() (string, error)` | 同上 |
| `CheckEmbeddedFFmpeg() bool` | 抽出后运行 `-version` |
| `ensureExtracted()` | `sync.Once` 包裹的单次执行 |
| `extractArchive()` | 主流程:检查 `.ok` → 解压 → 写标记 |
| `cacheDir()` | `~/.easy-ffmpeg/bin-<sha256[:4].hex>/` |
| `extractOne(f, destDir)` | 写一个文件并 chmod 755 |
| `progressWriter` | 包装 `io.Writer`,在每次 `Write` 更新全局 doneBytes/percent |
| `startProgressPrinter()` | 返回可 `Stop()` 的控制台进度条渲染器(每 200ms `\r` 重绘) |

解压流程详见 [build.md](build.md) §5。

## 6. `internal/job/`

**职责**:FFmpeg 任务状态管理 + 事件广播。

### 6.1 `manager.go`

| 符号 | 说明 |
|------|------|
| `Event` struct | `{Type, Line, Message, Running}`;Type ∈ `state\|log\|done\|error\|cancelled` |
| `Manager` struct | 持有 `cmd`、`subscribers map[chan Event]struct{}`、running 标志、cancelled 标志 |
| `New() *Manager` | 构造 |
| `Running() bool` | 线程安全读 |
| `Start(binary, args) error` | 构造 `exec.Cmd` → `StderrPipe` → `Start` → `go pump` |
| `Cancel()` | Kill 当前进程,设 cancelled 标志 |
| `Subscribe() (<-chan Event, func())` | 注册 → 立即发送 `state` 事件 → 返回 unsubscribe 闭包 |
| `pump(cmd, stderr)` | 核心:scanner + 进度行节流 + 广播 + Wait + 最终事件 |
| `broadcast(ev)` | 遍历订阅者,**非阻塞**发送(`select default`) |

### 6.2 进度行节流(`pump` 内)

```text
lastEmit := 零值 time.Time
pendingProgress := ""

for scanner.Scan():
    line := scanner.Text()
    if isProgressLine(line):         # 前缀 frame= / size=
        pendingProgress = line
        if time.Since(lastEmit) < 100ms:
            continue                  # 丢弃本次,只留最新
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

### 6.3 自定义 scanner splitter

`scanLinesOrCR` 同时在 `\r` 和 `\n` 处切分。这是必须的:FFmpeg 每次刷新进度写的是 `\r`(覆盖同一行),标准 `bufio.ScanLines` 只识别 `\n` 会导致所有进度在一整段累积,直到程序结束才吐出来。

### 6.4 子进程窗口抑制

`job.Manager.Start` 在 `exec.Command` 后立刻调 `procutil.HideWindow(cmd)`。Windows 下会设 `CREATE_NO_WINDOW` 标志位,防止每次转码/探测都弹黑色控制台;其他平台是空 no-op。同一 helper 被 `service.probe.go` 的 `runFFprobe` 复用。

## 7. `internal/procutil/`

**职责**:抽出 `job` 与 `service/probe` 共用的子进程跨平台适配。

| 文件 | 构建标签 | 导出 |
|------|----------|------|
| `hide_windows.go` | `//go:build windows`  | `HideWindow(cmd *exec.Cmd)` 设置 `CREATE_NO_WINDOW` |
| `hide_other.go`   | `//go:build !windows` | `HideWindow` no-op |

## 8. `internal/browser/`

**职责**:跨平台打开 URL 或本地路径。

```go
func Open(url string) error {
    switch runtime.GOOS {
    case "windows": exec.Command("cmd", "/c", "start", "", url)
    case "darwin":  exec.Command("open", url)
    default:        exec.Command("xdg-open", url)
    }
}
```

对 URL 和本地路径都适用(`start` 会派给 URL handler 或 Explorer,取决于参数形式)。

## 9. `config/`

**职责**:用户偏好持久化。

| 函数 | 存储文件 |
|------|----------|
| `GetInputDir` / `SaveInputDir` | `<UserConfigDir>/easy-ffmpeg/input_dir.txt` |
| `GetOutputDir` / `SaveOutputDir` | `<UserConfigDir>/easy-ffmpeg/output_dir.txt` |

纯文本单行。后续要加更多配置项可以升级为单个 JSON/TOML(见 [roadmap.md](roadmap.md) §2.2)。

## 10. 根级文件与 `tools/`

| 文件 | 作用 |
|------|------|
| `build.bat` / `build.sh` | 一键编译四平台 + 桌面版分支 |
| `go.mod` / `go.sum` | Go 依赖描述 |
| `tools/build_icon.go` | 开发期:把 PNG 图标烧成 Windows 资源文件(生成 `cmd/rsrc_windows.syso`) |
| `tools/download_windows.go` | 历史:从 gyan.dev 下载 Windows FFmpeg;当前已被 7z 嵌入方案取代 |
| `assets/icon.svg` / `icon.icns` | 品牌图标源文件 |

## 11. 依赖清单

| 依赖 | 用途 |
|------|------|
| `github.com/bodgit/sevenzip` | 纯 Go 7z 读取,支持 LZMA2+BCJ2 |
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
| `github.com/wailsapp/wails/v2` | 桌面版 only(`cmd/desktop/`),共享层不引用 |

Web 版的所有间接依赖都是纯 Go,无 cgo。前端零依赖(HTML/CSS/JS 原生)。
