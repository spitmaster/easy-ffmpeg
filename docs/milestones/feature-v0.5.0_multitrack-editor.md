# 多轨剪辑器(类 Premiere Pro)— 里程碑日志

> **对应分支**:`feature-v0.5.0/multitrack-editor`(分支驱动文件名:`feature-v0.5.0_multitrack-editor.md`)
> **目标**:在新 Tab 下提供类 Premiere Pro 的多轨剪辑器(自建工程 + 素材库 + 多视频/音频轨 + 跨轨拖动 + overlay/amix 导出),与单视频剪辑 Tab **共存**,共享底层(后端 `editor/common/`,前端 `components/timeline-shared/` + `composables/timeline/`)。
> **范围与设计**:[../tabs/multitrack/product.md](../tabs/multitrack/product.md)(PRD)+ [../tabs/multitrack/program.md](../tabs/multitrack/program.md)(技术设计)。
>
> **当前状态**:M1–M5 ✅(2026-04-30);M6 ✅(2026-05-01);**M7 代码完成 🚧 待用户手测**(后端 + 前端 + vue-tsc + vite build + `go test` 全绿,2026-05-01)。详见 `docs/todo/feature-v0.5.0_multitrack-editor.md`。

## 全局不变量(每个 M 落盘前必跑)

违反任何一条该 M 视为未完成:

- `go test ./...` + `CGO_ENABLED=0 go test ./...` 双绿(共享层不引入 cgo)
- 后端零分支(`server/` / `editor/` / `editor/common/` / `multitrack/` 不出现 `if wails {}` / build tag)
- 前端宿主无感(`web/` 不引入 Wails 原生 binding)
- **单视频剪辑 Tab 视觉与交互零回归**(每 M 终态手测:打开历史工程 → 分割 → 范围选区 → 删除 → 撤销 → 改音量 → 自动保存 → 导出 dryRun → 真实导出 → 取消 → 覆盖确认)
- JobRunner 仍全局单实例(多轨导出与转换/单视频导出互斥)
- `web/dist/` 由 `npm run build` 构建,产物 import 路径不脏(沿用 v0.5.x)

## Phase A — 准备阶段(无新功能,纯文档 + 重构)

| 里程碑 | 状态 | 完成日期 | Commit | 交付内容 |
|--------|------|---------|--------|---------|
| **M1** PRD | ✅ 完成 | 2026-04-30 | _本次提交_ | [../tabs/multitrack/product.md](../tabs/multitrack/product.md):目标/非目标、与单视频差异、核心概念(工程 / 素材 / 轨道 / Clip)、UI 低保真图(顶栏 / 素材库 / 预览 / 时间轴 / 工具条)、素材库交互、多轨时间轴交互(含跨轨拖动)、splitScope 升级、数据模型 schema、预览策略 v1/v2/v3 概念、导出 filter graph 概念、能力对照表、范围裁剪与未来演进 |
| **M2** 技术设计 | ✅ 完成 | 2026-04-30 | _本次提交_ | [../tabs/multitrack/program.md](../tabs/multitrack/program.md):**共享层抽取方案**(后端 `editor/common/domain` 边界、ports 共享、迁移策略 + 单视频零回归基线;前端 `components/timeline-shared/` + `composables/timeline/` 清单 + `web/src/types/timeline.ts` 类型契约 + props/emit driven 不直接 import store)、`multitrack/` 后端目录结构 + Go 数据模型、API 路由表、`MultitrackView` 草图、导出 filter graph 装配规则与边界、预览 v1 单 `<video>` 切源算法详解、状态机参数化共享 composable、主程序装配 wiring、测试矩阵、风险表 |

### M2 锁定的设计岔路(M3+ 不再返工)

