# 开发里程碑日志

> **本文档的唯一职责**:让接手者(无论是人还是新的 Claude session)在 30 秒内回答两个问题——"现在到哪一步了"、"下一步该做什么"。
>
> **它不是设计文档**:每个里程碑的范围/动机/验收清单见对应的设计文档。本文档只跟踪 **状态**(完成 / 进行中 / 未开始 / 阻塞)和 **指针**(commit、文档链接)。
>
> **维护规则**:每完成一个里程碑,在表格里把状态从 ⏳ 改为 ✅,补上 commit 哈希和日期。整个迁移项目结束后整段移到"已归档"区。

---

## 当前位置(Where We Are Now)

| 项 | 值 |
|---|---|
| 当前进行中的迁移 | **多轨剪辑器(类 Premiere Pro)新增** — Phase A 准备阶段 M1 待启动 |
| 当前版本 | **v0.5.1**(前端 Vue 化整体落地,多轨剪辑尚未启动开发) |
| 下一步 | **M1**:撰写多轨剪辑 PRD([docs/tabs/multitrack/product.md](tabs/multitrack/product.md)) |
| 主分支 | `v0.5.1`(开发分支,尚未合并到 `main`);多轨剪辑迭代过程中可能切到 `v0.6.x` 分支 |

**接手前必读**(按顺序):

1. 本文件全文 — 看清当前到第几个 M
2. [README.md](README.md) → [core/architecture.md](core/architecture.md) → [core/frontend.md](core/frontend.md) — 项目分层
3. 单视频剪辑器现状:[tabs/editor/product.md](tabs/editor/product.md) + [tabs/editor/program.md](tabs/editor/program.md) — 多轨是它的"专用 Tab"延伸,共享共用层
4. 多轨设计文档(M1/M2 完成后):[tabs/multitrack/product.md](tabs/multitrack/product.md) + [tabs/multitrack/program.md](tabs/multitrack/program.md)
5. [../CLAUDE.md](../CLAUDE.md) — 项目通用规约
6. `git log --oneline -10` — 看最近的提交,了解上一个 session 实际改了什么

**怎么验证当前代码是健康的**:

```bash
cd web && npm run build      # 类型检查 + Vite 构建,应输出 "✓ built in <1s"
cd .. && go build ./...      # Go 全量编译,应零输出
go test ./...                # 后端测试
```

**Phase A 期间的额外验收闸门**(每个 M 落盘前必跑):

- 单视频剪辑 Tab 视觉与交互**零回归**(手测:打开视频 → 分割 → 范围选区 → 删除 → 撤销 → 导出全链路)
- `CGO_ENABLED=0 go test ./...` 共享层不引入 cgo
- `/api/editor/*` 行为字节级不变(M3/M4 是重构,不是行为修改)

---

## 进行中:多轨剪辑器(v0.6.x,2026-04-30 → )

> 范围与设计:M1 后落到 [tabs/multitrack/product.md](tabs/multitrack/product.md);M2 后落到 [tabs/multitrack/program.md](tabs/multitrack/program.md)。
> 目标:在新 Tab 下提供类 Premiere Pro 的多轨剪辑器(多源导入 + 多视频/音频轨 + 视频叠加/PiP + 音频混流 + 多轨导出),与单视频剪辑 Tab **共存**,共享底层(后端 domain 抽出共用层、前端时间轴组件抽出共用层)。
> 全局不变量:`CGO_ENABLED=0` 仍可跨编 4 平台;后端零分支(共享层不出现宿主感知);单视频 Tab 自始至终零回归;JobRunner 仍全局单实例(多轨导出与转换/单视频导出互斥)。

### Phase A — 准备阶段(无新功能,纯文档 + 重构)

