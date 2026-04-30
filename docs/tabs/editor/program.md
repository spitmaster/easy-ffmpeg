# 单视频剪辑器 — 程序设计

> 模块代码结构、SOLID 分层、接口契约、独立编译路径。对应产品设计:[product.md](product.md)。共享后端模块见 [core/modules.md](../../core/modules.md);前端架构见 [core/frontend.md](../../core/frontend.md)。
>
> **v0.6.0 架构抽取(M3 完成)**:`Clip` / 时间轴纯函数(`Split`/`DeleteClip`/`Reorder`/`TrimLeft`/`TrimRight`/`SetProgramStart`/`ClipAtProgramTime`)/ 单轨 filter 构造(`BuildVideoTrackFilter`/`BuildAudioTrackFilter`/`PlanSegments`)/ codec 归一化 / `ExportSettings` / 通用 ports(`Clock`/`JobRunner`/`PathResolver`)已抽到 [editor/common/](../../../editor/common/),与 `multitrack/` 共用。`editor/domain/` 通过 type alias + 函数变量 re-export 保持外部 API 表面字节级不变。本文档继续作为单视频专属语义(`Project` / `Source` / schema 迁移 / `BuildExportArgs` 整体装配)的描述。

---

## 1. 设计目标

1. **与主程序其他功能完全解耦**:剪辑器不依赖 `server/handlers.go` / `server/handlers_audio.go` 的具体实现,只依赖**自己定义的接口**。反过来,主程序可以在不了解剪辑器内部的情况下把它装配进去。
2. **可独立编译**:未来新增 `cmd/easy-editor/` 入口时,只需引入 `editor/` 包(+ 必要的接口实现),不需要把 `server/` 的其它 handler 一起拖进去。
3. **符合 SOLID**(见 §3)。
4. **测试友好**:业务规则(domain)与 I/O(storage、ffmpeg 子进程)严格分离,domain 层全部纯函数,单元测试不需要真实文件/真实 ffmpeg。

---

## 2. 目录结构

```text
editor/
├── module.go                       对外:NewModule(deps) + Module.Register(mux)
├── config.go                       模块级常量(存储路径、schema 版本)
│
├── domain/                         纯业务逻辑,零 I/O,零第三方依赖
│   ├── project.go                  Project / Clip / Source / Export 结构体 + 不变量
│   ├── project_test.go
│   ├── timeline.go                 Split / Delete / Reorder / Trim 等纯函数
│   ├── timeline_test.go
│   ├── export.go                   BuildExportArgs(Project) → []string
│   └── export_test.go
│
├── ports/                          单视频专属端口(v0.6.0+ 收窄)
│   ├── repository.go               ProjectRepository 接口(依赖 domain.Project)
│   └── prober.go                   VideoProber 接口
│   # JobRunner / PathResolver / Clock 已抽到 editor/common/ports/(多轨共用)
│
├── storage/                        ports.ProjectRepository 的 JSON 实现
│   ├── jsonrepo.go
│   └── jsonrepo_test.go
│
└── api/                            HTTP handler;只依赖 ports.* 和 domain.*
    ├── handlers_projects.go        CRUD
    ├── handlers_export.go          export / cancel
    ├── handlers_probe.go           probe
    ├── handlers_source.go          /api/editor/source(Range 文件服务)
    ├── dto.go                      请求/响应 DTO;与 domain.* 解耦
    └── routes.go                   Register(mux, prefix)
```

### 2.1 依赖方向(严格单向)

```text
┌────────────────────────────────────────────────────────────┐
│  cmd/main.go  (DI 根)                                       │
│    ↓ 注入 ports 的实现                                       │
│  editor.Module                                              │
│    ├─ editor/api   ──────→  editor/ports  ←────  editor/storage │
│    ↓                         ↑                              │
│  editor/domain  (纯)  ←──────┘                              │
└────────────────────────────────────────────────────────────┘
                  ↓ (通过 ports 接口调用,不知道实现)
       ┌──────────┴──────────┐
       ▼                     ▼
  service/ (主程序适配)    job/ (主程序适配)
```

**关键约束**:

