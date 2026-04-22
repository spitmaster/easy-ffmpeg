# 音频处理功能设计

> 本文档定义"音频处理" Tab 的产品形态、交互流程、后端 API 与命令构建规则。
> 目标读者：开发、评审、后续维护者。
>
> **实现状态**：三种模式均已落地（见 §10 的 6 个切片）。核心命令构建器位于
> `server/audio_args.go`，对应表驱动测试在 `server/audio_args_test.go`。

---

## 1. 目标与非目标

### 1.1 目标

面向非专业用户，把常见的三类音频需求做得"点几下就完成"：

1. **音频转换 / 压缩**：把一个音频文件转成另一种格式，顺带控制码率、采样率、声道。
2. **从视频提取音频**：把 MP4/MKV 等视频里的声音抠出来，支持"无损拷贝"和"转码"两种方式。
3. **音频合并**：把多个音频首尾拼接成一个文件；必要时自动处理编码不一致的情形。

### 1.2 非目标（本版本不做）

- 变速 / 变调 / 降噪 / EQ 等音频效果器
- 任意时间区间裁剪（由未来"音频裁剪"独立设计）
- 混音（`amix`）—— 放到 roadmap，本期只做拼接（`concat`）
- 批量（队列多个输入）—— 全局只跑一个 Job 的现状保持

---

## 2. 总体设计决策

### 2.1 单 Tab + 模式切换（不是三个 Tab）

三种模式共享几乎相同的外壳（文件选择 → 输出设置 → 命令预览 → 日志），
只有中间的**参数区**不同。做成一个 Tab 内的 segmented control，避免代码重复。

```
┌─ 音频处理 Tab ─────────────────────────────────────────┐
│  [ 格式转换 ] [ 从视频提取 ] [ 音频合并 ]              │ ← segmented
├────────────────────────────────────────────────────────┤
│                                                        │
│              （对应模式的表单 + 参数）                  │
│                                                        │
├────────────────────────────────────────────────────────┤
│  命令预览 / 开始·取消 / 日志 / 完成条  （三模式共用）   │
└────────────────────────────────────────────────────────┘
```

### 2.2 复用既有机制

| 机制 | 复用方式 |
|------|----------|
| `internal/job.Manager` | 三种模式全部走 `Start()`；全局只允许一个 Job 在跑 |
| SSE `/api/convert/stream` | 本期沿用同一个流；**不**重开一个"音频专用"流（见 §6.3） |
| 文件选择器模态框 | 复用 `openPicker`；后端 `/api/fs/list` 不改 |
| 目录记忆 `/api/config/dirs` | 继续共用 `inputDir` / `outputDir` |
| 首次解压遮罩 | 与 Tab 无关，继续覆盖全屏 |

### 2.3 前置工程改造

在实现本功能前，必须先完成两件脚手架：

1. **Tab 切换逻辑**：目前 `index.html` 除"视频转换"外其他 Tab 都 `disabled`，`app.js` 没有切换代码。需要加：
   - 点击 `.tab` 切换 `.active` + 显示/隐藏对应 `.panel`
   - 切换时重置当前 Tab 的 UI 状态（表单不跨 Tab 持久化）
2. **命名冲突治理**：`app.js` 里的 `form` / `readForm` / `updateCommandPreview` 是视频 Tab 的全局单例，不兼容"同时存在多种表单"。建议把它们迁入 `convert` 模块（IIFE 或 ES module 命名空间），再在下面加 `audio` 模块。

> 这两项属于共用基础设施，不放在"音频处理"工作量里，但必须先做。

---

## 3. UI 设计

### 3.1 入口 Tab

- `<nav class="tabs">` 里的「音频处理」按钮去掉 `disabled`。
- 图标前缀用 🎵（可选）。

### 3.2 模式切换控件

采用三段式 segmented button（复用 `.btn` 家族）：

```html
<div class="segmented" role="tablist">
  <button class="seg active" data-mode="convert">格式转换</button>
  <button class="seg"        data-mode="extract">从视频提取</button>
  <button class="seg"        data-mode="merge">  音频合并</button>
</div>
```

切换时：
- 重绘参数区
- 清空命令预览
- 保留输出目录（用户可能跨模式同一个目录）
- **禁止在 Job 运行中切换**（切换按钮在 `running=true` 时 `disabled`）

### 3.3 共通模块（位于参数区下方）

- **命令预览**：等宽字体，实时反映当前表单
- **动作行**：`开始处理` / `取消` + 状态文字
- **日志区** + **完成条**：与视频转换 Tab 一模一样

---

## 4. 模式一：音频转换 / 压缩

### 4.1 表单字段

