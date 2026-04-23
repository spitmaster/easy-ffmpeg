# 视频裁剪功能设计

> 本文档定义"视频裁剪" Tab 的产品形态、交互流程、后端 API 与命令构建规则。
> 目标读者：开发、评审、后续维护者。
>
> **实现状态**：三段操作均已落地（时间 / 空间 / 分辨率）。核心命令构建器在
> `server/trim_args.go`，表驱动测试在 `server/trim_args_test.go`。

---

## 1. 目标与非目标

### 1.1 目标

三个子操作可独立启用、可自由组合，**单次 ffmpeg 执行完成**：

1. **时间裁剪**：按起止时间从视频里取一段
2. **空间裁剪**：裁出画面中的一个矩形
3. **分辨率缩放**：把画面缩放到目标尺寸（支持常见预设 + 保持比例）

### 1.2 非目标（本版本不做）

- "快速模式" / `-c copy` 关键帧对齐裁剪 —— 会引入互斥规则，v2 再加
- 帧级可视化时间轴（缩略图、拖动滑块） —— 复杂度高，v1 用数字输入
- 预览画面 —— 同上
- 多段拼接剪辑 —— 独立功能，与"合并"一起规划
- 旋转、翻转、色彩调整、滤镜
- 批量（单 Job 模型不变）

---

## 2. 总体设计决策

### 2.1 单表单 + 三组开关（不是 Tab 内模式）

三种操作**非互斥**：典型场景是"裁一段 720p 的小片段" = trim + scale 同时生效。
如果拆成三个子模式，用户就必须：输入 → 裁时间 → 输出临时文件 → 再用临时文件输入 → 裁空间 → ……
每一次都重编码一次，画质阶梯式降级。

所以做一个表单，三个 `<fieldset>`，每个都能独立启用，一次 ffmpeg 执行全部应用。

### 2.2 始终精确（v1 单编码路径）

本期所有时间裁剪都把 `-ss`/`-to` 放在 `-i` **之后**（先解码再定位），即"精确裁剪"。代价是**无论是否只做 trim，都会重编码**。

不做"快速模式"的理由：
- UI 会引入互斥（fast 与 crop/scale 冲突）
- 快速模式下 `-ss` 放 `-i` 前可能掉帧，用户一脸懵
- 后面若用户强烈要求，再加一个"快速"开关和互斥规则，成本可控

### 2.3 复用既有机制

| 机制 | 复用方式 |
|------|---------|
| `internal/job.Manager` / SSE | 沿用；全局单 Job |
| `Picker` / `Dirs` | 沿用 |
| 前端 `createJobPanel` | 沿用 |
| 视频编码器 / 容器 dropdown | 与 ConvertTab 同列表（h264/h265/vp9/...） |
| `service.probe` | 扩展 `ProbeVideo` |

### 2.4 前置工程

- `service/probe.go` 新增 `ProbeVideo(path) *VideoProbeResult`（首条视频流 + 首条音频流 + format）
- 新增 `/api/trim/probe`

无需改动 JS 框架（`createJobPanel` / `JobBus` 都已就绪）。

---

## 3. UI 设计

### 3.1 Tab 入口

`index.html` 里把 `data-tab="trim"` 的 `disabled` 去掉。

### 3.2 布局伪图

```
┌─ 视频裁剪 Tab ──────────────────────────────────────────┐
│ 输入视频                                                │
│ [选择文件]  <path>                                      │
│ 📊 01:23:45 · 1920×1080 · h264 · 29.97 fps              │
│                                                         │
│ ┌─ ☐ 时间裁剪 ─────────────────────────────────────┐   │
│ │ 起始 [00:00:00.000]   结束 [01:23:45.000]        │   │
│ └──────────────────────────────────────────────────┘   │
│                                                         │
│ ┌─ ☐ 空间裁剪 ─────────────────────────────────────┐   │
│ │ X [0]   Y [0]   宽 [1920]   高 [1080]            │   │
│ │ 💡 需满足 X+宽 ≤ 源宽、Y+高 ≤ 源高                │   │
│ └──────────────────────────────────────────────────┘   │
│                                                         │
│ ┌─ ☐ 分辨率缩放 ───────────────────────────────────┐   │
│ │ 预设 [原始 ▼]   ☐ 保持比例                       │   │
│ │ 宽 [1920]   高 [1080]                            │   │
│ └──────────────────────────────────────────────────┘   │
│                                                         │
│ 输出目录 / 文件名                                       │
│ [选择目录]  <outDir>  📂  [name]                       │
│                                                         │
│ 编码器 / 格式                                           │
│ [h264 ▼] [aac ▼] [mp4 ▼]                               │
│                                                         │
│ 命令预览                                                │
│ ffmpeg -y -i "..." -ss 00:00:10 -to 00:00:30 \         │
│        -vf crop=1280:720:0:0,scale=854:480 \           │
│        -c:v libx264 -c:a aac "<out>"                   │
│                                                         │
│ [开始裁剪] [取消]  空闲                                 │
│                                                         │
│ 处理日志                                                │
│ ...                                                     │
└─────────────────────────────────────────────────────────┘
```