- `editor/domain/` 不 import 任何其他包(除 stdlib)
- `editor/api/` 不 import `service`、`internal/job`、`internal/embedded`
- `editor/storage/` 不 import `editor/api`
- `editor/ports/` 只定义接口,不含实现

---

## 3. SOLID 原则映射

### 3.1 单一职责(S)

| 文件 | 唯一职责 |
|------|---------|
| `domain/project.go` | 定义数据结构与不变量 |
| `domain/timeline.go` | 时间轴操作的纯函数 |
| `domain/export.go` | 把 Project 映射到 ffmpeg 参数数组 |
| `storage/jsonrepo.go` | 把 Project 持久化到 JSON 文件 |
| `api/handlers_projects.go` | HTTP 层:解析请求 → 调 repo / domain → 返回 JSON |

### 3.2 开放/封闭(O)

- 新增存储方式(如将来上 sqlite)→ 新写一个 `sqliterepo.go` 实现 `ProjectRepository`,不改 `api/`
- 新增一种预览代理机制(proxy file / webcodecs)→ 新写一个 handler,不动 domain

### 3.3 Liskov(L)

- `ProjectRepository` 的所有实现(JSON / sqlite)必须对相同输入产生语义一致的输出,包括 `ErrNotFound` 语义
- 测试里用 `fakes.InMemoryRepo` 替代 `jsonrepo`,`api/` handler 逻辑应当无感

### 3.4 接口隔离(I)

**不**搞一个 "EditorDeps 上帝接口"。而是按能力拆:

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
    FFprobePath() string
}

// editor/ports/clock.go
type Clock interface {
    Now() time.Time
}
```

handler 只依赖它真正需要的接口(`projects.go` 只收 `ProjectRepository + Clock`;`export.go` 只收 `JobRunner + PathResolver + ProjectRepository`)。

### 3.5 依赖倒置(D)

- `api/` 依赖 `ports/` 接口;具体实现在 `cmd/main.go` 装配
- `editor.Module` 的构造函数只接受接口,不碰 `service.Probe*` / `internal/job.Manager` 的具体类型

---

## 4. 公开 API(`editor/module.go`)

对外唯一入口(公开面最小化):

```go
package editor

import (
    "net/http"

    "easy-ffmpeg/editor/api"
    commonports "easy-ffmpeg/editor/common/ports"
    "easy-ffmpeg/editor/ports"
    "easy-ffmpeg/editor/storage"
)

type Deps struct {
    Prober  ports.VideoProber           // 单视频专属
    Runner  commonports.JobRunner       // 共享(多轨同接口)
    Paths   commonports.PathResolver    // 共享
    Clock   commonports.Clock           // 共享。可为 nil → 用 wallClock
    DataDir string                      // e.g. "~/.easy-ffmpeg/projects"
}

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

func (m *Module) Register(mux *http.ServeMux, prefix string) {
    m.routes.Register(mux, prefix)  // 通常 prefix = "/api/editor"
}

// StaticAssets 返回嵌入的前端资源,调用方决定怎么服务
func (m *Module) StaticAssets() http.FileSystem {
    return api.WebAssets()
}
```

只有 `Deps`、`Module`、`NewModule` 暴露出去;`api.*` / `domain.*` / `storage.*` / `ports.*` 对 `editor` 外部不可见(靠约定)。

---

## 5. 各包细节

### 5.1 `editor/domain/`

**纯函数层**,完全没有 I/O / 数据库 / 网络 / 时钟。

```go
// domain/project.go
const SchemaVersion = 3

type Project struct {
    SchemaVersion int
    ID            string
    Name          string
    CreatedAt     time.Time
    UpdatedAt     time.Time
    Source        Source
    VideoClips    []Clip
    AudioClips    []Clip
    AudioVolume   float64        // 线性增益 0–2.0;默认 1.0
    Export        ExportSettings
    LegacyClips   []Clip         // 仅 v1 解码用,Migrate 后 nil
}

type Clip struct {
    ID           string
    SourceStart  float64  // 源时间秒,起点
    SourceEnd    float64  // 源时间秒,终点(开区间)
    ProgramStart float64  // 节目时间秒,该 clip 在轨道上的起点
}

