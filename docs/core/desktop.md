# 桌面版双产物拓扑(v0.4.0+)

> 本文档把 v0.4.0 的可行性论证、方案对比、架构设计、cgo 隔离合并为一份。包含决策(为什么这样做)与架构(怎么做)两部分。

---

## 一、为什么做(决策与方案对比)

### 1. 目标

发版后输出**两个**可分发产物,共用一份代码:

| 产物 | 形态 | 等价类比 |
|------|------|----------|
| **Web 版**(保留现状) | 命令行启动一个本地 HTTP 服务,调用系统浏览器打开 UI | Jupyter Notebook |
| **桌面版**(Wails) | 双击启动一个独立窗口,内置 WebView 直接渲染 UI | Electron / Tauri 应用 |

非目标(v0.4.0 不做):

- 移动端 / 平板专门适配
- 完全切换到 Wails 原生 binding(`runtime.EventsEmit` / `Bind`)取代 HTTP+SSE —— 见方案 B
- 把剪辑器 / 音频两个模块拆成独立的 Wails 子窗口
- 系统托盘、自动更新、原生菜单等桌面端"高级体验"

### 2. 改造方案对比

#### 方案 A:共享 HTTP 后端,Wails 仅充当 WebView 外壳(✅ 选定)

桌面版的 Wails 入口启动**和 Web 版一模一样的** `server.New() + Listen("127.0.0.1:0")`,拿到端口后把 Wails 主窗口的 URL 设到 `http://127.0.0.1:<port>/`。

```text
┌──────────────── easy-ffmpeg-desktop (Wails) ─────────────────┐
│                                                              │
│  cmd/desktop/main.go                                         │
│    ├─ wails.Run(&options.App{                                │
│    │     Title: "Easy FFmpeg",                               │
│    │     OnStartup: startBackend,  ← 内部启动 server.New     │
│    │     URL: "http://127.0.0.1:<port>/",                    │
│    │  })                                                     │
│    └─ OnShutdown: srv.RequestShutdown()                      │
│                                                              │
│  共享的后端代码(与 Web 版完全相同):                            │
│    server/  service/  editor/  internal/{job,embedded,...}   │
│                                                              │
└──────────────────────────────────────────────────────────────┘
```

**优点**:

- 前端零改动。SSE、`/api/fs/*`、`/api/editor/*`、剪辑器整套照旧
- 后端零改动。`server.go` 不需要任何 `if wails { ... }` 分支
- 风险面最小:Wails 出 bug 也不影响 Web 版
- 测试可达性最高:两个产物跑的是同一份后端,测一处覆盖两处

**缺点**:

- 仍然占用一个本地端口(127.0.0.1,外部不可访问,无安全损失,但端口冲突理论上存在)
- 没有完全用上 Wails 的"原生 binding"(即不通过 HTTP,直接 JS ↔ Go 调用)。但反过来想,**这正是为什么前端无需重写**

#### 方案 B:把后端方法 `Bind` 给 Wails,前端通过 `window.go.*` 调用(❌ 拒绝)

Wails 推荐的"原生模式"。前端废弃所有 `fetch(...)` 与 `EventSource(...)`,改用 `window.go.App.ConvertStart(...)` 这类绑定调用,进度通过 `runtime.EventsEmit` 推送。

**为什么不选**:

- 前端要重写一整套通信层(`app.js` 的 fetch / `JobBus` 的 EventSource / `EditorApi` / 三个 Tab 的 dryRun + overwrite 流程)
- Web 版必须保留 fetch / SSE,等于要**同时维护两套通信路径** —— 和"双产物共用代码"的目标背道而驰
- Wails binding 没有 SSE 等价物,要用 `EventsEmit/On` 自己重构进度广播;现有 `internal/job` 的 `Subscribe()` 模型适配成本不低
- 目标群体是"偶尔做格式转换的普通用户",不需要原生通信带来的微秒级延迟优化

#### 方案 C:完全不要 Wails,自己拉 WebView(❌ 拒绝)

例如直接调 `webview/webview` C 库或系统 API。比 Wails 多花精力实现窗口生命周期、菜单、dialog、信号处理,没收益。

### 3. 与既有"非目标"的协调

[product.md §5](product.md) 此前明文写过:

> 不打包 webview(不走 Electron / Tauri / Wails 路线)

这是 0.2.x 时代为了"零依赖、可跨编、二进制小"做的决策。本版本撤销该非目标,理由:

