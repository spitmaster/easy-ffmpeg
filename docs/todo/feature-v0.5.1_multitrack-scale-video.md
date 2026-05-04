# 多轨剪辑器画布与变换(v0.5.1)— `feature-v0.5.1/multitrack-scale-video` — 当前 M 的待办

> 对应 milestones 文件:[../milestones/feature-v0.5.1_multitrack-scale-video.md](../milestones/feature-v0.5.1_multitrack-scale-video.md)
> 当前 M:**M5 前端 clip 变换 UI**(M1–M4 ✅,待启动 M5)
> 设计参考:[../tabs/multitrack/program.md §12.4.2 / §12.4.4 / §12.5](../tabs/multitrack/program.md)

<!-- M5 启动时按 program.md §12.4.2 / §12.4.4 / §12.5 拆出可勾选清单。提示:
  - TransformOverlay.vue:8 手柄 + 中心拖动 + Shift 锁纵横比 + Alt 中心缩放 + 像素 ↔ canvas 坐标换算
  - MultitrackInspector.vue:画布段(只读引用 M4 dialog)+ 选中 clip 段 + 数字输入 + 重置按钮 + 与 ExportSidebar 互斥
  - stores/multitrack.ts:加 previewClipTransform(草稿,不入栈)+ commitClipTransform(提交,入栈+dirty)
  - MultitrackView.vue:接通选中 clip → 变换框可见
  - 键盘箭头微调(选中 clip 时 ←/→/↑/↓ 1px,Shift+ 10px,Ctrl+0 重置)
  - useMultitrackPreview / object-fit:object-fill 的预览仅显示顶层视频轨,变换框是 DOM overlay
-->

## 任务清单

- [ ] _M5 启动时填充_

## 阻塞 / 待澄清

- 无

## 完工标准

引用 [milestones/feature-v0.5.1_multitrack-scale-video.md](../milestones/feature-v0.5.1_multitrack-scale-video.md) M5 行的"交付内容"。