func (c Clip) Duration() float64   { return c.SourceEnd - c.SourceStart }
func (c Clip) ProgramEnd() float64 { return c.ProgramStart + c.Duration() }

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
    Format      string
    VideoCodec  string
    AudioCodec  string
    OutputDir   string
    OutputName  string
}

// VideoDuration / AudioDuration 各自轨道的最长 ProgramEnd(leading gap 计入)
func (p *Project) VideoDuration() float64
func (p *Project) AudioDuration() float64

// ProgramDuration 节目总时长 = 两轨之最大值
func (p *Project) ProgramDuration() float64

// Validate 返回所有不变量违反
func (p *Project) Validate() []error

// Migrate v1→v2 把 LegacyClips 拆成两轨;v2→v3 用累加给 ProgramStart 填默认值;
// AudioVolume<=0(缺省)→ 升到 1.0。多次调用幂等
func (p *Project) Migrate()
```

```go
// domain/timeline.go — 全部纯函数,返回新 slice 不就地修改
func Split(clips []Clip, programTime float64, newID string) ([]Clip, error)
func DeleteClip(clips []Clip, id string) ([]Clip, error)
func Reorder(clips []Clip, fromIdx, toIdx int) ([]Clip, error)
func TrimLeft(clips []Clip, id string, newSourceStart float64) ([]Clip, error)
func TrimRight(clips []Clip, id string, newSourceEnd float64) ([]Clip, error)
```

```go
// domain/export.go
func BuildExportArgs(p *Project) (args []string, outputPath string, err error)

// 内部辅助:
//   programDur = max(VideoDuration, AudioDuration)
//   planSegments(clips, totalDur) 在轨道末尾自动追加 trailing gap,
//     当 cursor < totalDur — 短轨用 color=c=black(视频)/ anullsrc(音频)补齐
//   buildVideoTrackFilter(clips, src, totalDur)
//   buildAudioTrackFilter(clips, volume, totalDur)
//     volume != 1.0 时把 concat 输出从 [a] 改为 [a_pre],再追加 [a_pre]volume=X[a]
```

**导出期校验**:

- 视频轨开头不能留空:`earliestProgramStart(VideoClips) > 0` → 返回错误。**音频轨开头允许 leading gap**(pre-roll 静音是常见用法,filter graph 用 `anullsrc` 自动填补)
- 没有 clip:错误
- 缺 `OutputDir / OutputName / Format`:错误

**两轨自动等长**:`programDur` 是 `max(VideoDuration, AudioDuration)`,两条 filter 链都按这个长度 pad。Chrome `<video>` 元素遇到一长一短的两个流会在短流处停止 —— padding 后两个流长度严格一致,所有播放器一致播完。

这三个文件合起来 < 700 行;覆盖测试率目标 ≥ 90%。

### 5.2 `editor/ports/`

接口定义配对应的 DTO 类型:

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

**注意 DTO 重复**:`ports.VideoInfo` 和 `domain.Source` 字段重合但**两个不同的类型**。理由:

- `domain.Source` 是工程内部结构(可能因 schema 演进变化)
- `ports.VideoInfo` 是与外部世界的契约
- 解耦后,domain 可以自由演化 Source 字段,不污染 port 接口;外部实现(`service.ProbeVideo`)不必因 domain 改字段而一起改

`api/` 负责在两者间转换(见 `dto.go`)。

### 5.3 `editor/storage/`

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

func (r *JSONRepo) List(...)     // 实现 ports.ProjectRepository
func (r *JSONRepo) Get(...)
func (r *JSONRepo) Save(...)
func (r *JSONRepo) Delete(...)

// 失败自愈:index 缺失 / 损坏时 glob("*.json") 重建
func (r *JSONRepo) loadOrRebuildIndex() error
```

- 读写用 `os.WriteFile` 原子写(先写临时再 rename)
- index 与文件双写:Save 先写 `<id>.json`,再更新 index;有中断在启动时 rebuild 兜底

### 5.4 `editor/api/`

