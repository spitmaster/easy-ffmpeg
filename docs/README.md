# Easy FFmpeg 设计文档索引

Easy FFmpeg 是一个跨平台的图形化 FFmpeg 工具。程序启动后自动在浏览器里打开一个本地 Web 界面,通过表单完成音视频处理 —— 类似 Jupyter Notebook 的使用模式。v0.4.0 起新增并列的 Wails 桌面版,共享同一份后端代码。

## 目录结构

```text
docs/
├── README.md                  本索引
├── roadmap.md                 ★ 粗粒度产品路线图(月级,功能级)
├── milestones.md              ★ 主索引(进行中 + 已归档)
├── milestones/                每个 active 分支一份独立文件
│   ├── multitrack.md            (例)分支 multitrack 的里程碑日志
│   └── archive/               完成的功能 git mv 至此
│       └── v0.5.1.md            (例)Vue 化迁移历史
├── todo/                      每个 active 分支的当前 M 待办清单
│   └── README.md              说明 + 模板;具体文件按需创建/删除
├── core/                      共享层(后端、前端、构建、桌面版)
└── tabs/                      已实现的 Tab,各自独立目录
    ├── convert/               视频转换
    ├── audio/                 音频处理(三模式)
    └── editor/                单视频剪辑器
```

每个 Tab 目录下分两个文件:`product.md` 是产品设计(目标、交互、字段、规则),`program.md` 是程序设计(代码组织、命令构建、API、测试)。

未实现的 Tab(媒体信息、设置、多轨剪辑)暂不建目录,见 [roadmap.md](roadmap.md)。

## 分支驱动的发现协议(rule v1.1 §5)

**Milestone / todo 文件路径从分支名机械推导**:

```text
docs/milestones/<branch_name with "/" → "_">.md
docs/todo/<branch_name with "/" → "_">.md
```

| 分支 | milestones | todo |
|------|------------|------|
| `multitrack` | `milestones/multitrack.md` | `todo/multitrack.md` |
| `feature-5.3.2/zhouyijin/eating-fish` | `milestones/feature-5.3.2_zhouyijin_eating-fish.md` | `todo/feature-5.3.2_zhouyijin_eating-fish.md` |
| `v0.5.1` | `milestones/archive/v0.5.1.md`(已归档) | _(已归档无 todo)_ |

**接手 session 第一件事**:`git branch --show-current` → 推导文件 → 直接打开;不存在就主动问用户。

## 三档规划文档与晋升规则

文档分**三个粒度**,职责清晰、流转有规则:

| 文档 | 粒度 | 回答的问题 | 更新频率 |
|------|------|-----------|---------|
| [roadmap.md](roadmap.md) | **粗** — 功能级 | 接下来要做哪些功能?边界在哪? | 月级 |
| [milestones.md](milestones.md) + `milestones/<branch>.md` | **中** — 单功能里程碑 | 当前在做哪个功能?到第几个 M? | 周级 |
| `todo/<branch>.md` | **细** — 当前 M 的具体动作 | 这个 M 还差哪几步具体动作? | 日级 |

**晋升触发条件**(三档之间内容如何流转):

| 触发 | 动作 |
|------|------|
| 某功能正式启动开发 | `roadmap.md` 那行标"⏳ 进行中";`milestones.md` 索引加一行;创建分支 + `milestones/<branch>.md` |
| 开始一个具体 M | 把那个 M 的交付内容拆成可勾选清单,**整段填入** `todo/<branch>.md`;`milestones/<branch>.md` 那行从 ⏳ 改 🚧 |
| M 完结 | `milestones/<branch>.md` 那行标 ✅ + commit + 日期;`todo/<branch>.md` **整段清空**(只留模板注释) |
| 整个功能完结 | **`git mv`** `milestones/<branch>.md` 到 `milestones/archive/`;主索引"进行中"挪到"已归档";`todo/<branch>.md` 删除;`roadmap.md` 在"已发布版本"加一行 |

> 接手 session 必读顺序:**先按"分支驱动的发现协议"打开 `milestones/<branch>` 与 `todo/<branch>`** → 再扫 `roadmap.md` 看大方向 → 主索引 `milestones.md` 备查。

## 共享层(core/)

