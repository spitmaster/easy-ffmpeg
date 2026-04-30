# 多轨剪辑器(类 Premiere Pro)— 里程碑日志

> **对应分支**:`multitrack`(或 `multitrack-m<N>` 系列)
> **目标**:在新 Tab 下提供类 Premiere Pro 的多轨剪辑器(多源导入 + 多视频/音频轨 + 视频叠加/PiP + 音频混流 + 多轨导出),与单视频剪辑 Tab **共存**,共享底层(后端 domain 抽出共用层、前端时间轴组件抽出共用层)。
> **范围与设计**:M1 后落到 [../tabs/multitrack/product.md](../tabs/multitrack/product.md);M2 后落到 [../tabs/multitrack/program.md](../tabs/multitrack/program.md)。
>
> **状态**:Phase A M1 待启动(2026-04-30 起草本计划)
> **当前 M**:见对应分支的 `docs/todo/<branch>.md`(分支创建后)

## 全局不变量

每个 M 落盘前必跑,违反任何一条该 M 视为未完成:

- `CGO_ENABLED=0 go test ./...` 通过(共享层不引入 cgo)
- 后端零分支(`server/` 不出现 `if wails {}`)
- 前端宿主无感(`web/` 不引入 Wails 原生 binding)
- **单视频剪辑 Tab 视觉与交互零回归**(手测:打开视频 → 分割 → 范围选区 → 删除 → 撤销 → 导出全链路)
- JobRunner 仍全局单实例(多轨导出与转换/单视频导出互斥)

## Phase A — 准备阶段(无新功能,纯文档 + 重构)

| 里程碑 | 状态 | 完成日期 | Commit | 交付内容 |
|--------|------|---------|--------|---------|
| **M1** PRD | ⏳ 待启动 | — | — | [../tabs/multitrack/product.md](../tabs/multitrack/product.md):目标/非目标、多源 + 多轨数据模型草图、UI 草图(在单视频基础上加哪些控件)、与单视频 Tab 的能力对比表、预览策略选型(单 `<video>` 切源 / 多 `<video>` 同步 / Canvas 合成,各方案权衡)、导出滤镜图概念、典型用户场景。明确 v1 范围(建议:多源 concat + 多视频轨叠加 + 音频 mix,**不**含转场/调色/关键帧) |
| **M2** 技术设计 | ⏳ 未开始 | — | — | [../tabs/multitrack/program.md](../tabs/multitrack/program.md):Project schema(`schemaVersion: 4` 或独立文件家族?)、`Track`/`Clip` 数据结构、后端共享层方案(包名 `editor/common/` vs 独立 `media/`)、前端共享组件清单与命名空间、API 路由(`/api/multitrack/*`)、JobRunner 共用策略确认、[../roadmap.md](../roadmap.md) §3 非目标与 `tabs/editor/product.md §1.2` 的修订计划;[../README.md](../README.md) 索引补 `tabs/multitrack/` |
| **M3** 后端共享层抽取 | ⏳ 未开始 | — | — | 把 `editor/domain/` 中通用部分(`Clip` 基础结构、`planSegments` 与 gap 填充、`buildXxxTrackFilter` 单轨 filter 构造、视频/音频 codec 枚举验证)提到 `editor/common/domain/`(M2 锁定包名);单视频 `editor/` 改为引用共享层;`editor/ports/` 中通用的 `JobRunner`/`PathResolver`/`Clock` 也提共享层;`go test ./...` 全绿;`/api/editor/*` 字节级不变 |
| **M4** 前端共享组件抽取 | ⏳ 未开始 | — | — | 把 `EditorTimeline.vue` / `EditorPlayBar.vue` / `EditorExportDialog.vue` / `EditorExportSidebar.vue` / `EditorProjectsModal.vue` / `useJobPanel.ts` / 时间轴标尺与 playhead / 范围选区逻辑抽到 `web/src/components/timeline-shared/` + `web/src/composables/timeline/`;`EditorView.vue` 重写为基于共享组件的薄壳;`npm run build` 通过;单视频 Tab 视觉与交互零回归 |

