# 构建与部署(程序设计)

> 本文档定义构建脚本、跨平台编译、7z 嵌入、首次启动解压、桌面版构建。桌面版的架构设计见 [desktop.md](desktop.md)。

## 1. 构建矩阵

### 1.1 Web 版(共享层)

| 平台 | 产物 | 大小 | CPU |
|------|------|------|-----|
| Windows | `dist/easy-ffmpeg.exe` | ~35 MB | amd64 |
| macOS Apple Silicon | `dist/easy-ffmpeg-macos-arm64` | ~27 MB | arm64 (M1/M2/M3/M4) |
| macOS Intel | `dist/easy-ffmpeg-macos-amd64` | ~27 MB | amd64 |
| Linux | `dist/easy-ffmpeg-linux` | ~29 MB | amd64 |

### 1.2 桌面版(v0.4.0+,Wails)

| 产物 | CGO | 编译方式 | 大小估计 |
|------|-----|---------|----------|
| `easy-ffmpeg-desktop.exe`(桌面 Win) | 1 | 仅 Windows 本机 | ~40–50 MB(Web + WebView2 胶水) |
| `easy-ffmpeg-desktop-macos-arm64.app` | 1 | 仅 macOS 本机 | ~32 MB |
| `easy-ffmpeg-desktop-macos-amd64.app` | 1 | 仅 macOS 本机 | ~32 MB |
| `easy-ffmpeg-desktop-linux` | 1 | 仅 Linux 本机 | ~35 MB(动态链接 libwebkit2gtk) |

桌面版 `.app` / `.exe` 内部仍嵌入相同的 `<os>.7z`,首次启动解压目录与 Web 版**共享**(`~/.easy-ffmpeg/bin-<hash>/`)—— 两个产物可在同一台机器共存,缓存复用。

## 2. 构建脚本

项目仓库根目录提供两个脚本,**各自**先把 Vue 前端编进 `web/dist/`,再一次性产出全部 4 个 Web 平台产物 + 当前平台可编的桌面版:

| 脚本 | 平台 | 用法 |
|------|------|------|
| `build.bat` | Windows (cmd.exe / PowerShell) | `build.bat` 或双击 |
| `build.sh`  | macOS / Linux / Windows Git Bash | `bash build.sh` 或 `./build.sh` |

两个脚本行为:

1. 切到脚本所在目录,创建 `dist/`(存在则跳过)
2. **前端构建**:检查 `npm` 在 PATH;`cd web && npm install --no-audit --no-fund && npm run build` 把 Vue 源码编入 `web/dist/`
3. 依次编译 4 个 Web 产物(Go 通过 `easy-ffmpeg/web` 包嵌入 `web/dist/`),任一失败立即停下
4. 检测 `wails` 命令是否存在 → 在场则追加桌面版构建分支(按当前 OS 决定能编哪个)
5. 列出产物

> 顺序关键:Go 编译 import `easy-ffmpeg/web`,该包通过 `//go:embed all:dist` 引用 `web/dist/`。如果先 `go build` 而 `web/dist/` 不存在或过期,前端会缺资源。所以两个脚本都把"前端构建"放在 Go 构建之前,且任一步失败立即终止。

### 2.1 `build.sh` 核心逻辑

```bash
# 前端构建(放最前)— v0.5.0 起 Vue 工程在 web/,产物在 web/dist/。
build_frontend() {
    if ! command -v npm >/dev/null 2>&1; then
        echo "ERROR: npm not found. Install Node.js >= 20 to build the frontend." >&2
        return 1
    fi
    (cd web && npm install --no-audit --no-fund && npm run build)
}
build_frontend

build() {
    CGO_ENABLED=0 GOOS="$1" GOARCH="$2" \
        go build -ldflags="$LDFLAGS" -o "dist/$3" ./cmd
}

build windows amd64 easy-ffmpeg.exe
build darwin  arm64 easy-ffmpeg-macos-arm64
build darwin  amd64 easy-ffmpeg-macos-amd64
build linux   amd64 easy-ffmpeg-linux

# 桌面版分支(当前平台可编时追加,详见 §8)
```

### 2.2 `build.bat` 前端段

```batch
where npm >nul 2>&1 || (echo ERROR: npm not found && goto :fail)
pushd web
call npm install --no-audit --no-fund || (popd & goto :fail)
call npm run build                    || (popd & goto :fail)
popd
```

`npm run build` 执行 `vue-tsc --noEmit && vite build`,既做类型检查又生成产物;任一阶段失败直接退出。

### 2.3 开发态(不走脚本)

调试时跑两个进程:

```bash
go run ./cmd          # 后端 8080
cd web && npm run dev # Vite dev server 5173,/api/* 经 vite.config.ts 代理到 8080
```

Hot Reload + 真实后端 API。生产构建仍按 §2.1 的顺序走。

### 2.4 为什么 Web 版能从 Windows 跨编 4 平台?

- `CGO_ENABLED=0`:项目共享层无 cgo,纯 Go 静态链接
- Go 的跨平台编译是内建能力;只需设 `GOOS`/`GOARCH` 环境变量
- 依赖的库(`github.com/bodgit/sevenzip` 等)全是纯 Go

