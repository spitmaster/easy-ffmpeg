# 多轨剪辑器 — `feature-v0.5.0/multitrack-editor` — 当前 M 的待办

> 对应 milestones 文件:[../milestones/feature-v0.5.0_multitrack-editor.md](../milestones/feature-v0.5.0_multitrack-editor.md)
> 当前 M:**M8 导出 v1:overlay + amix**(M1–M7 ✅,M7 commit `0d75dff`)
>
> **目标**(对照 milestones M8 行):多轨工程跑通真实 ffmpeg 导出 — 每条视频轨独立 concat,N 条视频轨链式 `overlay` 合成 `[V]`;每条音频轨独立 concat + 轨级 volume,N 条音频轨 `amix` 混合 + 全局 volume 出 `[A]`;dryRun / 命令预览 / 覆盖确认 / 取消 / 导出期阻断编辑全流程 — 全部走 M4 共享 ExportDialog / ExportSidebar / useJobPanel,后端走 M3 共享 JobRunner(全局单实例,与单视频导出 / 转换 / 音频处理互斥)。
> 设计参考:[../tabs/multitrack/program.md §5](../tabs/multitrack/program.md)(filter graph 装配规则与 §5.2 边界)+ [§5.3](../tabs/multitrack/program.md)(测试矩阵)+ [../tabs/multitrack/product.md §3.7](../tabs/multitrack/product.md)(导出 UX 沿用单视频)。
> 不在本 M 范围:PiP / 位置 / 缩放 / 不透明度(v2);视频轨 hidden / 音频轨 muted 实际生效(占位字段,M9+);转场 / 调色 / 关键帧;归档(M9 单独做)。

## 关键设计决定(M8 内必须落定)

- **filter graph 装配文件位置**:`multitrack/domain/export.go` + `multitrack/domain/filter.go`(多源版本的轨级 filter 构造,与 `editor/common/domain/filter.go` 平行,不污染单视频侧)。`multitrack/domain/export_test.go` 用表驱动覆盖 §5.3 矩阵。
- **input 顺序**:遍历 `Sources` slice 顺序,只把**至少被一个 clip 引用**的 source 加 `-i`,建立 `sourceID → input index` 映射;未被引用的 source 不进 ffmpeg 命令(不浪费 IO)。
- **canvas 尺寸**:`canvasW = max(W)`,`canvasH = max(H)`,`canvasFr = max(FrameRate)`,只在**视频轨用到的 source 集合**中取最大;音频源不参与。video 轨为空时 canvas 维度无意义(走纯音频路径)。
- **视频段统一格式**:每个段(clip 或 gap)在进入 concat 前都跑 `scale=W:H:force_original_aspect_ratio=decrease,pad=W:H:(W-iw)/2:(H-ih)/2:black,setsar=1,format=yuv420p`,保证 concat 看到的输入同质。gap 直接 `color=c=black:s=WxH:r=fr,format=yuv420p`(无需 scale/pad,生成时已是 canvas 维度)。
- **每条视频轨 → `[Vk]`**:trim/concat + 自动 trailing black pad 到 `programDur`(与单视频规则一致)。视频轨开头不允许留空(沿用单视频规则,errror 文案带轨号)。
- **多视频轨叠加**:N=1 时 `[V1]` 直接重命名为 `[V]`(实现:不发出 overlay,把第一条轨的 outLabel 设为 `[V]`);N≥2 时链式 `[V1][V2]overlay=0:0[V12]; [V12][V3]overlay=0:0[V123]; …`,最终 `→ [V]`。轨号小=底层,大=顶层(对照 product.md §3.5.1)。
- **每条音频轨 → `[Aj]`**:trim/concat + 轨级 `volume`(与 `common.BuildAudioTrackFilter` 同样的 useVolume 阈值 `volume>0 && (volume<0.999 || volume>1.001)`);unity 时不发 volume filter,保证字节稳定。
- **多音频轨混合**:N=1 时 `[A1]` 直接重命名为 `[A]`(unity)或 `[A1]volume=g[A]`(非 unity);N≥2 时 `[A1][A2]…amix=inputs=N:duration=longest:dropout_transition=0[A_pre]; [A_pre]volume=g[A]`,全局 volume unity 时省 volume filter,直接 `[A_pre]` 重命名为 `[A]`。
- **全空保护**:无视频轨且无音频轨 / 全部轨都没 clip → `errors.New("project has no clips")`。
- **video-only / audio-only**:有 video 无 audio → 不出 `[A]` chain,只 `-map [V]`、不 `-c:a`;反之亦然。和单视频一致。
- **导出期编辑阻断**:`useTimelinePlayback.isLocked` 改读 `store.exportLocked`;clip 拖拽 / 范围选区 / 工具条按钮在 `exportLocked` 时不响应(与 `EditorView` 现有锁逻辑对称,通过条件 prop 即可)。
- **撤销栈快照**:导出**不**进栈(纯只读操作);导出 settings 改名等仍走原有 dirty + autosave。
- **API 形状**:`POST /api/multitrack/export` body 同单视频(`{projectId, export?, overwrite, dryRun}`);返回 `{ok, command, outputPath}` 或 `{existing:true, path}` 409;`POST /api/multitrack/export/cancel` 沿用 JobRunner.Cancel。前端使用 SSE 共享通道(`/api/convert/stream`,JobRunner 全局单实例)。
- **JobRunner 独占**:M5 已在 wiring 把 multitrack 接到同一个 `s.jobs` JobManager;M8 不动 wiring,直接用。
- **手测清单顺序**:先简单(单视频单 clip 等价单视频导出)→ 双轨叠加 → 多源 → 边界 → 单视频 Tab 零回归。

