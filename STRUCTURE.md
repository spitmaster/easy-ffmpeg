# 项目结构

```
easy-ffmpeg/
├── cmd/                    # 程序入口
│   └── main.go             # 主程序入口
├── ui/                     # UI层 - 图形界面相关
│   └── ui.go               # 界面组件和布局
├── service/                # Service层 - 业务逻辑
│   └── ffmpeg.go           # FFmpeg命令封装
├── internal/               # 内部包
│   └── embedded/          # 嵌入的二进制文件
│       ├── embedded.go     # 嵌入二进制文件管理
│       ├── windows/        # Windows平台FFmpeg二进制
│       │   ├── ffmpeg.exe
│       │   └── ffprobe.exe
│       ├── darwin/         # macOS平台FFmpeg二进制
│       │   ├── ffmpeg
│       │   └── ffprobe
│       └── linux/          # Linux平台FFmpeg二进制
│           ├── ffmpeg
│           └── ffprobe
├── model/                  # Model层 - 数据结构
│   └── *.go                # 数据模型定义
├── config/                 # 配置文件
│   └── *.go                # 配置管理
├── go.mod                  # Go模块文件
├── go.sum                  # 依赖校验文件
├── .gitignore              # Git忽略文件
├── README.md               # 项目说明
└── STRUCTURE.md            # 本文件
```

## 目录说明

### cmd/
程序入口点，包含 main 函数，负责初始化应用和启动界面。

### ui/
负责图形界面组件的创建和布局，包括：
- 主窗口
- 各个功能模块的Tab页面
- 文件选择对话框
- 进度显示
- 设置页面

### service/
封装 FFmpeg 命令的调用，包括：
- FFmpeg 可用性检查（优先使用嵌入版本）
- 命令执行（自动降级到系统版本）
- FFprobe 支持
- 输出解析
- 进度监控

### model/
定义项目中使用的各种数据结构，如：
- 转换任务配置
- 媒体文件信息
- 用户配置

### config/
处理应用程序的配置管理。

### internal/embedded/
嵌入的二进制文件管理模块，负责：
- 嵌入各平台的 FFmpeg 和 FFprobe 二进制文件
- 运行时解压二进制文件到临时目录
- 提供跨平台的二进制访问接口
- 支持降级到系统 FFmpeg（当嵌入版本不可用时）

**嵌入机制：**
- 使用 Go 的 `embed` 指令将二进制文件编译到程序中
- 首次运行时自动解压到系统临时目录
- 后续运行检查文件是否存在且内容一致，避免重复解压
- 不同平台编译会自动包含对应平台的二进制文件

## 扩展开发

当需要添加新功能时：

1. 在 `service/` 中添加 FFmpeg 命令封装
2. 在 `model/` 中定义相关数据结构
3. 在 `ui/` 中创建对应的界面组件
4. 在 `cmd/main.go` 中注册新功能