### 3.3 启用开关

每个操作块用 `<fieldset>` 包住字段；`<legend>` 内嵌 checkbox。
- checkbox 勾上 → `fieldset.disabled = false`，字段变亮可编辑
- 关闭 → `disabled = true`，整块灰掉（原生 HTML 语义，0 成本）

### 3.4 ffprobe 自动填充

选择输入视频后立即 `POST /api/trim/probe`。收到响应后：

| 字段 | 填充值 |
|------|--------|
| 状态行 | `HH:MM:SS · WxH · codec · fps` |
| trim.start | `00:00:00` |
| trim.end | `duration` 格式化 |
| crop.x / y | `0` |
| crop.w / h | 源宽高 |
| scale 预设 | `原始` |
| scale.w / h | 源宽高 |

探测失败：状态行显示错误；**仍允许**用户手动输入所有字段（不阻塞）。

---

## 4. 各操作详细设计

### 4.1 时间裁剪

| 字段 | 控件 | 默认 | 说明 |
|------|------|------|------|
| 起始 | 文本框 | `00:00:00.000` | 支持 `HH:MM:SS[.ms]` 或纯秒数 |
| 结束 | 文本框 | `<duration>` | 同上 |

**命令贡献**：`-ss <start> -to <end>`，位置在 `-i` 之后。

**校验**：
- 两个时间都能被前端 parser 解析为秒
- `start < end`
- `end <= duration`（前端 clamp）

### 4.2 空间裁剪

| 字段 | 控件 | 默认 | 说明 |
|------|------|------|------|
| X | 数字 | `0` | 矩形左上角 X（像素） |
| Y | 数字 | `0` | 矩形左上角 Y |
| 宽 | 数字 | `<源宽>` | 矩形宽（像素） |
| 高 | 数字 | `<源高>` | 矩形高 |

**命令贡献**：加入 filter chain `crop=W:H:X:Y`

**校验**：
- 四值皆为非负整数
- `宽 > 0` 且 `高 > 0`
- `X + 宽 <= 源宽`、`Y + 高 <= 源高`

### 4.3 分辨率缩放

| 字段 | 控件 | 默认 | 说明 |
|------|------|------|------|
| 预设 | select | `原始` | `原始 / 480p / 720p / 1080p / 4K / 自定义` |
| 保持比例 | checkbox | 关 | 勾上时某一维可留空，另一维自动算 |
| 宽 | 数字 | `<源宽>` | |
| 高 | 数字 | `<源高>` | |

**预设联动**：切预设 → 自动写入对应宽高；手动改宽高 → 预设重置为"自定义"。

**保持比例**：勾上时，若宽为空/ 0 → 传 `W=-2`；若高为空/0 → 传 `H=-2`（ffmpeg 自动等比并对齐偶数）。

**命令贡献**：加入 filter chain `scale=W:H`

### 4.4 组合规则

**Filter chain**：crop 和 scale 同时启用 → 一个 `-vf`：
```
-vf crop=1280:720:0:0,scale=854:480
```
顺序：先 crop 后 scale（直观：先剪再缩放）。

**编码器**：裁剪/缩放都要求重编码，**禁止** `videoEncoder == "copy"`；前端把 Convert 的 `copy` 选项去掉，后端兜底校验。

**只启用 trim 时**：仍重编码（第 §2.2 决策）。

---

## 5. 后端 API

### 5.1 `POST /api/trim/probe`

**请求**
```json
{ "path": "C:/videos/sample.mp4" }
```

**响应**
```jsonc
{
  "format": { "duration": 5025.3, "bitRate": 8000000, "size": 123456789 },
  "video":  { "codecName": "h264", "width": 1920, "height": 1080, "frameRate": 29.97 },
  "audio":  { "codecName": "aac",  "channels": 2, "sampleRate": 48000, "bitRate": 128000 }
}
```

- 视频 / 音频任一可为空对象（例如纯音频源 → video 为 `{}`）
- 走 ffprobe `-show_format -show_streams`（不带 `-select_streams`，拿所有流）

### 5.2 `POST /api/trim/start`

```jsonc
{
  "inputPath": "...",
  "outputDir": "...",
  "outputName": "...",
  "format": "mp4",
  "videoEncoder": "h264",
  "audioEncoder": "aac",
  "overwrite": false,

  "trim":  { "enabled": true,  "start": "00:00:10", "end": "00:00:30" },
  "crop":  { "enabled": false, "x": 0, "y": 0, "w": 1920, "h": 1080 },
  "scale": { "enabled": true,  "w": 1280, "h": -2 }
}
```

- 未启用的块可以省略或 `enabled=false`
- `scale.h = -2` 表示"自动保持比例"（ffmpeg 语义），`-1` 也接受

返回：与 convert/audio 一致（`{ ok, command }` 或 409 覆盖确认）。

### 5.3 `POST /api/trim/cancel`

`s.jobs.Cancel()`。与 convert / audio 共享 Job。

---

## 6. 命令构建规则（`buildTrimArgs`）

