# 多轨剪辑器画布与变换(v0.5.1)— `feature-v0.5.1/multitrack-scale-video` — 当前 M 的待办

> 对应 milestones 文件:[../milestones/feature-v0.5.1_multitrack-scale-video.md](../milestones/feature-v0.5.1_multitrack-scale-video.md)
> 当前 M:**M4 前端工程画布 UI**(M1 PRD ✅ + M2 技术设计 ✅ + M3 后端数据模型 + filter 重写 ✅,待启动 M4)
> 设计参考:[../tabs/multitrack/program.md §12.4](../tabs/multitrack/program.md)

<!-- M4 启动时按 program.md §12.4 / §12.5 拆出可勾选清单。提示:
  - CanvasSettingsDialog.vue(三数字输入 + 预设按钮 + 出界二次确认)
  - MultitrackTopBar.vue 加"画布: WxH FRfps ▾"按钮(`v-if="hasProject"`)
  - MultitrackPreview.vue 容器改造为画布盒(aspectRatio 等比缩放)
  - 新建工程默认 canvas 1920×1080@30
  - 打开旧工程 Migrate 后端透传 canvas
  - stores/multitrack.ts 加 setCanvas(canvas) 入栈 + dirty + autosave
  - 改画布二次确认逻辑(出界 clip 列表)
  - useUndoStack.snapshot 加 canvas 字段
-->

## 任务清单

- [ ] _M4 启动时填充_

## 阻塞 / 待澄清

- (M3 起手时关于 N=1 fast path 的决策已落地:**统一走 base + overlay 路径**,不写 N=1 特例。理由:简化测试矩阵,性能损耗忽略不计)

## 完工标准

引用 [milestones/feature-v0.5.1_multitrack-scale-video.md](../milestones/feature-v0.5.1_multitrack-scale-video.md) M4 行的"交付内容"。
