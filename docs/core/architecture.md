# 后端架构(程序设计)

> 本文档描述后端的分层架构、目录结构、数据流、启动时序、并发模型、错误处理。前端架构(Vue 3 + Vite + Pinia,工程目录在仓库根 `web/`)见 [frontend.md](frontend.md);桌面版双入口拓扑见 [desktop.md](desktop.md)。

## 1. 分层架构

```text
┌─────────────────────────────────────────────────────────────┐
│                     cmd/main.go (入口)                       │
│   启动 HTTP 服务 → 异步触发 FFmpeg 解压 → 打开系统浏览器       │
│   监听 Ctrl+C / /api/quit 信号进行优雅关停                    │
└────────────┬────────────────────────────────┬───────────────┘
             │                                │
             ▼                                ▼
┌─────────────────────────────────┐ ┌──────────────────────────┐
│  server/ (HTTP 服务层)           │ │  service/ (业务层)         │
│  - server.go         路由 / 中间件 │ │  ffmpeg.go               │
│  - handlers.go       convert     │ │   · GetFFmpegPath         │
│  - handlers_audio.go audio probe │ │   · GetFFprobePath        │
│                      start/cancel│◀│   · Prepare               │
│  - audio_args.go     命令构建纯函数│ │   · CheckFFmpeg           │
│  - editor_wiring.go  适配器装配  │ │   · GetFFmpegDir          │
│       (前端资源由 web/ 包注入)    │ │  probe.go                │
└──────────┬───────────────────────┘ │   · ProbeAudio / Video   │
           │                         └────────────┬─────────────┘
           ▼                                      │
┌──────────────────────────────────┐              │
│  editor/  (剪辑器模块, SOLID)     │              │
│  - domain/     纯业务类型+函数    │              │
│  - ports/      DIP 接口           │              │
│  - storage/    JSONRepo (工程持久化)│             │
│  - api/        HTTP handlers     │              │
│  - module.go   对外入口 NewModule │◀─── 适配器注入 service / job
└──────────┬───────────────────────┘              │
           │                                      │
           ▼                                      ▼
┌──────────────────────────┐     ┌──────────────────────────┐
│ internal/job/            │     │ internal/embedded/       │
│  ffmpeg 进程管理           │     │  7z 解压 + 缓存 + 进度    │
│  - 订阅者广播(SSE)        │     │  - 平台分片嵌入           │
│  - 进度行节流             │     │  - ~/.easy-ffmpeg/bin-*  │
└──────────────────────────┘     └──────────────────────────┘

┌──────────────────────────┐ ┌──────────────────────────┐ ┌──────────────────────────┐
│ internal/browser/        │ │ internal/procutil/       │ │ config/                  │
│  跨平台打开 URL/路径       │ │  HideWindow(跨平台抽象)  │ │  输入/输出目录持久化      │
└──────────────────────────┘ └──────────────────────────┘ └──────────────────────────┘
```

依赖方向严格自上而下:`cmd → server, service → embedded, job, browser, procutil, config`。`editor` 子包内部也严格单向:`api → ports ← storage → domain`。`server` 通过 `editor_wiring.go` 适配具体实现为 `editor/ports` 接口,`editor/` 对主程序其余部分完全无感。

> **v0.4.0 双入口**:`cmd/desktop/main.go` 是与 `cmd/main.go` **并列**的入口,内部仍是 `server.New() + Listen()`,只是把端口交给嵌入的 Wails WebView 而非系统浏览器。后端、`server/`、`editor/`、`internal/*`、以及 v0.5.x 起的 `web/` 前端 bundle,在两个入口之间字节相同。详见 [desktop.md](desktop.md)。

## 2. 目录结构

