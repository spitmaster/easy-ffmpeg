# Easy FFmpeg 设计文档索引

Easy FFmpeg 是一个跨平台的图形化 FFmpeg 工具。程序启动后自动在浏览器里打开一个本地 Web 界面,通过表单完成音视频处理 —— 类似 Jupyter Notebook 的使用模式。v0.4.0 起新增并列的 Wails 桌面版,共享同一份后端代码。

## 目录结构

```text
design/
├── README.md         本索引
├── core/             共享层(后端、前端、构建、桌面版、路线图)
└── tabs/             已实现的 Tab,各自独立目录
    ├── convert/      视频转换
    ├── audio/        音频处理(三模式)
    └── editor/       单视频剪辑器
```

每个 Tab 目录下分两个文件:`product.md` 是产品设计(目标、交互、字段、规则),`program.md` 是程序设计(代码组织、命令构建、API、测试)。

未实现的 Tab(媒体信息、设置)暂不建目录,见 [core/roadmap.md](core/roadmap.md)。

## 共享层(core/)

| 文档 | 类型 | 说明 |
|------|------|------|
| [core/product.md](core/product.md) | 产品 | 项目定位、目标用户、核心价值、非目标 |
| [core/ui-system.md](core/ui-system.md) | 产品 | 共享 UI 设计系统:配色、控件家族、对话框约定、跨 Tab 共用的导出体验 |
| [core/architecture.md](core/architecture.md) | 程序 | 后端分层架构、目录结构、数据流、启动时序、并发模型、错误处理 |
| [core/modules.md](core/modules.md) | 程序 | 共享后端模块清单:`server/`、`service/`、`internal/{job,embedded,browser,procutil}`、`config/` |
| [core/frontend.md](core/frontend.md) | 程序 | 前端 JS 架构:零构建三件套、IIFE 模块、SSE、`createJobPanel`、`Confirm` dialog |
| [core/build.md](core/build.md) | 程序 | 构建脚本、跨平台编译、7z 嵌入、首次启动解压、桌面版构建 |
| [core/desktop.md](core/desktop.md) | 程序 | v0.4.0 双产物拓扑:决策、Wails 外壳设计、共享层不变量、cgo 隔离 |
| [core/roadmap.md](core/roadmap.md) | 综合 | 路线图、技术债、未实现 Tab、已完成里程碑 |

## Tab 详细设计(tabs/)

| Tab | 状态 | 产品设计 | 程序设计 |
|-----|------|---------|---------|
| 视频转换 | ✅ | [tabs/convert/product.md](tabs/convert/product.md) | [tabs/convert/program.md](tabs/convert/program.md) |
| 音频处理(三模式) | ✅ | [tabs/audio/product.md](tabs/audio/product.md) | [tabs/audio/program.md](tabs/audio/program.md) |
| 单视频剪辑器 | ✅ | [tabs/editor/product.md](tabs/editor/product.md) | [tabs/editor/program.md](tabs/editor/program.md) |
| 媒体信息 | 🚧 占位 | — | — |
| 设置 | 🚧 占位 | — | — |

## 快速定位

- 完全不了解项目 → [core/product.md](core/product.md) → [core/architecture.md](core/architecture.md)
- 改某个 Tab → 对应 `tabs/<tab>/product.md` + `tabs/<tab>/program.md`
- 改 UI / 加新控件 → [core/ui-system.md](core/ui-system.md) + [core/frontend.md](core/frontend.md)
- 改后端共享模块 → [core/modules.md](core/modules.md)
- 改构建/打包 → [core/build.md](core/build.md)
- 桌面版相关 → [core/desktop.md](core/desktop.md)
- 路线图 / 技术债 → [core/roadmap.md](core/roadmap.md)

## 当前状态(v0.4.0)

- **架构**:本地 HTTP 服务 + 浏览器 Web UI;v0.4.0 起增加并列的 Wails 桌面版
- **前端**:纯 HTML/CSS/JS,**零构建**,`go:embed` 打包
- **FFmpeg**:7z 压缩包嵌入 Go 二进制,首次启动解压到 `~/.easy-ffmpeg/bin-<hash>/`
- **产物大小**:Web 版 Windows 35 MB · macOS 27 MB · Linux 29 MB;桌面版各 +5–15 MB
- **测试**:`server/audio_args_test.go` + `editor/domain/*_test.go` + `editor/storage/jsonrepo_test.go`;跑 `go test ./...` 与 `CGO_ENABLED=0 go test ./...` 双验证
