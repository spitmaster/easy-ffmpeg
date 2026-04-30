# 多轨剪辑器 — `feature-v0.5.0/multitrack-editor` — 当前 M 的待办

> 对应 milestones 文件:[../milestones/feature-v0.5.0_multitrack-editor.md](../milestones/feature-v0.5.0_multitrack-editor.md)
> 当前 M:**M6 多源导入 + 多轨渲染**(M1 ✅ M2 ✅ M3 ✅ M4 ✅ M5 ✅ 已完成于 2026-04-30)
>
> **目标**(对照 milestones M6 行):跑通"导入素材 → 拖入时间轴 → 自动建轨 → N 条轨道渲染 → 预览看到顶层视频轨"。
> 不在本 M 范围:剪辑操作(split / delete / trim / 跨轨拖动)留 M7;导出留 M8。
> 设计参考:[../tabs/multitrack/program.md §3 §4 §6](../tabs/multitrack/program.md);[../tabs/multitrack/product.md §4 §5 §6](../tabs/multitrack/product.md)。

## 关键设计决定(M6 内必须落定)

- **`MultitrackClip` 必须带 `SourceID`**。M5 阶段 `multitrack/domain/Clip` 是 `common.Clip` 的 type alias,**不带** `SourceID`。M6 第一步把它替换成正经 struct(嵌入 `common.Clip` + 加 `SourceID string` 字段),否则预览 / 导出找不到 source。前端 `web/src/api/multitrack.ts` 的 `Clip` 类型也对应改。
- **共享层不动**:`editor/common/domain/Clip` 与 `web/src/types/timeline.ts` 的 `Clip` 形状保持不变(单视频不需要 sourceId)。`TimelineClip.vue` / `TimelineTrackRow.vue` 接收的还是共享 `Clip`,sourceId 由多轨视图层在 emit 之前用 closure / props 配套传递。这是 [program.md §2.2.3](../tabs/multitrack/program.md) 的契约。
- **预览方案**:v1 单 `<video>` 切源 + 每条音频轨独立 `<audio>` + WebAudio gain([program.md §6.1](../tabs/multitrack/program.md))。多视频轨叠加 v1 **预览只显示顶层**(轨号最大、命中 clip 的那条),UI 加灰底提示。
- **轨道顺序**:轨号小→底层,大→顶层。前端列表与 z-order 保持一致。
- **拖入空时间轴自动建轨规则**:视频 source → 视频轨 +(若 hasAudio)音频轨;纯音频 source → 音频轨。Clip 的 `programStart` 取 0,`sourceStart=0 / sourceEnd=Source.Duration`。
- **多 Source 分辨率不一致**导出端在 M8 处理(scale + pad);M6 预览端不强求,只放当前 source 自然尺寸进 `<video>`。

## 任务清单

### A. 后端

#### A.1 `multitrack/domain/`

- [x] `clip.go`(新文件):新建 multitrack 专属 `Clip` struct,**嵌入** `common.Clip` 并加 `SourceID` 字段(JSON tag `sourceId`),靠 promoted field 让 `clip.SourceStart` / `clip.ProgramStart` 现有访问继续可用
- [x] `project.go`:`VideoTrack.Clips` / `AudioTrack.Clips` 类型由 `[]common.Clip` 改为 `[]Clip`(本包的扩展型);所有现有引用同步更新
- [x] `Validate()`:对每个 clip 校验 `SourceID != ""` 且能在 `Project.Sources` 中找到;视频轨上的 clip 必须指向 `Kind=video` 的 source,音频轨可指向任一 source
- [x] `project_test.go`:补一个测例覆盖"clip 的 SourceID 不存在于 Sources" → expect 错误
- [x] `sources.go` 新文件:`AddSource(p, src) (*Project, error)` / `RemoveSource(p, sid) (*Project, error)` 纯函数;`RemoveSource` 须先确认无 clip 引用
- [x] `AddVideoTrack(p) / AddAudioTrack(p)` 返回新轨 id
- [x] `domain` 测试覆盖以上纯函数(含"删除被引用的 source 应失败")

#### A.2 `multitrack/api/`

- [x] `dto.go`:加 `importSourcesRequest { Paths []string }` + `importSourcesResponse { Sources []Source, Project *Project, Errors [...] }`
- [x] `handlers_sources.go`(新文件):
  - `POST /api/multitrack/projects/:id/sources` body `{paths: [...]}`,逐个 ffprobe(走 `MediaProber`),写回项目 + 自动 `Save`,返回新增的 sources(部分失败仍返回 200,失败 path 进 `errors[]`)
  - `DELETE /api/multitrack/projects/:id/sources/:sid`,先查无引用再删
- [x] `handlers_source_serve.go`(新文件):`GET /api/multitrack/source?projectId=&sourceId=`,沿 `editor/api/handlers_source.go` 的 `http.ServeContent` 写法,根据项目 + sourceId 解析路径 → 流式服务(支持 Range)
- [x] `routes.go`:挂上面 3 条路由;`/projects/` 通过 `handleProjectsTree` 分发给 `proj.getUpdateDelete` 或 `sources.dispatch`

#### A.3 验证

- [x] `go test ./multitrack/...` 全绿
- [x] `CGO_ENABLED=0 go test ./...` 全绿
- [x] `go build ./...` 通过

### B. 前端

#### B.1 类型

- [x] `web/src/api/multitrack.ts`:`MultitrackClip = TimelineClip & { sourceId: string }`,`MultitrackVideoTrack.clips` / `MultitrackAudioTrack.clips` 改类型;补 `importSources` / `removeSource` / `sourceUrl(projectId, sourceId)` 三个 API + `MultitrackImportResponse` 类型