## 任务清单

### A. 后端

#### A.1 `multitrack/domain/filter.go`(新文件)

- [x] `BuildMultitrackVideoTrackFilter(clips, sourceInputIdx, outLabel, totalDur, canvasW, canvasH, canvasFr, labelPrefix) ([]string, error)`:多源版本,每个 clip 段跑 `[<idx>:v]trim=...,setpts=PTS-STARTPTS,scale=W:H:force_original_aspect_ratio=decrease,pad=W:H:(W-iw)/2:(H-ih)/2:black,setsar=1,format=yuv420p[<prefix>i]`;gap 跑 `color=c=black:s=WxH:r=fr:d=...,format=yuv420p[<prefix>i]`;最终 `[refs]concat=n=N:v=1:a=0[outLabel]`。labelPrefix 用 `v0_`/`v1_`/... 防止跨轨标签冲突
- [x] `BuildMultitrackAudioTrackFilter(clips, sourceInputIdx, outLabel, preLabel, volume, totalDur, labelPrefix) ([]string, error)`:多源版本;clip 段 `[<idx>:a]atrim=...,asetpts=PTS-STARTPTS,aformat=...[<prefix>i]`;gap 段 `anullsrc=r=48000:cl=stereo:d=...,aformat=...[<prefix>i]`;concat → 可选 volume(unity 时省略)
- [x] 单测:覆盖跨源段、gap 填充、trailing pad、resolution scale+pad、unity vs 非 unity volume、labelPrefix 隔离 — 都集成到 `export_test.go` 通过 `BuildExportArgs` 端到端覆盖,不需要单独 filter_test.go

#### A.2 `multitrack/domain/export.go`(新文件)

