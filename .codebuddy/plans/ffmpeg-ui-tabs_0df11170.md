---
name: ffmpeg-ui-tabs
overview: 为Go Fyne桌面应用添加5个Tab占位符，每个Tab对应不同的FFmpeg功能模块
design:
  styleKeywords:
    - TabContainer
    - 占位符
    - 多标签页
  fontSystem:
    fontFamily: system-ui
    heading:
      size: 18px
      weight: 600
    subheading:
      size: 14px
      weight: 500
    body:
      size: 14px
      weight: 400
  colorSystem:
    primary:
      - "#2196F3"
    background:
      - "#FFFFFF"
    text:
      - "#333333"
    functional:
      - "#4CAF50"
      - "#F44336"
      - "#FF9800"
todos:
  - id: extend-tab-container
    content: 修改 ui/ui.go 中的 TabContainer，将 3 个 tab 扩展为 5 个
    status: completed
  - id: add-video-convert-tab
    content: 添加"视频转换"tab占位符
    status: completed
    dependencies:
      - extend-tab-container
  - id: add-video-trim-tab
    content: 添加"视频裁剪"tab占位符
    status: completed
    dependencies:
      - extend-tab-container
  - id: add-audio-process-tab
    content: 添加"音频处理"tab占位符
    status: completed
    dependencies:
      - extend-tab-container
  - id: add-media-info-tab
    content: 添加"媒体信息"tab占位符
    status: completed
    dependencies:
      - extend-tab-container
  - id: verify-build
    content: 验证编译是否成功
    status: completed
    dependencies:
      - add-video-convert-tab
      - add-video-trim-tab
      - add-audio-process-tab
      - add-media-info-tab
---

## 用户需求

创建窗口程序，包含多个tab，每个tab是不同的ffmpeg功能。用占位符做5个tab，功能后续规划。

## 核心功能

- 将现有的3个tab扩展为5个tab
- 建议tab名称: 视频转换、视频裁剪、音频处理、媒体信息、设置
- 每个tab使用占位符内容（显示"coming soon"提示）
- 为后续功能扩展预留基础结构

## UI 设计方案

采用 Fyne 框架的 TabContainer 组件实现多标签页面布局。

### 页面规划

1. **视频转换** - 视频格式转换功能入口
2. **视频裁剪** - 视频裁剪功能入口
3. **音频处理** - 音频格式转换、提取等功能入口
4. **媒体信息** - 媒体文件信息查看功能入口
5. **设置** - 程序设置入口

### 设计细节

- 每个 Tab 使用占位符 Label 显示 "功能名称 - coming soon..."
- Tab 顺序按照功能使用频率排列
- 顶部保留 FFmpeg 状态显示栏
- Tab 内容区域使用占位符，便于后续功能扩展