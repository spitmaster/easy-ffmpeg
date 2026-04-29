# 前端 Vue 化改造方案(v0.5.x)

> 本文档定义把现有原生 HTML+JS 前端(`server/web/`)迁移到 Vue 3 + Vite + TypeScript 的完整路径。改造的真正目的是**为后续多轨剪辑器(类 Premiere Pro)做技术储备**——多轨编辑器的状态复杂度(轨道 × clip × 选择 × 播放头 × 撤销栈 × 拖拽预览)用原生 JS 维护成本会指数级增长,框架是必经之路。

---

## 0. 决策摘要

| 问题 | 选择 | 关键原因 |
|------|------|---------|
| 用不用框架 | **用** | 多轨剪辑器的状态耦合无法用原生 JS 优雅维护 |
| 选哪个框架 | **Vue 3 + TypeScript** | 细粒度响应式适合"小操作触发局部更新"的剪辑器;Wails 一等公民;团队学习成本低 |
| 构建工具 | **Vite** | 事实标准,与 Vue/TS 零配置 |
| 状态管理 | **Pinia** | Vue 官方推荐;支持插件机制(撤销栈、持久化) |
| 样式方案 | **TailwindCSS + 自建组件**(不用通用 UI 库) | Element Plus / Naive UI 等为后台管理设计,与 Premiere 风格暗色密集界面冲突 |
| 时间轴渲染 | **Canvas(预留 Konva 或 Pixi)** | DOM 在百级 clip 后开始卡;先不引入,M3 评估 |
| 迁移策略 | **增量迁移,不一次性重写** | 老 UI 保持可用,新 UI 跑通后逐 tab 平移 |
| 版本切分 | **v0.5.0 = M1+M2**,**v0.5.1 = M3+M4** | 每个发版都有用户可见进展(详见 §9) |
| 桌面 splash | **不 Vue 化** | 启动画面要尽可能快,引入框架反而拖慢(详见 §9) |
| 测试 / 代码风格 | 本轮不引入 Vitest;ESLint + Prettier 最小集 | 优先把功能跑起来,避免节奏被工具配置拖慢(详见 §9) |

---

## 1. 目标与非目标

**目标(本次改造完成的判据)**

- 仓库根新增 `web/` 工程目录,Vue 3 + Vite + TS + Pinia + Tailwind 跑通
- Web 版与桌面版均通过 Vite 构建产物提供 UI,与现状视觉一致、功能等价
- `build.bat` / `build.sh` 自动跑 `npm install` + `npm run build`,然后 `go build`
- 现有 3 个 tab(视频转换 / 音频处理 / 单视频剪辑)在新栈上行为完全一致
- 老的 `server/web/` 静态文件目录被删除

**非目标(本次不做)**

- 不实现多轨剪辑器(留给后续里程碑)
- 不改后端任何 JSON API
- 不改桌面版 splash 页([cmd/desktop/frontend/dist/index.html](../../cmd/desktop/frontend/dist/index.html) 保持原样)
- 不引入 SSR / SSG(纯 SPA)
- 不引入测试框架(本轮节奏为先,后续单独评估 Vitest)

---

## 2. 技术栈版本基线

| 依赖 | 版本基线 | 用途 |
|------|---------|------|
| Node.js | ≥ 20.x LTS | Vite 5+ 要求 |
| Vue | `^3.5` | 主框架 |
| TypeScript | `^5.5` | 类型系统 |
| Vite | `^5.4` | 构建/开发服务器 |
| Pinia | `^2.2` | 状态管理 |
| Vue Router | `^4.4` | 多页面/Tab 路由(为多轨剪辑器铺路) |
| TailwindCSS | `^3.4` | 原子化样式 |
| @vueuse/core | `^11` | 通用 Composition 工具(键盘、节流等) |
| radix-vue | `^1.9` | 无样式可访问性原语(后续按需) |

---

## 3. 目录结构改造

### 3.1 改造前

```text
easy-ffmpeg/
├── server/
│   ├── server.go            // //go:embed web
│   └── web/                 // 静态前端(源码=产物)
│       ├── index.html
│       ├── app.js
│       ├── app.css
│       └── editor/
└── cmd/
    └── desktop/
        ├── main.go          // //go:embed all:frontend/dist
        └── frontend/
            ├── dist/        // splash shell(保持原样)
            └── wailsjs/     // Wails 生成的 runtime
```

