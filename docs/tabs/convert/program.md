# 视频转换 — 程序设计

> 对应产品设计:[product.md](product.md)。共享后端模块见 [core/modules.md](../../core/modules.md);前端架构见 [core/frontend.md](../../core/frontend.md)。

## 1. 后端文件

| 文件 | 职责 |
|------|------|
| `server/handlers.go` | `handleConvertStart` / `handleConvertCancel` / `handleConvertStream` + 共享 fs/config/ffmpeg 接口 |
| (内嵌于 `handlers.go`)`buildFFmpegArgs(req)` | 纯函数 convert 命令构建器 |
| (内嵌于 `handlers.go`)`normalizeVideoCodec` / `normalizeAudioCodec` | 前端 value → ffmpeg 参数映射 |

## 2. API 路由

| 路由 | Handler | 作用 |
|------|---------|------|
| `POST /api/convert/start` | `handleConvertStart` | 校验 → `buildFFmpegArgs` → `jobs.Start`;接受 `dryRun` / `overwrite` |
| `POST /api/convert/cancel` | `handleConvertCancel` | `jobs.Cancel()` |
| `GET /api/convert/stream` | `handleConvertStream` | SSE;订阅 `jobs.Subscribe` → 写 `data: <json>\n\n` + Flush(**所有 Tab 共享**) |

请求体:

```jsonc
POST /api/convert/start
{
  "inputPath": "...",
  "outputDir": "...",
  "outputName": "...",
  "videoCodec": "h264",
  "audioCodec": "aac",
  "format": "mp4",
  "overwrite": false,
  "dryRun": false
}
```

响应:

- 校验失败 → `400 { error: "..." }`
- 同一 Job 已运行 → `409 { error: "another job is running" }`
- 目标已存在且未授权覆盖 → `409 { error:"file exists", existing:true, path:"..." }`
- DryRun 成功 → `200 { ok:true, dryRun:true, command:"ffmpeg ..." }`
- 成功 → `200 { ok:true, command:"ffmpeg ..." }`

`overwrite` / `dryRun` 字段语义见 [core/modules.md §2.3](../../core/modules.md)。

## 3. 命令构建器

```go
// buildFFmpegArgs 是纯函数,无 I/O,便于测试
func buildFFmpegArgs(req convertRequest) []string {
    args := []string{"-y", "-i", req.InputPath}

    vcodec := normalizeVideoCodec(req.VideoCodec)
    acodec := normalizeAudioCodec(req.AudioCodec)

    if vcodec == "copy" && acodec == "copy" {
        args = append(args, "-c", "copy")
    } else {
        args = append(args, "-c:v", vcodec, "-c:a", acodec)
    }

    outPath := filepath.Join(req.OutputDir, req.OutputName + "." + req.Format)
    args = append(args, outPath)
    return args
}

func normalizeVideoCodec(name string) string {
    // h264 → libx264,h265 → libx265,其他原样
}

func normalizeAudioCodec(name string) string {
    // 空字符串默认 aac
}
```

详见 [tabs/convert/product.md §7](product.md)。

## 4. 前端结构

`server/web/index.html` 的 `<section id="panel-convert">` + `app.js` 的 `ConvertTab` IIFE。

`ConvertTab` 职责:

- 文件 / 目录选择(走 `Picker.open({mode:"file"})` / `Picker.open({mode:"dir"})`)
- 文件名自动补 `_converted` 后缀
- 编码器 / 格式下拉
- 实时命令预览(`updateCommandPreview` 监听所有 input/change)
- 调 `createJobPanel({startUrl: "/api/convert/start", cancelUrl: "/api/convert/cancel"})` 拿到任务面板控制器
- 启动按钮 → `panel.start({url, body, outputPath})` 自动走 dryRun + Confirm.command + Confirm.overwrite + SSE

整套交互流(命令预览 → 覆盖确认 → 进度条 → 完成条)由 `createJobPanel` 工厂封装,见 [core/frontend.md §3](../../core/frontend.md)。

## 5. 状态机

| Convert Tab 状态 | 表现 |
|-----------------|------|
| 空闲 | "开始转码" enabled、"取消" disabled、状态文字"空闲" |
| 启动中(dryRun + Confirm) | "开始转码" disabled、Confirm dialog 在前 |
| 转码中 | "开始转码" disabled、"取消" enabled、状态文字"转码中…"、进度条显示 |
| 完成 | 完成条出现、按钮恢复空闲、进度条 100% 600ms 后隐藏 |
| 失败 | 完成条红色错误、按钮恢复空闲、进度条立即隐藏 |
| 取消 | 完成条黄色"已取消"、按钮恢复空闲、进度条立即隐藏 |

## 6. 测试

当前缺测试。计划补:

- `buildFFmpegArgs` 表驱动:copy/copy、copy/aac、h264/copy、h264/aac 矩阵
- `normalizeVideoCodec` / `normalizeAudioCodec` 边界值

见 [core/roadmap.md §2.6](../../core/roadmap.md)。

## 7. 相关代码片段

```go
// server/handlers.go(简化示意)
func (s *Server) handleConvertStart(w http.ResponseWriter, r *http.Request) {
    var req convertRequest
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        http.Error(w, err.Error(), 400)
        return
    }

    args := buildFFmpegArgs(req)
    cmd := "ffmpeg " + strings.Join(args, " ")  // 命令字符串(预览用)

    if req.DryRun {
        json.NewEncoder(w).Encode(map[string]any{
            "ok": true, "dryRun": true, "command": cmd,
        })
        return
    }

    outPath := args[len(args)-1]
    if !req.Overwrite {
        if _, err := os.Stat(outPath); err == nil {
            w.WriteHeader(409)
            json.NewEncoder(w).Encode(map[string]any{
                "error": "file exists", "existing": true, "path": outPath,
            })
            return
        }
    }

    if err := os.MkdirAll(req.OutputDir, 0o755); err != nil {
        http.Error(w, err.Error(), 500)
        return
    }
    if err := s.jobs.Start(service.GetFFmpegPath(), args); err != nil {
        http.Error(w, err.Error(), 409)
        return
    }
    json.NewEncoder(w).Encode(map[string]any{"ok": true, "command": cmd})
}
```