| 文档 | 类型 | 说明 |
|------|------|------|
| [core/product.md](core/product.md) | 产品 | 项目定位、目标用户、核心价值、非目标 |
| [core/ui-system.md](core/ui-system.md) | 产品 | 共享 UI 设计系统:配色、控件家族、对话框约定、跨 Tab 共用的导出体验 |
| [core/architecture.md](core/architecture.md) | 程序 | 后端分层架构、目录结构、数据流、启动时序、并发模型、错误处理 |
| [core/modules.md](core/modules.md) | 程序 | 共享后端模块清单:`server/`、`service/`、`internal/{job,embedded,browser,procutil}`、`config/` |
| [core/frontend.md](core/frontend.md) | 程序 | 前端架构:Vue 3 + Vite + TS + Pinia + Tailwind;`web/` 工程目录、API 客户端层、SSE 总线、`useJobPanel` composable、全局对话框 |
| [core/build.md](core/build.md) | 程序 | 构建脚本、前端 npm 流水、跨平台 Go 编译、7z 嵌入、首次启动解压、桌面版构建 |
| [core/desktop.md](core/desktop.md) | 程序 | v0.4.0 双产物拓扑:决策、Wails 外壳设计、共享层不变量、cgo 隔离 |
| [core/frontend-vue-migration.md](core/frontend-vue-migration.md) | 程序 | 前端 Vue 化迁移方案(v0.5.x,**已完成**):整体规划、目录结构、构建脚本接线、四个里程碑的范围与验收。最终落地见 [frontend.md](core/frontend.md) |

## Tab 详细设计(tabs/)

| Tab | 状态 | 产品设计 | 程序设计 |
|-----|------|---------|---------|
| 视频转换 | ✅ | [tabs/convert/product.md](tabs/convert/product.md) | [tabs/convert/program.md](tabs/convert/program.md) |
| 音频处理(三模式) | ✅ | [tabs/audio/product.md](tabs/audio/product.md) | [tabs/audio/program.md](tabs/audio/program.md) |
| 单视频剪辑器 | ✅ | [tabs/editor/product.md](tabs/editor/product.md) | [tabs/editor/program.md](tabs/editor/program.md) |
| 多轨剪辑器 | 🚧 规划中(M1) | — | — |
| 媒体信息 | 🚧 占位 | — | — |
| 设置 | 🚧 占位 | — | — |

## 快速定位

- **接手进行中的开发** → 按"分支驱动的发现协议"打开 `milestones/<branch>.md` → `todo/<branch>.md`(主索引 [milestones.md](milestones.md) 备查)
- 看大方向 / 历史版本 → [roadmap.md](roadmap.md)
- 完全不了解项目 → [core/product.md](core/product.md) → [core/architecture.md](core/architecture.md)
- 改某个 Tab → 对应 `tabs/<tab>/product.md` + `tabs/<tab>/program.md`
- 改 UI / 加新控件 → [core/ui-system.md](core/ui-system.md) + [core/frontend.md](core/frontend.md)
- 改后端共享模块 → [core/modules.md](core/modules.md)
- 改构建/打包 → [core/build.md](core/build.md)
- 桌面版相关 → [core/desktop.md](core/desktop.md)

## 当前状态(v0.5.1)

- **架构**:本地 HTTP 服务 + 浏览器 Web UI;v0.4.0 起增加并列的 Wails 桌面版
- **前端**:Vue 3 + Vite + TypeScript + Pinia + Vue Router + TailwindCSS,工程目录在 `web/`,产物 `web/dist/` 由 `easy-ffmpeg/web` 包用 `//go:embed all:dist` 嵌入
- **FFmpeg**:7z 压缩包嵌入 Go 二进制,首次启动解压到 `~/.easy-ffmpeg/bin-<hash>/`
- **产物大小**:Web 版 Windows 35 MB · macOS 27 MB · Linux 29 MB;桌面版各 +5–15 MB(前端 bundle 经 Vite 构建后约几十 KB,体积影响可忽略)
- **测试**:`server/audio_args_test.go` + `editor/domain/*_test.go` + `editor/storage/jsonrepo_test.go`;跑 `go test ./...` 与 `CGO_ENABLED=0 go test ./...` 双验证;前端目前未引入单测(见 [frontend-vue-migration.md §0](core/frontend-vue-migration.md))
