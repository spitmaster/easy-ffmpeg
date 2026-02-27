# Easy FFmpeg

一个跨平台的图形化 FFmpeg 工具，让音视频处理变得更加简单直观。

## 功能特点

- 跨平台支持（Windows、macOS、Linux）
- 图形化界面，无需记忆复杂命令
- 常用音视频处理功能一键完成

### 视频处理

- 视频格式转换
- 视频压缩
- 视频裁剪
- 添加水印
- 视频合并

### 音频处理

- 音频格式转换
- 音频提取
- 音频压缩
- 音频合并

## 环境要求

- Go 1.21+

## 编译

本项目采用嵌入式 FFmpeg，编译后会自动包含 FFmpeg 二进制文件，无需系统安装 FFmpeg。

### Windows

```batch
# 执行编译脚本
build_windows.bat
```

编译输出：`dist/easy-ffmpeg.exe`

### macOS

```bash
# 执行编译脚本（需要先赋予权限）
chmod +x build_macos.sh
./build_macos.sh
```

编译输出：`dist/easy-ffmpeg`（通用二进制文件，支持 Intel 和 Apple Silicon）

### Linux

```bash
# 执行编译脚本（需要先赋予权限）
chmod +x build_linux.sh
./build_linux.sh
```

编译输出：`dist/easy-ffmpeg`

### 准备 FFmpeg 二进制文件

首次编译前，需要下载对应平台的 FFmpeg 二进制文件：

#### Windows
运行下载脚本：
```powershell
.\download_ffmpeg.ps1
```

或手动下载并放置到 `internal/embedded/windows/`

#### macOS
手动下载并放置到 `internal/embedded/darwin/`

下载地址：https://evermeet.cx/ffmpeg/

#### Linux
手动下载并放置到 `internal/embedded/linux/`

下载地址：https://johnvansickle.com/ffmpeg/releases/

## 使用方法

1. 运行编译后的程序
2. 选择需要的功能
3. 按照界面提示操作

**注意**：由于程序已内置 FFmpeg，无需系统安装 FFmpeg 即可运行。

## 技术栈

- Go
- Fyne (GUI框架)

## 许可证

MIT License