- **存储隔离**:多轨独立 `~/.easy-ffmpeg/multitrack/`,与单视频 `~/.easy-ffmpeg/projects/` 完全分家(避免 schema 冲突 + 误操作)
- **预览策略**:v1 走"单 `<video>` 切源 + 多 `<audio>` 同步",多视频轨叠加时**预览仅显示顶层视频轨**,导出端始终精确(`overlay`)。多 `<video>` 同步留给 v2;Canvas/WebCodecs 留给 v3
- **共享层包名**:后端 `editor/common/{domain,ports}/`;前端 `components/timeline-shared/` + `composables/timeline/`(平铺,不再嵌深目录)
- **跨 store 类型契约**:`web/src/types/timeline.ts` 定义 `Clip` / `TrackData<C>`,单视频与多轨都吃这两个形状(单视频隐式 `sourceId="main"`)
- **JobRunner**:全局单实例不变;多轨与单视频导出 + 转换 + 音频处理互斥
- **多视频轨 z-order**:轨号小→底层,大→顶层;视频 source 分辨率不一致时 scale + pad 到主分辨率(主分辨率 = max W × max H)
- **音频轨级 volume**:`audioTracks[].volume` 0–2.0(独立于全局 `audioVolume`,多轨**新增**)
- **PiP / position / scale / opacity**:推迟到 v2(0.7.x),v1 视频叠加只到全屏 overlay

## Phase B — 落地阶段(多轨功能逐层加,每个 M 都能独立 ship)