每个 handler 构造函数接受它所需的最小接口集合:

```go
type ProjectHandlers struct {
    repo   ports.ProjectRepository
    prober ports.VideoProber
    clock  commonports.Clock
}
```

`routes.go` 把它们组装进 `http.ServeMux`:

```go
func (r *Router) Register(mux *http.ServeMux, prefix string) {
    mux.HandleFunc(prefix+"/projects", r.proj.listOrCreate)    // GET/POST
    mux.HandleFunc(prefix+"/projects/", r.proj.getUpdateDelete) // GET/PUT/DELETE
    mux.HandleFunc(prefix+"/probe", r.probe.probe)
    mux.HandleFunc(prefix+"/export", r.expo.start)
    mux.HandleFunc(prefix+"/export/cancel", r.expo.cancel)
    mux.HandleFunc(prefix+"/source", r.src.serve)
}
```

**导出请求 DTO(与 convert / audio 同形)**:

```go
type exportRequest struct {
    ProjectID string                 `json:"projectId"`
    Export    *domain.ExportSettings `json:"export"`    // 可选 override,不 persist
    Overwrite bool                   `json:"overwrite"`
    DryRun    bool                   `json:"dryRun"`
}
```

`handlers_export.go` 流程:参数校验 → 读 Project → 合并 Export overrides → `BuildExportArgs` → 若 `DryRun` 直接返回 `{ok, dryRun, command, outputPath}` 不动文件不启进程;否则若 `!Overwrite && os.Stat(outPath) ok` 返回 409 + `existing:true`;否则 `runner.Start`。

---

## 6. 主程序装配

### 6.1 三个适配器(`server/editor_wiring.go`)

```go
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

### 6.2 装配(`server/server.go`)

```go
func New() *Server {
    s := &Server{ /* ... */ }
    mux := http.NewServeMux()

    // 现有路由注册 ...
    registerFsRoutes(mux, s)
    registerConvertRoutes(mux, s)
    registerAudioRoutes(mux, s)

    // 装配剪辑器
    dataDir, _ := editorDataDir()   // ~/.easy-ffmpeg/projects
    mod, err := editor.NewModule(editor.Deps{
        Prober:  proberAdapter{},
        Runner:  jobRunnerAdapter{m: s.jobs},
        Paths:   pathResolverAdapter{},
        DataDir: dataDir,
    })
    if err != nil { /* 日志,降级:Tab 禁用 */ }
    mod.Register(mux, "/api/editor")

    s.http = &http.Server{Handler: logMiddleware(mux)}
    return s
}
```

**关键**:editor 的 JobRunner 用的**就是** `s.jobs` —— 剪辑器的导出和视频转换共享同一个 job manager;同时只跑一个任务。既保持原 SSE 复用,也避免并发 ffmpeg 压垮 CPU。

---

## 7. API 路由

| 方法 | 路径 | 作用 |
|------|------|------|
| `GET` | `/api/editor/projects` | 列出工程(读 index.json) |
| `POST` | `/api/editor/projects` | 新建工程(需 source path;后端调 ffprobe 填 metadata) |
| `GET` | `/api/editor/projects/:id` | 读单个工程 |
| `PUT` | `/api/editor/projects/:id` | 全量替换工程(前端保存) |
| `DELETE` | `/api/editor/projects/:id` | 删除工程文件 + 更新 index |
| `POST` | `/api/editor/probe` | 探测视频(`service.ProbeVideo` 壳) |
| `POST` | `/api/editor/export` | 开始导出;body = 工程 + export settings;走 `jobs.Start` |
| `POST` | `/api/editor/export/cancel` | 取消当前导出 |
| `GET` | `/api/editor/source?id=<id>` | 以工程 id 为准把 source 文件通过 `http.ServeContent`(支持 Range)喂给 `<video>` |

`GET /api/convert/stream`(共享 SSE)在导出时复用。

### 7.1 新建工程请求

```jsonc
POST /api/editor/projects
{
  "sourcePath": "D:/videos/vacation.mp4",
  "name": "My Vacation Edit"        // 可选,默认 "未命名工程 <timestamp>"
}
```

**后端处理**:

1. `ProbeVideo(sourcePath)` 拿 duration/w/h/codec
2. 生成 `id` (8-hex)、`createdAt`、`updatedAt = createdAt`
3. 初始 `videoClips = [{id:"v1", sourceStart:0, sourceEnd:duration, programStart:0}]`(整段作为一个 clip);若 source.HasAudio 则同样初始 `audioClips`
4. 保存到 `projects/<timestamp>_<id>.json` + 更新 index
5. 返回完整 Project JSON

### 7.2 `/api/editor/source?id=<id>` 安全

此端点会把本地文件流给 `<video>`。和旧 `/api/fs/reveal` 同样,因为服务只绑 `127.0.0.1`,本机进程才能访问;但应当:

- 只允许 path 指向**当前已加载工程的 source.path**(服务端维护"已授权路径白名单",用工程加载时注册)
- 支持 HTTP Range(`<video>` seek 需要)

---

## 8. 前端结构

`server/web/editor/editor.js` 里的 IIFE 模块:

```js
const EditorStore = (() => { /* 发布订阅、commit、history、rangeSelection */ })();
const EditorApi   = (() => { /* fetch wrappers: listProjects / ... */ })();
const Preview     = (() => { /* 双 element:muted <video> + <audio>;
                                WebAudio GainNode 接通 audio,
                                gain.value = project.audioVolume(0–2.0);
                                gap clock 用 rAF 驱动播放头穿过空隙时
                                video.classList.add("in-gap") 显黑屏 */ })();
