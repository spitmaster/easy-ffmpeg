# 当前 M 的待办清单(per-branch)

> **本目录下每个文件对应一个 active 分支**。文件名 = `<分支名 with "/" → "_">.md`,例:
>
> | 分支 | 待办文件 |
> |------|---------|
> | `multitrack` | `multitrack.md` |
> | `feature/foo/bar` | `feature_foo_bar.md` |
> | `bugfix/login-crash` | `bugfix_login-crash.md` |
>
> **规则**(详见 `~/.claude/CLAUDE.md` v1.1 §3、§5):
>
> - 一个分支正在跑某个 M → 这里有一份对应文件,内容是该 M 的可勾选清单
> - M 完结 → 文件**整段清空**(只留模板注释),等下一个 M 启动再填
> - 整个 feature 完结(分支整体归档)→ 文件**直接删除**(M 已无意义,不归档)
>
> AI 接手 session:跑 `git branch --show-current` → 推导本目录对应文件 → 打开看具体待办。文件不存在就主动问用户。

## 当前 active 文件

> 这里只列出对应"进行中"分支的待办文件;归档的 feature 不留 todo。

| 分支 | 待办文件 | 当前 M |
|------|---------|--------|
| `feature-v0.5.0/multitrack-editor` | [feature-v0.5.0_multitrack-editor.md](feature-v0.5.0_multitrack-editor.md) | M3 后端共享层抽取 |

## 模板

启动新 M 时,新建 `<branch>.md`,内容形如:

```markdown
# <Feature 显示名> — <Branch 名> — 当前 M 的待办

> 对应 milestones 文件:[../milestones/<branch>.md](../milestones/<branch>.md)
> 当前 M:**M<N> <主题>**

## 任务清单

- [ ] 第一步具体动作(够细到可以 grep / 可勾选)
- [ ] 第二步具体动作
- [ ] ...

## 阻塞 / 待澄清

- (没有就留空,有就一行一条)

## 完工标准

- (M 完结的硬性条件,通常引用 milestones 文件中那行的"交付内容")
```
