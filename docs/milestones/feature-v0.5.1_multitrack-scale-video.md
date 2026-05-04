# 多轨剪辑器画布与变换(v0.5.1)— 里程碑日志

> **对应分支**:`feature-v0.5.1/multitrack-scale-video`(分支驱动文件名:`feature-v0.5.1_multitrack-scale-video.md`)
>
> **目标**:在 v0.5.0 多轨基线上加**工程级画布**(自定义分辨率 / 帧率)与**每个 video clip 的变换**(画布上的 (X, Y, W, H)),把导出端从"上层全遮下层"切换为**真合成 overlay 链**(下层在上层未覆盖区域可见 → 实现 PIP / 双机位 / 角标轨 等)。
>
> **范围与设计**:[../tabs/multitrack/product.md §12](../tabs/multitrack/product.md)(PRD 增量)+ [../tabs/multitrack/program.md §12](../tabs/multitrack/program.md)(技术设计增量)。基线 §1–§11 保留作为 v0.5.0 描述,§12 是 v0.5.1 增量。
>
> **当前状态**:M1 / M2 / M3 ✅(2026-05-04);M4–M6 ⏳ 待启动。

## 全局不变量(每个 M 落盘前必跑)

违反任何一条该 M 视为未完成:

- `go test ./...` + `CGO_ENABLED=0 go test ./...` 双绿(共享层不引入 cgo)
- 后端零分支(`server/` / `editor/` / `editor/common/` / `multitrack/` 不出现 `if wails {}` / build tag)
- 前端宿主无感(`web/` 不引入 Wails 原生 binding)
- **单视频剪辑 Tab 视觉与交互零回归**(每 M 终态手测:打开历史工程 → 分割 → 范围选区 → 删除 → 撤销 → 改音量 → 自动保存 → 导出 dryRun → 真实导出 → 取消 → 覆盖确认)
- **多轨 v0.5.0 旧工程视觉零回归**:打开 schemaVersion=1 的旧工程,导出视觉结果与 v0.5.0 相同(允许字节差异源于 Migrate 注入默认值,但帧像素一致)
- JobRunner 仍全局单实例(多轨导出与转换/单视频导出互斥)
- `web/dist/` 由 `npm run build` 构建,产物 import 路径不脏

## Phase A — 文档阶段(无新功能,纯设计 + 评审)

| 里程碑 | 状态 | 完成日期 | Commit | 交付内容 |
|--------|------|---------|--------|---------|
| **M1** PRD 增量 | ✅ 完成 | 2026-05-04 | _本次提交_ | [../tabs/multitrack/product.md §12](../tabs/multitrack/product.md):动机 / 目标 / 非目标(opacity / 旋转 / 关键帧仍留 v2/v3)/ 数据模型变更(Canvas + Transform)/ UI 变更(顶栏画布按钮、预览框变换框、Inspector)/ 边界与默认值 / 与单视频对照表 / 快捷键 / 验收标准。基线 §1.2 与 §11.2 内对 PiP 的"留 v2"标注就地划线 + 注释 "v0.5.1 已提前" |
| **M2** 技术设计增量 | ✅ 完成 | 2026-05-04 | _本次提交_ | [../tabs/multitrack/program.md §12](../tabs/multitrack/program.md):设计目标(真合成 + 零回归 + schema 演进)/ 数据模型 Go 类型 + Migrate v1→v2 + Validate 规则 / 导出 filter graph 真合成版(base 画布 + 平铺 overlay 链 + setpts PTS 平移 + enable gating + yuva420p alpha + eof_action=pass + fps 统一)/ 测试矩阵(含 v0.5.0 兼容回归)/ 前端预览端画布盒 + TransformOverlay 组件 + 画布对话框 + Inspector / 状态机草稿 vs 提交分离 / API 契约自动跟 schema / 共享层影响评估(`editor/common/` 完全不动)/ 风险表(setpts 边界、yuva 损耗、eof_action 兼容、出界 UX) |

### M2 锁定的设计岔路(M3+ 不再返工)