### 2.3 编译选项

- `-ldflags="-s -w"`:砍掉符号表(`-s`)和调试信息(`-w`),减小体积约 30%
- `-ldflags "-X main.Version=..."`:注入版本号,运行时 `/api/version` 返回
- `./cmd`:包路径,编译 `cmd/main.go`(Web 版)
- `./cmd/desktop`:桌面版入口(Wails)

## 3. FFmpeg 二进制准备

每个平台的 ffmpeg+ffprobe 必须以 7z 压缩包形式放在:

```text
internal/embedded/windows/windows.7z
internal/embedded/darwin/darwin.7z
internal/embedded/linux/linux.7z
```

### 3.1 手动打包(推荐)

```bash
# 1. 下载对应平台的 ffmpeg / ffprobe 原生二进制
#    Windows: https://www.gyan.dev/ffmpeg/builds/ → ffmpeg-release-essentials
#    macOS:   https://evermeet.cx/ffmpeg/
#    Linux:   https://johnvansickle.com/ffmpeg/releases/

# 2. 用 7z 打包(solid 模式,最高压缩,BCJ2 过滤器自动启用)
7z a -mx=9 -ms=on internal/embedded/windows/windows.7z \
    ffmpeg.exe ffprobe.exe
```

**关键参数**:

- `-mx=9`:最高压缩级别
- `-ms=on`:solid 模式,多文件当成单一数据流压缩(ffmpeg + ffprobe 共享代码段,合一压缩可省 30-50%)
- BCJ2 x86 可执行文件预处理器 7z 会自动对 `.exe` 启用

### 3.2 压缩效果(8.1 essentials)

| 文件 | 原始 | solid 7z |
|------|------|----------|
| ffmpeg.exe + ffprobe.exe | 202 MB | 28 MB |
| ffmpeg + ffprobe (darwin) | 160 MB | 20 MB |
| ffmpeg + ffprobe (linux) | 158 MB | 22 MB |

## 4. 嵌入机制(go:embed)

每个平台有独立的嵌入文件,通过构建标签控制:

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

**好处**:编 Windows 时不会把 darwin 和 linux 的 7z 也嵌进去。

## 5. 首次启动解压流程

```text
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
    │        ├─ 存在 → 跳过,立即 setProgress(ready)
    │        └─ 不存在 → sevenzip 逐个文件解压
    │           └─ progressWriter 累计字节,更新 percent
    │           └─ 启动控制台进度条 goroutine(\r 重绘)
    │           └─ 完成后写 .ok 标记
    │
    └─ browser.Open(url) 立即打开浏览器
        └─ 前端轮询 /api/prepare/status
           └─ 显示遮罩 + 进度条,直到 state=ready
```

**缓存路径**(`~/.easy-ffmpeg/bin-<hash>/`):

