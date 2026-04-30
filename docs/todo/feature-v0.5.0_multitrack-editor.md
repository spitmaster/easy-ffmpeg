# 多轨剪辑器 — `feature-v0.5.0/multitrack-editor` — 当前 M 的待办

> 对应 milestones 文件:[../milestones/feature-v0.5.0_multitrack-editor.md](../milestones/feature-v0.5.0_multitrack-editor.md)
> 当前 M:**M3 后端共享层抽取**(M1 PRD ✅ + M2 技术设计 ✅ 已完成)

## 任务清单

### 抽取边界确认

- [ ] 通读 [editor/domain/project.go](../../editor/domain/project.go) / [timeline.go](../../editor/domain/timeline.go) / [export.go](../../editor/domain/export.go),按 [program.md §2.1.1](../tabs/multitrack/program.md) 表逐条标记"共享 / 单视频专属"
- [ ] 通读 [editor/ports/](../../editor/ports/),确认 `Clock` / `JobRunner` / `PathResolver` 三个接口直接共享,`ProjectRepository` / `VideoProber` 留单视频(多轨另起 `MediaProber`)

### 创建 `editor/common/` 包

- [ ] `editor/common/domain/clip.go`:`Clip` 结构 + `Duration()` / `ProgramEnd()` + `Validate(clips []Clip) []error`(纯 clip 不变量,不含 Project 上下文)
- [ ] `editor/common/domain/segments.go`:`planSegments(clips, totalDur)` 切段 + gap 填充算法
- [ ] `editor/common/domain/filter.go`:`buildVideoTrackFilter(clips, srcLabel, totalDur, w, h)` + `buildAudioTrackFilter(clips, srcLabel, volume, totalDur)` 单轨 filter graph 构造
- [ ] `editor/common/domain/timeline.go`:`Split` / `DeleteClip` / `Reorder` / `TrimLeft` / `TrimRight` / `CarveRange` 纯函数
- [ ] `editor/common/domain/codec.go`:视频 / 音频 codec 枚举验证(`ValidateVideoCodec` / `ValidateAudioCodec`)
- [ ] `editor/common/domain/export.go`:`ExportSettings` 结构 + 通用校验(`OutputDir/OutputName/Format` 必填)
- [ ] `editor/common/ports/clock.go`:`Clock` 接口
- [ ] `editor/common/ports/runner.go`:`JobRunner` 接口
- [ ] `editor/common/ports/paths.go`:`PathResolver` 接口

### 共享层测试

- [ ] `editor/common/domain/*_test.go`:从 `editor/domain/*_test.go` 中提取数据无关的测例(`Split` / `TrimLeft` / `Reorder` 等),确保共享层独立 100% 通过
- [ ] `go test ./editor/common/...` 全绿

### 单视频侧迁移

- [ ] `editor/domain/project.go`:保持 `Project` / `Source` / `Migrate()` 不变(单视频专属)
- [ ] `editor/domain/timeline.go`:type alias `type Clip = common.Clip` + 函数 re-export(`Split = common.Split` 等)
- [ ] `editor/domain/export.go`:`BuildExportArgs(*Project)` 内部改用 `common.buildVideoTrackFilter` / `common.buildAudioTrackFilter`,装配逻辑保持单视频专属
- [ ] `editor/ports/`:删 `clock.go` / `runner.go` / `paths.go`,改为引用 `common.ports.*`(其余文件不动)
- [ ] `editor/api/dto.go` 与 `editor/api/handlers_*.go`:确认所有 import 路径正确,响应 JSON 结构不变
- [ ] `server/editor_wiring.go`:`Deps` 接收的接口类型从 `editor/ports/*` 切到 `editor/common/ports/*`

### 验收

- [ ] `go test ./...` 全绿
- [ ] `CGO_ENABLED=0 go test ./...` 全绿
- [ ] `go build ./...` 通过
- [ ] **API 字节级不变**核对:
  - [ ] 启动 Web 版,前端打开历史工程
  - [ ] `curl http://127.0.0.1:<port>/api/editor/projects` diff 抽取前后(用 git stash / 备份的二进制)
  - [ ] `curl POST /api/editor/probe` 同理
  - [ ] `curl POST /api/editor/export` dryRun 模式 diff 命令字符串
- [ ] 单视频零回归手测(`EditorView.vue` 视觉与交互):打开历史工程 → 分割 → 范围选区 → 删除 → 撤销 → 改音量浮窗 → 自动保存(改名等 1.5s 看 mtime)→ 导出 dryRun → 命令预览 → 真实导出 → 取消 → 覆盖确认。**全程视觉无差异**
- [ ] commit message 描述清晰,包含"共享层包名锁定 / 抽取了哪些函数 / 单视频侧用 type alias 保 API 不变"

### 文档同步(M3 完成时一并更新)

- [ ] 在本 milestones 文件 M3 行标 ✅ + 填 commit hash + 完成日期
- [ ] 本 todo 文件**整段清空**(只留模板注释,等 M4 启动再填)
- [ ] [editor/program.md](../tabs/editor/program.md) 注脚追加一句:"v0.6.x 起 `Clip` / 时间轴函数 / 单轨 filter 构造抽到 `editor/common/`,本文继续作为单视频专属语义的描述"

## 阻塞 / 待澄清

- (开工后随时记录)

## 完工标准

参见 milestones 文件 M3 行的"交付内容":
> 把 `editor/domain/` 中通用部分按 [program.md §2.1](../tabs/multitrack/program.md) 提到 `editor/common/domain/`;`editor/ports/{clock,runner,paths}.go` 提到 `editor/common/ports/`;单视频 `editor/` 改为引用共享层(type alias / re-export 保持 API 表面不变);`go test ./...` + `CGO_ENABLED=0 go test ./...` 双绿;`/api/editor/*` 响应字节级不变;abi 完成评审基线录屏

**触发硬性条件**:全局不变量清单(milestones 文件顶部"全局不变量")每条都过。
