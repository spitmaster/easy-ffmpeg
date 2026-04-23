# 视频剪辑器模块架构

> 本文档定义"视频剪辑器"的代码结构、包划分、接口契约、依赖注入方式、以及"独立编译"的实现路径。
>
> 配套产品文档：[editor-feature-design.md](editor-feature-design.md)。
>
> **实现状态**：✅ MVP 已落地（`editor/` 包按本文档布局实现，主程序通过 `server/editor_wiring.go` 装配）。独立 exe 入口（`cmd/easy-editor/`）尚未实现 —— 预留接口（`Module.StaticAssets` 返回独立资源等）将在未来切片补齐。

---

## 1. 设计目标

1. **与主程序其他功能完全解耦**：剪辑器不依赖 `server/handlers.go` / `server/handlers_audio.go` 的具体实现，只依赖**自己定义的接口**。反过来，主程序可以在不了解剪辑器内部的情况下把它装配进去。
2. **可独立编译**：未来新增 `cmd/easy-editor/` 入口时，只需引入 `editor/` 包（+ 必要的接口实现），不需要把 `server/` 的其它 handler 一起拖进去。
3. **符合 SOLID**（见 §3）。
4. **测试友好**：业务规则（domain）与 I/O（storage、ffmpeg 子进程）严格分离，domain 层全部纯函数，单元测试不需要真实文件/真实 ffmpeg。

---

## 2. 目录结构

```
easy-ffmpeg/
├── cmd/
│   ├── main.go                         现有入口：装配完整应用（convert + audio + editor）
│   └── easy-editor/                    【未来】只装配 editor 的独立入口
│       └── main.go
│
├── editor/                             【新增】编辑器模块，自成一体
│   ├── module.go                         对外：NewModule(deps) + Module.Register(mux)
│   ├── config.go                         模块级常量（存储路径、schema 版本）
│   │
│   ├── domain/                           纯业务逻辑，零 I/O，零第三方依赖
│   │   ├── project.go                    Project / Clip / Source / Export 结构体 + 不变量
│   │   ├── project_test.go
│   │   ├── timeline.go                   Split / Delete / Reorder / Trim 等纯函数
│   │   ├── timeline_test.go
│   │   ├── export.go                     BuildExportArgs(Project) → []string（导出命令构建）
│   │   └── export_test.go
│   │
│   ├── ports/                            编辑器依赖的抽象（Dependency Inversion）
│   │   ├── repository.go                 ProjectRepository 接口
│   │   ├── prober.go                     VideoProber 接口
│   │   ├── runner.go                     JobRunner 接口
│   │   ├── paths.go                      PathResolver 接口
│   │   └── clock.go                      Clock 接口（时间源，便于测试）
│   │
│   ├── storage/                          ports.ProjectRepository 的 JSON 实现
│   │   ├── jsonrepo.go
│   │   └── jsonrepo_test.go
│   │
│   ├── api/                              HTTP handler；只依赖 ports.* 和 domain.*
│   │   ├── handlers_projects.go          CRUD
│   │   ├── handlers_export.go            export / cancel
│   │   ├── handlers_probe.go             probe
│   │   ├── handlers_source.go            /api/editor/source（Range 文件服务）
│   │   ├── dto.go                        请求/响应 DTO；与 domain.* 解耦
│   │   └── routes.go                     Register(mux, prefix)
│   │
│   └── web/                              编辑器专属静态资源
│       ├── editor.css                    样式；引入到主 app.css 里
│       ├── editor.js                     IIFE：EditorTab / EditorStore / Timeline / Preview
│       └── editor.html                   【未来独立 exe 场景】一个极简独立页面
│
├── server/                             现有主 HTTP 服务
│   ├── server.go                        现有；新增装配代码：editor.NewModule + Register
│   ├── handlers.go                      现有
│   ├── handlers_audio.go                现有
│   ├── handlers_trim.go                 【删除】
│   ├── trim_args.go                     【删除】
│   ├── trim_args_test.go                【删除】
│   ├── audio_args.go                    现有
│   ├── audio_args_test.go               现有
│   └── web/
│       ├── index.html                   移除 trim 面板；引入 editor.js / editor.css
│       ├── app.css                      @import "editor.css" 或合并
│       ├── app.js                       现有 IIFE；不再引用 TrimTab
│       └── editor/                      【软链】或通过 go:embed 合并到上层资源
│
├── service/                            现有业务层
│   ├── ffmpeg.go                        现有；被 editor 通过 PathResolver 适配
│   └── probe.go                         现有；被 editor 通过 VideoProber 适配
│
└── internal/
    ├── job/                             现有；被 editor 通过 JobRunner 适配
    ├── embedded/                        现有
    ├── browser/                         现有
    └── procutil/                        现有
```