#### B.2 Store

- [x] `web/src/stores/multitrack.ts`:加 `importSources(paths) / removeSource(sid)` action;加 `addVideoTrack() / addAudioTrack() / appendClip(kind, trackId, clip)`;dirty 标记 + autosave 接通(已有);加 `topVideoActive(playhead)` / `audioActive(track, playhead)` getter,返回 `{ track, clip, source, srcTime } | null`
- [x] M6 暂不引入 selection / range / undo — 这些 M7 再加;加 `playhead` + `playing` + `pxPerSecond` + `programDuration` computed

#### B.3 组件

- [x] `web/src/components/multitrack/MultitrackLibrary.vue`(新):左侧 240px 栏,顶部"导入"按钮 + 文件列表;空状态文案;Ctrl+L 折叠 M7 再加(本 M 不阻断)
- [x] `web/src/components/multitrack/MultitrackLibraryItem.vue`(新):单 source 缩略卡,显示 kind 图标 + 文件名 + 时长 + 分辨率(视频);拖出 = 设 `dataTransfer` 携带 `application/x-easy-ffmpeg-source` JSON `{ sourceId }`(双击试听暂留 M7)
- [x] `web/src/components/multitrack/MultitrackPreview.vue`(新):上半屏,1 `<video ref=videoRef muted>` + N `<audio>`(v-for over audioTracks);`useMultitrackPreview` 接管同步;多视频轨叠加时右上角"预览仅显示顶层视频轨"提示
- [ ] `MultitrackTopBar.vue`:暂不抽,内联在 `MultitrackView.vue`(顶栏复杂度可控)

#### B.4 Composable

- [x] `web/src/composables/useMultitrackPreview.ts`(新):
  - 单 `<video>` 切源:`evaluate()` 调用 `topVideoActive`,变更则 `v.src = sourceUrl(...)` + `currentTime = srcTime`(`loadedmetadata` 后再次校正)
  - 多 `<audio>` 同步:逐轨 `audioActive(track, t)`,设 src + currentTime + play/pause;muted 轨道直接暂停
  - 每条音频轨独立 WebAudio GainNode(轨级 volume × 全局 audioVolume),回退到 `audio.volume` 上限 1.0
  - GapClock 复用 `composables/timeline/useGapClock.ts`(全空 / 全 gap 时驱动 playhead)

#### B.5 视图

- [x] `web/src/views/MultitrackView.vue`:
  - 替换 M5 中央占位,接入 `MultitrackLibrary` 左侧 + `MultitrackPreview` 上 + `TimelineRuler / TimelineTrackRow` v-for 多轨 + `PlayBar` 底
  - 时间轴空白处接 `dragover/drop`,解析 `dataTransfer` → 调 `addVideoTrack/addAudioTrack` + `appendClip` 自动建轨
  - 已有轨道接 `@dragover/@drop.stop`,落到该轨 + playhead 时间(M6 简化:精确落点 M7)
  - 多视频轨叠加场景:`MultitrackPreview` 自带提示文案,本 M5 视图不重复
- [x] 工具条:`+视频轨` / `+音频轨` / `关闭工程` 按钮内联在顶栏

### C. 验收

- [x] `cd web && npx vue-tsc --noEmit` + `cd web && npm run build` 全绿
- [x] `go build ./...` + `go test ./...` + `CGO_ENABLED=0 go test ./...` 全绿
- [ ] **手测清单**(M6 终态,用户侧):
  - [ ] 新建空多轨工程 → 进入工程
  - [ ] 通过素材库导入 1 个有视频有音频的 mp4 → 看到 source 卡
  - [ ] 把视频 source 拖到时间轴空白 → 自动建出 1 条视频轨 + 1 条音频轨,各 1 个 clip
  - [ ] 再导入 1 个纯音频 mp3 → 拖到时间轴空白 → 多出 1 条音频轨
  - [ ] 再导入 1 个不同分辨率的 mp4 → 拖到时间轴空白 → 多出 1 条视频轨 + 1 条音频轨,clip 落在 playhead 时间(或 0)
  - [ ] 顶部预览能看到**顶层**视频轨命中的 source 帧;切源时无明显闪屏(允许 ~100ms 切换间隙黑屏)
  - [ ] 三条音频轨混音可同时听到(以播放头为基准),全局 audioVolume 滑块影响所有轨,每条轨级 volume 独立工作
  - [ ] 多视频轨叠加预览只显示顶层 + UI 提示可见
  - [ ] 关闭工程 → 重开 → 时间轴恢复(JSON 保留 sources / clips)
  - [ ] 切回单视频 Tab → 视觉与功能零回归(打开历史工程 / 撤销 / 导出能跑通)
  - [ ] **两个 Tab 切换 store 不互相污染**
- [ ] 收尾 commit:milestone M6 行 ✅ + commit hash + 完成日期 + 本 todo 整段清空

## 阻塞 / 待澄清

- (无)

## 完工标准

参见 milestones 文件 M6 行的"交付内容":
> 后端:`POST /api/multitrack/projects/:id/sources` 多文件 ffprobe 写入(`mediaProberAdapter` 支持纯音频);前端:`MultitrackLibrary.vue` + `MultitrackLibraryItem.vue`(导入 / 双击试听 / 拖出);**拖入时间轴空白处自动建轨**(视频→V+A,音频→A);N 条轨道 v-for 渲染 + 垂直滚动;source 色条;预览走 M2 选定方案(单 `<video>` 切源 + 每条音频轨独立 `<audio>` + WebAudio gain);**多视频轨叠加预览仅顶层** + UI 提示

**触发硬性条件**:全局不变量清单(milestones 文件顶部"全局不变量")每条都过。
