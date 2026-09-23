# 产品总览

## 1. 项目定位

**Easy FFmpeg** 是一个跨平台的图形化 FFmpeg 前端。双击可执行文件后,程序自动在浏览器里打开一个本地 Web 界面,用户通过表单完成音视频处理 —— **交互模式类似 Jupyter Notebook**。

v0.4.0 起,与 Web 版**并列**输出 Wails 桌面版:双击启动一个独立窗口、内置 WebView,共享同一份后端代码。两者可在同一台机器共存,用户可随时回退。详见 [desktop.md](desktop.md)。

## 2. 目标用户

- 需要偶尔做格式转换、压缩、裁剪的普通用户(非开发者)
- 不想记忆 FFmpeg 命令行参数的视频创作者
- 需要批量处理但不想写脚本的轻度技术用户

## 3. 核心价值

| 价值点 | 说明 |
|--------|------|
| 零依赖分发 | FFmpeg 二进制以 7z 压缩包形式嵌入,首次启动自动解压;用户无需单独安装 FFmpeg |
| 体积小 | 7z LZMA2+BCJ2 压缩使最终产物降至 ~30 MB(未压缩方案需 ~200 MB) |
| 现代化 UI | 浏览器渲染,暗色主题,比原生 GUI 库更好看且定制自由 |
| 跨平台 | Web 版用 `CGO_ENABLED=0` 一份代码编出 Windows / macOS (arm64+amd64) / Linux 四产物 |
| 可视命令 | 界面实时显示即将执行的 FFmpeg 命令;执行前再以自绘对话框二次确认,提供一键复制供进阶用户离线使用 |
| 可取消 | 转码过程中可随时终止底层 FFmpeg 进程 |
| 进度可见 | 实时进度条解析 ffmpeg `time=` / `Duration:` 计算百分比,长任务也不焦虑 |
| 首次启动可视化 | 浏览器 + 控制台同时显示解压进度条,避免用户以为程序卡死 |
| 双产物 | Web 版(浏览器)+ 桌面版(Wails 独立窗口),共享 `~/.easy-ffmpeg/`、可任意切换 |

## 4. 关键产品决策

- **Jupyter 式启动**:Web 版不内置 webview,直接调用系统默认浏览器。优势:可执行文件只有 Go + 7z 大小,无 Chromium 几十 MB 包袱;劣势:不能隐藏地址栏,且依赖用户已安装浏览器。**桌面版用 Wails 内置 WebView 弥补了"不爱开浏览器"的用户**,且不替换 Web 版。
- **纯前端无构建**:不引入 React/Vue/Tailwind,因为需求简单到一个 HTML 文件能搞定;换来零 Node 工具链依赖,开发周期更短。
- **7z 嵌入而非直接嵌二进制**:LZMA2+BCJ2 专门针对 x86/x64 可执行文件做了预处理压缩,`ffmpeg.exe + ffprobe.exe`(200 MB)压到 28 MB。代价是加一个 Go 依赖(`bodgit/sevenzip`)和首次启动 ~40 秒解压时间。
- **解压缓存到用户主目录**:`~/.easy-ffmpeg/bin-<hash>/`,持久化、重启不重复解压。哈希是嵌入 7z 的 SHA256 前 8 位,程序升级后自动切换到新目录。
- **进度异步化**:`service.Prepare()` 放在 goroutine,浏览器立即打开,Web UI 轮询 `/api/prepare/status` 显示进度条。

## 5. 非目标

明确不做的事情,帮助保持产品聚焦:

- 不重新实现编解码逻辑,全部委托给 FFmpeg
- 不做服务器端批处理队列,纯桌面单机工具
- 不做云端同步、账号体系
- 短期不追求覆盖 FFmpeg 全部参数,只暴露最常用选项
- 不做高级剪辑(多素材拼接、多轨道叠加 / PiP / 转场 / 调色 / 滤镜 / 关键帧动画等专业场景)
- 不做移动端 / 平板专门适配
- ~~不打包 webview(不走 Electron / Tauri / Wails 路线)~~ —— **v0.4.0 撤销**:Web 版仍是核心,桌面版以 Wails 外壳形式作为可选并列产物;两者共享同一份后端代码,Web 版不退场。详见 [desktop.md](desktop.md)。

## 6. 全局功能(Tab 无关)

- ✅ FFmpeg 版本显示(右上角 chip,含"嵌入"/"系统"标识)
- ✅ 点击 chip 在文件管理器打开 FFmpeg 缓存目录
- ✅ 输出目录"打开"按钮(📂)
- ✅ 首次启动解压进度条(浏览器遮罩 + 控制台 `\r` 重绘)
- ✅ 优雅关停(右上角"退出"按钮 / Ctrl+C)
- ✅ 输入输出目录记忆
- ✅ 程序版本号 chip(`-ldflags -X` 注入)

## 7. Tab 全景

| Tab | 状态 | 详细文档 |
|-----|------|---------|
| 视频转换 | ✅ 已实现 | [tabs/convert/product.md](../tabs/convert/product.md) |
| 音频处理(三模式) | ✅ 已实现 | [tabs/audio/product.md](../tabs/audio/product.md) |
| 单视频剪辑器 | ✅ 已实现(v0.3.0) | [tabs/editor/product.md](../tabs/editor/product.md) |
| 媒体信息 | 🚧 占位 | 见 [roadmap.md](roadmap.md) §1.1 |
| 设置 | 🚧 占位 | 见 [roadmap.md](roadmap.md) §1.2 |