### 2.1 依赖方向（严格单向）

```
┌────────────────────────────────────────────────────────────┐
│  cmd/main.go  (DI 根)                                       │
│    ↓ 注入 ports 的实现                                       │
│  editor.Module                                              │
│    ├─ editor/api   ──────→  editor/ports  ←────  editor/storage │
│    ↓                         ↑                              │
│  editor/domain  (纯)  ←──────┘                              │
└────────────────────────────────────────────────────────────┘
                  ↓ （通过 ports 接口调用，不知道实现）
       ┌──────────┴──────────┐
       ▼                     ▼
  service/ (主程序适配)    job/ (主程序适配)
```

**关键约束**：

- `editor/domain/` 不 import 任何其他包（除 stdlib）
- `editor/api/` 不 import `service`、`internal/job`、`internal/embedded`
- `editor/storage/` 不 import `editor/api`
- `editor/ports/` 只定义接口，不含实现

---

## 3. SOLID 原则映射

### 3.1 单一职责（S）

| 文件 | 唯一职责 |
|------|---------|
| `domain/project.go` | 定义数据结构与不变量 |
| `domain/timeline.go` | 时间轴操作的纯函数（Split / Delete / Reorder / Trim） |
| `domain/export.go` | 把 Project 映射到 ffmpeg 参数数组 |
| `storage/jsonrepo.go` | 把 Project 持久化到 JSON 文件 |
| `api/handlers_projects.go` | HTTP 层：解析请求 → 调 repo / domain → 返回 JSON |

### 3.2 开放/封闭（O）

- 新增存储方式（如将来上 sqlite）→ 新写一个 `sqliterepo.go` 实现 `ProjectRepository`，不改 `api/`
- 新增一种预览代理机制（proxy file / webcodecs）→ 新写一个 handler，不动 domain

### 3.3 Liskov（L）

- `ProjectRepository` 的所有实现（JSON / sqlite）必须对相同输入产生语义一致的输出，包括 `ErrNotFound` 语义
- 测试里用 `fakes.InMemoryRepo` 替代 `jsonrepo`，`api/` handler 逻辑应当无感

### 3.4 接口隔离（I）

**不**搞一个 "EditorDeps 上帝接口"。而是按能力拆：

```go
// editor/ports/repository.go
type ProjectRepository interface {
    List(ctx context.Context) ([]ProjectSummary, error)
    Get(ctx context.Context, id string) (*Project, error)
    Save(ctx context.Context, p *Project) error
    Delete(ctx context.Context, id string) error
}

// editor/ports/prober.go
type VideoProber interface {
    Probe(ctx context.Context, path string) (*VideoInfo, error)
}

// editor/ports/runner.go
type JobRunner interface {
    Start(binary string, args []string) error
    Cancel()
    Running() bool
}

// editor/ports/paths.go
type PathResolver interface {
    FFmpegPath() string
    FFprobePath() string   // VideoProber 实现可能需要
}

// editor/ports/clock.go
type Clock interface {
    Now() time.Time
}
```

- handler 只依赖它真正需要的接口（`projects.go` 只收 `ProjectRepository + Clock`；`export.go` 只收 `JobRunner + PathResolver + ProjectRepository`）

### 3.5 依赖倒置（D）

