# 路线图与技术债

## 1. 短期功能迭代

按优先级：

### 1.0 单视频剪辑器（当前迭代 · v0.3.0）

- 替换旧的"视频裁剪"Tab，改为时间轴式单视频剪辑器
- 产品设计：[editor-feature-design.md](editor-feature-design.md)
- 代码架构：[editor-module-design.md](editor-module-design.md)
- 工程持久化到 `~/.easy-ffmpeg/projects/`（一工程一 JSON）
- 编辑器模块独立于其余功能，为未来"只剪辑"独立 exe 铺路
- 旧 trim 相关的 `server/handlers_trim.go` / `server/trim_args.go` / `server/trim_args_test.go` / `design/trim-feature-design.md` 整体删除

### 1.1 媒体信息 Tab（优先级最高，实现最简单）

- 实现难度低，纯展示，不涉及长耗时
- 已嵌入 ffprobe，只需：
  - 后端：`POST /api/media/info` 接收路径 → `ffprobe -v quiet -print_format json -show_format -show_streams` → 返回 JSON
  - 前端：新 Tab，文件选择器 + 结构化展示（视频/音频流、时长、码率、编码）
- 是打通 `service` 层调 FFprobe → 解析 JSON → 展示的样板

### 1.2 设置 Tab

- 默认输出目录、默认编码器
- 语言切换（配合 i18n）
- 是否显示 ffprobe 相关功能
- 清理 `~/.easy-ffmpeg/` 旧缓存目录
- FFmpeg 自定义附加参数（高级用户）

## 2. 现有代码技术债

### 2.1 `/api/fs/reveal` 缺乏路径白名单

目前任何本地路径都可以 POST 过去打开。由于服务只监听 127.0.0.1，只有本机进程能访问，风险可控；但如果未来要支持 LAN 访问或容器化，必须加：
- 只允许打开存在于 `~/` 或已保存目录下的路径
- 或至少要求先通过 `/api/fs/list` 校验

### 2.2 `config/config.go` 存储方案过于简陋

目前两个目录各存一个 `.txt`。加几个配置项就应该迁到 JSON：

```json
{
  "inputDir": "...",
  "outputDir": "...",
  "defaultVideoEncoder": "h264",
  "language": "zh-CN"
}
```

### 2.3 `handleConvertStart` 的参数构建太简陋

- 视频选 `copy` + 音频选非 `copy` 仍会走 `-c:v copy -c:a aac` —— 技术上可行，但不是"只换容器"的语义
- 没有比特率、分辨率、帧率等高级参数
- 没有 `-preset`、`-crf` 等质量控制

### 2.4 旧 Fyne 时代 / 根级文档的残留

- `tools/download_windows.go`：原 Fyne 时代的 ffmpeg 下载工具。现在 7z 方案下需要手动打包 7z，此工具路径失效。保留作为历史参考。
- `model/`：空目录，`STRUCTURE.md` 里有提但没内容。要么填充，要么删除。
- `EMBEDDED_SETUP.md`、`STRUCTURE.md`、`BUILD.md`：和 `design/` 有重复/过时内容（例如 `STRUCTURE.md` 里还在讲 `ui/ui.go` 的 Fyne 时代结构）。考虑合并到 `design/` 或直接删除。

### 2.5 `/api/fs/reveal` 在 macOS / Linux 的行为一致性

- Windows `cmd /c start "" <path>`：对目录用 Explorer 打开
- macOS `open <path>`：对目录用 Finder 打开 ✓
- Linux `xdg-open <path>`：依赖桌面环境（GNOME Files / Dolphin / Nautilus）

需要在 Linux 多桌面环境下测试。

### 2.6 测试覆盖仍偏薄

目前已有的测试：
- `server/audio_args_test.go`：convert / extract / merge 三模式的正反路径、concat 列表单引号转义、bitrate 条件矩阵
- `server/trim_args_test.go`：trim/crop/scale 组合、时间解析、保持比例 `-2` 语义

还缺的关键测试：
- `handlers.buildFFmpegArgs`（convert 分支）
- `job.scanLinesOrCR`（\r \n \r\n 混合）
- 前端 `parseFFmpegVersion` / `Time.parse` 正则（目前只在 Go 侧测了类似的 `parseTimeSeconds`）
- `service.ProbeAudio / ProbeVideo` 的 JSON 解析（可以用 testdata 固定样例，不调真实 ffprobe）

### 2.7 解压过程的 UX 细节

- 解压速度慢（25-45s）。若未来硬件更慢或嵌入更大，可考虑：
  - 预估剩余时间（用平均速度 + 剩余字节）
  - 多线程解压（sevenzip 库似乎单线程）
  - 切换压缩格式（zstd 比 LZMA 快很多，但压缩比稍弱）
- 解压失败后没有重试按钮；用户只能手动删除 `~/.easy-ffmpeg/` 重启

### 2.8 SSE 连接处理