| 字段 | 控件 | 默认 | 说明 |
|------|------|------|------|
| 输入音频 | Browse + 文本框 | — | 模态框走 `mode:"file"`，title="选择输入音频" |
| 输出目录 | Browse + 📂 + 文本框 | 上次 | 与视频 Tab 共享 `outputDir` |
| 输出文件名 | 文本框 | `<输入名>_converted` | 不含后缀 |
| 输出格式 | select | `mp3` | `mp3 · m4a · flac · wav · ogg · opus` |
| 编码器 | select | `libmp3lame` | 见 4.2 |
| 码率（kbps） | select | `192` | `64 / 96 / 128 / 160 / 192 / 256 / 320 / copy` |
| 采样率 | select | `原始` | `原始 / 44100 / 48000 / 22050 / 8000` |
| 声道 | select | `原始` | `原始 / 立体声(2) / 单声道(1)` |

#### 容器与编码器的合法组合

不是所有容器都能装所有编码器，前端需要在切换格式时自动换默认编码器：

| 格式 | 默认编码器 | 可选编码器 |
|------|-----------|-----------|
| `mp3`  | `libmp3lame` | `libmp3lame`, `copy` |
| `m4a`  | `aac`        | `aac`, `copy` |
| `flac` | `flac`       | `flac`, `copy`（无损编码，忽略码率控件） |
| `wav`  | `pcm_s16le`  | `pcm_s16le`, `pcm_s24le`, `copy`（PCM 忽略码率） |
| `ogg`  | `libvorbis`  | `libvorbis`, `libopus`, `copy` |
| `opus` | `libopus`    | `libopus`, `copy` |

**UX 约定**：选了无损/PCM 容器时，码率下拉控件灰掉不可选（`disabled + title="当前格式无需码率"`）。

### 4.2 命令构建规则

```
args := ["-y", "-i", inputPath, "-vn"]

if codec == "copy":
    args += ["-c:a", "copy"]
    // 忽略码率 / 采样率 / 声道（copy 意味着保持原样）
else:
    args += ["-c:a", codec]
    if bitrate != "copy" && codec is 有损 (非 flac / 非 pcm_*):
        args += ["-b:a", f"{bitrate}k"]
    if sampleRate != "原始":
        args += ["-ar", sampleRate]
    if channels != "原始":
        args += ["-ac", channels]

args += [outputDir/outputName.format]
```

- `-vn`：即便输入是纯音频也无妨，防御性地丢弃任何视频轨
- **禁止非法组合**：前端层面做白名单（4.1 表），后端再做一次兜底校验

### 4.3 命令预览样例

```
ffmpeg -y -i "C:/Music/raw.wav" -vn -c:a libmp3lame -b:a 192k -ar 44100 "C:/Out/raw_converted.mp3"
```

---

## 5. 模式二：从视频提取音频

### 5.1 表单字段

| 字段 | 控件 | 默认 | 说明 |
|------|------|------|------|
| 输入视频 | Browse + 文本框 | — | 模态框 title="选择输入视频" |
| 输出目录 | Browse + 📂 + 文本框 | 上次 | |
| 输出文件名 | 文本框 | `<输入名>_audio` | |
| 提取方式 | radio | `直接拷贝（不重编码）` | 见 5.2 |
| （当"转码"时）输出格式 | select | `mp3` | 与模式一同一套格式集 |
| （当"转码"时）编码器 / 码率 | select | 随格式 | 与模式一同一套 |
| 音轨选择 | select | `默认第 1 条` | 见 5.3 |

### 5.2 两种提取方式

**A. 直接拷贝（无损、秒完成）**

```
ffmpeg -y -i <video> -vn -map 0:a:<idx> -c:a copy <out>.<ext>
```

- 输出后缀**必须匹配原音轨的编码**（否则 `-c copy` 会报错）
- 策略：启动前先跑 `ffprobe -v quiet -print_format json -show_streams -select_streams a <video>`，
  - 拿到选中音轨的 `codec_name`（`aac` / `mp3` / `opus` / `vorbis` / …）
  - 自动选一个兼容容器填到"输出格式"（`aac → m4a`、`mp3 → mp3`、`opus → opus`、`vorbis → ogg`、其余 → `mka`）
- 文件名后缀自动对齐，用户仍可手改

**B. 转码为指定格式**

与模式一完全一致（输出格式 + 编码器 + 码率 + 采样率 + 声道），命令为：

```
ffmpeg -y -i <video> -vn -map 0:a:<idx> -c:a <codec> -b:a <bitrate>k ... <out>.<ext>
```

### 5.3 音轨选择

多音轨的视频常见（多语言、导演评论）。拉开"输入视频"选择后立即调 ffprobe：

```
POST /api/audio/probe     { path: "<video>" }
→ { streams: [
     { index: 0, codec: "aac",    channels: 2, lang: "und", sampleRate: 48000 },
     { index: 1, codec: "ac3",    channels: 6, lang: "eng", ... },
     { index: 2, codec: "ac3",    channels: 2, lang: "chi", ... }
   ]}
```

