# 多轨剪辑器 — `feature-v0.5.0/multitrack-editor` — 当前 M 的待办

> 对应 milestones 文件:[../milestones/feature-v0.5.0_multitrack-editor.md](../milestones/feature-v0.5.0_multitrack-editor.md)
> 当前 M:**M7 多源剪辑操作 + 跨轨拖动**(M1–M6 ✅ 已完成,M6 收尾日期 2026-05-01)
>
> **目标**(对照 milestones M7 行):把 M4 抽出的共享 composable(`useTimelineDrag` / `useUndoStack` / `useTimelineRangeSelect` / `useTimelinePlayback`)接到多轨 store 上,让 split / delete / trim / 范围选区 / 重排 / 撤销重做在多轨模型上跑通,并新增**跨轨拖动**(同类型轨)。
> 不在本 M 范围:导出(M8);position / scale / opacity / PiP(v2);多视频轨叠加的真合成预览(v2)。
> 设计参考:[../tabs/multitrack/program.md §7 §2.2](../tabs/multitrack/program.md);[../tabs/multitrack/product.md §5](../tabs/multitrack/product.md)。

## 关键设计决定(M7 内必须落定)

- **`splitScope` 形状**:单视频用 `'both' | 'video' | 'audio'`(`web/src/stores/editor.ts`)。多轨需要"全部 / 全部视频轨 / 全部音频轨 / 单条具体轨"。**采纳 `program.md §7` 提出的形状**:`'all' | 'video' | 'audio' | { kind: 'track'; id: string }`(对象形,避免字符串模板与 id 冲突);多轨 store 单独定义,不污染 editor.ts。
- **selection 形状**:editor 用 `{ track: 'video'|'audio', clipId }`。多轨需要 `{ trackId: string, clipId: string }`(track id 来自 `VideoTrack.ID` / `AudioTrack.ID`);跨 store 不复用 `Sel` 帮助函数,而是再写一份纯函数(避免类型反向污染 editor)。
- **撤销栈快照形状**:`{ videoTracks: VideoTrack[], audioTracks: AudioTrack[] }` 全量深拷贝(每条 track 的 clips 单独 `.map(c => ({...c}))`,`Sources` 和 `AudioVolume` 不进入快照,与 editor 的 `ClipsSnapshot` 一致原则:撤销只回退时间轴,不回退资源库)。
- **共享 `useTimelineDrag` 是否够用**:够。`getClips/setClips` 已是 `(trackId, clips[])` 化,`sourceMaxFor(clip, trackId)` 可由多轨 store 经 `clip.sourceId` 解析到 `Source.duration`。**新增**:跨轨拖动落点逻辑不在 `useTimelineDrag` 内(它只关心同 track 内重排);单独的 `useMultitrackOps.startCrossTrackReorder` 处理。
- **跨轨拖动规则**:同 kind 之间允许(V→V / A→A);V→A 禁止(产品规则,UI 上 dropEffect 设 `none`);拖动过程中视觉上 clip 跟随光标,落地时:从原 track 移除 + 在目标 track 追加 + program 时间用落地点(同 trim/reorder 的对齐逻辑;若落到现有 clip 内部则按吸附点对齐)。
- **删除轨道二次确认**:必须有(轨上若有 clip,确认对话框文案"将一并删除该轨上的 N 个 clip");空轨直接删。`window.confirm` 即可,不引入新模态。
- **`Ctrl+L` 折叠素材库**:`store.libraryCollapsed`(默认 false);`useTimelinePlayback` 在多轨场景下挂一个键监听(改造其 `attach()` 接受 caller 提供的额外按键 → 动作映射,避免 hardcode;或多轨写自己的 keyboard composable,选择前者保持架构对称)。
- **共享 composable 多轨复用方式**:与 editor 完全对称,`MultitrackView` 设置 `useTimelineZoom` / `useTimelineDrag` / `useTimelineRangeSelect` / `useTimelinePlayback` / `useUndoStack`,getter/setter 全部走 multitrack store。
- **后端范围**:M7 主要是**前端**工作。后端只需要补 `multitrack/domain/timeline_test.go` 用多轨场景覆盖共享 timeline ops(确认 `Split` / `DeleteClip` / `Reorder` / `TrimLeft` / `TrimRight` 在 `[]Clip`(嵌入 + SourceID)上行为正确),以及补 `RemoveTrack` 纯函数 + 测试。