- **真合成路径**:base + 全 video clip 平铺 overlay 链(N=1 仍走统一路径,简化测试矩阵;若回归基线要求字节相同,M3 评估 N=1 全画布 fast path,但默认走统一路径)
- **Transform 字段位置**:挂在 `multitrack/domain.Clip` 上,**不进 `editor/common/domain.Clip`**(单视频不需要画布概念)
- **Canvas 字段位置**:`Project` 顶层,与 `Sources` / `Tracks` 平级
- **像素粒度**:Canvas / Transform 全部整数像素(避免浮点累积误差);v2 再考虑百分比 / 浮点
- **alpha 路径**:segment 端 `format=yuva420p`,base 端 `format=yuv420p`,overlay 默认 alpha-aware,最终编码无 alpha(主流 mp4 不存)
- **PTS 平移**:`setpts=PTS-STARTPTS+programStart/TB`(不只是 `-STARTPTS`,**这是关键正确性差异**);叠加 `enable='between(t,start,end)'` 双保险;叠加 `eof_action=pass` 防止段尾"卡最后一帧"
- **帧率统一**:segment 端 `fps=canvasFR`,避免下游帧对齐异常
- **预览端不真合成**:仍走 v0.5.0 "顶层激活源"近似;变换框是 DOM overlay,不是真预览;真预览留 v2 多 `<video>` 同步方案
- **草稿 vs 提交**:拖手柄期间 `previewClipTransform`(不入栈、不 dirty);松开 `commitClipTransform`(入栈 + dirty + autosave)
- **撤销栈范围**:Canvas 改动入栈;Transform 改动入栈;`useUndoStack.snapshot` 加 canvas 字段
- **opacity / 旋转**:不做;留 v2
- **关键帧动画**:不做;留 v3
- **clip 内裁切(crop)**:不做;Transform 描述的是"clip 在画布上的位置与显示尺寸",源帧整帧按 (W, H) 缩放
- **出界 clip**:允许;UI 在 clip 上画"⚠ 不可见"角标;不强制自动 clamp
- **改画布致 clip 出界**:模态二次确认,用户决定继续 / 取消;不自动 clamp 数值

## Phase B — 落地阶段(后端先,前端跟,逐 M 可独立 ship)

