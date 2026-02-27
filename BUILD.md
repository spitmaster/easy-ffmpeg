# 编译指南

本项目提供三个平台的编译脚本，只需在对应平台执行相应脚本即可完成编译。

## 快速开始

### Windows
```batch
build_windows.bat
```
输出：`dist/easy-ffmpeg.exe`

### macOS
```bash
# 首次使用需要赋予权限
chmod +x build_macos.sh
./build_macos.sh
```
输出：`dist/easy-ffmpeg`（通用二进制，支持 Intel + Apple Silicon）

### Linux
```bash
# 首次使用需要赋予权限
chmod +x build_linux.sh
./build_linux.sh
```
输出：`dist/easy-ffmpeg`

## 前置准备

编译前需要确保对应平台的 FFmpeg 二进制文件已放置在正确位置：

| 平台 | 目录路径 | 下载链接 |
|------|----------|----------|
| Windows | `internal/embedded/windows/` | 运行 `download_ffmpeg.ps1` |
| macOS | `internal/embedded/darwin/` | https://evermeet.cx/ffmpeg/ |
| Linux | `internal/embedded/linux/` | https://johnvansickle.com/ffmpeg/releases/ |

**需要放置的文件：**
- Windows: `ffmpeg.exe`, `ffprobe.exe`
- macOS: `ffmpeg`, `ffprobe`
- Linux: `ffmpeg`, `ffprobe`

## 编译选项说明

### Windows 编译参数
- `-ldflags="-s -w"`：减小可执行文件体积
- `-H=windowsgui`：隐藏控制台窗口

### macOS 编译参数
- 编译 AMD64 和 ARM64 两个版本
- 使用 `lipo` 合并为通用二进制文件
- 支持 Intel 和 Apple Silicon Mac

### Linux 编译参数
- `-ldflags="-s -w"`：减小可执行文件体积
- 静态链接，无需额外依赖

## 编译输出

编译后的文件位于 `dist/` 目录：

```
dist/
├── easy-ffmpeg.exe      # Windows 可执行文件
├── easy-ffmpeg          # macOS/Linux 可执行文件
```

## 常见问题

### Q: 编译后文件很大？
A: 因为 FFmpeg 二进制文件被嵌入到了程序中。完整版约 80-100MB，精简版约 20-30MB。

### Q: 如何减小文件体积？
A: 使用精简版 FFmpeg 二进制文件（git master builds），参考 EMBEDDED_SETUP.md。

### Q: macOS 编译时报错权限不足？
A: 运行 `chmod +x build_macos.sh` 赋予执行权限。

### Q: Linux 编译后如何运行？
A: 运行 `chmod +x dist/easy-ffmpeg` 赋予执行权限，然后 `./dist/easy-ffmpeg`。
