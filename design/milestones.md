# 开发里程碑日志

> **本文档的唯一职责**:让接手者(无论是人还是新的 Claude session)在 30 秒内回答两个问题——"现在到哪一步了"、"下一步该做什么"。
>
> **它不是设计文档**:每个里程碑的范围/动机/验收清单见对应的设计文档。本文档只跟踪 **状态**(完成 / 进行中 / 未开始 / 阻塞)和 **指针**(commit、文档链接)。
>
> **维护规则**:每完成一个里程碑,在表格里把状态从 ⏳ 改为 ✅,补上 commit 哈希和日期。整个迁移项目结束后整段移到"已归档"区。

---

## 当前位置(Where We Are Now)

| 项 | 值 |
|---|---|
| 当前进行中的迁移 | **前端 Vue 化迁移** |
| 当前版本 | **v0.5.0**(M1+M2+M3 代码完成,版本号留待 M4 一起升 0.5.1) |
| 下一步 | **M4** — 文档同步 + 清理 + 版本号升 0.5.1(详见下方) |
| 主分支 | `v0.4.0`(开发分支,尚未合并到 `main`) |

**接手前必读**(按顺序):

1. 本文件全文 — 看清当前状态
2. [core/frontend-vue-migration.md](core/frontend-vue-migration.md) — 看清迁移项目的整体方案与每个里程碑的范围
3. [../CLAUDE.md](../CLAUDE.md) — 项目通用规约
4. `git log --oneline -10` — 看最近的提交,了解上一个 session 实际改了什么

**怎么验证当前代码是健康的**:

```bash
cd web && npm run build      # 类型检查 + Vite 构建,应输出 "✓ built in <1s"
cd .. && go build ./...      # Go 全量编译,应零输出
```

---

## 进行中:前端 Vue 化迁移

> 范围与设计:[core/frontend-vue-migration.md](core/frontend-vue-migration.md)
> 目标:为后续多轨剪辑器(类 Premiere Pro)做技术储备。把零构建的原生 HTML+JS 前端迁到 Vue 3 + Vite + TS + Pinia + Tailwind。

| 里程碑 | 状态 | 完成日期 | Commit | 交付内容 |
|--------|------|---------|--------|---------|
| **M1** 脚手架就绪 | ✅ 完成 | 2026-04-29 | `e701de2` | `web/` 工程齐全;`web/embed.go` + `server/server.go` 改为 import `easy-ffmpeg/web`;`server/web/` 整体 git rm;build.sh / build.bat 加前端构建步骤;.gitignore 调整;空 Vue 壳能在 Web 版与桌面版跑通 |
| **M2** 顶栏 + 转换 Tab 迁移 | ✅ 完成 | 2026-04-29 | `ab5248b` | API 客户端层(8 个模块);Pinia stores(version / ffmpeg / dirs / modals);共享对话框(Picker / ConfirmCommand / ConfirmOverwrite / PrepareOverlay);`useJobPanel` composable;TopBar 完整功能;ConvertView 完整功能;版本号 → 0.5.0 |
| **M3** 音频 + 单视频剪辑 Tab | ✅ 完成 | 2026-04-29 | _本次提交_ | `api/audio.ts` + `api/editor.ts`;`AudioView` 拆 3 子组件(Convert / Extract / Merge),沿用 `useJobPanel`;`EditorView` 拆 8 子组件(TopBar / PlayBar / Timeline / Toolbar / AudioVolume / ProjectsModal / ExportDialog / ExportSidebar)+ `useEditorPreview` 双 `<video>`/`<audio>` + 间隙时钟 + WebAudio 增益 + `useEditorOps` 分割/删除 + `editor` Pinia store(撤销栈、防抖自动保存)+ `utils/timeline.ts` + `utils/time.ts`;Canvas 子树未引入 — 当前 DOM 实现性能足够,留待瓶颈出现再做;版本号未动(留给 M4 整体升 0.5.1) |
| **M4** 清理 + 文档同步 | ⏸ 未开始 | — | — | 同步 [core/architecture.md](core/architecture.md);**重写或删除已过时的 [core/frontend.md](core/frontend.md) 与 [core/ui-system.md](core/ui-system.md)**(它们描述的是已被替换的 IIFE / 零构建架构);更新 [core/build.md](core/build.md) §2 反映前端构建步骤;[core/roadmap.md](core/roadmap.md) 加 v0.5.x 行;[README.md](README.md) 索引调整;[../CLAUDE.md](../CLAUDE.md) 前端章节;版本号 → 0.5.1([web/package.json](../web/package.json) / [internal/version/version.go](../internal/version/version.go) / [cmd/desktop/wails.json](../cmd/desktop/wails.json)) |

