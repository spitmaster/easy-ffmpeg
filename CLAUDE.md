# CLAUDE.md

> 给未来的 Claude:**先读 [docs/](docs/) 再动手**。本仓库的设计文档是真相源(canonical),代码注释和顶层 `README.md` 可能滞后。
>
> 本项目遵循 `~/.claude/CLAUDE.md` v1.1 的文档规范(三档规划体系 + 晋升规则 + 归档策略 + 多人协作 + 分支发现协议)。规则与项目实践冲突时优先信本文件,然后告知用户哪条规则需要升级。

## 第零步:发现当前工作上下文(分支驱动)

**Milestone / todo 文件路径从分支名机械推导,不需要猜**:

```text
docs/milestones/<branch_name with "/" → "_">.md
docs/todo/<branch_name with "/" → "_">.md
```

| 分支 | milestones 文件 | todo 文件 |
|------|-----------------|-----------|
| `multitrack` | `docs/milestones/multitrack.md` | `docs/todo/multitrack.md` |
| `feature/foo/bar` | `docs/milestones/feature_foo_bar.md` | `docs/todo/feature_foo_bar.md` |
| `v0.5.0` | `docs/milestones/v0.5.0.md`(可能在 archive/) | `docs/todo/v0.5.0.md` |

**接手序列**:

1. `git branch --show-current` → 拿当前分支名
2. 推导 `docs/milestones/<derived>.md` → 打开,看到当前 milestone 上下文
3. 推导 `docs/todo/<derived>.md` → 打开,看到具体待办
4. 任一文件不存在 → **主动问用户**:"当前分支没有对应文件,是要新建,还是这是探索性工作?"
5. 分支是 `main` / `develop` / `hotfix-*` 等非 feature 分支 → 不进 milestone 上下文,问用户做什么

如果只想看大方向(不进具体工作),读:

1. [docs/roadmap.md](docs/roadmap.md) — 粗粒度,功能级
2. [docs/milestones.md](docs/milestones.md) — 主索引(进行中 + 已归档)

## 三档规划文档体系

| 文档 | 粒度 | 回答的问题 | 更新频率 |
|------|------|-----------|---------|
| [docs/roadmap.md](docs/roadmap.md) | **粗** — 功能级 | 接下来要做哪些功能?边界在哪? | 月级 |
| [docs/milestones.md](docs/milestones.md) + `docs/milestones/` | **中** — 单功能里程碑 | 当前在做哪个功能?到第几个 M? | 周级 |
| `docs/todo/<branch>.md` | **细** — 当前 M 的具体动作 | 这个 M 还差哪几步? | 日级 |

**晋升触发条件**:

| 触发 | 动作 |
|------|------|
| 某功能正式启动开发 | `roadmap.md` 那行标"⏳ 进行中";`milestones.md` 索引加一行;创建对应分支 + `docs/milestones/<branch>.md` |
| 开始一个具体 M | 把那个 M 的交付拆成可勾选清单,**整段填入** `docs/todo/<branch>.md`;`docs/milestones/<branch>.md` 那行从 ⏳ 改 🚧 |
| M 完结 | `docs/milestones/<branch>.md` 那行标 ✅ + commit + 日期;`docs/todo/<branch>.md` 整段清空 |
| 整个功能完结 | **`git mv`** `docs/milestones/<branch>.md` 到 `docs/milestones/archive/`;主索引中"进行中"挪到"已归档";`docs/todo/<branch>.md` 删除;`roadmap.md` 在"已发布版本"加一行 |

详细规则见 [docs/README.md](docs/README.md) "三档规划文档" 一节,或 `~/.claude/CLAUDE.md` v1.1 §2-§5。

## 第一步:读设计文档

入口:[docs/README.md](docs/README.md) — 文档索引。

文档按"规划层 + 共享层 + 每 Tab 一个目录"组织:

```text
docs/
├── README.md
├── roadmap.md                       (规划)粗粒度功能路线图
├── milestones.md                    (规划)进行中功能的里程碑日志
├── todo.md                          (规划)当前 M 的待办清单(M 完结即清空)
├── core/                            共享层
│   ├── product.md       (产品)项目定位、价值、非目标
│   ├── ui-system.md     (产品)配色 token、控件、对话框、共享导出体验
│   ├── architecture.md  (程序)后端分层、数据流、启动时序
│   ├── modules.md       (程序)server / service / internal / config 模块清单
│   ├── frontend.md      (程序)Vue 3 工程、API 客户端层、Pinia store、SSE、useJobPanel
│   ├── build.md         (程序)构建脚本(npm + go)、7z 嵌入、桌面版构建
│   ├── desktop.md       (程序)v0.4.0 双产物拓扑、Wails 外壳、cgo 隔离
│   └── frontend-vue-migration.md (历史)v0.5.0 Vue 化迁移方案,落地见 frontend.md
└── tabs/
    ├── convert/{product,program}.md  视频转换
    ├── audio/{product,program}.md    音频处理(三模式)
    ├── editor/{product,program}.md   单视频剪辑器
    └── multitrack/{product,program}.md  多轨剪辑器(类 Premiere Pro)
```