### 3.2 改造后

```text
easy-ffmpeg/
├── web/                     // ★ 前端工程目录(源码 + 产物都在这里)
│   ├── package.json
│   ├── vite.config.ts
│   ├── tsconfig.json
│   ├── tailwind.config.js
│   ├── postcss.config.js
│   ├── index.html
│   ├── embed.go             // ★ 极薄 Go 文件,把 dist/ 暴露给 server
│   ├── public/              // 静态资源(favicon 等)
│   ├── src/
│   │   ├── main.ts          // 入口
│   │   ├── App.vue
│   │   ├── router/
│   │   │   └── index.ts
│   │   ├── stores/          // Pinia stores
│   │   │   ├── ffmpeg.ts    // ffmpeg 状态、版本号
│   │   │   └── jobs.ts      // 任务队列
│   │   ├── api/             // fetch 封装,对应后端 /api/*
│   │   │   ├── client.ts
│   │   │   ├── ffmpeg.ts
│   │   │   ├── convert.ts
│   │   │   ├── audio.ts
│   │   │   └── editor.ts
│   │   ├── views/           // 每个 tab 一个 .vue
│   │   │   ├── ConvertView.vue
│   │   │   ├── AudioView.vue
│   │   │   └── EditorView.vue
│   │   ├── components/
│   │   │   ├── layout/      // TopBar / TabNav
│   │   │   ├── form/        // 复用的输入控件
│   │   │   └── feedback/    // Toast / Modal / Progress
│   │   └── styles/
│   │       ├── main.css     // Tailwind 入口 + 全局变量
│   │       └── tokens.css   // 设计 token(颜色/间距,为剪辑器铺路)
│   └── dist/                // ★ Vite 构建产物(gitignore,由 npm run build 生成)
│       ├── index.html
│       └── assets/
│
├── server/
│   ├── server.go            // 改:删除 //go:embed,import "easy-ffmpeg/web"
│   └── (web/ 目录被删除)
│
└── cmd/
    └── desktop/
        ├── main.go          // 不变
        └── frontend/        // 不变(splash + wailsjs)
```

**为什么这么放(关键设计决策)**

1. **诉求**:用户改前端时只想看一个地方,不想在 `server/web/` 和 `web/` 之间来回切。
2. **`go:embed` 路径限制**:embed 指令只能引用所在 .go 文件目录或其子目录,无法跨目录上溯。所以 `server/server.go` 不能直接 embed 项目根的 `web/dist/`。
3. **解法**:把 embed 下沉到 `web/embed.go`(与 dist/ 同目录),形成一个独立的 Go 包 `easy-ffmpeg/web`,server 通过 import 消费。这也符合"前端资源跟前端工程绑定,server 只是消费方"的语义。

**改造后的目录所有权**

| 目录 | 归属 | 谁会改 |
|------|------|--------|
| `web/src/` | 前端源码 | 你改前端时唯一要看的地方 |
| `web/dist/` | 构建产物 | 自动生成,gitignore |
| `web/embed.go` | 胶水(<10 行) | 写一次基本不动 |
| `server/` | 后端 Go 代码 | 不再混入前端文件 |

---

## 4. 关键接线(Vite ↔ Go embed)

### 4.1 Vite 配置(`web/vite.config.ts`)

```ts
import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
import path from 'node:path'

export default defineConfig({
  plugins: [vue()],
  resolve: {
    alias: { '@': path.resolve(__dirname, 'src') },
  },
  build: {
    outDir: 'dist',           // ★ web/dist/,与 web/embed.go 的 //go:embed all:dist 对齐
    emptyOutDir: true,        // 每次构建前清空,避免老文件残留
    assetsDir: 'assets',
    sourcemap: false,
  },
  server: {
    port: 5173,
    // 开发态:转发 /api/* 到运行中的 Go 后端
    proxy: {
      '/api': {
        target: 'http://127.0.0.1:8080',
        changeOrigin: false,
      },
    },
  },
})
```

> **开发流**:开发时跑两个进程——`go run ./cmd`(后端,API 服务)和 `cd web && npm run dev`(Vite 在 5173)。前端通过代理打到后端 API。生产构建时,`npm run build` 把产物写入 `web/dist/`,然后 `go build` 嵌入。