- `api/` 依赖 `ports/` 接口；具体实现在 `cmd/main.go` 装配
- `editor.Module` 的构造函数只接受接口，不碰 `service.Probe*` / `internal/job.Manager` 的具体类型

---

## 4. 公开 API（editor 包对外）

`editor/module.go` 是对外唯一入口（公开面最小化）：

```go
package editor

import (
    "net/http"

    "easy-ffmpeg/editor/api"
    "easy-ffmpeg/editor/ports"
    "easy-ffmpeg/editor/storage"
)

// Deps 是构造 Module 所需的全部依赖
type Deps struct {
    Prober  ports.VideoProber
    Runner  ports.JobRunner
    Paths   ports.PathResolver
    Clock   ports.Clock        // 可为 nil → 用 wallClock
    DataDir string             // e.g. "~/.easy-ffmpeg/projects"
}

// Module 封装一次装配后的剪辑器，用 Register 挂载路由
type Module struct {
    routes *api.Router
}

func NewModule(d Deps) (*Module, error) {
    if d.Clock == nil {
        d.Clock = wallClock{}
    }
    repo, err := storage.NewJSONRepo(d.DataDir)
    if err != nil {
        return nil, err
    }
    r := api.NewRouter(api.Config{
        Repo:   repo,
        Prober: d.Prober,
        Runner: d.Runner,
        Paths:  d.Paths,
        Clock:  d.Clock,
    })
    return &Module{routes: r}, nil
}

// Register 把剪辑器的所有路由挂载到 mux，在 prefix 下
func (m *Module) Register(mux *http.ServeMux, prefix string) {
    m.routes.Register(mux, prefix)  // 通常 prefix = "/api/editor"
}

// StaticAssets 返回嵌入的前端资源，调用方决定怎么服务
func (m *Module) StaticAssets() http.FileSystem {
    return api.WebAssets()
}
```

**注意**：只有 `Deps`、`Module`、`NewModule` 暴露出去。`api.*` / `domain.*` / `storage.*` / `ports.*` 对 `editor` 外部不可见（小写包路径视作模块内部——我们靠约定，Go 不会强制）。

---

## 5. 各包细节

### 5.1 `editor/domain/`

**纯函数层**，完全没有 I/O / 数据库 / 网络 / 时钟。

```go
// domain/project.go
type Project struct {
    SchemaVersion int
    ID            string
    Name          string
    CreatedAt     time.Time
    UpdatedAt     time.Time
    Source        Source
    Clips         []Clip
    Export        ExportSettings
}

type Clip struct {
    ID          string
    SourceStart float64  // seconds
    SourceEnd   float64
}

type Source struct {
    Path        string
    Duration    float64
    Width       int
    Height      int
    VideoCodec  string
    AudioCodec  string
    FrameRate   float64
    HasAudio    bool
}

type ExportSettings struct {
    Format      string  // "mp4" ...
    VideoCodec  string  // "h264" ...
    AudioCodec  string  // "aac" ...
    OutputDir   string
    OutputName  string
}

// ProgramDuration 节目总时长（所有 clip 时长之和）
func (p *Project) ProgramDuration() float64 { ... }

// Validate 返回所有不变量违反（调用方决定是报错还是忽略）
func (p *Project) Validate() []error { ... }
```

```go
// domain/timeline.go — 全部纯函数，返回新 slice 不就地修改
func Split(clips []Clip, programTime float64, newID string) ([]Clip, error)
func DeleteClip(clips []Clip, id string) ([]Clip, error)
func Reorder(clips []Clip, fromIdx, toIdx int) ([]Clip, error)
func TrimLeft(clips []Clip, id string, newSourceStart float64) ([]Clip, error)
func TrimRight(clips []Clip, id string, newSourceEnd float64) ([]Clip, error)
```

```go
// domain/export.go
func BuildExportArgs(p *Project) (args []string, outputPath string, err error)
```

- 这三个文件合起来 < 500 行
- 覆盖测试率目标 ≥ 90%（没有 I/O，纯表驱动）

