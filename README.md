# Easy FFmpeg

跨平台图形化 FFmpeg 工具。架构是**本地 HTTP 服务 + 浏览器 Web UI**(类似 Jupyter Notebook),不是传统桌面 GUI;v0.4.0 起新增并列的 Wails 桌面版,共享同一份后端代码。FFmpeg 二进制以 7z 形式 `go:embed` 进 Go 二进制,首次启动解压到 `~/.easy-ffmpeg/bin-<hash>/`。

## 功能

- 视频转换(选编码器、容器、码率,命令预览,覆盖确认)
- 音频处理(格式转换 / 提取 / 合并,三模式同栏切换)
- 单视频剪辑器(双轨独立编辑、入出点修剪、范围选区、撤销重做、音量 0–200%、ffmpeg 导出)

跨平台支持 Windows / macOS / Linux,Web 版四平台跨编(`CGO_ENABLED=0`),桌面版需本机编译。

## 技术栈

- **后端**:Go 1.21+,纯 Go 静态链接;`net/http` + SSE
- **前端**(v0.5.x+):Vue 3 + Vite + TypeScript + Pinia + Vue Router + TailwindCSS,工程在 [web/](web/),产物 `web/dist/` 由 [web/embed.go](web/embed.go) 用 `//go:embed all:dist` 嵌入
- **桌面版**(v0.4.0+):Wails v2,与 Web 版共享同一份后端字节
- **嵌入 FFmpeg**:[internal/embedded/](internal/embedded/) 按平台 7z 分片,首次启动解压

详见 [design/](design/)。

## 环境要求

- Go ≥ 1.21
- Node.js ≥ 20 LTS(用来构建 Vue 前端)
- 桌面版另需:Wails CLI + 平台 C 工具链(详见 [design/core/build.md §8](design/core/build.md))

## 编译

仓库根有两个一键脚本,自动编 Vue 前端 + 4 平台 Web 产物 + 当前平台桌面版:

```bash
# Linux / macOS / Windows Git Bash
./build.sh

# Windows cmd / PowerShell
build.bat
```

产物落在 [dist/](dist/) 目录:`easy-ffmpeg.exe` / `easy-ffmpeg-macos-arm64` / `easy-ffmpeg-macos-amd64` / `easy-ffmpeg-linux`,以及当前平台可编的桌面版。

> **首次构建**:脚本会先在 [web/](web/) 跑 `npm install` + `npm run build`,然后再 `go build`。任意一步失败立即终止。

### 准备 FFmpeg 二进制

每个平台需要一份 7z 压缩包放到 `internal/embedded/<os>/<os>.7z`。打包方法见 [design/core/build.md §3](design/core/build.md)。

## 开发

调试时跑两个进程:

```bash
go run ./cmd                       # 后端 8080
cd web && npm run dev              # Vite dev server 5173,/api/* 代理到 8080
```

热加载 + 真实后端 API。

## 测试

```bash
go test ./...                  # 全部测试
CGO_ENABLED=0 go test ./...    # 验证共享层未渗入 cgo
cd web && npm run build        # 前端类型检查 + 构建
```

## 文档

- [design/README.md](design/README.md) — 设计文档总入口
- [design/milestones.md](design/milestones.md) — 进行中迁移项目的进度日志
- [CLAUDE.md](CLAUDE.md) — 给 AI 协作者的工作约定
- [design/core/architecture.md](design/core/architecture.md) — 后端分层
- [design/core/frontend.md](design/core/frontend.md) — 前端架构
- [design/core/build.md](design/core/build.md) — 构建脚本详解
- [design/core/desktop.md](design/core/desktop.md) — 桌面版双产物拓扑

## 许可证

MIT License
