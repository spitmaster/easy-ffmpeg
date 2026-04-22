# Easy FFmpeg 设计文档索引

Easy FFmpeg 是一个跨平台的图形化 FFmpeg 工具。程序启动后自动在浏览器里打开一个本地 Web 界面，通过表单完成音视频处理——类似 Jupyter Notebook 的使用模式。

## 文档列表

| 文档 | 说明 |
|------|------|
| [overview.md](overview.md) | 项目总览：定位、技术栈、关键决策 |
| [architecture.md](architecture.md) | 架构设计：分层模型、模块依赖、数据流、启动时序 |
| [module-design.md](module-design.md) | 模块设计：各包职责与关键函数 |
| [feature-design.md](feature-design.md) | 功能设计：已实现/待实现功能的交互与实现 |
| [audio-feature-design.md](audio-feature-design.md) | 音频处理功能专项设计：三种模式、API、命令构建规则 |
| [ui-design.md](ui-design.md) | UI 设计：HTML/CSS/JS 结构、自定义样式、交互细节 |
| [build-and-deploy.md](build-and-deploy.md) | 构建与部署：7z 嵌入机制、跨平台编译、首次启动解压 |
| [roadmap.md](roadmap.md) | 路线图：功能迭代计划与技术债务 |

## 快速定位

- 想了解整体架构 → [architecture.md](architecture.md)
- 想新增功能 → [feature-design.md](feature-design.md) + [module-design.md](module-design.md)
- 想改 UI → [ui-design.md](ui-design.md)
- 想了解打包 → [build-and-deploy.md](build-and-deploy.md)

## 当前状态

- **UI 技术**：本地 HTTP 服务 + 浏览器 Web 界面（纯 HTML/CSS/JS，零前端构建）
- **FFmpeg 分发**：7z 压缩包嵌入 Go 二进制，首次启动解压到 `~/.easy-ffmpeg/bin-<hash>/`
- **产物大小**：Windows 35MB · macOS 27MB · Linux 29MB（原始非压缩方案曾为 350MB+）
- **已实现**：视频格式转换（含自定义编码器/容器、实时日志、取消、目录记忆）
- **占位未实现**：视频裁剪、音频处理、媒体信息、设置