### 5.2 `editor/ports/`

见 §3.4。接口定义配对应的 DTO 类型，例如：

```go
// ports/prober.go
type VideoInfo struct {
    Duration   float64
    Width      int
    Height     int
    VideoCodec string
    AudioCodec string
    FrameRate  float64
    HasAudio   bool
}

type VideoProber interface {
    Probe(ctx context.Context, path string) (*VideoInfo, error)
}
```

**注意 DTO 重复**：`ports.VideoInfo` 和 `domain.Source` 有字段重合但是**两个不同的类型**。理由：
- `domain.Source` 是工程内部结构（可能因 schema 演进变化）
- `ports.VideoInfo` 是与外部世界的契约
- 解耦后，domain 可以自由演化 Source 字段，不污染 port 接口；外部实现（service.ProbeVideo）不必因 domain 改字段而一起改

`api/` 负责在两者间转换（见 `dto.go`）。

### 5.3 `editor/storage/`

`jsonrepo.go`：

```go
type JSONRepo struct {
    dir       string            // 存放目录
    indexPath string            // index.json
    mu        sync.RWMutex      // 保护并发
}

func NewJSONRepo(dir string) (*JSONRepo, error) {
    if err := os.MkdirAll(dir, 0o755); err != nil { ... }
    r := &JSONRepo{dir: dir, indexPath: filepath.Join(dir, "index.json")}
    if err := r.loadOrRebuildIndex(); err != nil { ... }
    return r, nil
}

// 实现 ports.ProjectRepository
func (r *JSONRepo) List(...)
func (r *JSONRepo) Get(...)
func (r *JSONRepo) Save(...)
func (r *JSONRepo) Delete(...)

// 失败自愈：index 缺失 / 损坏时 glob("*.json") 重建
func (r *JSONRepo) loadOrRebuildIndex() error
```

- 读写用 `os.WriteFile` 原子写（先写临时再 rename）
- index 与文件双写：Save 先写 `<id>.json`，再更新 index；有中断在启动时 rebuild 兜底

### 5.4 `editor/api/`

每个 handler 构造函数接受它所需的最小接口集合：

```go
type ProjectHandlers struct {
    repo   ports.ProjectRepository
    prober ports.VideoProber
    clock  ports.Clock
}

func NewProjectHandlers(repo ..., prober ..., clock ...) *ProjectHandlers { ... }

func (h *ProjectHandlers) list(w http.ResponseWriter, r *http.Request) { ... }
func (h *ProjectHandlers) create(w http.ResponseWriter, r *http.Request) { ... }
// ...
```

`routes.go` 把它们组装进 `http.ServeMux`：

```go
type Router struct {
    proj   *ProjectHandlers
    expo   *ExportHandlers
    probe  *ProbeHandlers
    src    *SourceHandlers
}

func (r *Router) Register(mux *http.ServeMux, prefix string) {
    mux.HandleFunc(prefix+"/projects", r.proj.listOrCreate)    // GET/POST
    mux.HandleFunc(prefix+"/projects/", r.proj.getUpdateDelete) // GET/PUT/DELETE
    mux.HandleFunc(prefix+"/probe", r.probe.probe)
    mux.HandleFunc(prefix+"/export", r.expo.start)
    mux.HandleFunc(prefix+"/export/cancel", r.expo.cancel)
    mux.HandleFunc(prefix+"/source", r.src.serve)
}
```

### 5.5 `editor/web/`

前端结构对应 PRD §5.1：

```js
// editor.js — IIFE 模块
const EditorStore = (() => { /* 发布订阅、commit、history */ })();
const EditorApi   = (() => { /* fetch wrappers: listProjects / ... */ })();
const Preview     = (() => { /* <video> + 节目时间映射 */ })();
const Timeline    = (() => { /* DOM 渲染 + 拖拽 */ })();
const EditorTab   = (() => { /* init、顶栏、全局快捷键 */ })();

window.EditorTab = EditorTab;  // 暴露给 app.js 的 init 序列
```