1. **目标用户拓展**:Jupyter 式启动对开发者很自然,但部分普通用户被"为啥要打开浏览器"困惑;桌面版降低这个心智门槛
2. **代价已变低**:剪辑器 / 音频 / 转换全部已经成熟,不再有"我们到底要不要做这块功能"的不确定性
3. **保留逃生通道**:方案 A 的本质是"加一个外壳",Web 版完全不退场。用户对桌面版不满意可以回到 Web 版,零迁移成本

---

## 二、怎么做(架构设计)

### 4. 设计原则(不变量)

落地切片中任何改动都必须满足这五条,违反即否决:

1. **后端零分支**:`server/` 及其下游不得出现 `if wails {}` / `// +build wails` 这类宿主感知代码。Web 版与桌面版跑的必须是**完全相同**的 `server.New() + Listen()` 字节。
2. **前端零改动**:`server/web/`(含 `editor/`)继续用 `fetch` + `EventSource` 与 `http://127.0.0.1:<port>/api/*` 对话;不引入 `window.go.*` 或 `runtime.EventsOn` 等 Wails 原生 binding。
3. **入口并列、不互相依赖**:`cmd/main.go`(Web)与 `cmd/desktop/main.go`(桌面)是 Go 链接器视角下的两个独立 main 包,各自从 `server` import 一份代码副本。Web 入口不感知 Wails 的存在。
4. **桌面版回退路径恒在**:用户对桌面版任何不满都可以回退到 Web 版,无需迁移配置/工程文件(两者共享 `~/.easy-ffmpeg/`)。
5. **跨平台编译保护**:Web 版**继续保持** `CGO_ENABLED=0` 跨编 4 平台。Wails 引入的 CGO 强制开启**只影响桌面版**,必须用 build tag 或独立目录把 cgo 依赖隔离在 `cmd/desktop/` 之内。

### 5. 高层架构

```text
                     ┌────────────────────────────────────────────┐
                     │       共享层(Web 版与桌面版完全相同)         │
                     │  server/      service/      editor/         │
                     │  internal/{job,embedded,browser,procutil}   │
                     │  config/                                    │
                     │  go:embed web/  +  embedded/<os>/*.7z       │
                     └─────────────────────┬──────────────────────┘
                                           │
                  ┌────────────────────────┼────────────────────────┐
                  │                        │                        │
                  ▼                        ▼                        ▼
         ┌─────────────────┐      ┌─────────────────┐      ┌─────────────────┐
         │  cmd/main.go    │      │ cmd/desktop/    │      │ (未来)           │
         │  Web 版入口      │      │  main.go        │      │ cmd/easy-editor │
         │                 │      │  桌面版入口       │      │  剪辑器独立 exe  │
         │ server.Listen   │      │ wails.Run        │      │                 │
         │ + browser.Open  │      │  + server.Listen │      │                 │
         │ + Ctrl+C 监听    │      │  + WebView 指向 │      │                 │
         └─────────────────┘      │    localhost:port│      └─────────────────┘
                                  └─────────────────┘
                                           │
                                           ▼
                                  ┌─────────────────┐
                                  │  WebView2  /    │
                                  │  WKWebView /    │
                                  │  WebKitGTK      │
                                  └─────────────────┘
```

**核心拓扑**:桌面版进程内同时运行

- 一个 Wails 主窗口(持有平台 WebView)
- 一个 `127.0.0.1:<random>` 上的 HTTP 服务(与 Web 版同构)
- WebView 的 `URL` 指向该服务

通信路径**仍是 HTTP/SSE**,不走 Wails binding。这是方案 A 的全部精髓。

### 6. 桌面版入口实现

#### 6.1 目录结构

```text
cmd/
├── main.go                       Web 版(不动)
├── icon.ico
├── rsrc_windows.syso
└── desktop/                      Wails 桌面版入口
    ├── main.go                   wails.Run + 生命周期钩子
    ├── app.go                    App 结构体 + startup/shutdown
    ├── frontend/dist/index.html  极简 shell(JS 跳转用,P2 路径)
    ├── wails.json                Wails 配置
    ├── icon.png                  Wails 应用图标
    └── build/                    Wails build 输出占位(.gitignore)
```

**为什么把 Wails 配置放 `cmd/desktop/` 而不是仓库根**:

- Wails 默认假设 `wails.json` 在仓库根并把它当作整个项目根目录。我们反过来:让 `cmd/desktop/` 自成一个 Wails 子项目,仓库根继续是 Go module 根
- 这样 `wails build` 的工作目录就是 `cmd/desktop/`,不污染仓库其他部分;同时 Go module 边界(`go.mod` 在仓库根)保持不变,`server/` 等共享包通过 `easy-ffmpeg/server` 路径正常 import
- `cmd/desktop/build/` 和 `cmd/desktop/frontend/` 等 Wails 工具链产物全部进 `.gitignore`

#### 6.2 `app.go`(职责切分)

```go
// cmd/desktop/app.go
package main

import (
    "context"
    "log"

    "easy-ffmpeg/server"
    "easy-ffmpeg/service"

    "github.com/wailsapp/wails/v2/pkg/runtime"
)

// App 是 Wails 应用的状态容器,**不是**业务逻辑承载者。所有业务都在
// 共享的 server.Server 里跑;这里只负责"何时启动 / 何时停止"。
type App struct {
    ctx context.Context
    srv *server.Server
    url string // 例如 "http://127.0.0.1:54321/"
}

func (a *App) startup(ctx context.Context) {
    a.ctx = ctx
    a.srv = server.New()
    bound, err := a.srv.Listen("127.0.0.1:0")
    if err != nil {
        log.Fatalf("desktop: listen failed: %v", err)
    }
    a.url = "http://" + bound + "/"
    log.Printf("desktop: backend bound at %s", a.url)

    go func() {
        if err := service.Prepare(); err != nil {
            log.Printf("desktop: ffmpeg prepare failed: %v", err)
        }
    }()

    runtime.EventsEmit(ctx, "backend-ready", a.url)
}

func (a *App) shutdown(ctx context.Context) {
    if a.srv != nil {
        a.srv.RequestShutdown()
    }
}
```

#### 6.3 WebView 加载本地 HTTP URL:主路径 + 回退路径

这是方案 A 唯一一处真正需要"按 Wails 版本核对"的胶水。设计上准备**两条路径**,主路径不行就走回退路径,对前端代码完全透明。

**主路径 P1**:让 Wails 直接 navigate 到 `http://127.0.0.1:<port>/`。如果当前 Wails v2 版本支持在 `OnDomReady` 或类似钩子中调用 `runtime.WindowReload(ctx)` 或直接设定外部 URL,则 `App.startup` 拿到端口后立刻把窗口导航过去。

**回退路径 P2**:shell.html + JS `location.replace`(当前实现选择)。

```html
<!doctype html>
<html><head><meta charset="utf-8"><title>Easy FFmpeg</title></head>
<body>
  <script>
    window.runtime.EventsOn('backend-ready', (url) => {
      location.replace(url);
    });
  </script>
</body></html>
```

跳转后 `document.URL` 都是 `http://127.0.0.1:<port>/`,所有 `fetch('/api/...')` / `new EventSource('/api/...')` 的相对路径都解析到同一个 origin。**前端代码无任何条件分支**。

### 7. 生命周期映射

| 事件 | Web 版 | 桌面版 |
|------|--------|--------|
| 启动 | `main.go` 同步 `Listen` + 异步 `Prepare` + `browser.Open` + `Wait` | `wails.Run` → `OnStartup` 调 `Listen` + `Prepare`;不调 `browser.Open` |
| 解压进度 | 控制台进度条 + 浏览器轮询 `/api/prepare/status` | **只有**浏览器轮询(无控制台)→ shell 加载完成后看到与 Web 版同款遮罩进度 |
| 退出 | `Ctrl+C` 或 `POST /api/quit` 触发 `srv.RequestShutdown` | 关窗触发 `OnShutdown` → `srv.RequestShutdown`;`POST /api/quit` 仍可用且无害 |
| 控制台 | 有(启动信息 + 进度条 + 日志) | 无(Wails 默认不附控制台) |

**已知行为差异**:

- **解压进度**桌面版用户**只在窗口里看到**遮罩进度,看不到 Web 版的控制台进度条。这是预期行为,不是 bug。后续切片可考虑桌面版**不启动** printer goroutine 节省一点资源;非必需。
- **`browser.Open` 的另一处用途仍保留**:在 reveal 文件夹(`/api/fs/reveal`)等场景下,桌面版仍 import `internal/browser/open.go` 调用系统文件管理器。

### 8. 端口冲突与单实例