const Timeline    = (() => { /* DOM 渲染 + 三列 grid(label/actions/scroll)
                                + clip 拖拽(reorder/trim)
                                + 右键拖刻度尺定义 rangeSelection
                                + 大游标 / 单轨小游标 */ })();
const ProjectsModal = (() => { /* 剪辑记录列表 */ })();
const ExportModal   = (() => { /* 导出配置 + 启动 panel.start,传 totalDurationSec */ })();
const TimelineOps   = (() => { /* split / delete (识别 rangeSelection);undo/redo */ })();
const EditorTab     = (() => { /* init、顶栏、全局快捷键 */ })();

window.EditorTab = EditorTab;  // 暴露给 app.js 的 init 序列
```

### 8.1 Preview 模块关键交互

- WebAudio gain pipeline(懒初始化):`MediaElementSource(<audio>) → GainNode → destination`,`gain.value = audioVolume`,`<audio>.volume` 名存实亡。`createMediaElementSource` 抛错时退化到 `audio.volume = min(1, v)`,预览静默封顶 100%
- Gap clock:播放头穿过视频轨空隙(包括尾部空隙),`<video>` 暂停 + `.in-gap` 类隐藏(容器 `#0b0b0b` 透出黑底);`requestAnimationFrame` 按真实时间推进 `playhead`;穿入下一段视频 clip 时把 `<video>.currentTime` 设到对应源时间并恢复播放;到达节目总长统一 pause
- `<video>` 的 `ended` 事件智能化:源 EOF 时若 `playhead < totalDuration`(音频还有内容),不直接停,挂 `.in-gap` + `startGapClock()` 接力,音频继续播完
- 播放头形态:`splitScope=both` → 大游标(跨双轨菱形头);`splitScope=video/audio` → 该轨内小游标。**播放一次即把 splitScope 永久提升为 both**
- 范围选区(`rangeSelection: {start, end}`):右键在刻度尺上拖定义,半透明黄虚线框;`splitAtPlayhead` 见到选区按 `splitScope=both` 在两端各切一刀;`deleteSelection` 见到选区按 `splitScope=both` 把 `[start, end]` 整段碾空(carveRange 函数);保留空隙不左移;Esc / 再次右键(零拖动)清除选区

---

## 9. 测试策略

### 9.1 分层

| 层 | 测试方式 | 覆盖率目标 |
|----|----------|-----------|
| `domain/` | 纯单元测试(表驱动) | ≥ 90% |
| `storage/` | 基于 `t.TempDir()` 的文件系统测试;无网络 / 无 ffmpeg | ≥ 80% |
| `api/` | `httptest.NewRecorder` + in-memory fake repo/prober/runner | ≥ 70% |
| `editor/module_test.go` | Smoke test:NewModule + Register + 请求 `/api/editor/projects` 走通 | 1 条 |