## 任务清单

### A. 后端

#### A.1 `multitrack/domain/`

- [ ] `tracks.go`(新文件,或加到 `sources.go`):`RemoveVideoTrack(p, id) (*Project, error)` / `RemoveAudioTrack(p, id) (*Project, error)` 纯函数,删 track + 该 track 上所有 clip;不依赖 source 引用计数(产品决定:删 track 不删 source)
- [ ] `MoveClipAcrossTracks(p, fromKind, fromTrackID, toTrackID, clipID, newProgramStart) (*Project, error)`:同一 kind 内移动一个 clip;视频→音频(或反之)返回 error
- [ ] `multitrack/domain/timeline_test.go`(新文件):
  - 表驱动覆盖共享 ops 在 `[]multitrack.Clip` 上的行为(`Split` / `DeleteClip` / `Reorder` / `TrimLeft` / `TrimRight` 通过 `toCommonClips` 转换后调用,结果再写回 `[]Clip` 的方式)
  - `RemoveTrack` 含 clip 的轨道
  - `MoveClipAcrossTracks` 同 kind 成功 / 跨 kind 失败 / 落点 0 / 落点超出当前轨长度
- [ ] `Validate` 在 `MoveClipAcrossTracks` 之后保持 invariant(SourceID 仍在 Sources 中、视频轨 clip 仍指向视频源、视频轨开头不可留空)

#### A.2 验证

- [ ] `go test ./multitrack/...` 全绿
- [ ] `CGO_ENABLED=0 go test ./...` 全绿
- [ ] `go build ./...` 通过

### B. 前端

#### B.1 Store 扩展(`web/src/stores/multitrack.ts`)

- [ ] 加状态:`selection: ClipSelection[]`(自定义类型 `{ trackId: string; clipId: string }`)+ `splitScope: SplitScope`(union `'all' | 'video' | 'audio' | { kind: 'track'; id: string }`)+ `rangeSelection: RangeSelection | null`(复用共享 `RangeSelection` 类型)+ `libraryCollapsed: boolean`
- [ ] 接 `useUndoStack`:`snapshot()` 取 `videoTracks/audioTracks` 深拷贝,`apply(s)` 通过 `applyProjectPatch` 写回;暴露 `pushHistory / undo / redo / canUndo / canRedo`(同 editor.ts)
- [ ] 选择 / 范围辅助:加纯函数 `selToggle / selReplace / selInTrack`(同 `Sel` 形状,但 trackId 替代 track kind)
- [ ] `loadProject(p)` 重置 selection / splitScope / rangeSelection / playhead / 撤销栈(对齐 editor.ts `loadProject` 行为)
- [ ] `removeVideoTrack(id) / removeAudioTrack(id)` action(走后端纯函数对应的 patch:本地直接重写 tracks 数组 + 调 `applyProjectPatch`,不需要新 API,后端 `PUT /:id` 已支持全量替换)
- [ ] `moveClipAcrossTracks(fromTrackId, toTrackId, kind, clipId, newProgramStart)`:同 kind 间移动,本地纯计算 + `applyProjectPatch`
- [ ] `playhead` 已在 M6 加;`splitScope` watch 用于 ops

#### B.2 Ops composable(`web/src/composables/useMultitrackOps.ts` 新文件)

参照 `useEditorOps`,但要支持四态 `splitScope`:

- [ ] `tracksInScope(): Array<{ kind, id }>` 把 `'all'` 展开为所有视频轨 + 所有音频轨;`'video'` 展开为所有视频轨;`'audio'` 展开为所有音频轨;`{ kind: 'track', id }` 展开为单条
- [ ] `splitAtPlayhead()`:遍历 `tracksInScope()`,对每条 track 的 clips 调 `splitTrack`(`@/utils/timeline`,可直接复用,因为它是按 `programStart/sourceStart/sourceEnd` 工作,与 sourceId 无关——但要确认新分裂出的 clip 复制 `sourceId` 字段);若 splitTrack 不复制扩展字段则需新增 `splitTrackMultitrack` 包装(读源 clip 的 sourceId 写到两个产物上)
- [ ] `deleteSelection()`:范围选区优先(`carveRange` 同样需检查是否复制 sourceId);否则按 `selection` 删
- [ ] 删除时同步清 `selection`、清 `rangeSelection`、`pushHistory()`
- [ ] 暴露:`splitAtPlayhead / deleteSelection / addVideoTrack / addAudioTrack / removeTrack(kind, id)`(removeTrack 内部 confirm)

> **疑似坑**:确认 `web/src/utils/timeline.ts` 的 `splitTrack` / `carveRange` 是否会保留输入 clip 上 `sourceId` 等多出字段。如果用 `{...c, sourceStart, sourceEnd}` 这种解构覆写则会保留;如果 hand-roll 新对象就不会。需要 grep + 必要时改为 `{...c, ...}` 模式或在多轨包装层补 sourceId。

#### B.3 跨轨拖动(扩展 `useTimelineDrag` 或新建 composable)

- [ ] 评估方案:
  - **A**:扩展 `useTimelineDrag.startReorder` 增加可选 `findTargetTrack(ev)` 回调;落地时若返回的 trackId 与原 trackId 不同,执行跨轨移动(`getClips(from)` 删 + `getClips(to)` 加)。
  - **B**:多轨独立 `useMultitrackOps.startCrossTrackReorder`,只在多轨视图上替换 `startReorder`;`useTimelineDrag` 不变。
  - **决策**:走 **A**,把 `findTargetTrack` 做成 opts 字段(默认返回原 trackId 即同轨重排,与现状等价);避免分裂出两套相似 reorder 逻辑。
- [ ] 视觉反馈:拖动时给跟随的"幽灵 clip"挂一个浅色覆盖块,跟光标移动(M7 简化:不做幽灵,直接动 store 里 clip 的 trackId,实时反映;若 UX 卡可加节流);跨 kind hover 时光标变成 `not-allowed`,落地拒绝
- [ ] `MultitrackView` 把 `findTargetTrack` 实现为读 `ev.target` 找 `[data-multitrack-track-kind][data-multitrack-track-id]`(给每条 `TimelineTrackRow` 包裹层加这两个 data-* 属性)

#### B.4 接入共享 composable(`MultitrackView.vue`)

- [ ] `useTimelineZoom`(已在 editor 用)+ `useTimelineDrag`(getClips/setClips 走 store) + `useTimelineRangeSelect` + `useTimelinePlayback`(togglePlay / split / delete / undo / redo / Esc 清范围)+ `useUndoStack` 已经在 store
- [ ] `useTimelinePlayback` 现签名:`{ togglePlay, splitAtPlayhead, deleteSelection, seekBackBoundary, seekForwardBoundary, undo, redo, clearRangeSelection }`。多轨需新增"折叠素材库"按键(Ctrl+L) → 评估扩展 `useTimelinePlayback` opts 还是另写。**决策**:`useTimelinePlayback` 加 `extraBindings?: Array<{ keys: string[]; ctrl?: boolean; action: () => void }>` 可选项;不破坏 editor 调用方
- [ ] 选中态、范围选区、playhead 渲染按 `splitScope` 切换(全局 / 仅视频 / 仅音频 / 单条轨):
  - `'all'`:整个时间轴一根 playhead(贯穿所有轨)
  - `'video'`:playhead 只渲染在视频轨区
  - `'audio'`:同理
  - `{ kind: 'track', id }`:playhead 只渲染在那一条轨上(类似单视频 splitScope=video / audio 的小游标)
