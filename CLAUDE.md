# CLAUDE.md

> 给未来的 Claude:**先读 [design/](design/) 再动手**。本仓库的设计文档是真相源(canonical),代码注释和顶层 `README.md` 可能滞后。

## 第零步:看当前进度

如果上一个 session 在做某个跨多 commit 的迁移项目(例如前端 Vue 化),先读 [design/milestones.md](design/milestones.md) ——它记录了"现在到第几个里程碑、下一步要做什么"。**不读这个就接手,大概率会重复别人已经做完的工作或破坏已有约定。**

## 第一步:读设计文档

入口:[design/README.md](design/README.md) — 文档索引。

文档按"共享层 + 每 Tab 一个目录"组织,每个目录里产品设计(`product.md`)和程序设计(`program.md`)分离:

```text
design/
├── README.md
├── milestones.md                    进行中迁移的进度日志(接手者必读)
├── core/                            共享层
│   ├── product.md       (产品)项目定位、价值、非目标
│   ├── ui-system.md     (产品)配色 token、控件、对话框、共享导出体验
│   ├── architecture.md  (程序)后端分层、数据流、启动时序
│   ├── modules.md       (程序)server / service / internal / config 模块清单
│   ├── frontend.md      (程序)Vue 3 工程、API 客户端层、Pinia store、SSE、useJobPanel
│   ├── build.md         (程序)构建脚本(npm + go)、7z 嵌入、桌面版构建
│   ├── desktop.md       (程序)v0.4.0 双产物拓扑、Wails 外壳、cgo 隔离
│   ├── roadmap.md       路线图、技术债、里程碑
│   └── frontend-vue-migration.md (历史)v0.5.x Vue 化迁移方案,落地见 frontend.md
└── tabs/
    ├── convert/{product,program}.md  视频转换
    ├── audio/{product,program}.md    音频处理(三模式)
    └── editor/{product,program}.md   单视频剪辑器
```

未实现的 Tab(媒体信息、设置)暂未建目录。

按场景挑读:

| 场景 | 必读 |
|------|------|
| 完全不了解项目 | [design/core/product.md](design/core/product.md) → [design/core/architecture.md](design/core/architecture.md) |
| 改某个 Tab | 对应 `design/tabs/<tab>/product.md` + `design/tabs/<tab>/program.md` |
| 改 UI / 加新控件 | [design/core/ui-system.md](design/core/ui-system.md) + [design/core/frontend.md](design/core/frontend.md) |
| 改后端共享模块 | [design/core/modules.md](design/core/modules.md) |
| 改构建/打包 | [design/core/build.md](design/core/build.md) |
| 桌面版(Wails)相关 | [design/core/desktop.md](design/core/desktop.md) |
| 路线图/技术债 | [design/core/roadmap.md](design/core/roadmap.md) |

## 项目一句话定位

跨平台图形化 FFmpeg 工具。**架构是本地 HTTP 服务 + 浏览器 Web UI**(类似 Jupyter Notebook),不是传统桌面 GUI。FFmpeg 二进制以 7z 形式 `go:embed` 进 Go 二进制,首次启动解压到 `~/.easy-ffmpeg/bin-<hash>/`。

## 双入口拓扑(v0.4.0+)

```text
共享层(server/ service/ editor/ internal/ config/)── 字节相同
    │
    ├── cmd/main.go         → Web 版,浏览器打开 localhost
    └── cmd/desktop/main.go → 桌面版,Wails WebView 指向同一个 localhost
```

详见 [design/core/desktop.md](design/core/desktop.md)。

## 不可违反的架构不变量

改动前必须确认不会破坏这些(完整列表见 [design/core/desktop.md §4](design/core/desktop.md)):

1. **后端零分支**:`server/` 及下游不得出现宿主感知代码(`if wails {}` / build tag)。Web 版与桌面版跑完全相同的字节。
2. **前端宿主无感**:`web/` 只用 `fetch` + `EventSource` 与 `127.0.0.1:<port>/api/*` 对话,不引入 Wails 原生 binding。
3. **CGO 隔离**:Web 版必须保持 `CGO_ENABLED=0` 跨编 4 平台。Wails 的 cgo 强制开启**只能影响 `cmd/desktop/`**,共享包严禁 import `github.com/wailsapp/wails/...`。
4. **桌面版回退路径恒在**:用户随时能回退到 Web 版,共享 `~/.easy-ffmpeg/`。

## 关键目录(对照 design 时的速查)

- [cmd/](cmd/) — 入口(Web `main.go` + `desktop/`)
- [server/](server/) — HTTP 服务、路由、handlers;通过 `import "easy-ffmpeg/web"` 拿前端资源
- [web/](web/) — 前端工程(v0.5.x+,Vue 3 + Vite + TS + Pinia + Tailwind);源码 `web/src/`,产物 `web/dist/` 由 `web/embed.go` 用 `//go:embed all:dist` 嵌入
- [editor/](editor/) — 单视频剪辑器(SOLID 分层:`domain`/`api`/`ports`/`storage`)
- [service/](service/) — FFmpeg/FFprobe 命令封装
- [internal/embedded/](internal/embedded/) — 7z 嵌入与解压
- [internal/job/](internal/job/) `internal/browser/` `internal/procutil/` — 进程内任务、打开浏览器、隐藏子进程窗口
- [design/](design/) — **真相源**

## 测试 / 健康检查

- `go test ./...`(普通)+ `CGO_ENABLED=0 go test ./...`(验证共享层未渗入 cgo)
- 前端类型检查 + 构建:`cd web && npm run build`(`vue-tsc --noEmit && vite build`,产物落 `web/dist/`)
- Go 全量编译:`go build ./...`

后端有测试:

- `server/audio_args_test.go` — 音频命令构建器
- `editor/domain/*_test.go` — 剪辑器纯函数(Project/Timeline/Export)
- `editor/storage/jsonrepo_test.go` — JSON 仓库

前端目前无单测(见 [design/core/frontend-vue-migration.md §0](design/core/frontend-vue-migration.md))。

## 沟通约定

- 用户以中文交流,设计文档也是中文。回复用中文。