- UI 把每条渲染成一个选项：`#1 · AAC · 2ch · 未标注`
- 只有一条时默认选中且不显示下拉（节省一眼）
- `-map 0:a:<ffmpeg-index>` 用的是"音频流内部序号"，注意不是全局 stream index

---

## 6. 模式三：音频合并

### 6.1 表单字段

| 字段 | 控件 | 默认 | 说明 |
|------|------|------|------|
| 输入列表 | 多选文件选择 + 可排序列表 | — | 见 6.2 |
| 输出目录 | Browse + 📂 + 文本框 | 上次 | |
| 输出文件名 | 文本框 | `merged` | |
| 输出格式 | select | `mp3` | 与模式一同 |
| 合并策略 | radio | `自动判断` | 见 6.3 |
| （策略=重编码时）编码器/码率 | select | 随格式 | 与模式一同 |

### 6.2 输入列表 UI

```
┌─ 输入列表（按顺序拼接） ───────────────────────────┐
│  ☰ 1. intro.mp3        48 kbps  · 00:00:12        🗑│
│  ☰ 2. main.mp3        192 kbps  · 00:03:21        🗑│
│  ☰ 3. outro.mp3       192 kbps  · 00:00:08        🗑│
│                                                    │
│  [ + 添加文件 ]                                    │
└────────────────────────────────────────────────────┘
```

- 拖拽 `☰` 改顺序（最小可用：用 ↑↓ 按钮替代也行，视工作量）
- 🗑 删除一项
- 每项展示 ffprobe 结果简要（码率 / 时长），帮助用户判断编码是否一致

### 6.3 合并策略

| 策略 | 适用 | 命令 |
|------|------|------|
| **快速拼接（-c copy）** | 所有输入编码/采样率/声道完全一致 | `concat demuxer` + `-c copy` |
| **重编码拼接** | 输入编码不一 / 用户手动选择 | `concat filter`（`-filter_complex`） |
| **自动判断**（默认） | 先用 ffprobe 检查是否一致，自动选上述之一 | — |

#### 快速拼接（concat demuxer）

- 生成临时列表文件：`%TEMP%/easy-ffmpeg-merge-<ts>.txt`
  ```
  file 'C:/Music/intro.mp3'
  file 'C:/Music/main.mp3'
  file 'C:/Music/outro.mp3'
  ```
- 命令：
  ```
  ffmpeg -y -f concat -safe 0 -i <list.txt> -c copy <out>
  ```
- **运行结束（无论成功失败）必须删掉列表文件**
- 列表文件中的路径需要转义单引号：`'` → `'\''`

#### 重编码拼接（concat filter）

```
ffmpeg -y -i f1 -i f2 -i f3 \
  -filter_complex "[0:a][1:a][2:a]concat=n=3:v=0:a=1[out]" \
  -map "[out]" -c:a <codec> -b:a <bitrate>k <out>
```

- `n=3` 需要等于输入数量
- 好处：不同编码 / 采样率 / 声道都能合并
- 代价：比 demuxer 慢 10×+（需要全程重编码）

### 6.4 自动判断规则

启动前对每个输入跑 ffprobe，比较这些字段是否完全一致：
`codec_name` · `sample_rate` · `channels` · `bit_rate`（允许 ±10% 浮动）

- 全一致 → 用快速拼接
- 有差异 → 退回重编码，并在日志区提示一行 `[info] 输入编码不一致，改用重编码拼接`

---

## 7. 后端 API 设计

复用当前 `/api/convert/*`，但入口拆分以便校验。

### 7.1 `POST /api/audio/start`

```jsonc
// 请求
{
  "mode": "convert" | "extract" | "merge",

  // convert 专用
  "inputPath": "...",
  "outputDir": "...",
  "outputName": "...",
  "format": "mp3",
  "codec": "libmp3lame",
  "bitrate": 192,          // kbps；数字 0 表示"不设置"
  "sampleRate": 44100,     // 0 表示保持原始
  "channels": 2,           // 0 表示保持原始
  "overwrite": false,

  // extract 额外
  "audioStreamIndex": 0,   // ffprobe 音频流内部序号
  "extractMethod": "copy" | "transcode",

  // merge 额外
  "inputPaths": ["...", "..."],
  "mergeStrategy": "auto" | "copy" | "reencode"
}
```

- 校验失败 → `400 { error: "..." }`
- 同一 Job 已运行 → `409 { error: "another job is running" }`（与视频 Tab 冲突）
- 目标已存在且未授权覆盖 → `409 { error:"file exists", existing:true, path:"..." }`
- 成功 → `200 { ok:true, command:"ffmpeg ..." }`

