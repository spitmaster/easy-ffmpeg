# 音频处理 — 产品设计

> "音频处理"Tab 三模式:格式转换 / 从视频提取 / 音频合并。**实现状态**:✅ 三种模式均已落地。
>
> 对应程序设计:[program.md](program.md)。共享导出体验见 [core/ui-system.md §6](../../core/ui-system.md)。

---

## 1. 目标与非目标

### 1.1 目标

面向非专业用户,把常见的三类音频需求做得"点几下就完成":

1. **音频转换 / 压缩**:把一个音频文件转成另一种格式,顺带控制码率、采样率、声道
2. **从视频提取音频**:把 MP4/MKV 等视频里的声音抠出来,支持"无损拷贝"和"转码"两种方式
3. **音频合并**:把多个音频首尾拼接成一个文件;必要时自动处理编码不一致的情形

### 1.2 非目标(本版本不做)

- 变速 / 变调 / 降噪 / EQ 等音频效果器
- 任意时间区间裁剪(由未来"音频裁剪"独立设计)
- 混音(`amix`)—— 放到 roadmap,本期只做拼接(`concat`)
- 批量(队列多个输入)—— 全局只跑一个 Job 的现状保持

---

## 2. 总体设计决策

### 2.1 单 Tab + 模式切换(不是三个 Tab)

三种模式共享几乎相同的外壳(文件选择 → 输出设置 → 命令预览 → 日志),只有中间的**参数区**不同。做成一个 Tab 内的 segmented control,避免代码重复。

```text
┌─ 音频处理 Tab ─────────────────────────────────────────┐
│  [ 格式转换 ] [ 从视频提取 ] [ 音频合并 ]              │ ← segmented
├────────────────────────────────────────────────────────┤
│                                                        │
│              (对应模式的表单 + 参数)                    │
│                                                        │
├────────────────────────────────────────────────────────┤
│  命令预览 / 开始·取消 / 日志 / 完成条  (三模式共用)     │
└────────────────────────────────────────────────────────┘
```

### 2.2 复用既有机制

| 机制 | 复用方式 |
|------|----------|
| `internal/job.Manager` | 三种模式全部走 `Start()`;全局只允许一个 Job 在跑 |
| SSE `/api/convert/stream` | 沿用同一个流(三 Tab 共享) |
| 文件选择器模态框 | 复用 `Picker`;后端 `/api/fs/list` 不改 |
| 目录记忆 `/api/config/dirs` | 继续共用 `inputDir` / `outputDir` |
| 首次解压遮罩 | 与 Tab 无关,继续覆盖全屏 |

---

## 3. UI 设计

### 3.1 入口与模式切换

`<nav class="tabs">` 里的「音频处理」按钮去掉 `disabled`。

模式切换控件采用三段式 segmented button:

```html
<div class="segmented" role="tablist">
  <button class="seg active" data-mode="convert">格式转换</button>
  <button class="seg"        data-mode="extract">从视频提取</button>
  <button class="seg"        data-mode="merge">  音频合并</button>
</div>
```

切换时:

- 重绘参数区
- 清空命令预览
- 保留输出目录(用户可能跨模式同一个目录)
- **禁止在 Job 运行中切换**(切换按钮在 `running=true` 时 `disabled`)

### 3.2 共通模块(位于参数区下方)

- **命令预览**:等宽字体,实时反映当前表单
- **动作行**:`开始处理` / `取消` + 状态文字
- **日志区** + **完成条** + **进度条**:与视频转换 Tab 一模一样

---

## 4. 模式一:音频转换 / 压缩

### 4.1 表单字段

| 字段 | 控件 | 默认 | 说明 |
|------|------|------|------|
| 输入音频 | Browse + 文本框 | — | 模态框走 `mode:"file"`,title="选择输入音频" |
| 输出目录 | Browse + 📂 + 文本框 | 上次 | 与视频 Tab 共享 `outputDir` |
| 输出文件名 | 文本框 | `<输入名>_converted` | 不含后缀 |
| 输出格式 | select | `mp3` | `mp3 · m4a · flac · wav · ogg · opus` |
| 编码器 | select | `libmp3lame` | 见 §4.2 |
| 码率(kbps) | select | `192` | `64 / 96 / 128 / 160 / 192 / 256 / 320 / copy` |
| 采样率 | select | `原始` | `原始 / 44100 / 48000 / 22050 / 8000` |
| 声道 | select | `原始` | `原始 / 立体声(2) / 单声道(1)` |