桌面版与 Web 版**不互斥** —— 它们各自 `Listen("127.0.0.1:0")` 拿随机端口,可以共存:

- 同时跑 Web 版和桌面版会**有两个独立的进程、两个端口、共享 `~/.easy-ffmpeg/`**;解压缓存复用,但运行任务彼此不可见(`internal/job` 是进程内单例)
- 用户大多不会主动这么做;不强制单实例锁

### 9. cgo 隔离

**核心约束**:Wails 强制 `CGO_ENABLED=1` 且依赖平台 C 工具链;Web 版必须保持 `CGO_ENABLED=0` 跨编。

实现手段:**入口隔离 + 依赖隔离**。

- 所有 Wails 相关 import (`github.com/wailsapp/wails/v2/...`) **只**出现在 `cmd/desktop/` 包内
- `server/`、`editor/`、`internal/`、`service/`、`config/` 等共享包**严禁** import Wails
- 这样 `go build ./cmd`(Web 版)即使在 `CGO_ENABLED=0` 下也能成功,因为依赖图根本不含 cgo 符号
- `go build ./cmd/desktop` 才会触发 cgo 编译,此时必须 `CGO_ENABLED=1` + 本机 C 工具链

**验收**:CI(v0.4.x)上加一条 `CGO_ENABLED=0 go build ./cmd` 检查,防止误把 Wails import 渗入共享层。

### 10. 后端共享层的"不可逾越的边界"

代码评审对照清单:

- `server/server.go`:`New()`、`Listen()`、`Wait()`、`RequestShutdown()` 签名与语义**完全冻结**;`routes(mux)` 不得新增宿主感知分支;`silentPaths` 列表不得为桌面版新增条目
- `server/web/`:`index.html` 不得引入任何 `if (window.runtime) {}` 之类的桌面端探测分支。整套前端**只知道自己在浏览器里** —— 是 Chrome、WebView2 还是 WKWebView 对它透明
- `editor/`:模块入口 `editor.NewModule(Deps)` 的 SOLID 设计天然支持双产物;桌面版的 `cmd/desktop/` 不直接 import `editor`,而是经由 `server.New()` 间接装配
- `internal/embedded/`:`service.Prepare()` 路径不变;`~/.easy-ffmpeg/bin-<hash>/` 缓存被两个产物共享;`Progress` 数据结构与 `/api/prepare/status` 端点不变
- `internal/{job,browser,procutil}/`:全部不动

### 11. 启动时序(桌面版)

对照 [architecture.md §3](architecture.md) 的 Web 版时序:

```text
T=0.0s   wails.Run 启动 → 创建主窗口(暂未加载内容)
T=0.0s   OnStartup 钩子触发
         ├─ a.srv = server.New()
         ├─ a.srv.Listen("127.0.0.1:0") → bound = "127.0.0.1:54321"
         ├─ a.url = "http://127.0.0.1:54321/"
         └─ go service.Prepare() 异步启动解压
T=0.05s  WebView 完成初始 navigate
         · P1 路径:直接 navigate 到 a.url
         · P2 路径:加载 shell.html → JS 监听 backend-ready 事件 → location.replace
T=0.3s   前端 app.js 启动 → 调 /api/prepare/status
         └─ state="extracting" → 显示遮罩 + 进度条
T=~40s   解压完成 → /api/prepare/status 返回 ready → 遮罩淡出
T=...    用户操作 = 与 Web 版完全相同
T=close  用户关窗
         ├─ Wails 触发 OnShutdown
         ├─ a.srv.RequestShutdown()
         ├─ http.Server.Shutdown(3s timeout)
         └─ 进程退出
```

**与 Web 版的唯一实质性差异**:T=0.05s 的 navigate 来源不同(一个是浏览器自己 `browser.Open`,一个是 Wails WebView 内部)。从 T=0.3s 之后两者完全同构。

### 12. 平台 runtime 依赖

| 平台 | WebView 实现 | 用户依赖 | 处理 |
|------|--------------|----------|------|
| Windows 10+ | WebView2 | Runtime 通常预装;个别老镜像没有 | README 写明;Wails 安装包可选地引导用户装 Evergreen Runtime |
| Windows 7/8 | — | WebView2 不支持 | **不支持桌面版**;引导回退到 Web 版 |
| macOS 11+ | WKWebView | 系统自带,零依赖 | — |
| Linux | WebKitGTK | `libwebkit2gtk-4.0-37` 或 `4.1-0` | README 列出 apt/dnf 包名;旧发行版(CentOS 7)受阻 |