| 里程碑 | 状态 | 完成日期 | Commit | 交付内容 |
|--------|------|---------|--------|---------|
| **M1** PRD | ⏳ 待启动 | — | — | [tabs/multitrack/product.md](tabs/multitrack/product.md):目标/非目标、多源 + 多轨数据模型草图、UI 草图(在单视频基础上加哪些控件)、与单视频 Tab 的能力对比表、预览策略选型(单 `<video>` 切源 / 多 `<video>` 同步 / Canvas 合成,各方案权衡)、导出滤镜图概念、典型用户场景。明确 v1 范围(建议:多源 concat + 多视频轨叠加 + 音频 mix,**不**含转场/调色/关键帧) |
| **M2** 技术设计 | ⏳ 未开始 | — | — | [tabs/multitrack/program.md](tabs/multitrack/program.md):Project schema(`schemaVersion: 4` 或独立文件家族?)、`Track`/`Clip` 数据结构、后端共享层方案(包名 `editor/common/` vs 独立 `media/`)、前端共享组件清单与命名空间、API 路由(`/api/multitrack/*`)、JobRunner 共用策略确认、[roadmap.md](roadmap.md) §3 非目标与 `tabs/editor/product.md §1.2` 的修订计划;[README.md](README.md) 索引补 `tabs/multitrack/` |
| **M3** 后端共享层抽取 | ⏳ 未开始 | — | — | 把 `editor/domain/` 中通用部分(`Clip` 基础结构、`planSegments` 与 gap 填充、`buildXxxTrackFilter` 单轨 filter 构造、视频/音频 codec 枚举验证)提到 `editor/common/domain/`(M2 锁定包名);单视频 `editor/` 改为引用共享层;`editor/ports/` 中通用的 `JobRunner`/`PathResolver`/`Clock` 也提共享层;`go test ./...` 全绿;`/api/editor/*` 字节级不变 |
| **M4** 前端共享组件抽取 | ⏳ 未开始 | — | — | 把 `EditorTimeline.vue` / `EditorPlayBar.vue` / `EditorExportDialog.vue` / `EditorExportSidebar.vue` / `EditorProjectsModal.vue` / `useJobPanel.ts` / 时间轴标尺与 playhead / 范围选区逻辑抽到 `web/src/components/timeline-shared/` + `web/src/composables/timeline/`;`EditorView.vue` 重写为基于共享组件的薄壳;`npm run build` 通过;单视频 Tab 视觉与交互零回归 |

### Phase B — 落地阶段(多轨功能逐层加,每个 M 都能独立 ship)

| 里程碑 | 状态 | 完成日期 | Commit | 交付内容 |
|--------|------|---------|--------|---------|
| **M5** 多轨工程骨架 | ⏳ 未开始 | — | — | 后端:`multitrack/` 包按 SOLID 分层(domain / ports / storage / api / module),`POST /api/multitrack/projects` 创建空工程,JSON 落 `~/.easy-ffmpeg/multitrack/`(M2 锁定路径);前端:新增 Tab + `MultitrackView.vue` 空壳 + `stores/multitrack.ts` + `api/multitrack.ts`;TabNav 加项 |
| **M6** 多源导入 + 多轨渲染 | ⏳ 未开始 | — | — | 多个视频/音频文件加进同一工程作为 `Source[]`;时间轴显示 N 条视频轨 + M 条音频轨;clip 可按 sourceId 拖到任意轨道任意时间;预览走 M2 选定方案(默认建议:单 `<video>` 切源 + 多 `<video>` 同步混合) |
| **M7** 多源剪辑操作 | ⏳ 未开始 | — | — | split / delete / trim / 范围选区 / 重排 / 撤销重做在多轨模型上跑通(共享 `useEditorOps` 复用,只换数据模型层 store);`multitrack/domain/timeline_test.go` 覆盖跨多轨场景 |
| **M8** 导出 v1:concat + amix | ⏳ 未开始 | — | — | `multitrack/domain/export.go` 构造 filter_complex:每条视频轨独立 concat → 顶层 `overlay` 链(z-order 按轨号);每条音频轨独立 concat → `amix`;短轨 pad 黑屏/静音;导出对话框复用 M4 共享组件;关键测试落 `multitrack/domain/export_test.go` |
| **M9** 视频叠加(PiP / overlay) | ⏳ 未开始 | — | — | clip 增加 `position`/`scale`/`opacity` 字段;预览端用 CSS 层叠或多 `<video>` 显示;导出端 `overlay=x:y` + `scale=`;边界:opacity=0 / 完全覆盖 / 越界 |
| **M10** 收尾 | ⏳ 未开始 | — | — | 本节整段归档至"已归档";`README.md` / `CLAUDE.md` 关键目录更新;[docs/roadmap.md](roadmap.md) §3 非目标修订(把"多轨叠加 / PiP"移除)+ §4 已发布版本加 0.6.0 行;[tabs/editor/product.md §1.2](tabs/editor/product.md) 措辞同步;版本号 bump 至 **0.6.0**([web/package.json](../web/package.json) / [internal/version/version.go](../internal/version/version.go) / [cmd/desktop/wails.json](../cmd/desktop/wails.json)) |

