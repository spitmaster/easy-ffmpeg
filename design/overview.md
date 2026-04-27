# 项目总览

## 1. 项目定位

**Easy FFmpeg** 是一个跨平台的图形化 FFmpeg 前端。双击可执行文件后，程序自动在浏览器里打开一个本地 Web 界面，用户通过表单完成音视频处理——**交互模式类似 Jupyter Notebook**。

## 2. 目标用户

- 需要偶尔做格式转换、压缩、裁剪的普通用户（非开发者）
- 不想记忆 FFmpeg 命令行参数的视频创作者
- 需要批量处理但不想写脚本的轻度技术用户

## 3. 核心价值

| 价值点 | 说明 |
|--------|------|
| 零依赖分发 | FFmpeg 二进制以 7z 压缩包形式嵌入，首次启动自动解压；用户无需单独安装 FFmpeg |
| 体积小 | 7z LZMA2+BCJ2 压缩使最终产物降至 ~30 MB（未压缩方案需 ~200 MB） |
| 现代化 UI | 浏览器渲染，暗色主题，比原生 GUI 库更好看且定制自由 |
| 跨平台 | 一套 Go 代码通过 `CGO_ENABLED=0` 编译出 Windows / macOS (arm64+amd64) / Linux |
| 可视命令 | 界面实时显示即将执行的 FFmpeg 命令；执行前再以自绘对话框二次确认,提供一键复制供进阶用户离线使用 |
| 可取消 | 转码过程中可随时终止底层 FFmpeg 进程 |
| 进度可见 | 实时进度条解析 ffmpeg `time=` / `Duration:` 计算百分比,长任务也不焦虑 |
| 首次启动可视化 | 浏览器 + 控制台同时显示解压进度条，避免用户以为程序卡死 |

## 4. 技术栈

| 层面 | 技术选型 | 说明 |
|------|----------|------|
| 语言 | Go 1.21.3 | 单一二进制，纯 Go 静态编译（`CGO_ENABLED=0`）跨平台 |
| HTTP 服务 | `net/http` 标准库 | 零框架，仅一个 `ServeMux` |
| 前端 | 原生 HTML + CSS + JS，**无构建步骤** | 以 `go:embed` 打包进二进制 |
| 媒体处理 | FFmpeg 8.1 essentials（gyan.dev 构建） | 通过 `os/exec` 调用 |
| 归档 | `github.com/bodgit/sevenzip` | 纯 Go 实现，支持 LZMA2+BCJ2（7z 最优压缩格式） |
| SSE 事件流 | 手写 EventSource 协议 | 用于实时推送 ffmpeg 日志与解压进度 |
| 配置存储 | 纯文本文件 | `os.UserConfigDir()/easy-ffmpeg/*.txt` |
| 构建 | 两个脚本：`build.bat` / `build.sh` | 都一次性产出 4 个平台的产物 |

## 5. 关键架构决策

- **Jupyter 式而非 Electron**：不内置 webview，直接调用系统默认浏览器。优势：可执行文件只有 Go + 7z 大小，无 Chromium 几十 MB 包袱；劣势：不能隐藏地址栏，且依赖用户已安装浏览器。
- **纯前端无构建**：不引入 React/Vue/Tailwind，因为需求简单到一个 HTML 文件能搞定；换来零 Node 工具链依赖，开发周期更短。
- **7z 嵌入而非直接嵌二进制**：LZMA2+BCJ2 专门针对 x86/x64 可执行文件做了预处理压缩，`ffmpeg.exe + ffprobe.exe`（200MB）压到 28MB。代价是加一个 Go 依赖（`bodgit/sevenzip`）和首次启动 ~40 秒解压时间。
- **分平台嵌入**：每个构建只嵌入自己平台的 7z（通过构建标签），不把三个平台的 ffmpeg 都打进一个可执行文件。
- **解压缓存到用户主目录**：`~/.easy-ffmpeg/bin-<hash>/`，持久化、重启不重复解压。哈希是嵌入 7z 的 SHA256 前 8 位，程序升级后自动切换到新目录。
- **进度异步化**：`service.Prepare()` 放在 goroutine，浏览器立即打开，Web UI 轮询 `/api/prepare/status` 显示进度条。

## 6. 非目标

明确不做的事情，帮助保持产品聚焦：

- 不重新实现编解码逻辑，全部委托给 FFmpeg
- 不做服务器端批处理队列，纯桌面单机工具
- 不做云端同步、账号体系
- 短期不追求覆盖 FFmpeg 全部参数，只暴露最常用选项
- 不打包 webview（不走 Electron / Tauri / Wails 路线）