`internal/embedded/<os>.7z` 与平台 runtime 解耦 —— 它只决定"程序能跑 ffmpeg",与"程序能开窗"是两件事。

### 13. WebView 引擎差异点(剪辑器关注)

剪辑器是三大 Tab 中**唯一**深度依赖浏览器多媒体能力的模块。三引擎差异点清单:

| 能力 | Chrome(Web 版) | WebView2(Win) | WKWebView(macOS) | WebKitGTK(Linux) |
|------|----------------|----------------|--------------------|--------------------|
| `<video>` HTTP Range 请求 | 标配 | 同 Chromium | 通常 OK,少数 codec 走不同路径 | OK |
| `<video>` muted 自动播放 | OK | OK | 严格策略(页面需用户手势) | OK |
| WebAudio `GainNode` 音量调节 | OK | OK | OK | OK |
| `MediaSource` / SourceBuffer | OK | OK | 部分 codec 受限 | 受 GStreamer 插件影响 |
| 拖拽 + DataTransfer | OK | OK | OK | OK |
| `EventSource`(SSE) | OK | OK | OK | OK |
| 编码兼容(h265/hevc) | 视系统 | 视系统 | macOS 12+ OK | 受 GStreamer 影响,常缺 |

**设计上的对策**:

- 不在前端做 `navigator.userAgent` 分支
- "无法预览"走与 Web 版相同的提示路径,按 codec 而非按引擎判定
- 验收必须**在三个引擎上各跑一次**整套剪辑流程(导入 → split → 调音量 → 导出)

### 14. 实现状态

| 切片 | 状态 | 说明 |
|------|------|------|
| 0 · 工具链准备 | ✅ 已完成 | Wails v2.12.0 + Go 1.22.0 + MinGW-w64 安装到位;`go mod tidy` 拉齐依赖 |
| 1 · `cmd/desktop/main.go` 骨架 | ✅ 已落地 | `cmd/desktop/main.go` + `app.go` |
| 2 · WebView 加载 URL | ✅ 走回退路径 P2 | shell HTML + Go 端 `runtime.EventsEmit` 推 URL → JS `location.replace` |
| 3 · 构建脚本扩展 | ✅ 已落地 | `build.bat` / `build.sh` — Wails 默认输出 + 后置 `move` 到 `dist/` |
| 4 · Windows 桌面版冒烟 | 🟡 编译通过,运行时未测 | `dist/easy-ffmpeg-desktop.exe` 42 MB 已产出;待双击运行验证 shell→localhost 跳转、解压进度遮罩、convert/audio/editor 三 Tab |
| 5 · 剪辑器在 WebView 中验证 | ⏳ 阻塞于切片 4 | — |
| 6–7 · macOS / Linux 联调 | ⏳ 阻塞于对应平台工具链 | — |
| 8 · 文档同步 | ✅ 已落地 | 设计文档结构已重组(本次重写) |

### 15. 不在 v0.4.0 做(推迟)

- 接入 Wails 原生 dialog(替换自绘文件浏览器)
- 系统托盘 / 最小化到托盘
- 原生菜单栏(File / Edit / Help)
- 自动更新 / 版本检查
- macOS 公证 + Windows 代码签名
- GitHub Actions 矩阵构建
- Linux AppImage 打包
- 启动 splash screen(替代当前的 HTML 遮罩)

任何此类需求出现在切片实施过程中都应被记录到 [roadmap.md](roadmap.md) 的 v0.4.x 槽位。

### 16. 验收标准

v0.4.0 视为完成的判据:

- [ ] `cmd/desktop/main.go` + `cmd/desktop/app.go` 存在且编译通过
- [ ] `cmd/main.go` 一行未改
- [ ] `server/`、`editor/`、`service/`、`internal/` 在两版本之间字节相同
- [ ] `CGO_ENABLED=0 go build ./cmd` 在 Win/macOS/Linux 三平台都能通过
- [ ] Windows 桌面版 `.exe` 双击启动、关窗即退
- [ ] 桌面版三个 Tab 全部跑通;剪辑器在三个 WebView 引擎里都过
- [ ] `build.bat` / `build.sh` 自动跳过当前机器编不了的桌面产物
- [ ] 设计文档同步(本次重组完成)
