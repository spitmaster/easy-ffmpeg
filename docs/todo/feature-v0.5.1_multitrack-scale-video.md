# 多轨剪辑器画布与变换(v0.5.1)— `feature-v0.5.1/multitrack-scale-video` — 当前 M 的待办

> 对应 milestones 文件:[../milestones/feature-v0.5.1_multitrack-scale-video.md](../milestones/feature-v0.5.1_multitrack-scale-video.md)
> 当前 M:**M3 后端数据模型 + filter 重写**(M1 PRD ✅ + M2 技术设计 ✅,待启动 M3)
> 设计参考:[../tabs/multitrack/program.md §12.2 / §12.3](../tabs/multitrack/program.md)

## 任务清单

### 数据模型(`multitrack/domain/`)

- [ ] `project.go`:加 `Canvas { Width, Height int; FrameRate float64 }` 类型
- [ ] `project.go`:`Project` 加 `Canvas Canvas` 字段(JSON tag `canvas`)
- [ ] `project.go`:`SchemaVersion` 从 `1` 升到 `2`
- [ ] `project.go`:`Migrate()` 加 v1→v2 兜底:`Canvas` 零值 → `deriveDefaultCanvas(p)`(把 `export.go:103-135` 的算法抽出);`Canvas.FrameRate ≤ 0` → `30`
- [ ] `project.go`:`Validate()` 加 Canvas 越界检查(W/H ≥ 16,FR ∈ (0, 240])
- [ ] `clip.go`:加 `Transform { X, Y, W, H int }` 类型
- [ ] `clip.go`:`Clip` 加 `Transform Transform` 字段(JSON tag `transform`)
- [ ] `project.go`:`Migrate()` 视频轨 clip 的 Transform 零值 → 全画布 `(0, 0, Canvas.W, Canvas.H)`
- [ ] `project.go`:`Validate()` 视频轨 clip 加 `Transform.W > 0 && H > 0` 检查;出界**不报错**
- [ ] `project.go`:`NewProject` 默认 `Canvas = {1920, 1080, 30}`
- [ ] `project_test.go`:加 Migrate v1→v2 测例(无 canvas/transform → 自动填默认)
- [ ] `project_test.go`:加 Validate Canvas 越界测例
- [ ] `project_test.go`:加 Validate Transform W/H ≤ 0 测例

### Filter graph 重写(`multitrack/domain/filter.go` + `export.go`)

- [ ] 新函数 `BuildVideoSegment(clip, sourceInputIdx, canvas, label) (string, error)`:单 clip 的 filter 字符串(`[i:v]trim=...,setpts=PTS-STARTPTS+programStart/TB,scale=W:H,setsar=1,fps=FR,format=yuva420p[seg_k]`)
- [ ] `export.go` 视频路径重写:起 base 画布(`color=c=black:s=CWxCH:r=FR:d=programDur,format=yuv420p[base]`)→ 收集所有视频 clip 按 (trackIdx 升序, programStart 升序) → 平铺生成 segment + overlay 链
- [ ] overlay 节点参数:`overlay=x=X:y=Y:enable='between(t,p_start,p_end)':eof_action=pass`
- [ ] 终端 label `[V]`,中间用 `[v_0] [v_1] ...`
- [ ] N=0 video clip(纯音频导出)→ 不进 video chain,沿用 v0.5.0 规则
- [ ] N=1 + 全画布 transform → 评估 fast path(走 v0.5.0 单轨直出);若不实现,统一走 base + 1 overlay,文档注明
- [ ] 视频轨开头留空检查保留(沿用 v0.5.0)
- [ ] 音频路径**不动**(`BuildMultitrackAudioTrackFilter` + amix + 全局 volume)
- [ ] 旧 `BuildMultitrackVideoTrackFilter`(每轨 concat 全长)删除或标 deprecated
- [ ] `deriveDefaultCanvas(p *Project) (w, h int, fr float64)`:从 `export.go` 抽出独立函数(Migrate 与 export 都用)

### 测试矩阵(`multitrack/domain/export_test.go`)

按 [program.md §12.3.4](../tabs/multitrack/program.md) 全矩阵:

