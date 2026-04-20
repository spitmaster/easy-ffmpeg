# 架构设计

## 1. 分层架构

```
┌─────────────────────────────────────────────────────────────┐
│                     cmd/main.go (入口)                      │
│   启动 HTTP 服务 → 异步触发 FFmpeg 解压 → 打开系统浏览器      │
│   监听 Ctrl+C / /api/quit 信号进行优雅关停                   │
└────────────┬────────────────────────────────┬───────────────┘
             │                                │
             ▼                                ▼
┌──────────────────────────┐     ┌──────────────────────────┐
│  server/ (HTTP 服务层)    │     │  service/ (业务层)        │
│  - server.go  路由/中间件  │     │  GetFFmpegPath           │
│  - handlers.go 所有 API   │◀───▶│  GetFFprobePath          │
│  - web/       静态资源    │     │  Prepare                 │
│                          │     │  CheckFFmpeg             │
└──────────┬───────────────┘     │  GetFFmpegDir            │
           │                     └────────────┬─────────────┘
           │ 依赖                              │ 依赖
           ▼                                  ▼
┌──────────────────────────┐     ┌──────────────────────────┐
│ internal/job/            │     │ internal/embedded/       │
│  ffmpeg 进程管理          │     │  7z 解压 + 缓存 + 进度    │
│  - 订阅者广播（SSE）       │     │  - 平台分片嵌入          │
│  - 进度行节流            │     │  - ~/.easy-ffmpeg/bin-*  │
└──────────────────────────┘     └──────────────────────────┘

┌──────────────────────────┐     ┌──────────────────────────┐
│ internal/browser/        │     │ config/                  │
│  跨平台打开 URL/路径       │     │  输入/输出目录持久化      │
└──────────────────────────┘     └──────────────────────────┘
```

依赖方向严格自上而下：`cmd → server, service → embedded, job, browser, config`。禁止反向依赖。

## 2. 目录结构

```
easy-ffmpeg/
├── cmd/main.go                       程序入口
├── server/
│   ├── server.go                     路由、日志中间件、生命周期
│   ├── handlers.go                   所有 API 处理函数
│   └── web/                          go:embed 打包的静态前端
│       ├── index.html
│       ├── app.css
│       └── app.js
├── service/ffmpeg.go                 业务层门面
├── internal/
│   ├── browser/open.go               跨平台打开 URL 或路径
│   ├── embedded/                     FFmpeg 嵌入管理
│   │   ├── common.go                 解压逻辑 + 进度跟踪
│   │   ├── embedded_windows.go       //go:build windows
│   │   ├── embedded_darwin.go        //go:build darwin
│   │   ├── embedded_linux.go         //go:build linux
│   │   ├── windows/windows.7z        Windows ffmpeg+ffprobe（7z 打包）
│   │   ├── darwin/darwin.7z          macOS 版本
│   │   └── linux/linux.7z            Linux 版本
│   └── job/
│       ├── manager.go                ffmpeg 进程状态 + 事件广播
│       ├── hide_windows.go           //go:build windows
│       └── hide_other.go             //go:build !windows
├── config/config.go                  用户目录/配置持久化
├── tools/download_windows.go         开发期工具（下载 Windows ffmpeg）
├── build.bat / build.sh              一键四平台构建脚本
├── design/                           本文档
└── dist/                             构建产物
```

## 3. 启动时序（首次运行）

```
T=0.0s   main.go 启动
         ├─ server.Listen() 绑定 127.0.0.1:随机端口
         └─ 打印"访问地址"
T=0.0s   go service.Prepare() 异步启动（不阻塞主流程）
         └─ 触发 embedded.ensureExtracted()
             └─ 读取 embed 的 .7z 字节
             └─ cacheDir() 计算 ~/.easy-ffmpeg/bin-<sha8>/
             └─ 没有 .ok 标记 → 开始解压
             └─ 启动控制台进度条 printer goroutine
             └─ sevenzip.NewReader → 遍历 File 逐个解压
             └─ progressWriter 在 io.Copy 里累计字节数
             └─ 每个字节写入都更新全局 progress 结构
T=0.1s   browser.Open(url) 启动系统默认浏览器
T=0.3s   浏览器加载页面 → app.js 调用 /api/prepare/status
         └─ state = "extracting" → 显示遮罩 + 进度条
         └─ 300ms 轮询一次直到 "ready"
T=~40s   解压完成，写入 .ok 标记
         ├─ progress.State = "ready"
         ├─ 控制台进度条最终行 + "解压完成 (39.5s)"
         └─ 浏览器轮询下次拿到 "ready" → 遮罩淡出
T=~40s   UI 就绪，用户可以开始转码
```

## 4. 启动时序（后续运行）

```
T=0.0s   server 启动
T=0.0s   go service.Prepare() 异步启动
         └─ fileExists(.ok) → 直接 setProgress(ready) 返回
T=0.1s   browser.Open
T=0.3s   浏览器加载页面 → /api/prepare/status 立刻返回 ready
         └─ app.js 检测已就绪，不显示遮罩
T=0.3s   UI 就绪
```

## 5. 核心数据流：一次转码

