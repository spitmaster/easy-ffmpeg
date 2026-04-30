# 音频处理 — 程序设计

> 对应产品设计:[product.md](product.md)。共享后端模块见 [core/modules.md](../../core/modules.md);前端架构见 [core/frontend.md](../../core/frontend.md)。

## 1. 后端文件

| 文件 | 职责 |
|------|------|
| `server/handlers_audio.go` | `/api/audio/*` 三个 handler:probe / start / cancel,外加 `scheduleCleanup` 帮助 merge 清理临时列表文件 |
| `server/audio_args.go` | 纯函数:`AudioRequest → ffmpeg args`,三模式分派 |
| `server/audio_args_test.go` | 表驱动测试 |

## 2. API 路由

| 路由 | Handler | 作用 |
|------|---------|------|
| `POST /api/audio/probe` | `handleAudioProbe` | `service.ProbeAudio` → JSON |
| `POST /api/audio/start` | `handleAudioStart` | `BuildAudioArgs`(convert / extract / merge;merge 的 `auto` 策略在此通过 `resolveMergeStrategy` 用 ffprobe 解析);接受 `dryRun` / `overwrite` |
| `POST /api/audio/cancel` | `handleAudioCancel` | `jobs.Cancel()` |

SSE 沿用 `GET /api/convert/stream`(三 Tab 共用)。

### 2.1 请求体 (`POST /api/audio/start`)

```jsonc
{
  "mode": "convert" | "extract" | "merge",

  // convert 专用
  "inputPath": "...",
  "outputDir": "...",
  "outputName": "...",
  "format": "mp3",
  "codec": "libmp3lame",
  "bitrate": 192,          // kbps;数字 0 表示"不设置"
  "sampleRate": 44100,     // 0 表示保持原始
  "channels": 2,           // 0 表示保持原始
  "overwrite": false,
  "dryRun": false,         // true → 走完构建但不启 ffmpeg、不查 overwrite、不动文件;merge 临时 list 文件立即 cleanup

  // extract 额外
  "audioStreamIndex": 0,   // ffprobe 音频流内部序号
  "extractMethod": "copy" | "transcode",

  // merge 额外
  "inputPaths": ["...", "..."],
  "mergeStrategy": "auto" | "copy" | "reencode"
}
```

### 2.2 响应

- 校验失败 → `400 { error: "..." }`
- 同一 Job 已运行 → `409 { error: "another job is running" }`
- 目标已存在且未授权覆盖 → `409 { error:"file exists", existing:true, path:"..." }`
- DryRun 成功 → `200 { ok:true, dryRun:true, command:"ffmpeg ..." }`
- 成功 → `200 { ok:true, command:"ffmpeg ..." }`

### 2.3 `POST /api/audio/probe`

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

调用:`ffprobe -v quiet -print_format json -show_format -show_streams -select_streams a <path>`。

## 3. 命令构建器(`audio_args.go`)

纯函数,无 I/O(merge 的 copy 策略涉及临时文件,但封装在 `writeConcatList` + 返回 `Cleanup` 闭包里,便于测试)。

| 符号 | 说明 |
|------|------|
| `AudioRequest` struct | 三模式的请求体联合(convert/extract/merge 各取所需字段) |
| `AudioBuildResult` struct | `{Args, OutputPath, Cleanup}` |
| `BuildAudioArgs(req)` | 分派到各模式构建器 |
| `buildConvertAudioArgs` | 音频格式转换 / 压缩 |
| `buildExtractAudioArgs` | 从视频提取音轨(`-vn -map 0:a:<idx>`,copy 或 transcode) |
| `buildMergeAudioArgs` | 合并:`copy` 走 concat demuxer + 临时列表文件;`reencode` 走 `-filter_complex concat` |
| `formatConcatList(paths)` | 生成 `-f concat` 列表文件内容;单引号转义 |
| `bitrateApplies(spec, codec, bitrate)` | 判定是否加 `-b:a`(lossless 容器 / PCM / copy 都抑制) |
| `audioFormatTable` | 容器 → 合法编码器白名单(mp3/m4a/flac/wav/ogg/opus) |

详见 [product.md §4.3 / §5.2 / §6.3](product.md)。

## 4. 测试覆盖

| 文件 | 覆盖 |
|------|------|
| `server/audio_args_test.go` | convert / extract / merge 三种模式的正反路径,formatConcatList 单引号转义,bitrateApplies 矩阵 |

## 5. 前端结构

`server/web/index.html` 的 `<section id="panel-audio">` + `app.js` 的若干 IIFE:

| 模块 | 职责 |
|------|------|
| `AudioCodecs` | 共享的容器/编码器/码率知识(DRY) |
| `AudioConvertMode` | 模式一表单字段组与命令预览 |
| `AudioExtractMode` | 模式二表单 + ffprobe 音轨选择 |
| `AudioMergeMode` | 模式三文件列表 + 拖拽/排序 |
| `AudioTab` | 顶层挂载:三模式 + segmented 切换 + 调用 `createJobPanel` |

每个模式独立持有状态;切 Tab 时 UI 销毁但 JS 里的表单值保留(用户切回来还能继续)。

## 6. Merge 模式的临时文件管理

```go
type AudioBuildResult struct {
    Args       []string
    OutputPath string
    Cleanup    func()  // 必须在任务结束(成功/失败/取消/dryRun)时调用
}
```

- `buildMergeAudioArgs` 的 copy 策略写临时 list 文件,返回的 `Cleanup` 删除它
- handler 必须保证调用:成功路径 → 在 `jobs.Subscribe` 的终态事件后调;dryRun 路径 → 立即调;启动失败 → 立即调
- 不调会留下 `%TEMP%/easy-ffmpeg-merge-*.txt` 残留 —— 不致命但难看

## 7. 自动判断的实现(`resolveMergeStrategy`)

```text
对每个输入跑 ffprobe,提取 codec_name / sample_rate / channels / bit_rate
比较所有输入的字段是否一致(bit_rate 允许 ±10% 浮动)
全一致 → 返回 "copy"
有差异 → 返回 "reencode"
任一 ffprobe 失败 → 返回 "reencode"(保守路径,反正能跑)
```

handler 接到 `mergeStrategy: "auto"` 时调此函数;接到 `"copy"` / `"reencode"` 直接尊重用户选择。

## 8. 协议字段对齐

`overwrite` / `dryRun` 与 convert / editor 同形,三 endpoint 共享同一份 `Confirm` + `createJobPanel.start` 前端流程。详见 [core/modules.md §2.3](../../core/modules.md)。
