# 嵌入 FFmpeg 二进制文件指南

本项目使用 Go 的 `embed` 指令将 FFmpeg 和 FFprobe 二进制文件直接嵌入到程序中，使编译后的程序可以独立运行，不依赖系统安装的 FFmpeg。

## 下载 FFmpeg 二进制文件

### Windows 平台
```bash
# 下载静态编译的 FFmpeg（包含 ffmpeg.exe 和 ffprobe.exe）
# 推荐来源：https://www.gyan.dev/ffmpeg/builds/

# 下载 ffmpeg-release-essentials.zip
# 解压后将 bin/ffmpeg.exe 和 bin/ffprobe.exe 复制到：
internal/embedded/windows/
```

### macOS 平台
```bash
# 使用 Homebrew 下载
brew install ffmpeg

# 复制二进制文件
cp $(which ffmpeg) internal/embedded/darwin/
cp $(which ffprobe) internal/embedded/darwin/

# 或者从 https://evermeet.cx/ffmpeg/ 下载静态编译版本
```

### Linux 平台
```bash
# 从 https://johnvansickle.com/ffmpeg/ 下载静态编译版本
wget https://johnvansickle.com/ffmpeg/releases/ffmpeg-release-amd64-static.tar.xz
tar -xf ffmpeg-release-amd64-static.tar.xz
cp ffmpeg-*/ffmpeg internal/embedded/linux/
cp ffmpeg-*/ffprobe internal/embedded/linux/
```

## 编译特定平台

### 编译 Windows 版本
```bash
# 确保已将 ffmpeg.exe 和 ffprobe.exe 放在 internal/embedded/windows/
GOOS=windows GOARCH=amd64 go build -o easy-ffmpeg.exe ./cmd
```

### 编译 macOS 版本
```bash
# 确保已将 ffmpeg 和 ffprobe 放在 internal/embedded/darwin/
GOOS=darwin GOARCH=amd64 go build -o easy-ffmpeg-mac ./cmd
GOOS=darwin GOARCH=arm64 go build -o easy-ffmpeg-mac-arm64 ./cmd
```

### 编译 Linux 版本
```bash
# 确保已将 ffmpeg 和 ffprobe 放在 internal/embedded/linux/
GOOS=linux GOARCH=amd64 go build -o easy-ffmpeg-linux ./cmd
```

## 跨平台编译（从 Windows 编译其他平台）

从 Windows 编译时，需要确保对应平台的二进制文件已放置在正确的目录中：

```bash
# 编译 Linux 版本（需要 Linux 的 ffmpeg 二进制文件）
set GOOS=linux
set GOARCH=amd64
go build -o easy-ffmpeg-linux ./cmd

# 编译 macOS 版本（需要 macOS 的 ffmpeg 二进制文件）
set GOOS=darwin
set GOARCH=amd64
go build -o easy-ffmpeg-mac ./cmd
```

## 注意事项

1. **二进制文件大小**：嵌入 FFmpeg 二进制文件后，程序体积会增加约 50-80MB
2. **权限**：确保二进制文件具有执行权限（在 Linux/macOS 上使用 `chmod +x`）
3. **版本一致性**：建议所有平台使用相同版本的 FFmpeg 以保证功能一致性
4. **降级机制**：即使嵌入的二进制不可用，程序仍会尝试使用系统 FFmpeg

## 验证嵌入是否成功

编译后运行程序，检查日志中的 FFmpeg 版本信息：
```bash
./easy-ffmpeg
# 查看输出，应显示 "使用嵌入的FFmpeg"
```

## 更新 FFmpeg 版本

1. 下载新版本的 FFmpeg 二进制文件
2. 替换对应平台目录下的旧文件
3. 重新编译程序

## 推荐的 FFmpeg 版本

- 稳定版：6.x 或更高
- 编译类型：静态编译（static），以避免依赖问题
- 组件：确保包含常用的编码器（如 libx264, libvpx, aac 等）