- `handleConvertStream` 在客户端断开时 `ctx.Done()` 正常清理
- 但如果服务端长期运行 + 客户端反复刷新，`subscribers` map 的清理依赖 `defer unsub()`。可以用 leak 检测验证

### 2.9 ~~Tab 切换逻辑~~（已完成）

Tab 切换已在 `app.js` 的 `Tabs` IIFE 中落地：识别 `[data-tab]`、给对应 button 加 `.active`、按 id 切 `.panel .hidden`。加 Tab 时不用再改切换代码。

## 3. 功能增强候选

- ~~**进度百分比**：解析 FFmpeg stderr 中的 `time=` + ffprobe 的总时长，换算百分比~~ —— v0.3.x 落地：`createJobPanel` 解析 `time=HH:MM:SS.ms` 与 `Duration:`，三 Tab 共享的进度条实时刷新
- **拖拽输入**：支持把文件拖到浏览器窗口即自动填充
- **批量转码**：多个输入文件队列执行
- **预设系统**：保存常用编码配置
- **最近文件列表**：不只记目录，还记文件
- **转码后动作**：完成时弹通知 / 打开输出目录 / 播放结果
- **硬件加速选项**：NVENC / QSV / VideoToolbox 下拉
- **主题切换**：深色 / 浅色
- **国际化**：至少支持 zh-CN / en-US
- **WebCodecs 帧精确预览**：当前剪辑器预览精度对齐到关键帧（0~1.5s 误差）；用 MP4Box.js + WebCodecs 可做到帧精确

## 4. 工程化候选

- **CI 构建**：GitHub Actions 三平台矩阵
- **Release 自动化**：tag 触发构建 + 签名 + 上传 artifact
- **版本号注入**：`-ldflags "-X main.Version=..."`；UI 右下角/关于对话框显示
- **崩溃上报**：捕获 panic 写入本地文件
- **嵌入 7z 自动化**：写一个 Go 工具一键从各平台源下载 + 打包（替代当前手动步骤）
- **端到端测试**：headless browser 操作 UI + 实际跑小文件转码
- **减少嵌入体积的激进方案**：
  - 改用 stdlib gzip（简单，体积 +20-30MB）
  - 自编译 minimal ffmpeg（工作量大，体积 -50MB+）
  - UPX 加壳（运行时解压到内存，体积 -40MB，但可能被 AV 误报）

## 5. 已完成的历史里程碑

| 大版本 | 阶段主题 | 关键改动汇总 |
|--------|---------|------|
| **0.1.x** | Fyne 原型 | 早期桌面 GUI 原型，视频转换功能落地；文件对话框、配置持久化、状态栏 |
| **0.2.x** | HTTP + Web UI 重构 | 删除 Fyne，整体迁移到本地 HTTP + 浏览器前端；`go:embed` 嵌入 FFmpeg（先全嵌，再按平台构建标签 195MB → 35MB；最后改 7z 压缩 → ~30MB 首启动解压）；控制台进度条 + 浏览器遮罩；FFmpeg 版本 chip + 缓存目录打开；统一构建脚本 `build.bat` / `build.sh`；音频处理 Tab 三模式（格式转换 / 提取 / 合并）；`app.js` 重构为模块化 IIFE；旧"视频裁剪"Tab 在 0.2.x 末期上线（0.3.0 被剪辑器替换） |
| **0.3.0** | 单视频剪辑器 | 时间轴式单视频剪辑器（替代旧裁剪），双轨独立编辑；独立 `editor/` 模块（domain / ports / storage / api 严格分层）；工程 JSON 持久化（SchemaVersion 演进到 3，支持 ProgramStart / AudioVolume）；`/api/editor/*` 端点；空隙第一类公民（filter graph 用 `color`/`anullsrc` 填补，预览端 gap clock 同步黑屏）；范围选区右键拖动；音频轨音量浮窗 0–200%（WebAudio gain + ffmpeg `volume=` 滤镜）；导出日志侧栏占满整个剪辑功能区右侧（导出时 main 撤掉 1200px 上限） |
| **0.3.x** | 导出体验体系化 + 修补 | 三 Tab 共享的实时进度条（解析 `time=` / `Duration:`）；dryRun 命令预览 dialog（拉真实 ffmpeg 命令一键复制再确认执行）；自绘覆盖确认 dialog 替代原生 `confirm`；所有模态去掉点背景关闭，统一 × / Esc / 取消关闭；导出末端短轨自动 pad 黑屏 / 静音保证两流同长（修复预览停在视频结尾的 bug）；`mac .app` 打包路径移除（保留 darwin 原生二进制） |

## 6. 非目标（本阶段不做）

- 云端功能 / 账号系统
- 实时流处理
- 高级视频剪辑（多素材拼接、多轨道叠加 / PiP / 转场 / 调色 / 滤镜 / 关键帧动画等专业场景）
- 非 FFmpeg 的处理引擎
- 移动端 / 平板专门适配
- 嵌入 webview（走 Electron / Tauri / Wails 路线）