### 9.2 现有测试

| 文件 | 覆盖 |
|------|------|
| `editor/domain/project_test.go` | `NewProject`、`ProgramDuration`、`Validate` 各类不变量违反、`Migrate` 各 schema 版本升级 |
| `editor/domain/timeline_test.go` | `Split` / `DeleteClip` / `Reorder` / `TrimLeft` / `TrimRight` 正反路径、不改原 slice |
| `editor/domain/export_test.go` | 多 clip / 无音轨 / 各种缺参的 `BuildExportArgs`;中间 / 尾部空隙;视频开头不允许 leading-gap,音频开头允许;`AudioVolume` unity / 非 unity / 0 默认;短轨自动 pad black/silence 到 `programDur` |
| `editor/storage/jsonrepo_test.go` | roundtrip、删除后再 Get、按更新时间排序、索引损坏后重建 |

### 9.3 fakes 目录(规划)

```text
editor/
└── internal/
    └── fakes/
        ├── repo.go     InMemoryRepo
        ├── prober.go   StubProber{result, err}
        ├── runner.go   RecordingRunner{startedArgs, canceled}
        └── clock.go    FixedClock
```

注意放 `editor/internal/fakes/`:Go 的 `internal` 规则保证这些 fake 不被 `editor/` 外的包引用,不会污染公开 API。

---

## 10. 独立 exe 的实现路径(未来)

目标:一个只有剪辑功能的小型 exe(不含 convert / audio)。

```go
// cmd/easy-editor/main.go(规划)
package main

func main() {
    jobMgr := job.New()
    mod, _ := editor.NewModule(editor.Deps{
        Prober:  proberAdapter{},
        Runner:  jobRunnerAdapter{m: jobMgr},
        Paths:   pathResolverAdapter{},
        DataDir: defaultDataDir(),
    })

    mux := http.NewServeMux()
    mux.Handle("/", http.FileServer(mod.StaticAssets()))
    mod.Register(mux, "/api/editor")

    // 最小化 HTTP 服务 + 浏览器拉起 + 优雅关停
}
```

体积优化:

- `service` 和 `internal/job` 仍会被拉进来,但它们只有几百行
- `internal/embedded` 的 7z 仍嵌入一份(可选:单独出一个 `cmd/easy-editor-slim` 依赖系统 PATH 的 ffmpeg)
- 前端资源:可以启用独立的 `editor.html`(在 `editor/web/` 下)作为完整页面

---

## 11. 性能与资源注意

- `JSONRepo.List` 只读 `index.json`,O(1) 次磁盘读,不打开每个工程 JSON
- `JSONRepo.Save` 先写 `tmp` 再 rename,保证崩溃安全;同时更新 index 的写也走原子替换
- `/api/editor/source` 必须支持 Range,否则 `<video>` seek 会退化为每次重下整个文件(卡顿)
- 导出期间前端不 poll 工程(`dirty=false`,无 commit 触发 save);避免导出与保存竞争

---

## 12. 风险与已知妥协

| 风险 | 妥协 | 回应 |
|------|------|------|
| 浏览器不支持源视频的编码 | 预览黑屏 | MVP 用文案提示;v2 上 proxy |
| 同一 job manager 导致"正在转码时不能导出" | 全局单 job 语义不变 | UI 上看到当前状态即可 |
| 工程 JSON 被外部编辑 → 数据不合法 | Validate 返回错误列表,打开工程时过滤不合法 clip | 不致命 |
| 路径白名单(`/api/editor/source`)被绕过 | 127.0.0.1 绑定 + 工程加载时注册的一次性许可 | 与现有 `/api/fs/reveal` 同等级 |
| `<video>` 的 `currentTime` 精度 | 原生浏览器差异(Chromium > Safari > Firefox) | 采用"预期 vs 实际"日志警告;v3 上 WebCodecs |