```text
easy-ffmpeg/
├── cmd/
│   ├── main.go                       Web 版入口
│   ├── icon.ico                      Windows 图标(rsrc 生成 .syso 时引用)
│   ├── rsrc_windows.syso             Windows 资源文件(图标)
│   └── desktop/                      Wails 桌面版入口(v0.4.0+)
│       ├── main.go                   wails.Run + 生命周期钩子
│       ├── app.go                    App 结构体 + startup/shutdown
│       ├── frontend/dist/index.html  极简 splash(JS 跳转到 localhost:port)
│       └── wails.json
├── web/                              ★ 前端工程(v0.5.x+,Vue 3 + Vite + TS + Pinia + Tailwind)
│   ├── package.json / vite.config.ts / tsconfig.json
│   ├── tailwind.config.js / postcss.config.js / index.html
│   ├── embed.go                      `//go:embed all:dist` → easy-ffmpeg/web 包
│   ├── dist/                         构建产物(gitignore;npm run build 写入)
│   └── src/                          main.ts / App.vue / api/ stores/ composables/
│                                     components/ views/ router/ utils/ styles/
│                                     (详见 frontend.md §2)
├── server/
│   ├── server.go                     路由、日志中间件、生命周期;装配 editor.Module;
│   │                                 import easy-ffmpeg/web,fs.Sub(web.FS, "dist") 挂 /
│   ├── handlers.go                   convert + 共享 fs/config/ffmpeg 接口
│   ├── handlers_audio.go             audio probe / start / cancel
│   ├── audio_args.go                 纯函数:AudioRequest → ffmpeg args
│   ├── audio_args_test.go            表驱动测试
│   └── editor_wiring.go              把 service.* / job.Manager 适配成 editor/ports 接口
├── editor/                           剪辑器模块(可单独提取为独立 exe)
│   ├── module.go                     对外入口:Deps + NewModule + Module.Register
│   ├── domain/                       纯业务层:Project / Clip / Timeline / Export
│   │   ├── project.go                Project/Source/Clip/ExportSettings + Validate
│   │   ├── timeline.go               Split/Delete/Reorder/TrimLeft/TrimRight
│   │   ├── export.go                 BuildExportArgs → ffmpeg filter_complex
│   │   └── *_test.go                 表驱动测试(90%+ 覆盖)
│   ├── ports/                        DIP 接口:repository/prober/runner/paths/clock
│   ├── storage/                      ports.ProjectRepository 的 JSON 实现
│   │   ├── jsonrepo.go               原子写 + 索引 + 损坏自愈
│   │   └── jsonrepo_test.go
│   └── api/                          HTTP handlers(只依赖 ports + domain)
│       ├── handlers_projects.go      CRUD
│       ├── handlers_probe.go         probe 代理
│       ├── handlers_export.go        export start / cancel
│       ├── handlers_source.go        <video> 源文件 HTTP Range 服务
│       └── dto.go / http_util.go / routes.go
├── service/
│   ├── ffmpeg.go                     ffmpeg/ffprobe 路径 + 版本 + 预热
│   └── probe.go                      ProbeAudio / ProbeVideo,封装 ffprobe JSON
├── internal/
│   ├── browser/open.go               跨平台打开 URL 或路径
│   ├── embedded/                     FFmpeg 嵌入管理(按平台构建标签分片)
│   │   ├── common.go                 解压逻辑 + 进度跟踪
│   │   ├── embedded_windows.go       //go:build windows
│   │   ├── embedded_darwin.go        //go:build darwin
│   │   ├── embedded_linux.go         //go:build linux
│   │   ├── windows/windows.7z
│   │   ├── darwin/darwin.7z
│   │   └── linux/linux.7z
│   ├── job/manager.go                ffmpeg 进程状态 + 事件广播
│   └── procutil/
│       ├── hide_windows.go           //go:build windows · HideWindow
│       └── hide_other.go             //go:build !windows · no-op
├── config/config.go                  用户目录/配置持久化
├── tools/                            构建工具(图标、ffmpeg 下载)
├── assets/                           品牌图标源文件
├── build.bat / build.sh              一键四平台构建 + 桌面版分支
├── docs/                             设计文档(本目录)
└── dist/                             构建产物
```

## 3. 启动时序(首次运行,Web 版)

```text
T=0.0s   main.go 启动
         ├─ server.Listen() 绑定 127.0.0.1:随机端口
         └─ 打印"访问地址"