`app.js` 的改动：在 init 序列里增加 `EditorTab.init()`，其它不动。

---

## 6. 主程序装配（`cmd/main.go` 与 `server/server.go`）

### 6.1 三个适配器（在主程序侧，不在 editor 里）

主程序写三个小适配器把现有能力桥接到 ports：

```go
// server/editor_wiring.go （新增）
package server

import (
    "context"
    "easy-ffmpeg/editor/ports"
    "easy-ffmpeg/internal/job"
    "easy-ffmpeg/service"
)

type proberAdapter struct{}

func (proberAdapter) Probe(_ context.Context, path string) (*ports.VideoInfo, error) {
    res, err := service.ProbeVideo(path)
    if err != nil { return nil, err }
    return &ports.VideoInfo{
        Duration:   res.Format.Duration,
        Width:      res.Video.Width,
        Height:     res.Video.Height,
        VideoCodec: res.Video.CodecName,
        // ...
        HasAudio:   res.Audio != nil,
    }, nil
}

type jobRunnerAdapter struct{ m *job.Manager }

func (a jobRunnerAdapter) Start(binary string, args []string) error {
    return a.m.Start(binary, args)
}
func (a jobRunnerAdapter) Cancel()       { a.m.Cancel() }
func (a jobRunnerAdapter) Running() bool { return a.m.Running() }

type pathResolverAdapter struct{}

func (pathResolverAdapter) FFmpegPath() string  { return service.GetFFmpegPath() }
func (pathResolverAdapter) FFprobePath() string { return service.GetFFprobePath() }
```

### 6.2 装配

```go
// server/server.go 的 New() 内
func New() *Server {
    s := &Server{ /* ... */ }
    mux := http.NewServeMux()

    // 现有路由注册 ...
    registerFsRoutes(mux, s)
    registerConvertRoutes(mux, s)
    registerAudioRoutes(mux, s)

    // 新增：装配剪辑器
    dataDir, _ := editorDataDir()   // ~/.easy-ffmpeg/projects
    mod, err := editor.NewModule(editor.Deps{
        Prober:  proberAdapter{},
        Runner:  jobRunnerAdapter{m: s.jobs},
        Paths:   pathResolverAdapter{},
        DataDir: dataDir,
    })
    if err != nil { /* 日志，降级：Tab 禁用 */ }
    mod.Register(mux, "/api/editor")

    s.http = &http.Server{Handler: logMiddleware(mux)}
    return s
}
```

这里**关键**：editor 的 JobRunner 用的 **就是** `s.jobs` —— 剪辑器的导出和视频转换共享同一个 job manager；同时只跑一个任务。既保持原 SSE 复用，也避免并发 ffmpeg 压垮 CPU。

---

## 7. 独立 exe 的实现路径（未来）

目标：一个只有剪辑功能的小型 exe（不含 convert / audio）。

### 7.1 新入口

```go
// cmd/easy-editor/main.go
package main

import (
    "net/http"
    "os"

    "easy-ffmpeg/editor"
    "easy-ffmpeg/internal/browser"
    "easy-ffmpeg/internal/job"
    "easy-ffmpeg/service"
    // ...
)

func main() {
    jobMgr := job.New()
    mod, _ := editor.NewModule(editor.Deps{
        Prober:  proberAdapter{},
        Runner:  jobRunnerAdapter{m: jobMgr},
        Paths:   pathResolverAdapter{},
        DataDir: defaultDataDir(),
    })

    mux := http.NewServeMux()
    mux.Handle("/", http.FileServer(mod.StaticAssets()))  // 独立 HTML 入口
    mod.Register(mux, "/api/editor")

    // 最小化 HTTP 服务 + 浏览器拉起 + 优雅关停
    // （复制自 cmd/main.go 的骨架）
}
```

### 7.2 体积优化

- `service` 和 `internal/job` 仍会被拉进来，但它们只有几百行
- `internal/embedded` 的 7z 仍嵌入一份（可选：单独出一个 `cmd/easy-editor-slim` 依赖系统 PATH 的 ffmpeg）
- 前端资源：可以启用独立的 `editor.html`（在 `editor/web/` 下）作为完整页面

