# 多轨剪辑器 — `feature-v0.5.0/multitrack-editor` — 当前 M 的待办

> 对应 milestones 文件:[../milestones/feature-v0.5.0_multitrack-editor.md](../milestones/feature-v0.5.0_multitrack-editor.md)
> 当前 M:**M3 后端共享层抽取**(M1 PRD ✅ + M2 技术设计 ✅ 已完成)
>
> **进度**(2026-04-30):代码部分全部完成 + 测试双绿。剩 **API 字节级运行时核对** + **单视频零回归手测**(用户侧)→ 通过后 commit + milestone 行打 ✅ + 本文件清空。

## 任务清单

### 抽取边界确认

- [x] 通读 [editor/domain/project.go](../../editor/domain/project.go) / [timeline.go](../../editor/domain/timeline.go) / [export.go](../../editor/domain/export.go),按 [program.md §2.1.1](../tabs/multitrack/program.md) 表逐条标记"共享 / 单视频专属"
- [x] 通读 [editor/ports/](../../editor/ports/),确认 `Clock` / `JobRunner` / `PathResolver` 三个接口直接共享,`ProjectRepository` / `VideoProber` 留单视频(多轨另起 `MediaProber`)

### 创建 `editor/common/` 包

- [x] `editor/common/domain/clip.go`:`Clip` 结构 + `Duration()` / `ProgramEnd()` + `ValidateClips(clips, label, sourceDuration)`(纯 clip 不变量;`sourceDuration=0` 跳过 source 范围检查,多轨用)+ `TrackDuration` + `EarliestProgramStart` + `SnapEpsilon` + `ErrClipNotFound`
- [x] `editor/common/domain/segments.go`:`SegmentPlan` 结构 + `PlanSegments(clips, totalDur)` 切段 + gap 填充
- [x] `editor/common/domain/filter.go`:`BuildVideoTrackFilter(clips, srcLabel, outLabel, totalDur, w, h, fr)` + `BuildAudioTrackFilter(clips, srcLabel, outLabel, preLabel, volume, totalDur)` + `FormatFloat` + `AudioFormatExpr` 常量
- [x] `editor/common/domain/timeline.go`:`Split` / `DeleteClip` / `Reorder` / `TrimLeft` / `TrimRight` / `SetProgramStart` / `ClipAtProgramTime`(`CarveRange` 单视频当前未用,M3 不抽 — M7 多轨范围操作时再加)
- [x] `editor/common/domain/codec.go`:`NormalizeVideoCodec` / `NormalizeAudioCodec`(M3 范围限定为现有 normalize 行为;`ValidateVideoCodec`/`ValidateAudioCodec` 当前单视频未用,推迟到首次需要时再加,避免 M3 引入新行为)
- [x] `editor/common/domain/export.go`:`ExportSettings` 结构 + `ValidateExportSettings`(OutputDir/OutputName/Format 必填)
- [x] `editor/common/ports/clock.go`:`Clock` 接口
- [x] `editor/common/ports/runner.go`:`JobRunner` 接口
- [x] `editor/common/ports/paths.go`:`PathResolver` 接口

### 共享层测试

- [x] `editor/common/domain/*_test.go`:从 `editor/domain/*_test.go` 提取数据无关测例(`Split` / `TrimLeft` / `Reorder` 等)+ 补充 `Clip` / `TrackDuration` / `ValidateClips` / `PlanSegments` / `BuildVideoTrackFilter` / `BuildAudioTrackFilter` / `FormatFloat` / 自定义 label 多轨场景 / codec normalize / `ValidateExportSettings`
- [x] `go test ./editor/common/...` 全绿

### 单视频侧迁移