### 4.2 嵌入胶水(`web/embed.go`,新增,约 10 行)

```go
// Package web exposes the built frontend assets to the Go side. The
// actual UI sources live in src/; the dist/ subdirectory is produced by
// `npm run build` and is gitignored.
package web

import "embed"

//go:embed all:dist
var FS embed.FS
```

> **`all:` 前缀的作用**:默认 `go:embed` 会跳过以 `_` 或 `.` 开头的文件;Vite 在某些插件下可能产生 `.vite/` 临时目录,加 `all:` 一并嵌入,避免静态资源缺失。

### 4.3 server.go 改动(小手术)

把 [server/server.go](../../server/server.go) 当前的:

```go
//go:embed web
var webRoot embed.FS

// ...
func (s *Server) routes(mux *http.ServeMux) {
    sub, _ := fs.Sub(webRoot, "web")
    mux.Handle("/", http.FileServer(http.FS(sub)))
    // ...
}
```

改为:

```go
import (
    // ...
    web "easy-ffmpeg/web"
)

// (删除 //go:embed 与 webRoot 变量)

func (s *Server) routes(mux *http.ServeMux) {
    sub, _ := fs.Sub(web.FS, "dist")
    mux.Handle("/", http.FileServer(http.FS(sub)))
    // ...
}
```

> **改动幅度**:删 2 行,改 1 行,加 1 行 import。其他后端代码完全不动。

### 4.4 桌面版无改动

