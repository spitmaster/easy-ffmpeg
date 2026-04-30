# 多轨剪辑器 — `feature-v0.5.0/multitrack-editor` — 当前 M 的待办

> 对应 milestones 文件:[../milestones/feature-v0.5.0_multitrack-editor.md](../milestones/feature-v0.5.0_multitrack-editor.md)
> 当前 M:**M4 前端共享层抽取**(M1 ✅ M2 ✅ M3 ✅ 已完成于 2026-04-30)
>
> **进度**(2026-04-30):代码部分全部完成 + 三阶段构建/测试全绿(`npx vue-tsc --noEmit` + `npm run build` + `go test ./...` + `CGO_ENABLED=0 go test ./...` + `go build ./...`)。代码 commit:`202d2c1 v0.5.0-M4`。剩 **单视频零回归手测**(用户侧)→ 通过后 commit + milestone 行打 ✅ + 本文件清空。

## 任务清单

### A. 类型契约 `web/src/types/timeline.ts`

- [x] `Clip { id, sourceStart, sourceEnd, programStart }`(镜像 Go `editor/common/domain/Clip`,sourceId 留给多轨在自己的 `MultitrackClip` 扩展加 — 与 program.md §2.2.3 略有偏离,理由见 commit `202d2c1`)
- [x] `TrackData<C extends Clip = Clip> { id, kind, clips, volume?, tone?, label? }`
- [x] `ClipSelection`、`RangeSelection`、`TrackTone`、`ExportSettings`、`ProjectsModalItem` 一并入档
- [x] `api/editor.ts` 的 `Clip` / `ExportSettings` 改为 type alias 复用,wire shape 字节级不变

### B. 共享 composables `web/src/composables/timeline/`

- [x] `useUndoStack.ts` — 通用撤销栈,参数化 `snapshot`/`apply`,`stores/editor.ts` 已切到该 composable,store API 表面(canUndo/canRedo/pushHistory/undo/redo)未变
- [x] `useAutosave.ts` — debounce 1.5s 自动保存,参数化 `isDirty`/`save`,`stores/editor.ts` 已切到该 composable
- [x] `useAudioGain.ts` — WebAudio MediaElementSource → GainNode pipeline,`useEditorPreview.ts` 已切
- [x] `useGapClock.ts` — rAF 间隙时钟,参数化 `shouldContinue`/`onTick`,`useEditorPreview.ts` 已切
- [x] `useTimelineZoom.ts` — Ctrl+滚轮缩放 + 滚轮平移 + applyFit,`EditorView.vue` 已用
- [x] `useTimelineRangeSelect.ts` — 右键拖出范围选区(含 50ms 最小阈值兜底),`EditorView.vue` 已用
- [x] `useTimelineDrag.ts` — clip 拖拽(trim 左/右 + reorder + snap),参数化 `getClips`/`setClips`/`sourceMaxFor`,`EditorView.vue` 已用
- [x] `useTimelinePlayback.ts` — 文档级键盘快捷键(Space / S / Del / ←→ / Ctrl+Z/Y / Esc),参数化所有动作 + isLocked,`EditorView.vue` 已用

### C. 共享 components `web/src/components/timeline-shared/`

- [x] `TimelineRuler.vue` — 刻度尺(zoom-aware step 选择),defineExpose `rootEl` 给 useTimelineRangeSelect 用
- [x] `TimelineTrackRow.vue` — 单条轨道,接收 `TrackData`,emit 详细 mousedown 携带 `{ ev, clipId?, handle? }`
- [x] `TimelineClip.vue` — 单 clip,tone-driven 配色(accent / success / danger;v0.6 多源时扩展)
- [x] `TimelinePlayhead.vue` — 单条垂直线,top/height 可配
- [x] `TimelineRangeSelection.vue` — 黄色覆盖,自动反转 start>end
- [x] `PlayBar.vue` — ⏮⏸▶⏭ + 时间码(props/emit driven)
- [x] `ProjectsModal.vue` — 工程列表,参数化 `list` / `remove` / 标题 / 空状态文案
- [x] `ExportDialog.vue` — 导出对话框,参数化 `defaults` / `pickDir`
- [x] `ExportSidebar.vue` — 导出期日志侧栏(JobLog 容器 + 取消按钮)
- [x] `AudioVolumePopover.vue` — 0–200% 音量浮窗,`v-model:modelValue`

### D. 单视频侧迁移

- [x] `stores/editor.ts`:撤销栈/自动保存切到 composable;状态字段(project / dirty / selection / splitScope / playhead / playing / pxPerSecond / rangeSelection)保持
- [x] `useEditorPreview.ts`:GapClock / AudioGain 切到共享 composable,~80 行精简,单 source 切片逻辑保留
- [x] `views/EditorView.vue` 重写为基于共享层的视图:吸收原 `EditorTimeline.vue` 的布局(左标签列 + 右滚动区 + 三档 playhead),通过 props/emit 接通共享原子
- [x] 删除单视频专属壳:`EditorTimeline.vue` 完全溶解;`EditorPlayBar.vue` / `EditorAudioVolume.vue` / `EditorProjectsModal.vue` / `EditorExportDialog.vue` / `EditorExportSidebar.vue` 已 git mv 到 `timeline-shared/` 同名(rename 检测保留历史血脉)
- [x] `EditorTopBar.vue` / `EditorToolbar.vue` 保留(单视频专属)

### E. 验收

- [x] `cd web && npx vue-tsc --noEmit` 全绿
- [x] `cd web && npm run build` 通过(产物 EditorView-*.js 43.78 kB / index-*.js 120.52 kB)
- [x] `go build ./...` 通过
- [x] `go test ./...` + `CGO_ENABLED=0 go test ./...` 全绿(共享层未引入 cgo)
- [ ] **单视频零回归手测**(用户侧,[program.md §2.2.4](../tabs/multitrack/program.md)):
  - [ ] 打开历史工程
  - [ ] 拖动 clip 重排(snap 到 0 / playhead / 邻 clip 边界)
  - [ ] 拖动 clip 左 / 右 端修剪
  - [ ] 在播放头分割(`S`)+ 范围选区分割
  - [ ] 删除选中 + 删除范围
  - [ ] 撤销 / 重做(Ctrl+Z / Ctrl+Shift+Z / Ctrl+Y)
  - [ ] 修改音量浮窗(0–200% 滑块 + WebAudio gain 生效)
  - [ ] 自动保存触发(改名 → 等 1.5s → 看 disk 上 mtime)
  - [ ] 导出 dryRun / 命令预览 / 真实导出 / 取消导出 / 覆盖确认
  - [ ] 导出期间侧栏占据右侧、阻断编辑
  - [ ] **全程视觉无差异**(与 M3 之前对比录屏 / 截图)
- [ ] 收尾 commit:milestone M4 行 ✅ + commit hash + 完成日期 + 本 todo 整段清空

## 阻塞 / 待澄清

- (无)

## 完工标准

参见 milestones 文件 M4 行的"交付内容":
> 抽 `components/timeline-shared/` + `composables/timeline/`;定义 `web/src/types/timeline.ts`;共享组件**不直接 import store**,全部 props/emit driven;`EditorView.vue` 重写为基于共享组件的薄壳;`npm run build` 通过;**单视频零回归手测清单**全过

**触发硬性条件**:全局不变量清单(milestones 文件顶部"全局不变量")每条都过。
