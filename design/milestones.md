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
| 当前版本 | **v0.5.0**(M1+M2 完成) |
| 下一步 | **M3** — 音频 Tab + 单视频剪辑 Tab + Canvas 渲染基础 |
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
| **M3** 音频 + 单视频剪辑 Tab | ⏸ 未开始 | — | — | AudioView(对照老 `app.js` 三模式:格式转换 / 提取 / 合并);引入 Canvas 渲染基础(Konva 或原生)与 `components/canvas/` 子树为多轨编辑器铺路;EditorView(对照老 `editor/editor.js` 1908 行,时间轴 / 入出点 / 波形) |
| **M4** 清理 + 文档同步 | ⏸ 未开始 | — | — | 同步 [core/architecture.md](core/architecture.md);**重写或删除已过时的 [core/frontend.md](core/frontend.md) 与 [core/ui-system.md](core/ui-system.md)**(它们描述的是已被替换的 IIFE / 零构建架构);更新 [core/build.md](core/build.md) §2 反映前端构建步骤;[core/roadmap.md](core/roadmap.md) 加 v0.5.x 行;[README.md](README.md) 索引调整;[../CLAUDE.md](../CLAUDE.md) 前端章节;版本号 → 0.5.1 |

---

## 接手 M3 时的具体提示

> 这一节是给**下一个开始 M3 的 session** 看的。完成 M3 后请把内容删掉(它的内容已落到代码里),并把 M3 状态改为 ✅。

### 已经在 M1/M2 铺好的基础

| 你想用的能力 | 现成的位置 |
|-------------|----------|
| 发 API 调用 | `web/src/api/client.ts`(`getJson` / `postJson` / `postJsonRaw` / `ApiError`)+ 各域模块 |
| Pinia store 模板 | `web/src/stores/{version,ffmpeg,dirs,modals}.ts`(用 setup-store 风格,看 ffmpeg.ts 最完整) |
| 文件/目录浏览器 | `useModalsStore().showPicker({ mode, title, startPath })` 返回 `Promise<string|null>` |
| 命令预览模态框 | `useModalsStore().showCommand(cmd)` 返回 `Promise<boolean>` |
| 覆盖确认模态框 | `useModalsStore().showOverwrite(path)` 返回 `Promise<boolean>` |
| 任务进度/日志/完成条 | `useJobPanel({ cancelUrl, ... })` composable + `<JobLog>` 组件,见 ConvertView 的用法 |
| SSE 事件总线 | `jobBus.subscribe(fn)` — 已在 `App.vue` 的 onMounted 里 connect 过 |
| 路径工具 | `web/src/utils/path.ts`(forward-slash,backend 全用 `/`) |
| 单位格式化 | `web/src/utils/fmt.ts`(`Fmt.human` 字节单位) |
| 设计 token | `web/src/styles/tokens.css` + `tailwind.config.js` 的 `bg-bg-*` / `text-fg-*` / `text-accent` / `text-danger` / `text-success` |

### M3 需要新增的 API 模块

参照老 `app.js` 与 `editor/editor.js`,M3 要在 `web/src/api/` 下新建:

- `audio.ts` — `/api/audio/probe`(GET 探测音轨),`/api/audio/start`(POST,支持 dryRun + overwrite),`/api/audio/cancel`
- `editor.ts` — `/api/editor/*` 一整套(legacy `editor.js` 用了大量,需要梳理)

### M3 的两个最大风险

1. **EditorView 复杂度**:1908 行老代码混合了时间轴拖拽、播放头同步、入出点、波形、撤销栈、工程持久化。**强烈建议拆成多个组件**(`Timeline.vue` / `PlayBar.vue` / `Toolbar.vue` / `ExportPanel.vue` / 等),不要一个大文件。
2. **Canvas 渲染基础**:不要陷入"先把整个时间轴用 Canvas 重写"的陷阱。先用 DOM 实现一版能跑(老前端就是 DOM),等性能瓶颈出现再考虑 Canvas。Canvas 子树位置:`web/src/components/canvas/`。

### 临时参考老前端代码

老前端代码已在 M1 commit (`e701de2`) 之前的提交 `d00b8e6` 里。要看原始实现:

```bash
git show d00b8e6:server/web/app.js > /tmp/old-app.js
git show d00b8e6:server/web/editor/editor.js > /tmp/old-editor.js
git show d00b8e6:server/web/index.html > /tmp/old-index.html
git show d00b8e6:server/web/app.css > /tmp/old-app.css
```

(对照功能即可,**不要把它们落回仓库**——M1 的清理动作必须保持。)

---

## 已归档(完成的迁移项目)

(暂无)