### M2 必须早做决定的设计岔路

下列不锁不准开 M5,否则 M5+ 反复返工:

- **存储隔离**:单视频用 `~/.easy-ffmpeg/projects/`,多轨建议另起 `~/.easy-ffmpeg/multitrack/` 还是共用目录加 `kind` 字段?
- **预览策略**:多 source 同步播放是大坑。评估:(a) 单 `<video>` 在轨道边界换 `src`,只支持 concat 不支持叠加;(b) 多 `<video>` 同步播放靠 `requestVideoFrameCallback` 对齐;(c) Canvas + `drawImage` 合成,精度高但帧率风险。建议 v1 走 (a)+(b),叠加场景预览近似即可,导出始终精确
- **共享层包名**:后端 `editor/common/` vs 独立 `media/`;前端 `components/timeline-shared/` vs 其他

---

## 已归档(完成的迁移项目)

### 前端 Vue 化迁移(v0.5.x,2026-04-29 → 2026-04-30)

> 范围与设计:[core/frontend-vue-migration.md](core/frontend-vue-migration.md)
> 落地架构:[core/frontend.md](core/frontend.md)
> 目标:为后续多轨剪辑器(类 Premiere Pro)做技术储备。把零构建的原生 HTML+JS 前端迁到 Vue 3 + Vite + TS + Pinia + Tailwind。

| 里程碑 | 状态 | 完成日期 | Commit | 交付内容 |
|--------|------|---------|--------|---------|
| **M1** 脚手架就绪 | ✅ 完成 | 2026-04-29 | `e701de2` | `web/` 工程齐全;`web/embed.go` + `server/server.go` 改为 import `easy-ffmpeg/web`;`server/web/` 整体 git rm;build.sh / build.bat 加前端构建步骤;.gitignore 调整;空 Vue 壳能在 Web 版与桌面版跑通 |
| **M2** 顶栏 + 转换 Tab 迁移 | ✅ 完成 | 2026-04-29 | `ab5248b` | API 客户端层(8 个模块);Pinia stores(version / ffmpeg / dirs / modals);共享对话框(Picker / ConfirmCommand / ConfirmOverwrite / PrepareOverlay);`useJobPanel` composable;TopBar 完整功能;ConvertView 完整功能;版本号 → 0.5.0 |
| **M3** 音频 + 单视频剪辑 Tab | ✅ 完成 | 2026-04-29 | `f20d345` | `api/audio.ts` + `api/editor.ts`;`AudioView` 拆 3 子组件(Convert / Extract / Merge),沿用 `useJobPanel`;`EditorView` 拆 8 子组件(TopBar / PlayBar / Timeline / Toolbar / AudioVolume / ProjectsModal / ExportDialog / ExportSidebar)+ `useEditorPreview` 双 `<video>`/`<audio>` + 间隙时钟 + WebAudio 增益 + `useEditorOps` 分割/删除 + `editor` Pinia store(撤销栈、防抖自动保存)+ `utils/timeline.ts` + `utils/time.ts`;Canvas 子树未引入 — 当前 DOM 实现性能足够,留待瓶颈出现再做;版本号未动(留给 M4 整体升 0.5.1) |
| **M4** 清理 + 文档同步 | ✅ 完成 | 2026-04-30 | _本次提交_ | 重写 [core/frontend.md](core/frontend.md)(原 IIFE 描述 → Vue 工程 / API 客户端层 / Pinia / composable / SSE 总线);重写 [core/ui-system.md](core/ui-system.md)(原 CSS 变量描述 → Tailwind + tokens.css + utility 模式语言);[core/architecture.md](core/architecture.md) 目录结构与数据流措辞同步到 `web/`;[core/build.md](core/build.md) §2 加前端 npm 流水段;路线图 §5 加 v0.5.0–0.5.1 行(注:此版本路线图位于 `core/roadmap.md`,后于规划文档重组中迁至顶层 [roadmap.md](roadmap.md));[README.md](README.md) 当前状态行更新到 v0.5.1;[../README.md](../README.md) 全文重写(原 Fyne 时代描述);[../CLAUDE.md](../CLAUDE.md) 关键目录与不变量、设计文档索引同步;版本号 → 0.5.1([web/package.json](../web/package.json) / [internal/version/version.go](../internal/version/version.go) / [cmd/desktop/wails.json](../cmd/desktop/wails.json));milestones 整段归档至本节 |