- [ ] 工具条:删除选中 / 撤销 / 重做按钮(可选,键盘已可达;若加按钮就 `MultitrackToolbar.vue` 抽出,顶栏不堆)

#### B.5 删除轨道 / 折叠素材库

- [ ] 每条 `TimelineTrackRow` 的左侧 label 区加一个 "×" 按钮(hover 显示);点击 → `confirm(`删除"${track.label}"?其上 N 个 clip 将一并删除`)` → store.removeVideoTrack/removeAudioTrack
- [ ] `MultitrackLibrary` 加 collapse 开关(顶栏一个箭头按钮 + Ctrl+L);折叠后只剩一条 32px 宽的 sidebar,导入按钮和列表全隐;再按 Ctrl+L 展开

### C. 验收

- [ ] `cd web && npx vue-tsc --noEmit` + `cd web && npm run build` 全绿
- [ ] `go build ./...` + `go test ./...` + `CGO_ENABLED=0 go test ./...` 全绿
- [ ] **手测清单**(M7 终态,用户侧):
  - [ ] 三视频轨各 1 个 clip(高/中/低层) → 顶层预览正确,Ctrl+Z 撤销 / Ctrl+Y 重做依次回溯三次添加
  - [ ] 选中 clip(单击 / Shift 多选 / Ctrl 多选)→ Delete 删除;撤销恢复
  - [ ] 范围选区(右键拖):跨多条轨道生效,Delete 删除范围,空白部分前后对接;撤销恢复
  - [ ] 在播放头按 `S` 分割:`splitScope='all'` 时所有轨同时分;切到 `'video'` / `'audio'` / 单轨 后再分,只影响 scope
  - [ ] clip 边缘拖拽 trim:左、右 handle 都能修剪,左 handle 同时调 programStart 让右沿不动;最大不超 source 时长
  - [ ] clip 体拖动重排:同轨内吸附 0 / playhead / 其他 clip 边缘
  - [ ] **跨轨拖动**:视频 clip 从 V1 拖到 V2 → 成功;同 clip 拖到 A1 → 拒绝(光标 not-allowed,落地无变化)
  - [ ] 删除轨道:×按钮 → 二次确认 → 走;撤销恢复;空轨删除直接走
  - [ ] Ctrl+L 折叠素材库 → 时间轴宽度变大;再按一次展开
  - [ ] 自动保存:任意编辑 → 等 1.5s → 关闭工程 → 重开 → 状态恢复
  - [ ] **单视频 Tab 视觉与功能零回归**(切到单视频 Tab,再过一遍 M4 §2.2.4 手测清单)
  - [ ] **两个 Tab 切换 store 不互相污染**(在多轨 Tab 选中 clip,切到单视频 Tab,选中态不应渗漏)
- [ ] 收尾 commit:milestone M7 行 ✅ + commit hash + 完成日期 + 本 todo 整段清空

## 阻塞 / 待澄清

- (无)新出现的设计岔路在第二段"关键设计决定"已锁定;实现过程中发现新问题再补到这里

## 完工标准

参见 milestones 文件 M7 行的"交付内容":
> split / delete / trim / 范围选区 / 重排 / 撤销重做在多轨模型上跑通(完全靠 M4 共享 composable + 多轨 store 实现);**跨轨拖动**(同类型轨之间;视频→音频禁止);splitScope 扩展到 `all/video/audio/track:<id>`;轨道删除二次确认;`multitrack/domain/timeline_test.go` 覆盖多轨场景;锁 `Ctrl+L` 折叠素材库

**触发硬性条件**:全局不变量清单(milestones 文件顶部"全局不变量")每条都过。
