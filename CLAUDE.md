# CLAUDE.md

> 给未来的 Claude:**先读 [design/](design/) 再动手**。本仓库的设计文档是真相源(canonical),代码注释和顶层 `README.md` / `STRUCTURE.md` 可能滞后。

## 第一步:读设计文档

入口:[design/README.md](design/README.md) — 文档索引。

按场景挑读:

| 场景 | 必读 |
|------|------|
| 完全不了解项目 | [design/overview.md](design/overview.md) → [design/architecture.md](design/architecture.md) |
| 改/加功能 | [design/feature-design.md](design/feature-design.md) + [design/module-design.md](design/module-design.md) |
| 改音频相关 | [design/audio-feature-design.md](design/audio-feature-design.md) |
| 改剪辑器 | [design/editor-feature-design.md](design/editor-feature-design.md) + [design/editor-module-design.md](design/editor-module-design.md) |
| 改 UI / 前端 | [design/ui-design.md](design/ui-design.md) |
| 改构建/打包 | [design/build-and-deploy.md](design/build-and-deploy.md) |
| 桌面版(Wails)相关 | [design/v0.4.0.md](design/v0.4.0.md) + [design/v0.4.0-architecture.md](design/v0.4.0-architecture.md) |
| 路线图/技术债 | [design/roadmap.md](design/roadmap.md) |

> 顶层的 [STRUCTURE.md](STRUCTURE.md) 已过时(还在描述早期 `ui/` 包),**别用它指导工作**,以 `design/` 为准。

## 项目一句话定位

跨平台图形化 FFmpeg 工具。**架构是本地 HTTP 服务 + 浏览器 Web UI**(类似 Jupyter Notebook),不是传统桌面 GUI。FFmpeg 二进制以 7z 形式 `go:embed` 进 Go 二进制,首次启动解压到 `~/.easy-ffmpeg/bin-<hash>/`。

## 双入口拓扑(v0.4.0+)

```
共享层(server/ service/ editor/ internal/ config/)── 字节相同
    │
    ├── cmd/main.go         → Web 版,浏览器打开 localhost
    └── cmd/desktop/main.go → 桌面版,Wails WebView 指向同一个 localhost
```

详见 [design/v0.4.0-architecture.md](design/v0.4.0-architecture.md)。

## 不可违反的架构不变量

改动前必须确认不会破坏这些(完整列表见 [design/v0.4.0-architecture.md](design/v0.4.0-architecture.md) §1):

1. **后端零分支**:`server/` 及下游不得出现宿主感知代码(`if wails {}` / build tag)。Web 版与桌面版跑完全相同的字节。
2. **前端零改动**:`server/web/` 只用 `fetch` + `EventSource` 与 `127.0.0.1:<port>/api/*` 对话,不引入 Wails 原生 binding。
3. **CGO 隔离**:Web 版必须保持 `CGO_ENABLED=0` 跨编 4 平台。Wails 的 cgo 强制开启**只能影响 `cmd/desktop/`**,共享包严禁 import `github.com/wailsapp/wails/...`。
4. **桌面版回退路径恒在**:用户随时能回退到 Web 版,共享 `~/.easy-ffmpeg/`。

## 关键目录(对照 design 时的速查)

- [cmd/](cmd/) — 入口(Web `main.go` + `desktop/`)
- [server/](server/) — HTTP 服务、路由、handlers、`go:embed web/`
- [server/web/](server/web/) — 纯 HTML/CSS/JS 前端,**零构建**
- [editor/](editor/) — 单视频剪辑器(SOLID 分层:`domain`/`api`/`ports`/`storage`)
- [service/](service/) — FFmpeg/FFprobe 命令封装
- [internal/embedded/](internal/embedded/) — 7z 嵌入与解压
- [internal/job/](internal/job/) `internal/browser/` `internal/procutil/` — 进程内任务、打开浏览器、隐藏子进程窗口
- [design/](design/) — **真相源**

## 测试

- `server/audio_args_test.go` — 音频命令构建器
- `editor/domain/*_test.go` — 剪辑器纯函数(Project/Timeline/Export)
- `editor/storage/jsonrepo_test.go` — JSON 仓库

跑测试:`go test ./...`(普通)+ `CGO_ENABLED=0 go test ./...`(验证共享层未渗入 cgo)。

## 沟通约定

- 用户以中文交流,设计文档也是中文。回复用中文。