| 里程碑 | 状态 | 完成日期 | Commit | 交付内容 |
|--------|------|---------|--------|---------|
| **M3** 后端共享层抽取 | ✅ 完成 | 2026-04-30 | `2e8660d`(代码)+ _本次收尾_ | 把 `editor/domain/` 中通用部分(`Clip` 基础结构、`PlanSegments` + gap 填充、`BuildVideoTrackFilter` / `BuildAudioTrackFilter`、`Split / Delete / Reorder / TrimLeft / TrimRight / SetProgramStart / ClipAtProgramTime`、codec normalize、`ValidateClips`)按 [program.md §2.1](../tabs/multitrack/program.md) 提到 `editor/common/domain/`;`editor/ports/{clock,runner,paths}.go` 提到 `editor/common/ports/`;单视频 `editor/` 改为引用共享层(type alias / 函数变量 re-export 保持 API 表面不变);`go test ./...` + `CGO_ENABLED=0 go test ./...` + `go build ./...` 三绿;API 字节级运行时核对 + 单视频零回归手测全过 |
| **M4** 前端共享层抽取 | ✅ 完成 | 2026-04-30 | `202d2c1` + `0cddefe`(bugfix) | 抽 `components/timeline-shared/`(TimelineRuler / TrackRow / Clip / Playhead / RangeSelection / PlayBar / ProjectsModal / ExportDialog / ExportSidebar / AudioVolumePopover)+ `composables/timeline/`(useTimelineZoom / Playback / RangeSelect / Drag / UndoStack / Autosave / GapClock / AudioGain);定义 `web/src/types/timeline.ts`(`Clip` 与 program.md §2.2.3 略有偏离:`sourceId` 留给多轨在自己的扩展类型上加,单视频侧不引入隐式 `"main"`);共享组件不直接 import store,全部 props/emit driven;`EditorView.vue` 重写为基于共享组件的薄壳;`npm run build` 通过;单视频零回归手测清单全过(用户手测后于 `0cddefe` 修复了一处预览 / EditorView 联动 bug) |
| **M5** 多轨工程骨架 | ✅ 完成 | 2026-04-30 | `76acf42` | 后端 `multitrack/` 包按 SOLID 分层(domain/ports/storage/api/module),`/api/multitrack/projects` 五个端点(list / create / get / put / delete),JSON 落 `~/.easy-ffmpeg/multitrack/`,自愈索引 + 异类 Kind 文件忽略;`mediaProberAdapter` 桥接到 `service.ProbeVideo`(支持纯音频);前端 `MultitrackView.vue` 骨架 + `stores/multitrack.ts`(共享 `useAutosave`)+ `api/multitrack.ts` + `/multitrack` 路由 + `TabNav` 加项;`go test ./...` + `CGO_ENABLED=0 go test ./...` + `vue-tsc` + `npm run build` 全绿;用户手测:多轨 Tab 打开无报错、新建工程基本正常、单视频 Tab 零回归 |
| **M6** 多源导入 + 多轨渲染 | ✅ 完成 | 2026-05-01 | _本次提交_ | 后端:`multitrack/domain/clip.go`(嵌入 `common.Clip` + `SourceID`)+ `sources.go`(`AddSource` / `RemoveSource` 拒在用 / `AddVideoTrack` / `AddAudioTrack` 纯函数)+ `Validate` 校验 SourceID 在 `Sources` 中且视频轨拒音频源;`api/handlers_sources.go`(`POST /:id/sources` 多文件 ffprobe + 部分失败 200/`errors[]`;`DELETE /:id/sources/:sid`)+ `handlers_source_serve.go`(`GET /source?projectId=&sourceId=` `http.ServeContent`,Range);`routes.go` `handleProjectsTree` 分发 `/projects/:id` 与 `/projects/:id/sources`。前端:`api/multitrack.ts`(`MultitrackClip = TimelineClip & { sourceId }` + `importSources / removeSource / sourceUrl`);`stores/multitrack.ts` 加 `playhead/playing/pxPerSecond/programDuration/sourcesById` + `importSources/removeSource/addVideoTrack/addAudioTrack/appendClip/topVideoActive/audioActive`;`components/multitrack/MultitrackLibrary.vue` + `MultitrackLibraryItem.vue`(导入 / 列表 / 拖出 `application/x-easy-ffmpeg-source`);`MultitrackPreview.vue`(单 `<video>` + N `<audio>`,叠加场景"预览仅顶层"提示);`composables/useMultitrackPreview.ts`(单 video 切源 + 每条 audio 独立 GainNode + 共享 `useGapClock`);`MultitrackView.vue` 接入素材库 / 预览 / N 轨 / PlayBar,拖入空白自动建轨(视频→V+A,音频→A),拖入轨道在 playhead 落 clip,顶栏内联 `+视频轨/+音频轨/关闭工程`。`go test ./...` + `CGO_ENABLED=0 go test ./...` + `vue-tsc` + `npm run build` 全绿;用户手测清单全过(单视频 Tab 零回归 + 多轨基本流程 + Tab 切换 store 不污染) |
| **M7** 多源剪辑操作 + 跨轨拖动 | 🚧 进行中 | — | — | split / delete / trim / 范围选区 / 重排 / 撤销重做在多轨模型上跑通(完全靠 M4 共享 composable + 多轨 store 实现);**跨轨拖动**(同类型轨之间;视频→音频禁止);splitScope 扩展到 `all/video/audio/track:<id>`;轨道删除二次确认;`multitrack/domain/timeline_test.go` 覆盖多轨场景;锁 `Ctrl+L` 折叠素材库(`+视频轨` / `+音频轨` 工具条按钮 M6 已落顶栏,M7 不重复) |
| **M8** 导出 v1:overlay + amix | ⏳ 未开始 | — | — | `multitrack/domain/export.go` 按 [program.md §5](../tabs/multitrack/program.md) 装配 filter graph:每条视频轨 trim/concat → 链式 overlay 出 `[V]`;每条音频轨 trim/concat + volume → amix → 全局 volume → `[A]`;多 source 分辨率 scale+pad;短轨 black/silence pad;dryRun / 命令预览 / 真实导出 / 取消 / 覆盖确认走 M4 共享对话框;`multitrack/domain/export_test.go` 覆盖 §5.3 测试矩阵全条 |
| **M9** 收尾 + 归档 | ⏳ 未开始 | — | — | 本文件 `git mv` 至 `archive/feature-v0.5.0_multitrack-editor.md`;主索引 [../milestones.md](../milestones.md) 中"进行中"挪到"已归档";`docs/todo/feature-v0.5.0_multitrack-editor.md` 删除;[../README.md](../README.md) `tabs/` 表加 multitrack 行 ✅;`../../README.md` / `../../CLAUDE.md` 关键目录补 `multitrack/` + `editor/common/`;[../roadmap.md](../roadmap.md) §4 加 0.6.0 行 + §3 非目标修订(把"多轨叠加 / PiP"措辞改为"PiP/位置/缩放/不透明度,留 v2");[../tabs/editor/product.md §1.2](../tabs/editor/product.md) 文案同步("多素材剪辑由多轨 Tab 提供");版本号 bump 至 **0.6.0**(`web/package.json` / `internal/version/version.go` / `cmd/desktop/wails.json`) |

## 后续(v2 推迟项,不在本里程碑表)

- **PiP / overlay 进阶**:clip 加 `position`/`scale`/`opacity`/`rotate`;预览端多 `<video>` 同步;导出端 `overlay=x:y` + `scale=W:H`
- **转场**(crossfade / fade)、关键帧动画、调色
- **WebCodecs 帧精确预览**

这些进 v0.6.x 里程碑后再开新分支。