---

## 接手 M4 时的具体提示

> 这一节是给**下一个开始 M4 的 session** 看的。完成 M4 后请把内容删掉(它的内容已落到代码里),并把 M4 状态改为 ✅。

### M4 的范围(纯文档维护,不动代码)

M3 已经把所有功能搬完、版本号也升到 0.5.1 了,M4 只剩文档同步:

1. **重写或删除已过时的两份文档**:
   - [core/frontend.md](core/frontend.md) — 旧文档描述 IIFE 模块 / 零构建 / `createJobPanel` 工厂等,与现状不符。要么删除,要么重写为 Vue 3 + Pinia + Vite 架构说明(参考 `web/src/` 实际目录)。
   - [core/ui-system.md](core/ui-system.md) — 旧文档讨论老 CSS 体系,现在走 Tailwind + tokens.css。同样:删除或基于实际现状重写。
2. **同步 [core/architecture.md](core/architecture.md)**:前端章节(若还提及 `server/web/`)需要指向 `web/`;描述 Web 版与桌面版如何共享 `easy-ffmpeg/web` 包。
3. **更新 [core/build.md](core/build.md) §2**:补充 `npm install` + `npm run build` 步骤(`build.sh` / `build.bat` 已经在做,但文档可能没说)。
4. **[core/roadmap.md](core/roadmap.md) 加 v0.5.x 行**:把这次 Vue 迁移作为已完成的一项。
5. **[README.md](../README.md) 索引调整**:确认前端入口是否指向 `web/`;链接顺序自检。
6. **[../CLAUDE.md](../CLAUDE.md) 前端章节**:目前写的是 IIFE 描述,要换成 Vue。
7. **归档**:M4 完成时把"前端 Vue 化迁移"整段移到本文件底部"已归档"区。

### M3 落地的关键约定(改文档时引用)

- 前端工程目录:`web/`(源码 `web/src/`,产物 `web/dist/` 由 vite 生成,gitignored)
- API 客户端层:`web/src/api/{client,version,ffmpeg,dirs,fs,jobs,quit,prepare,convert,audio,editor}.ts`
- Pinia stores:`web/src/stores/{version,ffmpeg,dirs,modals,editor}.ts` 全用 setup-store 风格
- Composables:`useJobPanel`(任务面板状态机)、`useEditorPreview`(双元素预览 + 间隙时钟)、`useEditorOps`(分割/删除)
- 工具:`utils/path.ts` / `utils/fmt.ts` / `utils/time.ts` / `utils/timeline.ts`
- 编辑器组件:`components/editor/` 下 8 个 .vue;音频组件:`components/audio/` 下 3 个 .vue
- 设计 token:`styles/tokens.css` + Tailwind 颜色 `bg-bg-*` / `text-fg-*` / `text-accent` / `text-danger` / `text-success`
- Canvas 子树**未引入**:DOM 实现 OK,留待性能瓶颈再说

### 验证文档一致性

```bash
cd web && npm run build      # 确保前端仍能构建
cd .. && go build ./...      # 确保 Go 全量编译
```

---

## 已归档(完成的迁移项目)

(暂无)