### 4.2 容器与编码器的合法组合

不是所有容器都能装所有编码器,前端在切换格式时自动换默认编码器:

| 格式 | 默认编码器 | 可选编码器 |
|------|-----------|-----------|
| `mp3`  | `libmp3lame` | `libmp3lame`, `copy` |
| `m4a`  | `aac`        | `aac`, `copy` |
| `flac` | `flac`       | `flac`, `copy`(无损编码,忽略码率控件) |
| `wav`  | `pcm_s16le`  | `pcm_s16le`, `pcm_s24le`, `copy`(PCM 忽略码率) |
| `ogg`  | `libvorbis`  | `libvorbis`, `libopus`, `copy` |
| `opus` | `libopus`    | `libopus`, `copy` |

**UX 约定**:选了无损/PCM 容器时,码率下拉控件灰掉不可选(`disabled + title="当前格式无需码率"`)。

### 4.3 命令构建规则

```text
args := ["-y", "-i", inputPath, "-vn"]

if codec == "copy":
    args += ["-c:a", "copy"]
    // 忽略码率 / 采样率 / 声道(copy 意味着保持原样)
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

- `-vn`:即便输入是纯音频也无妨,防御性地丢弃任何视频轨
- **禁止非法组合**:前端层面做白名单,后端再做一次兜底校验

### 4.4 命令预览样例

```text
ffmpeg -y -i "C:/Music/raw.wav" -vn -c:a libmp3lame -b:a 192k -ar 44100 "C:/Out/raw_converted.mp3"
```

---

## 5. 模式二:从视频提取音频

### 5.1 表单字段

| 字段 | 控件 | 默认 | 说明 |
|------|------|------|------|
| 输入视频 | Browse + 文本框 | — | 模态框 title="选择输入视频" |
| 输出目录 | Browse + 📂 + 文本框 | 上次 | |
| 输出文件名 | 文本框 | `<输入名>_audio` | |
| 提取方式 | radio | `直接拷贝(不重编码)` | 见 §5.2 |
| (转码时)输出格式 | select | `mp3` | 与模式一同一套格式集 |
| (转码时)编码器 / 码率 | select | 随格式 | 与模式一同一套 |
| 音轨选择 | select | `默认第 1 条` | 见 §5.3 |

### 5.2 两种提取方式

**A. 直接拷贝(无损、秒完成)**

```text
ffmpeg -y -i <video> -vn -map 0:a:<idx> -c:a copy <out>.<ext>
```

- 输出后缀**必须匹配原音轨的编码**(否则 `-c copy` 会报错)
- 策略:启动前先跑 `ffprobe -v quiet -print_format json -show_streams -select_streams a <video>`,
  - 拿到选中音轨的 `codec_name`(`aac` / `mp3` / `opus` / `vorbis` / …)
  - 自动选一个兼容容器填到"输出格式"(`aac → m4a`、`mp3 → mp3`、`opus → opus`、`vorbis → ogg`、其余 → `mka`)
- 文件名后缀自动对齐,用户仍可手改

**B. 转码为指定格式**

与模式一完全一致(输出格式 + 编码器 + 码率 + 采样率 + 声道),命令为:

```text
ffmpeg -y -i <video> -vn -map 0:a:<idx> -c:a <codec> -b:a <bitrate>k ... <out>.<ext>
```

### 5.3 音轨选择

多音轨的视频常见(多语言、导演评论)。拉开"输入视频"选择后立即调 ffprobe:

```text
POST /api/audio/probe     { path: "<video>" }
→ { streams: [
     { index: 0, codec: "aac",    channels: 2, lang: "und", sampleRate: 48000 },
     { index: 1, codec: "ac3",    channels: 6, lang: "eng", ... },
     { index: 2, codec: "ac3",    channels: 2, lang: "chi", ... }
   ]}
```

- UI 把每条渲染成一个选项:`#1 · AAC · 2ch · 未标注`
- 只有一条时默认选中且不显示下拉(节省一眼)
- `-map 0:a:<ffmpeg-index>` 用的是"音频流内部序号",注意不是全局 stream index

---

## 6. 模式三:音频合并

### 6.1 表单字段