- [x] `editor/domain/project.go`:保持 `Project` / `Source` / `Migrate()` / `NewProject` / `Validate()` 不变;内部改用 `commondomain.TrackDuration` / `commondomain.ValidateClips`;`Clip` / `ExportSettings` 改为 type alias
- [x] `editor/domain/timeline.go`:`type Clip = commondomain.Clip` + `var Split = commondomain.Split` 等 7 个函数 re-export + `var ErrClipNotFound = commondomain.ErrClipNotFound`(同时新增 `ClipAtProgramTime` 公开导出名,原 `clipAtProgramTime` 私有版本退场)
- [x] `editor/domain/export.go`:`BuildExportArgs` 改为内部调 `commondomain.BuildVideoTrackFilter("[0:v]","[v]",...)` + `commondomain.BuildAudioTrackFilter("[0:a]","[a]","[a_pre]",...)`,filter graph 字符串字节级不变(`export_test.go` 现有断言全绿)
- [x] `editor/ports/`:删除 `clock.go` / `runner.go` / `paths.go`;`editor/ports/` 收窄为 `repository.go` + `prober.go`
- [x] `editor/api/dto.go` 与 `editor/api/handlers_*.go`:`handlers_export.go` / `handlers_projects.go` / `routes.go` 切到 `commonports.{Clock,JobRunner,PathResolver}` 引用;`handlers_probe.go` / `handlers_source.go` / `dto.go` 只用单视频专属 ports,无需改;响应 JSON 结构不变
- [x] `server/editor_wiring.go`:adapter 是 duck-type 实现,无需改接口引用;同步注释从 `ports.JobRunner` → `commonports.JobRunner` / `ports.PathResolver` → `commonports.PathResolver`
- [x] `editor/module.go`:`Deps` 字段类型从 `ports.{Clock,JobRunner,PathResolver}` 切到 `commonports.*`

### 验收

- [x] `go test ./...` 全绿
- [x] `CGO_ENABLED=0 go test ./...` 全绿
- [x] `go build ./...` 通过
- [ ] **API 字节级不变**运行时核对(用户侧):
  - [ ] 启动 Web 版,前端打开历史工程
  - [ ] `curl http://127.0.0.1:<port>/api/editor/projects` diff 抽取前后(用 git stash / 备份的二进制)
  - [ ] `curl POST /api/editor/probe` 同理
  - [ ] `curl POST /api/editor/export` dryRun 模式 diff 命令字符串
  > 注:代码层论据已成立(`Clip` / `ExportSettings` 是 Go 类型别名 → JSON 完全透明;filter graph 字符串等值断言全绿;handler 逻辑未改),仅缺运行时确认。
- [ ] 单视频零回归手测(`EditorView.vue` 视觉与交互):打开历史工程 → 分割 → 范围选区 → 删除 → 撤销 → 改音量浮窗 → 自动保存(改名等 1.5s 看 mtime)→ 导出 dryRun → 命令预览 → 真实导出 → 取消 → 覆盖确认。**全程视觉无差异**
- [ ] commit message 描述清晰,包含"共享层包名锁定 / 抽取了哪些函数 / 单视频侧用 type alias 保 API 不变"

### 文档同步(M3 完成时一并更新)

- [x] [editor/program.md](../tabs/editor/program.md) 顶部追加 v0.6.0 抽取注脚 + 目录结构 / `Deps` / `ProjectHandlers` 代码示例同步到 `commonports`
- [ ] 在本 milestones 文件 M3 行标 ✅ + 填 commit hash + 完成日期(等运行时验收 + commit 后)
- [ ] 本 todo 文件**整段清空**(只留模板注释,等 M4 启动再填)

## 阻塞 / 待澄清

- (无)

## 完工标准

参见 milestones 文件 M3 行的"交付内容":
> 把 `editor/domain/` 中通用部分按 [program.md §2.1](../tabs/multitrack/program.md) 提到 `editor/common/domain/`;`editor/ports/{clock,runner,paths}.go` 提到 `editor/common/ports/`;单视频 `editor/` 改为引用共享层(type alias / re-export 保持 API 表面不变);`go test ./...` + `CGO_ENABLED=0 go test ./...` 双绿;`/api/editor/*` 响应字节级不变;ABI 完成评审基线录屏

**触发硬性条件**:全局不变量清单(milestones 文件顶部"全局不变量")每条都过。
