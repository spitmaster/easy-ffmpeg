# 多轨剪辑器 — `feature-v0.5.0/multitrack-editor` — 当前 M 的待办

> 对应 milestones 文件:[../milestones/feature-v0.5.0_multitrack-editor.md](../milestones/feature-v0.5.0_multitrack-editor.md)
> 当前状态:**M1–M8 ✅ 已完成**(M8 收尾日期 2026-05-01,commit `6d739a5`)
> 下一个:**M9 收尾 + 归档**(未开始)— 整个 feature 的最后一步,完成后分支归档

启动 M9 时,把 [milestones 文件 M9 行](../milestones/feature-v0.5.0_multitrack-editor.md)的"交付内容"拆成可勾选清单整段填入此处,并把 milestones M9 行从 ⏳ 改 🚧。

M9 主要动作清单(摘自 milestones M9 行):
- 本里程碑文件 `git mv` 至 `docs/milestones/archive/feature-v0.5.0_multitrack-editor.md`
- 主索引 [../milestones.md](../milestones.md) 中"进行中"挪到"已归档"
- 本 todo 文件删除
- [../README.md](../README.md) `tabs/` 表加 multitrack 行 ✅
- `../../README.md` / `../../CLAUDE.md` 关键目录补 `multitrack/` + `editor/common/`
- [../roadmap.md](../roadmap.md) §4 加 0.6.0 行 + §3 非目标修订(把"多轨叠加 / PiP"措辞改为"PiP/位置/缩放/不透明度,留 v2")
- [../tabs/editor/product.md §1.2](../tabs/editor/product.md) 文案同步("多素材剪辑由多轨 Tab 提供")
- 版本号 bump 至 **0.6.0**:`web/package.json` / `internal/version/version.go` / `cmd/desktop/wails.json`
- 单视频 Tab 零回归再确认 + 多轨基本流程一遍