T=0.0s   go service.Prepare() 异步启动(不阻塞主流程)
         └─ 触发 embedded.ensureExtracted()
             └─ 读取 embed 的 .7z 字节
             └─ cacheDir() 计算 ~/.easy-ffmpeg/bin-<sha8>/
             └─ 没有 .ok 标记 → 开始解压
             └─ 启动控制台进度条 printer goroutine
             └─ sevenzip.NewReader → 遍历 File 逐个解压
             └─ progressWriter 在 io.Copy 里累计字节数
             └─ 每个字节写入都更新全局 progress 结构
T=0.1s   browser.Open(url) 启动系统默认浏览器
T=0.3s   浏览器加载页面 → PrepareOverlay.vue 轮询 /api/prepare/status
         └─ state = "extracting" → 显示遮罩 + 进度条
         └─ 300ms 轮询一次直到 "ready"
T=~40s   解压完成,写入 .ok 标记
         ├─ progress.State = "ready"
         ├─ 控制台进度条最终行 + "解压完成 (39.5s)"
         └─ 浏览器轮询下次拿到 "ready" → 遮罩淡出
T=~40s   UI 就绪,用户可以开始转码
```

后续运行:`fileExists(.ok) → 直接 setProgress(ready) 返回`,T=0.3s 起就绪,不显示遮罩。

桌面版时序见 [desktop.md §9](desktop.md)。

## 4. 核心数据流:一次任务

convert / audio / editor 三个 Tab 都走同一条数据流,只是起点端点和命令构建器不同。`jobs.Manager` 全局唯一 —— 同一时刻只有一个任务在跑。

```text
用户填表单 / 完成剪辑 → 点击"开始 …"
    │
    ▼
POST /api/{convert|audio}/start  或  /api/editor/export
    │
    ▼
对应构建器 → []string{"-y","-i",...}
  · buildFFmpegArgs                (convert)
  · BuildAudioArgs                 (audio,含 merge 的临时列表文件 cleanup)
  · editor.domain.BuildExportArgs  (editor,filter_complex trim+concat)
    │
    ▼
jobs.Start(ffmpegPath, args)
    ├─ exec.Command + hideWindow (Windows)
    ├─ StderrPipe, cmd.Start()
    └─ go pump(cmd, stderr)
        │
        ▼
    bufio.Scanner + scanLinesOrCR
    (同时识别 \r \n 分隔,捕获进度刷新行)
        │
        ├─ 真正消息 (frame= 之外)     → 立即 broadcast
        └─ 进度行 (frame= / size=)   → 节流 100ms 一次
        │
        ▼
    broadcast → 遍历所有订阅者 chan Event,非阻塞发送

同时:
GET /api/convert/stream (SSE — 所有 Tab 共用这同一条流)
    │
    ▼
handlers.handleConvertStream
    ├─ jobs.Subscribe() 返回 <-chan Event
    ├─ 立即下发 {"type":"state","running":bool}
    └─ 循环读 chan → json.Marshal → "data: ...\n\n" + Flush

前端 jobBus(api/jobs.ts,单例 EventSource)
    └─ subscribe → 广播给所有调用过 useJobPanel 的视图
    └─ 每个 useJobPanel 实例用 "owning" 标志只响应自己发起的任务
        ├─ type=log:appendLog → 进度行原地覆盖,普通行追加
        ├─ type=done/error/cancelled:更新按钮 + 完成条 + reveal
        └─ type=state:同步 running 状态

取消:
POST /api/{convert|audio}/cancel  或  /api/editor/export/cancel
    → jobs.Cancel() → cmd.Process.Kill()
    → pump 的 cmd.Wait() 返回 → 广播 cancelled 事件