| 里程碑 | 状态 | 完成日期 | Commit | 交付内容 |
|--------|------|---------|--------|---------|
| **M3** 后端数据模型 + filter 重写 | ✅ 完成 | 2026-05-04 | _本次提交_ | `multitrack/domain/project.go` 加 `Canvas` 类型 + Project.Canvas 字段 + Validate 规则(W/H ≥ 16,FR ∈ (0, 240],Transform W/H > 0);`multitrack/domain/clip.go` 加 `Transform` 类型 + Clip.Transform 字段;`Migrate` v1→v2(Canvas 默认 = max sources;Transform 默认 = 全画布;通过 `deriveDefaultCanvas` 抽出复用);`multitrack/domain/filter.go` 重写为 `BuildVideoSegment`(单 clip → 单 segment 字符串,trim+setpts shift+scale+setsar+fps+yuva420p);`multitrack/domain/export.go` 切换为 base 画布 + 平铺 overlay 链(z-order = trackIndex 升序 + programStart 升序;每 overlay 带 `enable='between(t,start,end)':eof_action=pass`);`export_test.go` §12.3.4 全矩阵 + v0.5.0 兼容回归(PIP / z-order / OOB 全 / OOB 部分 / 跨源不同 transform / FR 应用 / PTS 平移 / base duration / no-pad / no-black-gap / 旧工程 Migrate);`project_test.go` 加 Migrate v1→v2 + 幂等 + Canvas/Transform Validate;音频路径未动;前端 `api/multitrack.ts` 加 `MultitrackCanvas` + `MultitrackTransform` 类型,`MultitrackClip.transform` 必填,`MultitrackProject.canvas` 必填(透传,UI 不暴露);`MultitrackView.makeClip` 默认 transform = 全画布;`go test ./...` + `CGO_ENABLED=0 go test ./...` + `go build ./...` + `vue-tsc --noEmit` + `vite build` 全绿;PIP dryRun 人工核对通过(`color=...black:s=1920x1080:r=30:d=10[base];[0:v]...→seg_0;[1:v]...→seg_1;[base][seg_0]overlay=0:0:enable='between(t,0,10)':eof_action=pass[v_0];[v_0][seg_1]overlay=1440:720:enable='between(t,0,10)':eof_action=pass[V]`) |
| **M4** 前端工程画布 UI | ⏳ 待启动 | — | — | `web/src/components/multitrack/CanvasSettingsDialog.vue`(三数字输入 + 预设按钮 + 出界二次确认);`MultitrackTopBar.vue` 加"画布: WxH FRfps ▾"按钮(`v-if="hasProject"`);`MultitrackPreview.vue` 容器改造为画布盒(`aspectRatio` 等比缩放,`object-fit: fill` 顶层激活源拉伸到画布盒);新建工程默认 canvas `1920×1080@30`;打开旧工程 Migrate 后端透传 canvas;`stores/multitrack.ts` 加 `setCanvas(canvas)` 入栈 + dirty + autosave;改画布二次确认逻辑(出界 clip 列表);用户手测清单(新建/旧工程/改画布/撤销/重做/autosave 往返/单视频零回归 / 多轨导出仍正常) |
| **M5** 前端 clip 变换 UI | ⏳ 待启动 | — | — | `web/src/components/multitrack/TransformOverlay.vue`(8 手柄 + 中心拖动 + Shift 锁纵横比 + Alt 中心缩放 + 像素 ↔ canvas 坐标换算);`MultitrackInspector.vue`(画布段 + 选中 clip 段 + 数字输入 + 重置按钮 + 与 ExportSidebar 互斥);`stores/multitrack.ts` 加 `previewClipTransform`(草稿,不入栈)+ `commitClipTransform`(提交,入栈+dirty);`MultitrackView.vue` 接通选中 clip → 变换框可见;键盘箭头微调(选中 clip 时 ←/→/↑/↓ 1px,Shift+ 10px,Ctrl+0 重置);`useUndoStack.snapshot` 加 canvas 字段;用户手测:新建工程 → 拖手柄 PIP → 数字框联动 → 撤销 → autosave → 导出真合成可见;v0.5.0 旧工程零回归;单视频零回归 |
| **M6** 收尾 + 归档 | ⏳ 待启动 | — | — | 用户手测全过(v0.5.0 旧工程零回归 + 新建 PIP 导出比对 + 改画布二次确认 + 撤销重做 + autosave + 单视频 Tab 零回归);`web/dist/` 重建;版本号 bump 至 **v0.5.1**(`web/package.json` / `internal/version/version.go` / `cmd/desktop/wails.json`);本文件 `git mv` 至 `archive/feature-v0.5.1_multitrack-scale-video.md`;主索引 [../milestones.md](../milestones.md) 中"进行中"挪到"已归档";`docs/todo/feature-v0.5.1_multitrack-scale-video.md` 删除;[../roadmap.md](../roadmap.md) §1 当前版本 → v0.5.1 + §3 多轨进阶非目标行更新("PiP / position / scale 已 v0.5.1 落地;opacity / 旋转 / 关键帧 / 转场 / 调色 仍留 v0.7.x");[../tabs/multitrack/product.md](../tabs/multitrack/product.md) §12 状态行从"进行中"改"已发布";三绿(`go test ./...` + `CGO_ENABLED=0 go test ./...` + `vue-tsc --noEmit && vite build`) |

## 后续(v2+ 推迟项,不在本里程碑表)

- **opacity**:clip 加 `opacity: 0..1`,filter 端 `colorchannelmixer=aa=opacity` 或 overlay 端混合参数
- **旋转**:`rotate` 滤镜
- **关键帧动画**:transform / opacity / rotate 随时间变化(`overlay=x='if(...)':y='if(...)'` 或 sendcmd)
- **真合成预览**:多 `<video>` CSS 层叠 + `requestVideoFrameCallback` 同步;OffscreenCanvas / WebCodecs 帧精确预览
- **clip 内 crop**:Transform 之前先 `crop=W:H:X:Y` 取源内一块再 scale

这些进 v0.7.x 多轨进阶里程碑(单独立项),不挤进 v0.5.1。
