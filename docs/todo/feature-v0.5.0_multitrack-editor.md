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

- [x] `tracks.go`(新文件):`RemoveVideoTrack(p, id) (*Project, error)` / `RemoveAudioTrack(p, id) (*Project, error)` 纯函数,删 track + 该 track 上所有 clip;不依赖 source 引用计数(产品决定:删 track 不删 source)
- [x] `MoveClipAcrossTracks(p, kind, fromTrackID, toTrackID, clipID, newProgramStart) (*Project, error)`:同一 kind 内移动一个 clip;跨 kind 返回 `ErrCrossKindMove`
- [x] `multitrack/domain/timeline_test.go`(新文件):
  - 共享 ops 在多轨 `[]Clip` 上经 `applyOnTrack` 包装跑一遍,断言 SourceID 不丢(`Split` / `DeleteClip` / `Reorder` / `TrimLeft` / `TrimRight` 都覆盖)
  - `RemoveVideoTrack` / `RemoveAudioTrack` 含 clip 删除 + ErrTrackNotFound
  - `MoveClipAcrossTracks` 同 kind 成功 / 同轨重排(programStart 更新) / 跨 kind 拒绝(`ErrCrossKindMove`) / 不存在的轨道 / 不存在的 clip / 负 programStart 钳到 0
- [x] `Validate` 在 `MoveClipAcrossTracks` 之后保持 invariant — 由测试中断言 SourceID 仍指向 Sources 间接覆盖;前端在执行后调 store 走 `applyProjectPatch`,Validate 仍以全量保存路径生效

#### A.2 验证

- [x] `go test ./multitrack/...` 全绿
- [x] `CGO_ENABLED=0 go test ./...` 全绿
- [x] `go build ./...` 通过

### B. 前端

#### B.1 Store 扩展(`web/src/stores/multitrack.ts`)

- [x] 加状态:`selection: MultitrackClipSelection[]`(`{ trackId, clipId }`)+ `splitScope: MultitrackSplitScope`(`'all' | 'video' | 'audio' | { kind: 'track'; id: string }`)+ `rangeSelection: RangeSelection | null` + `libraryCollapsed: boolean`
- [x] 接 `useUndoStack`:`snapshot()` 取 `videoTracks/audioTracks` 深拷贝,`apply(s)` 通过 `applyProjectPatch` 写回;暴露 `pushHistory / undo / redo / canUndo / canRedo`(同 editor.ts)
- [x] 选择辅助:`MultitrackSel` 提供 `has / toggle / replace / inTrack`(独立于 editor 的 `Sel`)
- [x] `loadProject(p)` 重置 selection / splitScope / rangeSelection / playhead / libraryCollapsed / 撤销栈;`createNew` 与 `openProject` 都走它,`closeProject` 同步重置
- [x] `removeVideoTrack(id) / removeAudioTrack(id)`:本地纯计算 + `applyProjectPatch`,删 track 的同时清 selection/splitScope 中失效的引用
- [x] `moveClipAcrossTracks(kind, fromTrackId, toTrackId, clipId, newProgramStart)`:同 kind 间移动,本地纯计算 + `applyProjectPatch`;**不**自动 `pushHistory`,由调用方(拖拽 onUp)负责一次性 push
- [x] `playhead`(M6)+ M7 新增 selection/scope/range 的 watch 经由 `programDuration` 自动钳制保持

#### B.2 Ops composable(`web/src/composables/useMultitrackOps.ts` 新文件)

参照 `useEditorOps`,支持四态 `splitScope`:

- [x] `tracksInScope(): Array<{ kind, id }>` 把 `'all'` / `'video'` / `'audio'` / `{kind:'track',id}` 各自展开
- [x] `splitAtPlayhead()`:对每条 in-scope track 调 `splitTrack`(共享 `@/utils/timeline`,用 `{...c, ...}` 已自动保留 `sourceId`,不需要新增包装)
- [x] `deleteSelection()`:范围选区优先(`carveRange`,同样保留 `sourceId`);否则按 `selection` 删,清 `selection` + `rangeSelection`,`pushHistory()`
- [x] 暴露:`splitAtPlayhead / deleteSelection / tracksInScope / removeTrack(kind, id)` + `Sel` (=`MultitrackSel`)
- [x] `removeTrack` 内部:track 有 clip 时 `window.confirm` 二次确认;空 track 直接删

> **关于 sourceId 保留**:已确认 `splitTrack` / `carveRange` 全部走 `{ ...c, ...overrides }`,扩展字段(包括 `sourceId`)自动跟随,不需要新包装。

#### B.3 跨轨拖动(扩展 `useTimelineDrag`)

- [x] **方案 A**:在 `useTimelineDrag.startReorder` 内加可选 `findTargetTrack(ev): string | null` + `onCrossTrack(from,to,clipId,programStart): boolean`。光标进入不同 trackId 时调 `onCrossTrack`,接受后重置 `currentTrackId/baseX/origProgramStart/snapPoints`,继续在新轨道上拖
- [x] 视觉反馈:不做幽灵层,直接动 store(实时反映);跨 kind 时 `findTargetTrack` 返回 `null`,onCrossTrack 不触发,clip 留在原轨
- [x] `MultitrackView` 给视频/音频轨外层套 `[data-mt-track-id][data-mt-track-kind]`,`findTrackUnderCursor` 用 `closest()` 解析

#### B.4 接入共享 composable(`MultitrackView.vue`)

- [x] `useTimelineZoom` + `useTimelineDrag<MultitrackClip>`(getClips/setClips 走 store,sourceMaxFor 走 sourcesById) + `useTimelineRangeSelect` + `useTimelinePlayback` + `useUndoStack`(已在 store)
- [x] `useTimelinePlayback` 加 `extraBindings?: Array<{ keys, ctrl?, shift?, alt?, meta?, action }>`,`Ctrl+L` 折叠素材库走它;不破坏 editor 调用方
- [x] playhead 渲染按 `splitScope` 切换:`'all'` 全高 / `'video'` 视频区高度 / `'audio'` 音频区高度 / `{kind:'track',id}` 单条 48px(top 由索引推导)
- [x] 工具条:顶栏直接挂"撤销 / 重做 / 收起素材库"按钮(未抽 `MultitrackToolbar.vue`,以免堆叠;键盘已可达)

#### B.5 删除轨道 / 折叠素材库

- [x] 每条轨道左侧 label 区加 "×" 按钮 → `useMultitrackOps.removeTrack` → 含 clip 时 `confirm` 二次确认
- [x] `Ctrl+L` / 顶栏按钮 切换 `store.libraryCollapsed`;折叠后用 32px 宽 sidebar 替代 `MultitrackLibrary`,只露一个 "»" 展开按钮

### C. 验收

- [x] `cd web && npx vue-tsc --noEmit` + `cd web && npm run build` 全绿(2026-04-30)
- [x] `go build ./...` + `go test ./...` + `CGO_ENABLED=0 go test ./...` 全绿(2026-04-30)
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