### 7.3 让未来更顺：eager 设计点

- 所有 `editor/web/` 的资源**自己管理**（不与 `server/web/` 共享 CSS 变量）；**或者**提取一份共享的 `shared.css` 放到新顶层 `assets/css/`，被两边 import。MVP 暂定方案：把主题变量放 `editor.css` 的 `:root`，复用成本最低。
- handler 不假设 host 路径（不硬编码 `/api/editor`，路由挂载时注入 `prefix`）✓ 已在 §5.4 体现

---

## 8. 测试策略

### 8.1 分层

| 层 | 测试方式 | 覆盖率目标 |
|----|----------|-----------|
| `domain/` | 纯单元测试（表驱动） | ≥ 90% |
| `storage/` | 基于 `t.TempDir()` 的文件系统测试；无网络 / 无 ffmpeg | ≥ 80% |
| `api/` | `httptest.NewRecorder` + in-memory fake repo/prober/runner | ≥ 70% |
| `editor/module_test.go` | Smoke test：NewModule + Register + 请求 `/api/editor/projects` 走通 | 1 条 |

### 8.2 fakes 目录

```
editor/
└── internal/
    └── fakes/
        ├── repo.go     InMemoryRepo
        ├── prober.go   StubProber{result, err}
        ├── runner.go   RecordingRunner{startedArgs, canceled}
        └── clock.go    FixedClock
```

注意放 `editor/internal/fakes/`：Go 的 `internal` 规则保证这些 fake 不被 `editor/` 外的包引用，不会污染公开 API。

### 8.3 不写的测试

- 不写 e2e（启动真实 ffmpeg 跑小视频）—— 成本大于收益
- 不为 `service/probe.go`、`internal/job` 追加测试（它们已有既定稳定性，且不属于本迭代新增）

---

## 9. 性能与资源注意

- `JSONRepo.List` 只读 `index.json`，O(1) 次磁盘读，不打开每个工程 JSON
- `JSONRepo.Save` 先写 `tmp` 再 rename，保证崩溃安全；同时更新 index 的写也走原子替换
- `/api/editor/source` 必须支持 Range，否则 `<video>` seek 会退化为每次重下整个文件（卡顿）
- 导出期间前端不 poll 工程（`dirty=false`，无 commit 触发 save）；避免导出与保存竞争

---

## 10. 风险与已知妥协

| 风险 | 妥协 | 回应 |
|------|------|------|
| 浏览器不支持源视频的编码 | 预览黑屏 | MVP 用文案提示；v2 上 proxy |
| 同一 job manager 导致"正在转码时不能导出" | 全局单 job 语义不变 | UI 上看到当前状态即可 |
| 工程 JSON 被外部编辑 → 数据不合法 | Validate 返回错误列表，打开工程时过滤不合法 clip | 不致命 |
| 路径白名单（`/api/editor/source`）被绕过 | 127.0.0.1 绑定 + 工程加载时注册的一次性许可 | 与现有 `/api/fs/reveal` 同等级 |
| `<video>` 的 `currentTime` 精度 | 原生浏览器差异（Chromium > Safari > Firefox） | 采用"预期 vs 实际"日志警告；v3 上 WebCodecs |

---

## 11. 与现有文档的差异一览

| 主题 | 旧 | 新 |
|------|-----|-----|
| 包组织 | `server/` 下放所有 handler | `editor/` 自成一体；`server/` 只加一段 wiring 代码 |
| 依赖方向 | handler 直接调 `service.*` / `jobs.*` | handler → ports → 主程序注入实现 |
| 测试位置 | `server/*_test.go` | `editor/domain/*_test.go`（主力）+ `editor/storage/*_test.go` + `editor/api/*_test.go` |
| 前端资源 | 全在 `server/web/` | `editor/web/` 专属；主 HTML 在 `server/web/` 引入 |