- [x] `BuildExportArgs(*Project) ([]string, string, error)` — 全部子项落地:
  - [x] 校验:`p == nil` / `ValidateExportSettings` / 全空保护
  - [x] 视频轨开头留空逐条校验,error 文案带 `videoTracks[i]`
  - [x] 收集 referenced sources,顺序保留,建 `sourceID → input index` 映射
  - [x] canvas:从 video 轨涉及到的 sources 取 max(W) / max(H) / max(FrameRate)
  - [x] programDur = max(track durations across video + audio)
  - [x] 视频部分:每条调 `BuildMultitrackVideoTrackFilter` → N=1 直接 `[V]`,N≥2 链式 overlay
  - [x] 音频部分:每条调 `BuildMultitrackAudioTrackFilter` → N=1 直接合并 + 可选全局 volume,N≥2 amix + 可选全局 volume
  - [x] argv 装配 + `common.NormalizeVideoCodec/NormalizeAudioCodec` 兜底
  - [x] outPath = `filepath.Join(OutputDir, OutputName + "." + Format)`

#### A.3 `multitrack/domain/export_test.go`(新文件)

§5.3 测试矩阵全覆盖:

- [x] 单 video 轨 + 单 clip + 单 audio 轨 + 单 clip(等价单视频)
- [x] 双 video 轨各一 clip 等长 → overlay
- [x] 双 video 轨长度不等 → 短轨 black pad
- [x] 视频轨开头留空 → error 含 `videoTracks[`
- [x] 三 video 轨链式 overlay z-order
- [x] 单 audio 轨 + 全局 volume(unity / 非 unity 双分支)
- [x] 双 audio 轨 amix + 各轨独立 volume(全局 unity / 非 unity 双分支)
- [x] 跨源 clip 序列([0:v]trim + [1:v]trim 都出现)
- [x] 多源分辨率不一致 → scale + pad
- [x] 全空 → error
- [x] 仅视频轨 / 仅音频轨各自不出对方的 map+codec
- [x] outPath = OutputDir + OutputName + "." + Format
- [x] 未引用 source 不进 `-i`
- [x] 中段 gap → black + silence + concat n=3
- [x] labelPrefix 隔离

#### A.4 `multitrack/api/handlers_export.go`(新文件)

- [x] `ExportHandlers` 镜像 editor/api,字段 repo / runner / paths / mu / lastCommand
- [x] `start(w, r)`:全套 flow(decode / repo.Get / 覆盖 settings / mkdir / BuildExportArgs / dryRun 分支 / 覆盖 409 / runner.Start)
- [x] `cancel(w, r)`:method check + runner.Cancel()

#### A.5 `multitrack/api/dto.go` 扩展

- [x] `exportRequest` struct

#### A.6 `multitrack/api/routes.go` 路由

- [x] Router 加 `export *ExportHandlers`
- [x] `Register` 挂 `/export` 与 `/export/cancel`

#### A.7 验证

- [x] `go test ./multitrack/...` 全绿(2026-05-01)
- [x] `CGO_ENABLED=0 go test ./...` 全绿(2026-05-01)
- [x] `go build ./...` 通过(2026-05-01)

### B. 前端

#### B.1 `web/src/api/multitrack.ts` 扩展

- [x] `MultitrackExportBody` / `MultitrackExportStartResponse` 接口(命名以 Multitrack 前缀,避免与 editor.ts 重名)
- [x] `exportPreview(body)` 走 dryRun
- [x] `startExport(body)` 用 postJsonRaw
- [x] `cancelExport()`

#### B.2 `stores/multitrack.ts` 扩展

- [x] `exportLocked: ref<boolean>(false)`,`loadProject` / `closeProject` 重置
- [x] 暴露在 store 返回对象中,view 直接 `store.exportLocked = true/false`

#### B.3 `MultitrackView.vue` 接入导出