### 7.2 `POST /api/audio/cancel`

等价于现有 `/api/convert/cancel`。可以直接给后者起个别名：`POST /api/job/cancel` 两边都用。**推荐迁移命名**（见 §9 工程化）。

### 7.3 `POST /api/audio/probe`

```jsonc
// 请求
{ "path": "..." }

// 响应
{
  "format":   { "duration": 123.45, "bitrate": 192000, "size": 2940000 },
  "streams":  [
    { "index": 0, "codecName": "aac", "channels": 2, "sampleRate": 48000, "bitRate": 128000, "lang": "und" }
  ]
}
```

- 调用：`ffprobe -v quiet -print_format json -show_format -show_streams -select_streams a <path>`
- 专供提取 & 合并模式使用

### 7.4 SSE 事件不变

沿用 `/api/convert/stream`（或重命名成 `/api/job/stream`）。前端不需要区分是哪种任务，
因为 `Job.Manager` 全局唯一，事件即当前任务的事件。

---

## 8. 前端状态管理

### 8.1 模块拆分建议

```
app.js
├── shared/
│   ├── picker.js        复用的文件选择器
│   ├── sse.js           订阅 /api/.../stream
│   ├── log.js           appendLog + 完成条
│   └── dirs.js          /api/config/dirs
├── convert/             视频转换 Tab（现有 form 迁进去）
├── audio/
│   ├── index.js         模式切换 + 共用动作行
│   ├── convert.js       模式一表单
│   ├── extract.js       模式二表单 + ffprobe 音轨选择
│   └── merge.js         模式三文件列表 + 拖拽
└── tabs.js              Tab 切换调度
```

> 项目约定是"纯静态三件套，零构建"，所以上面的拆分用多个 `<script src=>` 加 IIFE 实现，不引入 ESM 打包。也可以继续单文件 `app.js`，但要加注释块划分。

### 8.2 状态机

每个模式的表单独立持有状态；切 Tab 时 UI 销毁但 JS 里的表单值保留（用户切回来还能继续）。

---

## 9. 边界情况与错误处理

| 情形 | 处理 |
|------|------|
| 非法编码器 × 容器组合 | 前端切换格式时重置编码器；后端兜底拒绝并返回中文错误 |
| ffprobe 解析失败（文件损坏 / 非媒体） | 前端 alert；模式二/三不允许进入下一步 |
| 合并列表为空或只有 1 项 | 开始按钮 `disabled` |
| 合并 demuxer 列表中路径含特殊字符 | 列表文件里用 `file '...'`；单引号转义 `'\''` |
| 合并重编码时某个输入损坏中途失败 | ffmpeg 自己会退出；走 `error` 事件；半成品文件保留不清理（与视频 Tab 对齐） |
| 提取"直接拷贝"但后缀选错 | 后端用 ffprobe 校验 `codec_name` 兼容性；不兼容 → 400，提示改后缀或改"转码" |
| 临时列表文件清理失败 | 日志输出 warn，不中断 |
| 目标文件已存在 | 409 + 弹 `confirm`（与视频 Tab 同一条路径） |

---

## 10. 实现拆分（建议的 PR 切片）

按独立可交付的增量分：

| # | 范围 | 说明 |
|---|------|------|
| 1 | 基础设施：Tab 切换 + `app.js` 模块化 | 不含音频逻辑；把视频 Tab 的全局变量先圈起来 |
| 2 | `/api/audio/probe` + 前端封装 | 只返回 ffprobe JSON，不做任何 UI 接入 |
| 3 | 模式一：音频转换 / 压缩 | 最简单，表单 + `buildAudioArgs` + 共用日志 |
| 4 | 模式二：从视频提取音频 | 加音轨选择；复用模式一的参数区 |
| 5 | 模式三：音频合并 | 含列表 UI、demuxer / filter 两条分支、临时文件清理 |
| 6 | 文档 + 少量自动化测试 | `buildAudioArgs` 的表驱动测试；更新 `feature-design.md` 的 Tab 全景表 |

> 也可以在 #1 之前再加 0 号：把 `handleConvertStart` 里 `buildFFmpegArgs` 的参数构造抽成纯函数并补单元测试，
> 作为后续"音频 build" 的参考模板。

---

## 11. 与现有文档的关系

- `design/feature-design.md` 的「功能全景」表需要把"音频处理"从 🚧 改为 ✅（实现后）
- `design/ui-design.md` 加新章节"音频处理 Tab 布局"，引用本文档
- `design/roadmap.md` 的「1.3 音频处理 Tab」删除，改为指向本文档
- `design/module-design.md` 添加 `buildAudioArgs` 的位置（建议 `server/audio_args.go`，与 `handlers.go` 分离）