- Windows: `C:\Users\<user>\.easy-ffmpeg\bin-b9b48d4f\`
- macOS: `/Users/<user>/.easy-ffmpeg/bin-<hash>/`
- Linux: `/home/<user>/.easy-ffmpeg/bin-<hash>/`

**升级行为**:7z 内容变化 → hash 变化 → 自动使用新目录;老的 `bin-<oldhash>/` 保留,可手动清理。

## 6. 用户首次启动时间

本机实测(纯 Go 7z 解压比原生 `7z.exe` 慢约 5-10 倍):

| 平台 | 首次启动 | 后续启动 |
|------|----------|----------|
| Windows (Intel i5/i7) | 25-45 秒 | <1 秒 |
| macOS (M1) | 15-25 秒(推测) | <1 秒 |
| Linux (现代 amd64) | 15-30 秒 | <1 秒 |

首次启动慢主要因为 `bodgit/sevenzip` 的 LZMA2+BCJ2 解码是纯 Go 实现。若未来需要加速,可以考虑:

- 切换到 gzip/zstd(压缩比稍逊,但解码快 3-5 倍)
- 引入 cgo LZMA 绑定(但破坏纯 Go 跨平台编译)

## 7. Windows 平台特殊处理

- **ffmpeg 子进程窗口隐藏**:`internal/procutil/hide_windows.go` 用 `CREATE_NO_WINDOW`(`0x08000000`)标志,防止每次转码弹黑窗
- **路径规范化**:前端 / 后端处理时统一用正斜杠 `/`(`filepath.ToSlash`),显示层面也用 `/`;真正跟系统 API 打交道才转反斜杠
- **盘符枚举**:`listWindowsDrives()` 从 A 到 Z 逐个 `os.Stat(X:\)`,作为文件浏览器的"盘符切换下拉"

## 8. 桌面版构建(v0.4.0+)

详见 [desktop.md §6](desktop.md)。要点:

### 8.1 工具链前置依赖

桌面版强制 `CGO_ENABLED=1` 且**必须本机编译**(不能跨编)。每个目标平台需要:

| 平台 | 必装组件 | 安装命令(参考) |
|------|---------|----------------|
| Windows | MinGW-w64(gcc) + WebView2 Runtime(Win10+ 通常预装) | MSYS2: `pacman -S mingw-w64-x86_64-gcc`;或装 [TDM-GCC](https://jmeubank.github.io/tdm-gcc/) |
| macOS | Xcode Command Line Tools | `xcode-select --install` |
| Linux | gcc + WebKit2GTK 开发包 | Ubuntu/Debian: `apt install build-essential libwebkit2gtk-4.0-dev`;Fedora: `dnf install gcc webkit2gtk4.1-devel` |

所有平台都需要 Wails CLI:

```bash
go install github.com/wailsapp/wails/v2/cmd/wails@latest
wails doctor   # 自检环境是否齐全
```

### 8.2 首次拉取依赖(Go + 前端)

仓库根目录执行(只需一次):

```bash
go mod tidy            # 拉 Wails + 后端依赖
(cd web && npm install) # 拉 Vue / Vite / Pinia / Tailwind 等前端依赖
```

`go mod tidy` 把 Wails 及其传递依赖写入 `go.mod` / `go.sum`(Web 版的 `cmd/main.go` 不 import Wails,但 `go mod tidy` 跨整个 module 扫描,一次 tidy 后两个产物的依赖图都齐全)。`npm install` 在 `web/node_modules/` 下落地前端依赖;后续 `build.sh` / `build.bat` 也会自己跑一次,但首次手动跑可让 IDE 类型补全立即生效。

### 8.3 手动单独构建桌面版

```bash
cd cmd/desktop
wails build -clean -o ../../dist/easy-ffmpeg-desktop.exe         # Windows 本机
wails build -clean -platform darwin/arm64 -o ../../dist/...      # macOS arm64 本机
wails build -clean -o ../../dist/easy-ffmpeg-desktop-linux       # Linux 本机
```

## 9. 常见构建陷阱

### 9.1 Web 版

| 问题 | 原因 | 解决 |
|------|------|------|
| `npm: command not found` / `ERROR: npm not found` | 没装 Node.js 或不在 PATH | 装 Node.js ≥ 20 LTS;v0.5.0 起前端必须先编出 `web/dist/` |
| `web/embed.go: pattern all:dist: no matching files found` | `web/dist/` 不存在 | 跑过 `cd web && npm run build`?build.sh / build.bat 顺序保障了这一步 |
| `embedded_darwin.go: pattern darwin/darwin.7z: no matching files found` | 对应平台的 7z 不存在 | 准备好所有 3 个平台的 .7z 文件后再编译 |
| 编译后体积过大 | 可能误嵌了未压缩的二进制 | 确认 `embedded_<os>.go` 的 embed 指令是 `<os>/<os>.7z` 而非目录 |
| Windows 运行弹出 cmd 黑窗 | 老版本 `-H=windowsgui` 被移除(Web 模式需要控制台) | 正常行为;未来若要纯后台模式再讨论 |
| 跨编译报 cgo 错误 | 某处引入了 cgo | 检查 `CGO_ENABLED=0`,必要时 `go build -x` 诊断 |
| macOS 运行报"无法打开" | Gatekeeper 未签名警告 | 右键"打开"或 `xattr -d com.apple.quarantine <path>` |
| macOS ARM64 运行失败 | 嵌入的是 amd64 ffmpeg | 确认 `darwin.7z` 里是 arm64 构建的 ffmpeg |

### 9.2 桌面版

| 问题 | 原因 | 解决 |
|------|------|------|
| `wails: command not found` | Wails CLI 没装或不在 PATH | `go install github.com/wailsapp/wails/v2/cmd/wails@latest`;确认 `$GOPATH/bin` 在 PATH |
| `cgo: C compiler "gcc" not found` | 没装 C 工具链 | 见 §8.1 |
| Windows 构建找不到 `windows.h` | MinGW 不完整或 PATH 顺序错 | 确认 MSYS2 mingw64 工具链在 PATH 前列 |
| Linux `package webkit2gtk-4.0 was not found` | 开发包未装 | `apt install libwebkit2gtk-4.0-dev` 或 `4.1-dev`(Ubuntu 22.04+) |
| 双击桌面版 `.exe` 闪退无窗口 | WebView2 Runtime 缺失(Win7/8/旧 Win10) | 引导用户装 [Evergreen Runtime](https://developer.microsoft.com/microsoft-edge/webview2/);或回退用 Web 版 |
| macOS 启动报"无法打开" | 未签名 / 未公证 | `xattr -d com.apple.quarantine easy-ffmpeg-desktop-macos-arm64.app`;v0.4.x 后续切片接入正式签名 |

## 10. 发布与分发(未实现)

当前没有 CI/CD。未来可做:

- GitHub Actions 矩阵构建(三平台 4 Web 产物 + 4 桌面产物)
- 打 tag 自动 release
- macOS 开发者签名 + 公证(避免 Gatekeeper 警告)
- Windows 代码签名(避免 SmartScreen 警告)
- Linux 分发:AppImage / Flatpak / deb / rpm

详见 [roadmap.md §4](roadmap.md)。
