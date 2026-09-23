# 前端架构(程序设计)

> 本文档定义前端的技术栈、目录组织、API 客户端层、Pinia store、composable、SSE 通道、解压进度轮询。视觉规范、控件外观、对话框约定见 [ui-system.md](ui-system.md)。
>
> v0.5.0 起,前端是一个独立的 Vue 3 + Vite + TypeScript 工程,源码与构建产物都在仓库根的 `web/` 目录下;Go 端通过 `easy-ffmpeg/web` 包消费 `web/dist/`。前端从此走"npm install + npm run build → go build"的两段式构建,见 [build.md §2](build.md)。

## 1. 技术栈

| 依赖 | 版本基线 | 用途 |
|------|---------|------|
| Vue | `^3.5` | 主框架(`<script setup>` + Composition API) |
| TypeScript | `^5.6` | 类型系统 |
| Vite | `^5.4` | 开发服务器 + 构建 |
| Pinia | `^2.2` | 状态管理(setup-store 风格) |
| Vue Router | `^4.5` | 三 Tab 路由(hash 模式) |
| TailwindCSS | `^3.4` | 原子化样式;颜色全部通过 token 注入,见 [ui-system.md](ui-system.md) |

通信协议没有变化(server-side 是同一个后端):

- `fetch + JSON` 用于命令式操作(版本、目录、转换、音频、剪辑器 CRUD 等)
- `EventSource` (SSE) 用于 FFmpeg 日志实时推送

> **桌面版宿主无感**:Wails WebView 加载的也是 `http://127.0.0.1:<port>`,所以前端代码不感知 WebView2 / WKWebView / WebKitGTK,见 [desktop.md §8](desktop.md)。

## 2. 目录结构