## Phase B — 落地阶段(多轨功能逐层加,每个 M 都能独立 ship)

| 里程碑 | 状态 | 完成日期 | Commit | 交付内容 |
|--------|------|---------|--------|---------|
| **M5** 多轨工程骨架 | ⏳ 未开始 | — | — | 后端:`multitrack/` 包按 SOLID 分层(domain / ports / storage / api / module),`POST /api/multitrack/projects` 创建空工程,JSON 落 `~/.easy-ffmpeg/multitrack/`(M2 锁定路径);前端:新增 Tab + `MultitrackView.vue` 空壳 + `stores/multitrack.ts` + `api/multitrack.ts`;TabNav 加项 |
| **M6** 多源导入 + 多轨渲染 | ⏳ 未开始 | — | — | 多个视频/音频文件加进同一工程作为 `Source[]`;时间轴显示 N 条视频轨 + M 条音频轨;clip 可按 sourceId 拖到任意轨道任意时间;预览走 M2 选定方案(默认建议:单 `<video>` 切源 + 多 `<video>` 同步混合) |
| **M7** 多源剪辑操作 | ⏳ 未开始 | — | — | split / delete / trim / 范围选区 / 重排 / 撤销重做在多轨模型上跑通(共享 `useEditorOps` 复用,只换数据模型层 store);`multitrack/domain/timeline_test.go` 覆盖跨多轨场景 |
| **M8** 导出 v1:concat + amix | ⏳ 未开始 | — | — | `multitrack/domain/export.go` 构造 filter_complex:每条视频轨独立 concat → 顶层 `overlay` 链(z-order 按轨号);每条音频轨独立 concat → `amix`;短轨 pad 黑屏/静音;导出对话框复用 M4 共享组件;关键测试落 `multitrack/domain/export_test.go` |
| **M9** 视频叠加(PiP / overlay) | ⏳ 未开始 | — | — | clip 增加 `position`/`scale`/`opacity` 字段;预览端用 CSS 层叠或多 `<video>` 显示;导出端 `overlay=x:y` + `scale=`;边界:opacity=0 / 完全覆盖 / 越界 |
| **M10** 收尾 | ⏳ 未开始 | — | — | 本文件 `git mv` 至 `archive/multitrack.md`;主索引 [../milestones.md](../milestones.md) 中"进行中"挪到"已归档";`README.md` / `CLAUDE.md` 关键目录更新;[../roadmap.md](../roadmap.md) §3 非目标修订(把"多轨叠加 / PiP"移除)+ §4 已发布版本加 0.6.0 行;[../tabs/editor/product.md §1.2](../tabs/editor/product.md) 措辞同步;版本号 bump 至 **0.6.0**([../../web/package.json](../../web/package.json) / [../../internal/version/version.go](../../internal/version/version.go) / [../../cmd/desktop/wails.json](../../cmd/desktop/wails.json)) |

## M2 必须早做决定的设计岔路

下列不锁不准开 M5,否则 M5+ 反复返工:

- **存储隔离**:单视频用 `~/.easy-ffmpeg/projects/`,多轨建议另起 `~/.easy-ffmpeg/multitrack/` 还是共用目录加 `kind` 字段?
- **预览策略**:多 source 同步播放是大坑。评估:(a) 单 `<video>` 在轨道边界换 `src`,只支持 concat 不支持叠加;(b) 多 `<video>` 同步播放靠 `requestVideoFrameCallback` 对齐;(c) Canvas + `drawImage` 合成,精度高但帧率风险。建议 v1 走 (a)+(b),叠加场景预览近似即可,导出始终精确
- **共享层包名**:后端 `editor/common/` vs 独立 `media/`;前端 `components/timeline-shared/` vs 其他