未实现的 Tab(媒体信息、设置)暂未建目录。

按场景挑读:

| 场景 | 必读 |
|------|------|
| 完全不了解项目 | [docs/core/product.md](docs/core/product.md) → [docs/core/architecture.md](docs/core/architecture.md) |
| 改某个 Tab | 对应 `docs/tabs/<tab>/product.md` + `docs/tabs/<tab>/program.md` |
| 改 UI / 加新控件 | [docs/core/ui-system.md](docs/core/ui-system.md) + [docs/core/frontend.md](docs/core/frontend.md) |
| 改后端共享模块 | [docs/core/modules.md](docs/core/modules.md) |
| 改构建/打包 | [docs/core/build.md](docs/core/build.md) |
| 桌面版(Wails)相关 | [docs/core/desktop.md](docs/core/desktop.md) |
| 看功能路线/历史版本 | [docs/roadmap.md](docs/roadmap.md) |

## 项目一句话定位

跨平台图形化 FFmpeg 工具。**架构是本地 HTTP 服务 + 浏览器 Web UI**(类似 Jupyter Notebook),不是传统桌面 GUI。FFmpeg 二进制以 7z 形式 `go:embed` 进 Go 二进制,首次启动解压到 `~/.easy-ffmpeg/bin-<hash>/`。

## 双入口拓扑(v0.4.0+)

```text
共享层(server/ service/ editor/ editor/common/ multitrack/ internal/ config/)── 字节相同
    │
    ├── cmd/main.go         → Web 版,浏览器打开 localhost
    └── cmd/desktop/main.go → 桌面版,Wails WebView 指向同一个 localhost
```

详见 [docs/core/desktop.md](docs/core/desktop.md)。

## 不可违反的架构不变量

改动前必须确认不会破坏这些(完整列表见 [docs/core/desktop.md §4](docs/core/desktop.md)):

1. **后端零分支**:`server/` 及下游不得出现宿主感知代码(`if wails {}` / build tag)。Web 版与桌面版跑完全相同的字节。
2. **前端宿主无感**:`web/` 只用 `fetch` + `EventSource` 与 `127.0.0.1:<port>/api/*` 对话,不引入 Wails 原生 binding。
3. **CGO 隔离**:Web 版必须保持 `CGO_ENABLED=0` 跨编 4 平台。Wails 的 cgo 强制开启**只能影响 `cmd/desktop/`**,共享包严禁 import `github.com/wailsapp/wails/...`。
4. **桌面版回退路径恒在**:用户随时能回退到 Web 版,共享 `~/.easy-ffmpeg/`。

## 关键目录(对照 docs/ 时的速查)

- [cmd/](cmd/) — 入口(Web `main.go` + `desktop/`)
- [server/](server/) — HTTP 服务、路由、handlers;通过 `import "easy-ffmpeg/web"` 拿前端资源
- [web/](web/) — 前端工程(v0.5.0+,Vue 3 + Vite + TS + Pinia + Tailwind);源码 `web/src/`,产物 `web/dist/` 由 `web/embed.go` 用 `//go:embed all:dist` 嵌入
- [editor/](editor/) — 单视频剪辑器(SOLID 分层:`domain`/`api`/`ports`/`storage`)
- [editor/common/](editor/common/) — 单视频与多轨共享的纯函数与端口(`domain`:Clip / PlanSegments / BuildVideoTrackFilter / BuildAudioTrackFilter / Split-Delete-Reorder-Trim / ValidateClips;`ports`:clock/runner/paths)
- [multitrack/](multitrack/) — 多轨剪辑器(类 Premiere Pro,SOLID 分层:`domain`/`api`/`ports`/`storage`),复用 `editor/common/`
- [service/](service/) — FFmpeg/FFprobe 命令封装
- [internal/embedded/](internal/embedded/) — 7z 嵌入与解压
- [internal/job/](internal/job/) `internal/browser/` `internal/procutil/` — 进程内任务、打开浏览器、隐藏子进程窗口
- [docs/](docs/) — **真相源**

## 测试 / 健康检查

- `go test ./...`(普通)+ `CGO_ENABLED=0 go test ./...`(验证共享层未渗入 cgo)
- 前端类型检查 + 构建:`cd web && npm run build`(`vue-tsc --noEmit && vite build`,产物落 `web/dist/`)
- Go 全量编译:`go build ./...`

后端有测试:

- `server/audio_args_test.go` — 音频命令构建器
- `editor/common/domain/*_test.go` — 共享纯函数(Clip / PlanSegments / BuildVideoTrackFilter / BuildAudioTrackFilter)
- `editor/domain/*_test.go` — 单视频剪辑器纯函数(Project/Timeline/Export)
- `editor/storage/jsonrepo_test.go` — JSON 仓库
- `multitrack/domain/*_test.go` — 多轨 timeline / filter / export(含 §5.3 全矩阵)

前端目前无单测(见 [docs/core/frontend-vue-migration.md §0](docs/core/frontend-vue-migration.md))。

## 沟通约定

- 用户以中文交流,设计文档也是中文。回复用中文。
