# 里程碑主索引

> **本文件只是索引**——具体里程碑详情在 [`milestones/<branch>.md`](milestones/) 各自的文件里。
>
> 本仓库遵循 `~/.claude/CLAUDE.md` v1.1:**分支名 → 文件名机械推导**(`/` → `_`),per-branch 文件天然解耦,主索引每行 ≤ 120 字符。
>
> **接手 session 第一件事**:跑 `git branch --show-current`,推导对应 milestones 文件,跳过本文件直接打开它。

---

## 进行中

| Feature | 分支 | 启动日期 | 当前 M | 详情 |
|---------|------|---------|--------|------|
| 多轨画布 + clip 变换(v0.5.1) | `feature-v0.5.1/multitrack-scale-video` | 2026-05-04 | M1 ✅ M2 ✅ M3 ✅;M4 待启动 | [milestones/feature-v0.5.1_multitrack-scale-video.md](milestones/feature-v0.5.1_multitrack-scale-video.md) |

---

## 已归档(完成的功能/版本)

> 完工时:`git mv` 该 feature 的文件到 `milestones/archive/`,这里加一行索引。

| Feature | 分支 | 完成日期 | 详情 |
|---------|------|---------|------|
| 多轨剪辑器(类 Premiere Pro,v0.5.0) | `feature-v0.5.0/multitrack-editor` | 2026-05-01 | [milestones/archive/feature-v0.5.0_multitrack-editor.md](milestones/archive/feature-v0.5.0_multitrack-editor.md) |
| 前端 Vue 化迁移(v0.5.0) | `v0.5.0` | 2026-04-30 | [milestones/archive/v0.5.0.md](milestones/archive/v0.5.0.md) |

---

## 三档规划文档(rule v1.1)

| 文档 | 粒度 | 回答的问题 | 更新频率 |
|------|------|-----------|---------|
| [roadmap.md](roadmap.md) | **粗** — 功能级 | 接下来要做哪些功能?边界在哪? | 月级 |
| 本文件 + `milestones/<branch>.md` | **中** — 单功能里程碑 | 当前在做哪个功能?到第几个 M? | 周级 |
| `todo/<branch>.md` | **细** — 当前 M 的具体动作 | 这个 M 还差哪几步? | 日级 |

**晋升规则**:

| 触发 | 动作 |
|------|------|
| 某功能正式启动 | `roadmap.md` 标"⏳ 进行中";本索引"进行中"加一行;创建分支 + `milestones/<branch>.md` |
| 开始一个具体 M | 把 M 的交付拆成可勾选清单填入 `todo/<branch>.md`;`milestones/<branch>.md` 那行从 ⏳ 改 🚧 |
| M 完结 | `milestones/<branch>.md` 那行标 ✅ + commit + 日期;`todo/<branch>.md` 整段清空 |
| 整个功能完结 | **`git mv`** `milestones/<branch>.md` 到 `milestones/archive/`;本索引"进行中"挪到"已归档";`todo/<branch>.md` 删除;`roadmap.md` "已发布版本"加一行 |

详见 `~/.claude/CLAUDE.md` v1.1 §2-§5。