| 字段 | 控件 | 默认 | 说明 |
|------|------|------|------|
| 输入列表 | 多选文件选择 + 可排序列表 | — | 见 §6.2 |
| 输出目录 | Browse + 📂 + 文本框 | 上次 | |
| 输出文件名 | 文本框 | `merged` | |
| 输出格式 | select | `mp3` | 与模式一同 |
| 合并策略 | radio | `自动判断` | 见 §6.3 |
| (策略=重编码时)编码器/码率 | select | 随格式 | 与模式一同 |

### 6.2 输入列表 UI

```text
┌─ 输入列表(按顺序拼接) ───────────────────────────┐
│  ☰ 1. intro.mp3        48 kbps  · 00:00:12        🗑│
│  ☰ 2. main.mp3        192 kbps  · 00:03:21        🗑│
│  ☰ 3. outro.mp3       192 kbps  · 00:00:08        🗑│
│                                                    │
│  [ + 添加文件 ]                                    │
└────────────────────────────────────────────────────┘
```

- 拖拽 `☰` 改顺序(MVP 用 ↑↓ 按钮替代)
- 🗑 删除一项
- 每项展示 ffprobe 结果简要(码率 / 时长),帮助用户判断编码是否一致

### 6.3 合并策略

| 策略 | 适用 | 命令 |
|------|------|------|
| **快速拼接(-c copy)** | 所有输入编码/采样率/声道完全一致 | `concat demuxer` + `-c copy` |
| **重编码拼接** | 输入编码不一 / 用户手动选择 | `concat filter`(`-filter_complex`) |
| **自动判断**(默认) | 先用 ffprobe 检查是否一致,自动选上述之一 | — |

#### 快速拼接(concat demuxer)

- 生成临时列表文件:`%TEMP%/easy-ffmpeg-merge-<ts>.txt`

  ```text
  file 'C:/Music/intro.mp3'
  file 'C:/Music/main.mp3'
  file 'C:/Music/outro.mp3'
  ```

- 命令:

  ```text
  ffmpeg -y -f concat -safe 0 -i <list.txt> -c copy <out>
  ```

- **运行结束(无论成功失败)必须删掉列表文件**
- 列表文件中的路径需要转义单引号:`'` → `'\''`

#### 重编码拼接(concat filter)

```text
ffmpeg -y -i f1 -i f2 -i f3 \
  -filter_complex "[0:a][1:a][2:a]concat=n=3:v=0:a=1[out]" \
  -map "[out]" -c:a <codec> -b:a <bitrate>k <out>
```

- `n=3` 需要等于输入数量
- 好处:不同编码 / 采样率 / 声道都能合并
- 代价:比 demuxer 慢 10×+(需要全程重编码)

### 6.4 自动判断规则

启动前对每个输入跑 ffprobe,比较这些字段是否完全一致:`codec_name` · `sample_rate` · `channels` · `bit_rate`(允许 ±10% 浮动)。

- 全一致 → 用快速拼接
- 有差异 → 退回重编码,并在日志区提示一行 `[info] 输入编码不一致,改用重编码拼接`

---

## 7. 边界情况与错误处理

| 情形 | 处理 |
|------|------|
| 非法编码器 × 容器组合 | 前端切换格式时重置编码器;后端兜底拒绝并返回中文错误 |
| ffprobe 解析失败(文件损坏 / 非媒体) | 前端 alert;模式二/三不允许进入下一步 |
| 合并列表为空或只有 1 项 | 开始按钮 `disabled` |
| 合并 demuxer 列表中路径含特殊字符 | 列表文件里用 `file '...'`;单引号转义 `'\''` |
| 合并重编码时某个输入损坏中途失败 | ffmpeg 自己会退出;走 `error` 事件;半成品文件保留不清理(与视频 Tab 对齐) |
| 提取"直接拷贝"但后缀选错 | 后端用 ffprobe 校验 `codec_name` 兼容性;不兼容 → 400,提示改后缀或改"转码" |
| 临时列表文件清理失败 | 日志输出 warn,不中断 |
| 目标文件已存在 | 409 + 自绘 `Confirm.overwrite` 对话框(共享流程) |

---

## 8. 字段交互细节

- **模式切换时**:命令预览清空重建;运行中禁止切换(看 start 按钮是否 disabled)
- **从视频提取**在选完输入后自动 ffprobe 音轨;单音轨时下拉隐藏
- **合并**列表里每项展示 codec · 声道 · kbps · 时长;↑↓ 排序,🗑 移除;添加按钮触发 Picker
