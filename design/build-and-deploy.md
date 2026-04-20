# 构建与部署

## 1. 构建矩阵

| 平台 | 产物 | 大小 | CPU |
|------|------|------|-----|
| Windows | `dist/easy-ffmpeg.exe` | ~35 MB | amd64 |
| macOS Apple Silicon | `dist/easy-ffmpeg-macos-arm64` | ~27 MB | arm64 (M1/M2/M3/M4) |
| macOS Intel | `dist/easy-ffmpeg-macos-amd64` | ~27 MB | amd64 |
| Linux | `dist/easy-ffmpeg-linux` | ~29 MB | amd64 |

## 2. 构建脚本

项目仓库根目录提供两个脚本，**各自**一次性产出全部 4 个平台的产物：

| 脚本 | 平台 | 用法 |
|------|------|------|
| `build.bat` | Windows (cmd.exe / PowerShell) | `build.bat` 或双击 |
| `build.sh`  | macOS / Linux / Windows Git Bash | `bash build.sh` 或 `./build.sh` |

两个脚本行为完全一致：
1. 切到脚本所在目录
2. 创建 `dist/`（存在则跳过）
3. 依次编译 4 个产物，任一失败立即停下
4. 列出产物

### `build.sh` 核心逻辑

```bash
build() {
    CGO_ENABLED=0 GOOS="$1" GOARCH="$2" \
        go build -ldflags="-s -w" -o "dist/$3" ./cmd
}

build windows amd64 easy-ffmpeg.exe
build darwin  arm64 easy-ffmpeg-macos-arm64
build darwin  amd64 easy-ffmpeg-macos-amd64
build linux   amd64 easy-ffmpeg-linux
```

### 为什么能从 Windows 跨平台编译？

- `CGO_ENABLED=0`：项目无 cgo，纯 Go 静态链接
- Go 的跨平台编译是内建能力；只需设 `GOOS`/`GOARCH` 环境变量
- 依赖的库（`github.com/bodgit/sevenzip` 等）全是纯 Go

### 编译选项

- `-ldflags="-s -w"`：砍掉符号表（`-s`）和调试信息（`-w`），减小体积约 30%
- `./cmd`：包路径，编译 `cmd/main.go`

## 3. FFmpeg 二进制准备

每个平台的 ffmpeg+ffprobe 必须以 7z 压缩包形式放在：

```
internal/embedded/windows/windows.7z
internal/embedded/darwin/darwin.7z
internal/embedded/linux/linux.7z
```

可以**手动打包**（推荐）：

```bash
# 1. 下载对应平台的 ffmpeg / ffprobe 原生二进制
#    Windows: https://www.gyan.dev/ffmpeg/builds/ → ffmpeg-release-essentials
#    macOS:   https://evermeet.cx/ffmpeg/
#    Linux:   https://johnvansickle.com/ffmpeg/releases/

# 2. 用 7z 打包（solid 模式，最高压缩，BCJ2 过滤器自动启用）
7z a -mx=9 -ms=on internal/embedded/windows/windows.7z \
    ffmpeg.exe ffprobe.exe
```

**关键参数**：
- `-mx=9`：最高压缩级别
- `-ms=on`：solid 模式，多文件当成单一数据流压缩（ffmpeg + ffprobe 共享代码段，合一压缩可省 30-50%）
- BCJ2 x86 可执行文件预处理器 7z 会自动对 `.exe` 启用

**压缩效果**（8.1 essentials）：
| 文件 | 原始 | solid 7z |
|------|------|----------|
| ffmpeg.exe + ffprobe.exe | 202 MB | 28 MB |
| ffmpeg + ffprobe (darwin) | 160 MB | 20 MB |
| ffmpeg + ffprobe (linux) | 158 MB | 22 MB |

## 4. 嵌入机制（go:embed）

每个平台有独立的嵌入文件，通过构建标签控制：

```go
// internal/embedded/embedded_windows.go
//go:build windows

//go:embed windows/windows.7z
var archiveData []byte

const (
    ffmpegBinaryName  = "ffmpeg.exe"
    ffprobeBinaryName = "ffprobe.exe"
)
```

**好处**：编 Windows 时不会把 darwin 和 linux 的 7z 也嵌进去。

## 5. 首次启动解压流程

