# 产品路线图

> **最大粒度**的产品功能规划。回答的是"这个项目接下来要做哪些**功能**"。
>
> 与本目录其他规划文档的分工:
>
> - 本文(`roadmap.md`):粗粒度,**功能级**,月级更新。某个功能正式启动开发时在此标"⏳ 进行中,见 milestones.md"
> - [milestones.md](milestones.md):中粒度,**单功能开发的里程碑**(M1, M2, …),周级更新
> - [todo.md](todo.md):细粒度,**当前正在做的那一个 M 的具体动作清单**,日级更新,M 完结时清空
>
> 晋升规则与三档分工详见 [README.md](README.md)。

---

## 1. 当前状态

| 项 | 值 |
|---|---|
| 当前版本 | **v0.5.1** |
| 进行中 | **多轨剪辑器**(类 Premiere Pro 的多源/多轨/叠加剪辑,新 Tab),Phase A M1 待启动。详见 [milestones.md](milestones.md) |

---

## 2. 待启动功能(尚未排进 milestones)

> 这些是**已识别的功能机会**,尚未承诺开发时点。任意一项启动开发时,在 `milestones.md` 创建对应的里程碑表,并把本节那条标为"⏳ 进行中,见 milestones.md"。

### 2.1 媒体信息 Tab

- 实现难度低,纯展示
- 后端:`POST /api/media/info` → `ffprobe -v quiet -print_format json -show_format -show_streams`
- 前端:新 Tab,文件选择器 + 结构化展示(视频流 / 音频流 / 时长 / 码率 / 编码)
- 价值:打通"调 ffprobe → 解析 JSON → 展示"的样板,后续设置 Tab 可复用

### 2.2 设置 Tab

- 默认输出目录、默认编码器
- 语言切换(配合 i18n)
- FFmpeg 自定义附加参数(高级用户)
- 清理 `~/.easy-ffmpeg/` 旧缓存目录
- 当前 `config/config.go` 用两个 `.txt` 存路径,加任何配置项都需要先迁到 JSON

### 2.3 桌面版完善(v0.4.x 收尾)

- macOS / Linux 桌面版完整联调(目前阻塞于对应平台工具链)
- Wails 原生 dialog 替换自绘文件浏览器(可选增强)
- 系统托盘 / 最小化、原生菜单栏(File / Edit / Help)
- 自动更新 / 版本检查
- macOS 公证 + Windows 代码签名

详见 [core/desktop.md §15](core/desktop.md)。

### 2.4 远期功能候选(无承诺)

- 拖拽输入(把文件拖到浏览器窗口即自动填充)
- 批量转码(多输入文件队列)
- 预设系统(保存常用编码配置)
- 转码后动作(完成弹通知 / 打开输出目录 / 播放结果)
- 硬件加速选项(NVENC / QSV / VideoToolbox)
- 主题切换(深色 / 浅色)
- 国际化(zh-CN / en-US)
- WebCodecs 帧精确预览(替换当前关键帧对齐方案)

---

## 3. 非目标

明确**不做**的方向 — 任何把这些列入计划的提议都该先回到本节讨论。

- 云端功能 / 账号系统 / 协作
- 实时流处理
- 非 FFmpeg 的处理引擎
- 移动端 / 平板专门适配
- ~~嵌入 webview(走 Electron / Tauri / Wails 路线)~~ — **v0.4.0 撤销**:Wails 桌面版作为 Web 版的并列产物(不替换 Web 版)。详见 [core/desktop.md](core/desktop.md)

---

## 4. 已发布版本(粗粒度)

> 每行只记**版本主题**;具体里程碑(M1, M2, …)与 commit 见 [milestones.md](milestones.md) 的"已归档"区。

| 版本 | 主题 |
|------|------|
| **0.1.x** | Fyne 原型(早期桌面 GUI,视频转换功能首发) |
| **0.2.x** | HTTP + Web UI 重构(删除 Fyne,迁本地 HTTP + 浏览器前端;7z 嵌入 ffmpeg;音频处理 Tab 三模式) |
| **0.3.0** | 单视频剪辑器(时间轴式,替代旧裁剪 Tab,双轨独立编辑,工程 JSON 持久化) |
| **0.3.x** | 导出体验体系化(共享进度条 / dryRun 命令预览 / 自绘确认对话框 / 短轨自动 pad) |
| **0.4.0** | Wails 桌面版并列产物(共享后端字节相同,cgo 隔离在 `cmd/desktop/`) |
| **0.5.0–0.5.1** | 前端 Vue 化(Vue 3 + Vite + TS + Pinia + Tailwind;为多轨剪辑器铺路) |