```go
func buildTrimArgs(req TrimRequest) ([]string, string, error) {
    if req.InputPath == "" || req.OutputDir == "" || req.OutputName == "" || req.Format == "" {
        return nil, "", fmt.Errorf("missing required fields")
    }
    if !req.Trim.Enabled && !req.Crop.Enabled && !req.Scale.Enabled {
        return nil, "", fmt.Errorf("至少启用一项操作")
    }

    videoCodec := normalizeVideoCodec(req.VideoEncoder)  // 与 convert 共享
    if videoCodec == "copy" {
        return nil, "", fmt.Errorf("裁剪/缩放需要重编码，请选择具体的视频编码器")
    }

    outputPath := filepath.Join(req.OutputDir, req.OutputName+"."+req.Format)
    args := []string{"-y", "-i", req.InputPath}

    if req.Trim.Enabled {
        args = append(args, "-ss", req.Trim.Start, "-to", req.Trim.End)
    }

    var filters []string
    if req.Crop.Enabled {
        filters = append(filters, fmt.Sprintf("crop=%d:%d:%d:%d",
            req.Crop.W, req.Crop.H, req.Crop.X, req.Crop.Y))
    }
    if req.Scale.Enabled {
        filters = append(filters, fmt.Sprintf("scale=%d:%d", req.Scale.W, req.Scale.H))
    }
    if len(filters) > 0 {
        args = append(args, "-vf", strings.Join(filters, ","))
    }

    args = append(args, "-c:v", videoCodec, "-c:a", req.AudioEncoder, outputPath)
    return args, outputPath, nil
}
```

纯函数 → 表驱动测试友好。

---

## 7. 前端状态管理

### 7.1 模块定位
新增 `TrimTab`，与 `ConvertTab` / `AudioTab` 平级。不需要子模式。

### 7.2 时间字符串 helper
```js
Time.parse("01:23:45")    → 5025.0
Time.parse("1:23")        → 83.0
Time.parse("90")          → 90.0
Time.parse("00:01:30.5")  → 90.5
Time.format(5025.0)       → "01:23:45.000"
```
提交前强制 format 一次，保证后端拿到 `HH:MM:SS.mmm` 标准格式。

### 7.3 启用开关
每个 `<fieldset>` 的 checkbox 绑定：
```js
checkbox.addEventListener("change", () => {
  fieldset.disabled = !checkbox.checked;
  onChange();
});
```

### 7.4 预设联动
```js
presetSelect.addEventListener("change", () => {
  const p = PRESETS[presetSelect.value];
  if (p) { widthInput.value = p.w; heightInput.value = p.h; }
  // preset == "custom" 时什么都不做
  onChange();
});

[widthInput, heightInput].forEach(el => {
  el.addEventListener("input", () => {
    // 用户改了手动值 → preset 回到 custom
    presetSelect.value = "custom";
    onChange();
  });
});
```

---

## 8. 边界情况与错误处理

| 情形 | 处理 |
|------|------|
| 起始 >= 结束 | 前端禁止提交 |
| 结束 > duration | 前端 clamp；后端不强验（让 ffmpeg 自然截到末尾，更宽容） |
| 时间格式不合法 | 前端 parser 抛错，alert |
| crop 越界 | 前端禁止；后端用 ffprobe 再验一次（可选） |
| crop 宽/高 <= 0 | 前端禁止 |
| scale 宽高都为空/0 | 前端禁止（至少一维明确） |
| 未启用任何操作 | 开始按钮 disabled |
| videoEncoder=copy | 后端 400 返回中文错误 |
| ffprobe 失败 | 前端状态行显示错误；允许手动填所有字段 |
| 目标文件已存在 | 409 + confirm（与 convert/audio 同一条路径） |

---

## 9. 实现切片

| # | 范围 | 说明 |
|---|------|-----|
| 1 | `service.ProbeVideo` + `/api/trim/probe` | 纯后端 |
| 2 | UI 骨架：Tab 启用、`<fieldset>` + 字段、命令预览占位 | 不发起任务 |
| 3 | ffprobe 自动填充 + 时间 parser / formatter | 状态行 + 三块默认值 |
| 4 | `buildTrimArgs` + `/api/trim/start` + 时间裁剪 MVP | 端到端可工作 |
| 5 | 空间裁剪 | 引入 filter chain |
| 6 | 分辨率缩放（预设联动 + 保持比例 -2） | 合并 filter |
| 7 | 表驱动测试（`server/trim_args_test.go`）+ 文档同步 | 同 audio slice 6 |

> 第 4 片之前都是纯"搭建"。4 片落地后就有可交付 MVP（只支持时间裁剪），之后 5 / 6 是增量。

---

## 10. 与既有文档的关系

- `feature-design.md`：Tab 表把「视频裁剪」从 🚧 改 ✅；加一句指向本文档
- `roadmap.md`：移除 §1.2；在里程碑加 v1.8 条目
- `module-design.md`：新增 `server/trim_args.go` 说明
- 本文档：作为视频裁剪功能的权威参考