```text
web/
├── package.json / vite.config.ts / tsconfig.json
├── tailwind.config.js / postcss.config.js
├── index.html              SPA 入口,挂 #app
├── embed.go                Go 端的胶水:`//go:embed all:dist` → easy-ffmpeg/web 包
├── env.d.ts                Vite/Vue 类型声明
├── dist/                   ★ 构建产物;gitignore;由 npm run build 写入
└── src/
    ├── main.ts             createApp + Pinia + Router + main.css
    ├── App.vue             外壳:TopBar + TabNav + RouterView + 4 个全局对话框
    ├── router/index.ts     hash 模式,3 路由(/convert /audio /editor)
    ├── api/                fetch 封装(REST + SSE),与 server/ 路由一一对应
    │   ├── client.ts        fetchJson / postJson / postJsonRaw + ApiError
    │   ├── version.ts       /api/version
    │   ├── ffmpeg.ts        /api/ffmpeg/status
    │   ├── dirs.ts          /api/config/dirs
    │   ├── fs.ts            /api/fs/{home,list,reveal}
    │   ├── jobs.ts          单例 EventSource(/api/convert/stream)+ 订阅总线
    │   ├── prepare.ts       /api/prepare/status
    │   ├── quit.ts          /api/quit
    │   ├── convert.ts       /api/convert/{preview,start,cancel}
    │   ├── audio.ts         /api/audio/* + probe
    │   └── editor.ts        /api/editor/* + sourceUrl 拼接
    ├── stores/             Pinia(全部 setup-store)
    │   ├── version.ts       version 字符串
    │   ├── ffmpeg.ts        FFmpeg 状态 + 缓存目录揭示
    │   ├── dirs.ts          上次输入/输出目录(API 持久化)
    │   ├── modals.ts        4 个全局对话框的请求/解析(Promise 风格)
    │   └── editor.ts        Project + 选择 + 播放头 + 撤销栈 + 防抖自动保存
    ├── composables/
    │   ├── useJobPanel.ts       任务面板状态机(替代旧 createJobPanel 工厂)
    │   ├── useEditorPreview.ts  剪辑器双元素 <video>/<audio> 预览 + 间隙时钟
    │   └── useEditorOps.ts      剪辑器分割 / 删除 / 重排
    ├── components/
    │   ├── layout/         TopBar.vue / TabNav.vue
    │   ├── modals/         PickerModal.vue / ConfirmCommandModal.vue /
    │   │                  ConfirmOverwriteModal.vue / PrepareOverlay.vue
    │   ├── job/            JobLog.vue(三 Tab 共用的日志 + 进度条 + 完成条)
    │   ├── audio/          AudioConvertMode.vue / AudioExtractMode.vue / AudioMergeMode.vue
    │   └── editor/         EditorTopBar / EditorPlayBar / EditorTimeline /
    │                       EditorToolbar / EditorAudioVolume / EditorProjectsModal /
    │                       EditorExportDialog / EditorExportSidebar
    ├── views/              ConvertView.vue / AudioView.vue / EditorView.vue
    ├── utils/              path.ts / fmt.ts / time.ts / timeline.ts(纯函数)
    └── styles/
        ├── main.css         Tailwind 三件套 + 全局 box-sizing
        └── tokens.css       :root 颜色 token(供 Tailwind 通过 rgb(var(--…)) 读)
```

> **空 `assets/` / `public/` 没有刻意建**——目前所有静态依赖都是 emoji 文本,首屏不需要图片;有需要再补 `web/public/`(Vite 会原样拷到 `dist/` 根)。

## 3. 开发与构建流

```bash
# 开发(两进程)
go run ./cmd                       # 后端 8080
cd web && npm run dev              # Vite 5173,/api/* 经 vite.config.ts 代理到 8080

# 构建(集成,build.sh / build.bat 自动跑)
cd web && npm install --no-audit --no-fund
cd web && npm run build            # vue-tsc --noEmit + vite build → web/dist/
go build ./cmd                     # easy-ffmpeg/web 经 //go:embed all:dist 嵌入
```

`web/embed.go` 是仅 ~10 行的胶水:

```go
package web

import "embed"

//go:embed all:dist
var FS embed.FS
```

`server/server.go` 通过 `fs.Sub(web.FS, "dist")` 把它挂到 `/`,SPA 路由用 hash 模式,所以 Go FileServer 永远只命中 `/`,无需 fallback。

## 4. 启动顺序(`App.vue` + `main.ts`)

```ts
// main.ts
createApp(App).use(createPinia()).use(router).mount('#app')
```

```vue
<!-- App.vue (节选) -->
<script setup lang="ts">
const dirs = useDirsStore()
onMounted(async () => {
  jobBus.connect()   // 单例 SSE,启动即连;断开自动 1.5s 重连
  await dirs.load()  // 上次使用的输入/输出目录,失败静默
})
</script>

<template>
  <TopBar />
  <TabNav />
  <main><RouterView /></main>
  <PrepareOverlay />        <!-- 解压遮罩,内部自己轮询 -->
  <PickerModal />           <!-- 监听 modals.picker -->
  <ConfirmCommandModal />   <!-- 监听 modals.command -->
  <ConfirmOverwriteModal /> <!-- 监听 modals.overwrite -->
</template>
```

四个全局对话框只在 App 根挂载一次,通过 `useModalsStore()` 的 `showCommand` / `showOverwrite` / `showPicker` 函数命令式调用并 `Promise<boolean | string | null>` 等待用户操作。这是旧 `Confirm.command` / `Confirm.overwrite` / `Picker.open` Promise API 的 Vue 化等价物。

`TopBar` 内部 onMounted 时拉版本和 FFmpeg 状态;`PrepareOverlay` 通过 `prepareApi.status()` 自轮询直到 `state === "ready"`。

## 5. API 客户端层(`src/api/`)

每个文件对应后端的一个 endpoint 簇,只导出纯函数对象。共有的错误处理在 `client.ts`:

- `fetchJson<T>` / `getJson<T>` / `postJson<T>`:HTTP 非 2xx 抛 `ApiError`,保留服务端 `{error}` 文本
- `postJsonRaw`:返回 `{res, data}`,让调用方区分 200 / 409(覆盖确认)等状态码

各模块通常包一层"语义动词":`convertApi.preview(body)` / `convertApi.start(body)` / `convertApi.cancel()`。dryRun 协议封进 `preview` 内部(POST `{...body, dryRun: true}` 拿 `command` 字符串)。

## 6. SSE 总线(`api/jobs.ts`)

```ts
export type JobEvent =
  | { type: 'state'; running: boolean }
  | { type: 'log'; line: string }
  | { type: 'done' }
  | { type: 'error'; message: string }
  | { type: 'cancelled' }

export const jobBus = {
  connect(),                        // 启动一个 EventSource,onerror 1.5s 重连
  subscribe(fn): () => void,        // 返回 unsubscribe
}
```

只有一条 `EventSource('/api/convert/stream')`,所有 Tab 共用。`useJobPanel` 内部 `subscribe`,通过本地 `owning` 标志只让"自己发起任务的 panel"响应 log/done/error/cancelled,这是"全局事件流 + 各 Tab 独立 UI"的关键。

## 7. `useJobPanel` composable(替代旧 `createJobPanel` 工厂)

`composables/useJobPanel.ts` 是 convert / audio / editor 三视图共用的任务面板状态机,返回纯 ref + 几个 action。调用方:

```ts
const job = useJobPanel({
  cancelUrl: '/api/convert/cancel',
  runningLabel: '转码中...',
  doneLabel: '✓ 转码完成',
})

await job.startJob({
  outputPath,
  totalDurationSec,           // 编辑器导出时显式传节目时间总长
  request: () => realPost(),  // 内部完整生命周期:catch 启动失败、置 owning、回显命令
})
```

state 机制和旧版相同:

- `running` / `stateLabel` / `log[]` / `progress` / `progressVisible` / `finishVisible / Kind / Text` / `lastOutputPath`
- `appendLog`:进度行(正则 `^(frame=|size=|video:|Lsize=)`)就地覆盖上一行,日志增长不爆
- `parseForProgress`:嗅探 `Duration:` 与 `time=`,算 0..1 进度;`startJob({totalDurationSec})` 显式传值则不依赖 `Duration:`
- `done` 后停 100% 600ms 再隐藏进度条;`error/cancelled` 立即隐藏

调用 `cancel()` 是 POST 到构造时的 `cancelUrl`(`/api/convert/cancel` / `/api/audio/cancel` / `/api/editor/export/cancel`);`revealOutput()` 调 `fsApi.reveal(lastOutputPath)`。

## 8. 视图层(`views/`)+ 子组件

每个 Tab 对应一个 `views/<Name>View.vue`,内部按子区拆分:

- **ConvertView.vue**:单文件,reactive form + computed 命令预览 + `useJobPanel` + `JobLog`
- **AudioView.vue**:`<segmented>` 切换 → `AudioConvertMode` / `AudioExtractMode` / `AudioMergeMode` 三子组件,每个子组件自带表单与命令预览,共享 `useJobPanel({cancelUrl: '/api/audio/cancel'})`
- **EditorView.vue**:8 个子组件挂在一起,以 `useEditorStore` 为单一状态源:
  - 顶部:`EditorTopBar`(标题 + 撤销/重做)
  - 预览:`<video>` + `<audio>`(`useEditorPreview` 接管)
  - 工具栏:`EditorToolbar`(分割 / 删除 / 范围标记)
  - 时间轴:`EditorTimeline`(DOM 渲染轨道、clip、播放头、范围选区)
  - 播放控制:`EditorPlayBar`
  - 浮窗:`EditorAudioVolume`(0–200% WebAudio gain)
  - 模态:`EditorProjectsModal`(打开历史工程)、`EditorExportDialog`(导出配置)
  - 导出期间:`EditorExportSidebar`(占据右侧,放 `JobLog` + 进度)

时间轴选用 **DOM 实现**,M3 实测 100 级 clip 内性能足够;Canvas 子树为多轨剪辑器铺路,**M4 时仍未引入**,等出现性能瓶颈再上 Konva / 自绘。

## 9. Editor 状态机(`stores/editor.ts`)

setup-store,`useEditorStore` 暴露:

- 状态:`project / dirty / selection[] / splitScope / playhead / playing / pxPerSecond / rangeSelection`
- 历史:`history[]`(快照只含 `videoClips + audioClips`,`HISTORY_MAX=100`)+ `historyCursor` + `canUndo` / `canRedo`
- 防抖自动保存:`scheduleSave()` / `flushSave()`,`SAVE_DEBOUNCE_MS=1500`
- 操作:`applyProjectPatch(patch, {save})` / `pushHistory()` / `undo()` / `redo()` / `loadProject(p)` / `setPlayhead(t)`

`watch(() => totalDuration(project))` 在删除导致总长缩短时把 playhead clamp 回去,避免视觉穿越。

## 10. 解压进度遮罩(`PrepareOverlay.vue`)

```ts
async function loop() {
  while (true) {
    const p = await prepareApi.status()
    if (p.state === 'ready')   { fade out; break }
    if (p.state === 'error')   { show error message; break }
    progress = p.percent
    currentFile = p.currentFile
    await sleep(300)
  }
}
```

样式细节(毛玻璃、绿→蓝渐变进度条、淡出)见 [ui-system.md §5.3](ui-system.md)。

## 11. 跨 Tab 通用交互(由共享层组件提供)

- **任务进度条 / 完成条 / 日志**:`<JobLog>` 组件,`useJobPanel` 暴露的 ref 一一传入;成功完成时显示"📂 打开文件夹"按钮(用 `lastOutputPath`)
- **执行前命令预览**:`useModalsStore().showCommand(cmd)` 调 `<ConfirmCommandModal>`;Promise<boolean>
- **覆盖确认**:`useModalsStore().showOverwrite(path)` 调 `<ConfirmOverwriteModal>`;Promise<boolean>
- **文件 / 目录选择**:`useModalsStore().showPicker({mode, title, startPath})` 调 `<PickerModal>`;Promise<string|null>
- **模态弹窗约定**:不响应点背景关闭(误触代价高);× / Esc / 取消 三种方式退出;Enter 在确认型 dialog 等价于"确认"

## 12. 路由

```ts
// router/index.ts
createRouter({
  history: createWebHashHistory(),
  routes: [
    { path: '/',        redirect: '/convert' },
    { path: '/convert', component: () => import('@/views/ConvertView.vue') },
    { path: '/audio',   component: () => import('@/views/AudioView.vue') },
    { path: '/editor',  component: () => import('@/views/EditorView.vue') },
  ],
})
```

hash 模式让 Go `http.FileServer` 永远命中 `/index.html`,无需 SPA fallback;视图懒加载让初屏只下载当前 Tab 的代码块。

## 13. 新增 Tab 的入口

1. `web/src/views/<Name>View.vue` 新增视图,按需用 `useJobPanel` 接 SSE
2. `web/src/router/index.ts` 加路由
3. `web/src/components/layout/TabNav.vue` 加按钮(失能态可去掉)
4. 后端加对应 API endpoint(`server/handlers_<name>.go`)+ 纯函数 args 构建器 + 测试

## 14. DOM 命名约定

由于 Vue 组件天然作用域隔离,旧 IIFE 时代的 id 前缀(`edVideo`、`pickerBackdrop` 等)已不需要。只保留:

- 全局唯一根:`#app`(`index.html` 里 `<div id="app"></div>`)
- 三个 Tab 路由路径:`/convert` / `/audio` / `/editor`(对应旧的 `panel-<tab>`)

样式靠 Tailwind utilities + 组件内 `<style scoped>`(目前只有少量 hover/transition 用 scoped,绝大多数靠 utility),token 走 `styles/tokens.css`(详见 [ui-system.md §2](ui-system.md))。