- [ ] 单视频轨单 clip + transform 全画布 → 与 v0.5.0 视觉等价(filter 字符串可不同,断言关键节点存在)
- [ ] 单视频轨双 clip 不重叠 + 各自不同 transform → 两个 segment + 两个 overlay 节点
- [ ] 双视频轨各一 clip 时间相同 + 上层小窗(右下角 PIP) → 下层 segment + 上层 segment + 顺序正确
- [ ] 双视频轨上层 transform 全画布 → 与 v0.5.0 视觉等价(下层不可见)
- [ ] 三视频轨 z-order(`vt[0] vt[1] vt[2]`)→ overlay 链顺序 = `seg_0 → seg_1 → seg_2`(轨号小=底)
- [ ] Clip transform 完全出画布 → segment 仍生成,filter 字符串包含
- [ ] Clip transform 部分出画布 → 不报错,filter 不裁
- [ ] Canvas FR ≠ 源 FR → segment 端 `fps=` 出现且值正确
- [ ] v0.5.0 工程(`schemaVersion=1`,无 canvas/transform)→ Migrate 后 export 不报错;canvas 默认 = max sources;transform 全画布
- [ ] Canvas W/H/FR 越界 → Validate 报错
- [ ] Transform W/H ≤ 0 → Validate 报错
- [ ] 跨源 + 不同分辨率 + 不同 transform → 各 segment scale 到自己的 transform.W × transform.H,不再 pad
- [ ] PTS 平移正确性:`programStart=2.5` 的 clip → segment filter 含 `setpts=PTS-STARTPTS+2.5/TB`(或等价表达)
- [ ] base 画布时长 = `programDur`,且 `r=` 帧率正确
- [ ] 视频轨开头留空 → 沿用 v0.5.0 错误信息

### 前端 schema 透传(无 UI)

- [ ] `web/src/api/multitrack.ts`:`MultitrackProject` 加 `canvas: { width, height, frameRate }` 类型
- [ ] `web/src/api/multitrack.ts`:`MultitrackClip` 加 `transform: { x, y, w, h }` 类型
- [ ] `stores/multitrack.ts`:打开 / 保存往返不丢字段(只透传,不暴露 UI 修改)
- [ ] `vue-tsc --noEmit` + `vite build` 全绿

### 完工门槛

- [ ] `go test ./...` 全绿
- [ ] `CGO_ENABLED=0 go test ./...` 全绿
- [ ] `go build ./...` 全绿
- [ ] `cd web && npm run build` 全绿
- [ ] dryRun 命令字符串人工核对:新建工程 + 双轨 PIP → 命令含 base + 两 segment + 两 overlay,顺序正确
- [ ] 单视频 Tab 零回归手测清单全过(打开旧工程 / 分割 / 范围 / 删除 / 撤销 / 音量 / autosave / 导出 dryRun / 真实导出 / 取消 / 覆盖确认)
- [ ] 多轨 v0.5.0 旧工程导出视觉零回归(打开 v0.5.0 commit `6d739a5` 时存的工程文件,导出后逐帧对比 — 允许 base + overlay 引入的极小数值差异,主体画面应一致)

## 阻塞 / 待澄清

- N=1 + 全画布 transform 的 fast path 是否要保留?保留 = v0.5.0 字节相同的回归;不保留 = 测试矩阵更简单。**M3 起手时决定**,影响 ~30 行 filter 装配代码

## 完工标准

引用 [milestones/feature-v0.5.1_multitrack-scale-video.md](../milestones/feature-v0.5.1_multitrack-scale-video.md) M3 行的"交付内容":

> `multitrack/domain/project.go` 加 `Canvas` 类型 + Project.Canvas 字段 + Validate 规则;`multitrack/domain/clip.go` 加 `Transform` 类型 + Clip.Transform 字段;`Migrate` v1→v2(Canvas 默认 = max sources;Transform 默认 = 全画布);`multitrack/domain/filter.go` 重写;`multitrack/domain/export.go` 切换为 base 画布 + 平铺 overlay 链;`export_test.go` §12.3.4 全矩阵 + v0.5.0 兼容回归;音频路径不动;前端 schema 类型同步(透传,不暴露 UI);`go test ./...` + `CGO_ENABLED=0 go test ./...` + `vue-tsc` + `npm run build` 全绿;dryRun 命令字符串人工核对
