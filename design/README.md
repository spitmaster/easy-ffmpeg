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
| [editor-feature-design.md](editor-feature-design.md) | 单视频剪辑器 PRD：单视频时间轴剪辑、工程持久化、导出规则 |
| [editor-module-design.md](editor-module-design.md) | 单视频剪辑器模块架构：SOLID 分层、接口契约、独立编译路径 |
| [ui-design.md](ui-design.md) | UI 设计：HTML/CSS/JS 结构、自定义样式、交互细节 |
| [build-and-deploy.md](build-and-deploy.md) | 构建与部署：7z 嵌入机制、跨平台编译、首次启动解压、桌面版构建(§8.5) |
| [roadmap.md](roadmap.md) | 路线图：功能迭代计划与技术债务 |
| [v0.4.0.md](v0.4.0.md) | v0.4.0 双产物可行性论证：方案对比、风险权衡、最终选定方案 A |
| [v0.4.0-architecture.md](v0.4.0-architecture.md) | v0.4.0 落地蓝图：Wails 外壳代码组织、入口设计、共享层契约、cgo 隔离 |

## 快速定位

- 想了解整体架构 → [architecture.md](architecture.md)
- 想新增功能 → [feature-design.md](feature-design.md) + [module-design.md](module-design.md)
- 想改 UI → [ui-design.md](ui-design.md)
- 想了解打包 → [build-and-deploy.md](build-and-deploy.md)

## 当前状态

- **UI 技术**：本地 HTTP 服务 + 浏览器 Web 界面（纯 HTML/CSS/JS，零前端构建；`app.js` 为模块化 IIFE 结构）
- **FFmpeg 分发**：7z 压缩包嵌入 Go 二进制，首次启动解压到 `~/.easy-ffmpeg/bin-<hash>/`
- **产物大小**：Windows 35MB · macOS 27MB · Linux 29MB（原始非压缩方案曾为 350MB+）
- **已实现**：
  - 视频转换（格式 / 编解码 / 容器）
  - 音频处理（格式转换 / 从视频提取 / 合并 三模式；详见 [audio-feature-design.md](audio-feature-design.md)）
  - 单视频剪辑器（v0.3.0）—— 时间轴式单视频剪辑，**视频轨 / 音频轨独立编辑**（split / 删除 / 重排可分轨），工程持久化到 `~/.easy-ffmpeg/projects/`（SchemaVersion **3**：Clip 加 ProgramStart 支持任意位置 + 空隙，Project 加 AudioVolume），filter_complex 双轨独立 concat 导出，自动用 black/silence pad 短轨到节目总长；详见 [editor-feature-design.md](editor-feature-design.md) + [editor-module-design.md](editor-module-design.md)
  - 三个 Tab 共用的导出体验：**实时进度条**（解析 ffmpeg `time=` / `Duration:` 算百分比）+ **自绘命令预览 dialog**（dryRun 拉真实命令 → 一键复制 → 用户确认才执行）+ **自绘覆盖确认 dialog**（替代浏览器原生 `confirm`，× 关闭按钮 / Esc / Enter，不靠点背景关闭）
  - 全局：ffprobe 探测、SSE 日志、取消、首次解压进度、目录记忆、退出按钮
- **占位未实现**：媒体信息、设置
- **已覆盖测试**：
  - `server/audio_args_test.go` —— 音频命令构建器
  - `editor/domain/*_test.go` —— Project / Timeline / Export 纯函数（含 AudioVolume / 短轨 padding / 视频开头 leading-gap 拒绝 / 音频允许 leading-gap 等场景）
  - `editor/storage/jsonrepo_test.go` —— JSON 仓库 roundtrip + 索引自愈