```
用户填表单 → 点击"开始转码"
    │
    ▼
POST /api/convert/start { inputPath, outputDir, outputName,
                          videoEncoder, audioEncoder, format }
    │
    ▼
handlers.buildFFmpegArgs() → []string{"-y","-i",...}
    │
    ▼
jobs.Start(ffmpegPath, args)
    ├─ exec.Command + hideWindow (Windows)
    ├─ StderrPipe, cmd.Start()
    └─ go pump(cmd, stderr)
        │
        ▼
    bufio.Scanner + scanLinesOrCR
    （同时识别 \r \n 分隔，捕获进度刷新行）
        │
        ├─ 真正消息 (frame= 之外)     → 立即 broadcast
        └─ 进度行 (frame= / size=)   → 节流 100ms 一次
        │
        ▼
    broadcast → 遍历所有订阅者 chan Event，非阻塞发送

同时：
GET /api/convert/stream (SSE)
    │
    ▼
handlers.handleConvertStream
    ├─ jobs.Subscribe() 返回 <-chan Event
    ├─ 立即下发 {"type":"state","running":bool}
    └─ 循环读 chan → json.Marshal → "data: ...\n\n" + Flush
    
前端 EventSource
    ├─ type=log：append / 替换上条进度行
    ├─ type=done/error/cancelled：更新按钮状态 + 日志尾
    └─ type=state：同步 running 状态

取消：
POST /api/convert/cancel
    → jobs.Cancel() → cmd.Process.Kill()
    → pump 的 cmd.Wait() 返回 → 广播 cancelled 事件
```

## 6. 进度广播的节流设计

FFmpeg stderr 每秒可能输出 30-60 条 `frame=...` 的进度刷新。如果每条都经 SSE 推送：

- 服务端：每条 JSON marshal + HTTP Flush 一次
- 前端：每条触发 DOM + scrollTop 回流
- 结果：浏览器 UI 卡顿

**解决**：在 `pump()` 里识别以 `frame=` / `size=` 开头的进度行，维持"最多 100ms 发一次"的节流；非进度行（真正的错误/信息）一条不丢。循环结束前补发最后一次 `pendingProgress`，保证用户看到最终帧数总结。

前端额外做一次 DOM 优化：进度行原地覆盖上一行（不追加新 `<span>`），避免几千个空转 DOM 节点。

## 7. 嵌入式 FFmpeg 机制

```
编译期：
  //go:embed windows/windows.7z  → var archiveData []byte
  构建标签按 GOOS 选择对应文件，只嵌入一个平台的 7z

运行期：
  archiveData (28MB) 存在于可执行文件中
     │
     ▼
  sevenzip.NewReader(bytes.NewReader(archiveData), len)
     │
     ▼
  Walk reader.File → 每个文件：
     ├─ os.Create(~/.easy-ffmpeg/bin-<hash>/ffmpeg.exe)
     ├─ io.Copy(file, f.Open()) 经 progressWriter 计数
     └─ os.Chmod 755
     │
     ▼
  全部完成写入 .ok 标记文件

  cacheDir hash = sha256(archiveData)[:4].hex
     → 8 字符 hex，即嵌入 7z 变更自动换目录
     → 例如 bin-b9b48d4f/
```

## 8. 并发与线程模型

- **HTTP handler 并发**：Go 的 `net/http` 每个请求一个 goroutine
- **转码 pump goroutine**：`jobs.Start` 启动一个专属 goroutine 读 stderr 并广播事件
- **SSE 订阅者**：每个浏览器连接在 `handleConvertStream` 里跑一个 goroutine 写事件
- **解压 goroutine**：`main.go` 的 `go service.Prepare()`（后台，不阻塞浏览器打开）
- **进度条 printer goroutine**：解压期间后台每 200ms 重绘控制台
- **同步原语**：
  - `sync.Once`：`ensureExtracted` 保证 7z 只解压一次
  - `sync.Mutex`：`jobs` 的 subscribers 映射、进度状态的 setProgress
  - 非阻塞 chan send：`broadcast` 用 `select default`，订阅者慢不拖累 ffmpeg

## 9. 错误处理与降级

| 场景 | 处理 |
|------|------|
| 解压失败（磁盘满/权限） | progress.State=error；前端遮罩显示错误，不隐藏；服务不崩溃 |
| 嵌入 FFmpeg 不可用 | `service.CheckFFmpeg` 降级到系统 PATH 里的 `ffmpeg` |
| 转码 FFmpeg 非零退出 | `cmd.Wait()` 返回 err → 广播 `type: "error"` 事件 |
| 客户端取消 | Kill 子进程，设 cancelled 标记，广播 `type: "cancelled"` 事件 |
| 浏览器关闭重连 | SSE 连接断开，前端 onerror 自动 1.5s 后重连 |
| 用户点"退出" | `POST /api/quit` → server.RequestShutdown → http.Server.Shutdown |
| Ctrl+C | `signal.Notify` 触发同样的关停路径 |

## 10. 跨平台适配点

| 关注点 | 处理位置 |
|--------|----------|
| 二进制文件名（`.exe` 与否） | `embedded/embedded_<os>.go` 中的常量 |
| 哪个平台的 7z 被嵌入 | `//go:build <os>` 构建标签 |
| FFmpeg 子进程隐藏窗口 | `job/hide_windows.go` 用 `CREATE_NO_WINDOW` |
| 用户主目录 | `os.UserHomeDir()` |
| 用户配置目录 | `os.UserConfigDir()` |
| 打开系统浏览器 / 文件管理器 | `internal/browser/open.go`：Windows `start` / macOS `open` / Linux `xdg-open` |
| 盘符（Windows 专属）| `handlers.listWindowsDrives()` A-Z 逐个 stat |