```

## 5. 进度广播的节流设计

FFmpeg stderr 每秒可能输出 30-60 条 `frame=...` 的进度刷新。每条都经 SSE 推送会让浏览器卡顿(每条 JSON marshal + HTTP Flush + DOM 回流)。

**解决**:在 `pump()` 里识别以 `frame=` / `size=` 开头的进度行,维持"最多 100ms 发一次"的节流;非进度行(真正的错误/信息)一条不丢。循环结束前补发最后一次 `pendingProgress`,保证用户看到最终帧数总结。

前端额外做一次 DOM 优化:进度行原地覆盖上一行(不追加新 `<span>`),避免几千个空转 DOM 节点。

## 6. 嵌入式 FFmpeg 机制

```text
编译期:
  //go:embed windows/windows.7z  → var archiveData []byte
  构建标签按 GOOS 选择对应文件,只嵌入一个平台的 7z

运行期:
  archiveData (28MB) 存在于可执行文件中
     │
     ▼
  sevenzip.NewReader(bytes.NewReader(archiveData), len)
     │
     ▼
  Walk reader.File → 每个文件:
     ├─ os.Create(~/.easy-ffmpeg/bin-<hash>/ffmpeg.exe)
     ├─ io.Copy(file, f.Open()) 经 progressWriter 计数
     └─ os.Chmod 755
     │
     ▼
  全部完成写入 .ok 标记文件

  cacheDir hash = sha256(archiveData)[:4].hex
     → 8 字符 hex,即嵌入 7z 变更自动换目录
     → 例如 bin-b9b48d4f/
```

详见 [build.md](build.md) §3-§5。

## 7. 并发与线程模型

- **HTTP handler 并发**:Go 的 `net/http` 每个请求一个 goroutine
- **转码 pump goroutine**:`jobs.Start` 启动一个专属 goroutine 读 stderr 并广播事件
- **SSE 订阅者**:每个浏览器连接在 `handleConvertStream` 里跑一个 goroutine 写事件
- **解压 goroutine**:`main.go` 的 `go service.Prepare()`(后台,不阻塞浏览器打开)
- **进度条 printer goroutine**:解压期间后台每 200ms 重绘控制台
- **同步原语**:
  - `sync.Once`:`ensureExtracted` 保证 7z 只解压一次
  - `sync.Mutex`:`jobs` 的 subscribers 映射、进度状态的 setProgress
  - 非阻塞 chan send:`broadcast` 用 `select default`,订阅者慢不拖累 ffmpeg

## 8. 错误处理与降级

| 场景 | 处理 |
|------|------|
| 解压失败(磁盘满/权限) | progress.State=error;前端遮罩显示错误,不隐藏;服务不崩溃 |
| 嵌入 FFmpeg 不可用 | `service.CheckFFmpeg` 降级到系统 PATH 里的 `ffmpeg` |
| 转码 FFmpeg 非零退出 | `cmd.Wait()` 返回 err → 广播 `type: "error"` 事件 |
| 客户端取消 | Kill 子进程,设 cancelled 标记,广播 `type: "cancelled"` 事件 |
| 浏览器关闭重连 | SSE 连接断开,前端 onerror 自动 1.5s 后重连 |
| 用户点"退出" | `POST /api/quit` → server.RequestShutdown → http.Server.Shutdown |
| Ctrl+C | `signal.Notify` 触发同样的关停路径 |

## 9. 跨平台适配点

| 关注点 | 处理位置 |
|--------|----------|
| 二进制文件名(`.exe` 与否) | `embedded/embedded_<os>.go` 中的常量 |
| 哪个平台的 7z 被嵌入 | `//go:build <os>` 构建标签 |
| FFmpeg / FFprobe 子进程隐藏窗口 | `internal/procutil/hide_windows.go` 的 `HideWindow`,用 `CREATE_NO_WINDOW`;`job.Manager.Start` 与 `service.ProbeAudio` / `ProbeVideo` 都调用 |
| 用户主目录 | `os.UserHomeDir()` |
| 用户配置目录 | `os.UserConfigDir()` |
| 打开系统浏览器 / 文件管理器 | `internal/browser/open.go`:Windows `start` / macOS `open` / Linux `xdg-open` |
| 盘符(Windows 专属) | `handlers.listWindowsDrives()` A-Z 逐个 stat |