[cmd/desktop/main.go:23](../../cmd/desktop/main.go#L23) 的 `//go:embed all:frontend/dist` 不变。桌面版的链路:

1. Wails 加载 splash([cmd/desktop/frontend/dist/index.html](../../cmd/desktop/frontend/dist/index.html))
2. Splash 收到 `backend-ready` 事件 → `location.replace(http://127.0.0.1:port)`
3. WebView 跳转到 HTTP server,加载新的 Vue UI(由 `easy-ffmpeg/web` 包嵌入)

桌面版的 splash 与主 UI 是两套独立资源,互不干扰。

### 4.3 桌面版无改动确认

[cmd/desktop/main.go:23](../../cmd/desktop/main.go#L23) 的 `//go:embed all:frontend/dist` 不变。桌面版的工作流:

1. Wails 加载 splash([cmd/desktop/frontend/dist/index.html](../../cmd/desktop/frontend/dist/index.html))
2. Splash 收到 `backend-ready` 事件 → `location.replace(http://127.0.0.1:port)`
3. WebView 跳转到 HTTP server,加载新的 Vue UI

**唯一需要验证**:Vue 的 SPA 路由不能与 Wails AssetServer 冲突。由于步骤 2 后 WebView 的 origin 是 HTTP server 而非 Wails AssetServer,无冲突风险。

### 4.5 SPA 路由与 Go FileServer 兼容

Vue Router 用 **hash 模式**(`createWebHashHistory`),URL 形如 `/#/convert`、`/#/editor`。这样 Go 的 `http.FileServer` 永远只命中 `/`(返回 `index.html`),无需写 fallback 路由。

> 备选:用 history 模式 + 在 [server/server.go](../../server/server.go) 加 SPA fallback。但 hash 模式改动最小,本轮采用。

---

## 5. 构建脚本改造

### 5.1 `build.sh` 增量

在现有 `build()` 之前插入前端构建:

```bash
build_frontend() {
    if ! command -v npm >/dev/null 2>&1; then
        echo "ERROR: npm not found. Install Node.js >= 20." >&2
        exit 1
    fi
    (cd web && npm install --no-audit --no-fund && npm run build)
}

build_frontend     # ← 新增
build windows amd64 easy-ffmpeg.exe
# ... 后面不变
```

### 5.2 `build.bat` 增量

对应的 cmd.exe 版本:

```batch
where npm >nul 2>nul || (echo ERROR: npm not found && exit /b 1)
pushd web && call npm install --no-audit --no-fund && call npm run build && popd || exit /b 1

REM ... 后面 go build 部分不变
```

### 5.3 `.gitignore` 调整

```text
web/node_modules/
web/dist/         # ★ 构建产物,不入 git
```

`web/embed.go` 必须入 git(它是 Go 源码,不是产物)。

> **迁移期一次性操作**:首先 `git rm -r server/web/` 把老的手写静态文件从仓库删除。M4 完成时整个 `server/web/` 目录消失。

---

## 6. 迁移里程碑(里程碑式分阶段)

每个里程碑结束都是一个**可发版状态**——任何里程碑后停下来,产物都能跑。

### M1 — 脚手架就绪(v0.5.0,预计 2 天)

**产出**

- `web/` 工程齐全(package.json / vite / TS / Tailwind / Pinia / Router 都接通)
- `web/embed.go` 写入,`server/server.go` 改为 import `easy-ffmpeg/web`,删除原 `//go:embed web`
- `server/web/` 整个目录从仓库删除(`git rm -r server/web/`)
- 一个最小 `App.vue`,显示 "Easy FFmpeg" 顶栏 + 占位三 tab(无功能)
- `npm run build` → 写入 `web/dist/`
- `go run ./cmd` 启动后浏览器看到新的 Vue 空壳
- 桌面版本机构建一次,验证 splash → Vue 主页跳转链路通

**验收**

- ✅ 浏览器访问 `http://127.0.0.1:<port>` 显示 Vue 顶栏
- ✅ 桌面版双击 exe,WebView 显示 Vue 顶栏
- ✅ `build.sh` / `build.bat` 一键跑通

### M2 — 顶栏 + 转换 Tab 迁移(v0.5.0,预计 2 天)

**范围**

- TopBar 组件:logo / 标题 / 版本 chip / ffmpeg 状态 chip / 退出按钮
- `useFfmpegStore`:`/api/ffmpeg/status`、`/api/version` 接入
- ConvertView:输入文件、输出目录、编码器/格式、码率、分辨率、覆盖确认、进度条
- API 封装层:`api/client.ts` 统一 fetch 错误处理 + 取消信号

**验收**

- ✅ 三 tab 中"视频转换"功能与老版完全一致(对照 [server/web/app.js](../../server/web/app.js) 逐项核对)
- ✅ 文件浏览器 / 路径补全行为一致
- ✅ 任务进度轮询、错误提示、完成后打开输出目录都正常

### M3 — 音频 Tab + 单视频剪辑 Tab 迁移(v0.5.1,预计 3–4 天)

**范围**

- AudioView:对照 `server/web/app.js` 的音频处理逻辑平移
- EditorView:迁移 [server/web/editor/editor.js](../../server/web/editor/editor.js)(1908 行,最复杂)
  - 时间轴交互、入出点标记、波形显示等
  - 可在此阶段引入 Canvas 渲染基础设施(Konva 或原生),为多轨编辑器铺路

**验收**

- ✅ 三个 tab 全部用 Vue 实现,功能等价
- ✅ 老的 [server/web/app.js](../../server/web/app.js) / [server/web/editor/](../../server/web/editor/) 不再被任何代码引用

### M4 — 清理(v0.5.1,预计 0.5 天)

**范围**

- 确认 `server/web/` 已从仓库消失(M1 已执行 git rm,这里只是复核)
- `.gitignore` 已包含 `web/dist/` 与 `web/node_modules/`
- 设计文档同步:[design/core/architecture.md](architecture.md)、[CLAUDE.md](../../CLAUDE.md) 更新前端章节(指向 `web/`)
- **重写或删除已过时的 [frontend.md](frontend.md) 与 [ui-system.md](ui-system.md)**(它们描述的是已被替换的零构建 IIFE 架构,M2 完成后内容大部分已失效)
- [design/core/roadmap.md](roadmap.md) 增加 v0.5.x 行
- [design/milestones.md](../milestones.md):把"前端 Vue 化迁移"整段移到底部"已归档"区

**验收**

- ✅ `git status` 干净
- ✅ 任意干净 clone 后跑一次 `build.sh` 能产出可用 Web + 桌面版
- ✅ 文档与代码一致

---

## 7. 多轨剪辑器的预备(本次只做铺路)

虽然多轨剪辑器留待后续里程碑,但本轮的目录结构和依赖选型为它预留:

| 预留项 | 在哪里铺路 |
|--------|----------|
| 撤销/重做 | Pinia + 自定义历史中间件(M2 写 store 时埋接口) |
| 设计 token(暗色 / 紧凑) | `web/src/styles/tokens.css`,Tailwind 主题继承(M1) |
| Canvas 渲染 | M3 引入 Konva 时建立 `components/canvas/` 子树 |
| 键盘快捷键 | `@vueuse/core` 的 `useMagicKeys`,M2 起在 EditorView 试点 |
| 大状态序列化 | Pinia + `pinia-plugin-persistedstate`(后续按需) |

---

## 8. 风险与回滚

| 风险 | 影响 | 缓解 |
|------|------|------|
| Wails AssetServer 与 SPA 路由冲突 | 桌面版白屏 | hash 路由(§4.4)规避;实际 Vue UI 走 HTTP server,与 Wails 无关 |
| Vite `emptyOutDir: true` 误删 `web/embed.go` | 编译失败 | embed.go 在 `web/` 根,outDir 是 `web/dist/`,层级隔离;Vite 的 emptyOutDir 只清 outDir 内 |
| `npm install` 在没网络的环境失败 | 构建中断 | 文档明确要求开发机有 npm + 网络;CI 加 npm 缓存 |
| 老 editor.js 行为细节迁移遗漏 | 单视频剪辑回归 | M3 完成后并排开两版手工对照,验收清单逐项过 |
| Tailwind 与现有暗色基调不匹配 | 视觉走样 | 把现有 [app.css](../../server/web/app.css) 的颜色提取成 token,Tailwind 主题继承 |

**回滚策略**:每个里程碑独立 commit。M2/M3 出现严重阻塞时,`git revert` 到 M1 之前(`server/web/` 还在的状态)即可恢复老版本发版。M1 这一步合并删除老前端 + 引入新工程,所以**回滚单位是整个 M1**;若担心,可以把 M1 拆成 "M1a 引入 web/" 与 "M1b 删除 server/web/" 两个 commit。

---

## 9. 已决策事项

| # | 议题 | 决策 | 理由 |
|---|------|------|------|
| 1 | 版本切分 | **v0.5.0 = M1+M2**,**v0.5.1 = M3+M4** | 每个发版都有用户可见进展;v0.5.0 出来时三 tab 已有"转换"功能跑在新栈上,可以提前暴露问题 |
| 2 | 是否本轮引入 Vitest 单元测试 | **不引入** | 与"先跑起来"节奏冲突;留到多轨剪辑器开工时单独评估 |
| 3 | 桌面版 splash 是否 Vue 化 | **不** | splash 是启动画面(见下文说明),加载越快越好,引入 Vue 运行时反而拖慢 |
| 4 | ESLint + Prettier | **最小集** | Prettier 默认配置 + Vue 官方 ESLint preset,避免风格争议又不过度配置 |

**关于 splash(启动画面)的说明**

[cmd/desktop/frontend/dist/index.html](../../cmd/desktop/frontend/dist/index.html) 是桌面版的 splash——一个 97 行的纯 HTML 占位页,显示 logo + "正在启动后端服务…"。桌面版的启动顺序:

1. 用户双击 `easy-ffmpeg-desktop.exe`
2. Wails 在 0.5 秒内弹窗,WebView 立即显示 splash
3. Go 后端在后台启动 HTTP server(首次启动还要解压 ffmpeg,可能 25–45 秒)
4. 后端就绪后发出 `backend-ready` 事件,splash 的 JS 把 WebView 跳转到 `http://127.0.0.1:<port>`
5. 此后看到的就是 Vue 主 UI

splash 的核心目标是**尽可能快地让用户看到画面**,这正好是 Vue 这类框架的反方向。所以保持现有纯 HTML 实现,Vue 改造完全不动这个文件。

---

## 10. 文档维护

本次改造涉及的设计文档同步:

- [architecture.md](architecture.md):前端章节(若有)更新,新增 `web/` 入口说明
- [build.md](build.md):§2.1 构建脚本核心逻辑加 `build_frontend`;§8 桌面版构建说明前端依赖
- [roadmap.md](roadmap.md):新增 v0.5.x 里程碑行
- [README.md](../README.md):索引表加本文档
- [CLAUDE.md](../../CLAUDE.md):前端章节(若有)更新

文档同步在 M4 一次性完成,避免中间态文档与代码不一致。