```
用户双击 easy-ffmpeg.exe
    │
    ▼
main.go 启动 HTTP server 于 127.0.0.1:随机端口
    │
    ├─ go service.Prepare() 后台启动解压
    │     └─ 读 embedded 7z (28MB)
    │     └─ sha256(archiveData)[:4].hex 生成 hash
    │     └─ 目标路径 ~/.easy-ffmpeg/bin-<hash>/
    │     └─ 检查 .ok 标记文件
    │        ├─ 存在 → 跳过，立即 setProgress(ready)
    │        └─ 不存在 → sevenzip 逐个文件解压
    │           └─ progressWriter 累计字节，更新 percent
    │           └─ 启动控制台进度条 goroutine（\r 重绘）
    │           └─ 完成后写 .ok 标记
    │
    └─ browser.Open(url) 立即打开浏览器
        └─ 前端轮询 /api/prepare/status
           └─ 显示遮罩 + 进度条，直到 state=ready
```

**缓存路径**（`~/.easy-ffmpeg/bin-<hash>/`）：
- Windows: `C:\Users\<user>\.easy-ffmpeg\bin-b9b48d4f\`
- macOS: `/Users/<user>/.easy-ffmpeg/bin-<hash>/`
- Linux: `/home/<user>/.easy-ffmpeg/bin-<hash>/`

**升级行为**：7z 内容变化 → hash 变化 → 自动使用新目录；老的 `bin-<oldhash>/` 保留，可手动清理。

## 6. 用户首次启动时间

本机实测（纯 Go 7z 解压比原生 `7z.exe` 慢约 5-10 倍）：

| 平台 | 首次启动 | 后续启动 |
|------|----------|----------|
| Windows (Intel i5/i7) | 25-45 秒 | <1 秒 |
| macOS (M1) | 15-25 秒（推测） | <1 秒 |
| Linux (现代 amd64) | 15-30 秒 | <1 秒 |

首次启动慢主要因为 `bodgit/sevenzip` 的 LZMA2+BCJ2 解码是纯 Go 实现。若未来需要加速，可以考虑：
- 切换到 gzip/zstd（压缩比稍逊，但解码快 3-5 倍）
- 引入 cgo LZMA 绑定（但破坏纯 Go 跨平台编译）

## 7. Windows 平台特殊处理

- **ffmpeg 子进程窗口隐藏**：`internal/job/hide_windows.go` 用 `CREATE_NO_WINDOW`（`0x08000000`）标志，防止每次转码弹黑窗
- **路径规范化**：前端 / 后端处理时统一用正斜杠 `/`（`filepath.ToSlash`），显示层面也用 `/`；真正跟系统 API 打交道才转反斜杠
- **盘符枚举**：`listWindowsDrives()` 从 A 到 Z 逐个 `os.Stat(X:\)`，作为文件浏览器的"盘符切换下拉"

## 8. 常见构建陷阱

| 问题 | 原因 | 解决 |
|------|------|------|
| `embedded_darwin.go: pattern darwin/darwin.7z: no matching files found` | 对应平台的 7z 不存在 | 准备好所有 3 个平台的 .7z 文件后再编译 |
| 编译后体积过大 | 可能误嵌了未压缩的二进制 | 确认 `embedded_<os>.go` 的 embed 指令是 `<os>/<os>.7z` 而非目录 |
| Windows 运行弹出 cmd 黑窗 | 老版本 `-H=windowsgui` 被移除（Web 模式需要控制台） | 正常行为；未来若要纯后台模式再讨论 |
| 跨编译报 cgo 错误 | 某处引入了 cgo | 检查 `CGO_ENABLED=0`，必要时 `go build -x` 诊断 |
| macOS 运行报"无法打开" | Gatekeeper 未签名警告 | 右键"打开"或 `xattr -d com.apple.quarantine <path>` |
| macOS ARM64 运行失败 | 嵌入的是 amd64 ffmpeg | 确认 `darwin.7z` 里是 arm64 构建的 ffmpeg |

## 9. 发布与分发（未实现）

当前没有 CI/CD。未来可做：
- GitHub Actions 矩阵构建（三平台 4 产物）
- 打 tag 自动 release
- macOS 开发者签名 + 公证（避免 Gatekeeper 警告）
- Windows 代码签名（避免 SmartScreen 警告）
- Linux 分发：AppImage / Flatpak / deb / rpm

## 10. 版本号注入（暂未使用）

推荐的做法（供未来参考）：

```bash
go build -ldflags="-s -w -X main.Version=$(git describe --tags)" ...
```

```go
// main.go
var Version = "dev"

// 暴露在 /api/version 或 UI 右下角
```