- [x] 顶栏「导出」按钮(在工具按钮之间,`disabled` 通过 `exportDisabled` 计算)
- [x] `ExportDialog` + `ExportSidebar` 引入,状态 `exportOpen` / `exportSidebarOpen` / `useJobPanel`
- [x] `exportDefaults` 从 `store.project.export` 取,fallback 到 `dirs.outputDir + project.name`
- [x] `pickOutputDir` 复用 `modals.showPicker({mode:'dir'})` + `dirs.saveOutput`
- [x] `onExportSubmit`:全空校验 + 视频轨开头留空前端预检 + `flushSave` + `exportPreview` + `modals.showCommand` + sidebar 切换 + `exportLocked=true` + `previewRef.pause()` + `job.startJob` + `sendStart(overwrite)` 处理 409
- [x] `closeExportSidebar`:running 时 confirm + `cancelExport`,关 sidebar + 释放 lock
- [x] `useTimelinePlayback.isLocked: () => store.exportLocked`
- [x] track / ruler / drop handlers 在 `exportLocked` 时早 return
- [x] 顶栏「+ 视频轨」「+ 音频轨」「关闭工程」按钮加 `:disabled="store.exportLocked"`,以及每条轨道左侧 × 删除按钮同步禁用
- [x] `watch(job.running)`:false 时同步释放 `store.exportLocked`,保证 ffmpeg 异常退出也能解锁

#### B.4 验证

- [x] `cd web && npx vue-tsc --noEmit` 全绿(2026-05-01)
- [x] `cd web && npm run build` 全绿(2026-05-01)

### C. 验收

- [ ] **手测清单**(M8 终态,用户侧):
  - [ ] 单视频源 1 video 轨 1 clip + 1 audio 轨 1 clip → 导出 mp4 能播放,时长正确
  - [ ] 双视频轨叠加(同源 / 不同源)→ 顶层覆盖 + 短轨黑屏 pad 正确
  - [ ] 双音频轨混合 → 听感两轨叠加,各轨 volume 与全局 volume 都生效
  - [ ] 跨源 clip(同一视频轨上来自两个 source 的 clip)→ 切换无瑕疵
  - [ ] 多源分辨率不一致(720p + 1080p)→ 720p 居中黑边 pad,不变形
  - [ ] 视频轨开头留空 → dryRun 失败,弹错误信息(后端文案)
  - [ ] 全空 → 导出按钮 disabled
  - [ ] 仅视频(无 audio 轨) / 仅音频(无 video 轨)→ 各自能跑通
  - [ ] dryRun 命令预览 → 用户能看到完整 ffmpeg 命令
  - [ ] 覆盖确认:目标文件已存在 → 弹 modals.showOverwrite
  - [ ] 取消导出:导出中点 sidebar 取消 → ffmpeg 终止,文件可能残留(行为同单视频)
  - [ ] 导出期间 timeline 编辑被阻断:点 clip / 拖拽 / split 按 S / Delete / Ctrl+Z 都无响应
  - [ ] 自动保存:导出 settings 改后 1.5s 自动保存,关 reopen 还在
  - [ ] **单视频 Tab 视觉与功能零回归**(切到单视频 Tab,跑一遍 §2.2.4 手测清单)
  - [ ] **JobRunner 互斥**:多轨导出运行中 → 切到单视频 Tab 点导出 → 提示已有 job 在运行(409 / 后端拒绝)
- [ ] 收尾 commit:milestone M8 行 ✅ + commit hash + 完成日期 + 本 todo 整段清空

## 阻塞 / 待澄清

- (无)实现过程中发现新问题再补到这里

## 完工标准

参见 milestones 文件 M8 行的"交付内容":
> `multitrack/domain/export.go` 按 [program.md §5](../tabs/multitrack/program.md) 装配 filter graph:每条视频轨 trim/concat → 链式 overlay 出 `[V]`;每条音频轨 trim/concat + volume → amix → 全局 volume → `[A]`;多 source 分辨率 scale+pad;短轨 black/silence pad;dryRun / 命令预览 / 真实导出 / 取消 / 覆盖确认走 M4 共享对话框;`multitrack/domain/export_test.go` 覆盖 §5.3 测试矩阵全条

**触发硬性条件**:全局不变量清单(milestones 文件顶部"全局不变量")每条都过。
