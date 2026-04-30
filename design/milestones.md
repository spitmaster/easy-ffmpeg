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
| 当前进行中的迁移 | _无_ |
| 当前版本 | **v0.5.1**(M4 完成,版本号已升 0.5.1,前端 Vue 化整体落地) |
| 下一步 | _暂无大型迁移;待启动新计划时在此重建本节_ |
| 主分支 | `v0.5.1`(开发分支,尚未合并到 `main`) |

**接手前必读**(按顺序):

1. 本文件全文 — 看清当前状态(目前无进行中迁移)
2. [README.md](README.md) → [core/architecture.md](core/architecture.md) → [core/frontend.md](core/frontend.md) — 项目分层
3. [../CLAUDE.md](../CLAUDE.md) — 项目通用规约
4. `git log --oneline -10` — 看最近的提交,了解上一个 session 实际改了什么

**怎么验证当前代码是健康的**:

```bash
cd web && npm run build      # 类型检查 + Vite 构建,应输出 "✓ built in <1s"
cd .. && go build ./...      # Go 全量编译,应零输出
go test ./...                # 后端测试
```

---

## 已归档(完成的迁移项目)

### 前端 Vue 化迁移(v0.5.x,2026-04-29 → 2026-04-30)

> 范围与设计:[core/frontend-vue-migration.md](core/frontend-vue-migration.md)
> 落地架构:[core/frontend.md](core/frontend.md)
> 目标:为后续多轨剪辑器(类 Premiere Pro)做技术储备。把零构建的原生 HTML+JS 前端迁到 Vue 3 + Vite + TS + Pinia + Tailwind。

| 里程碑 | 状态 | 完成日期 | Commit | 交付内容 |
|--------|------|---------|--------|---------|
| **M1** 脚手架就绪 | ✅ 完成 | 2026-04-29 | `e701de2` | `web/` 工程齐全;`web/embed.go` + `server/server.go` 改为 import `easy-ffmpeg/web`;`server/web/` 整体 git rm;build.sh / build.bat 加前端构建步骤;.gitignore 调整;空 Vue 壳能在 Web 版与桌面版跑通 |
| **M2** 顶栏 + 转换 Tab 迁移 | ✅ 完成 | 2026-04-29 | `ab5248b` | API 客户端层(8 个模块);Pinia stores(version / ffmpeg / dirs / modals);共享对话框(Picker / ConfirmCommand / ConfirmOverwrite / PrepareOverlay);`useJobPanel` composable;TopBar 完整功能;ConvertView 完整功能;版本号 → 0.5.0 |
| **M3** 音频 + 单视频剪辑 Tab | ✅ 完成 | 2026-04-29 | `f20d345` | `api/audio.ts` + `api/editor.ts`;`AudioView` 拆 3 子组件(Convert / Extract / Merge),沿用 `useJobPanel`;`EditorView` 拆 8 子组件(TopBar / PlayBar / Timeline / Toolbar / AudioVolume / ProjectsModal / ExportDialog / ExportSidebar)+ `useEditorPreview` 双 `<video>`/`<audio>` + 间隙时钟 + WebAudio 增益 + `useEditorOps` 分割/删除 + `editor` Pinia store(撤销栈、防抖自动保存)+ `utils/timeline.ts` + `utils/time.ts`;Canvas 子树未引入 — 当前 DOM 实现性能足够,留待瓶颈出现再做;版本号未动(留给 M4 整体升 0.5.1) |
| **M4** 清理 + 文档同步 | ✅ 完成 | 2026-04-30 | _本次提交_ | 重写 [core/frontend.md](core/frontend.md)(原 IIFE 描述 → Vue 工程 / API 客户端层 / Pinia / composable / SSE 总线);重写 [core/ui-system.md](core/ui-system.md)(原 CSS 变量描述 → Tailwind + tokens.css + utility 模式语言);[core/architecture.md](core/architecture.md) 目录结构与数据流措辞同步到 `web/`;[core/build.md](core/build.md) §2 加前端 npm 流水段;[core/roadmap.md](core/roadmap.md) §5 加 v0.5.0–0.5.1 行;[design/README.md](README.md) 当前状态行更新到 v0.5.1;[../README.md](../README.md) 全文重写(原 Fyne 时代描述);[../CLAUDE.md](../CLAUDE.md) 关键目录与不变量、design 索引同步;版本号 → 0.5.1([web/package.json](../web/package.json) / [internal/version/version.go](../internal/version/version.go) / [cmd/desktop/wails.json](../cmd/desktop/wails.json));milestones 整段归档至本节 |
