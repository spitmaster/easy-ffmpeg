"use strict";

// ============================================================
//  helpers：纯工具，无副作用
// ============================================================

const $ = (id) => document.getElementById(id);

const Http = {
  async fetchJSON(url, opts = {}) {
    const res = await fetch(url, opts);
    const data = await res.json().catch(() => ({}));
    if (!res.ok) throw new Error(data.error || `HTTP ${res.status}`);
    return data;
  },
  postJSON(url, body) {
    return Http.fetchJSON(url, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(body),
    });
  },
  getJSON(url) {
    return Http.fetchJSON(url, { method: "GET" });
  },
  putJSON(url, body) {
    return Http.fetchJSON(url, {
      method: "PUT",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(body),
    });
  },
  deleteJSON(url) {
    return Http.fetchJSON(url, { method: "DELETE" });
  },
};

const Fmt = {
  human(size) {
    if (!size) return "";
    const units = ["B", "KB", "MB", "GB", "TB"];
    let i = 0, v = size;
    while (v >= 1024 && i < units.length - 1) { v /= 1024; i++; }
    return `${v.toFixed(i === 0 ? 0 : 1)} ${units[i]}`;
  },
};

const Path = {
  join(dir, name) {
    if (!dir) return name;
    if (dir.endsWith("/")) return dir + name;
    return dir + "/" + name;
  },
  basename(p) {
    const i = p.lastIndexOf("/");
    return i >= 0 ? p.slice(i + 1) : p;
  },
  dirname(p) {
    const i = p.lastIndexOf("/");
    return i >= 0 ? p.slice(0, i) : "";
  },
  stripExt(name) {
    const i = name.lastIndexOf(".");
    return i > 0 ? name.slice(0, i) : name;
  },
};

// ============================================================
//  Dirs：输入/输出目录的读写
// ============================================================

const Dirs = (() => {
  let cache = {};
  async function load() {
    cache = await Http.fetchJSON("/api/config/dirs").catch(() => ({}));
    return cache;
  }
  const saveInput  = (dir) => Http.postJSON("/api/config/dirs", { inputDir: dir });
  const saveOutput = (dir) => Http.postJSON("/api/config/dirs", { outputDir: dir });
  const get = () => cache;
  return { load, saveInput, saveOutput, get };
})();

// ============================================================
//  AppVersion：顶栏右侧显示当前程序版本
// ============================================================

const AppVersion = (() => {
  async function init() {
    const el = $("appVersion");
    if (!el) return;
    try {
      const r = await fetch("/api/version");
      if (!r.ok) return;
      const j = await r.json();
      if (j && j.version) el.textContent = "v" + j.version;
    } catch {
      // network failure → leave chip empty (CSS hides :empty)
    }
  }
  return { init };
})();

// ============================================================
//  FFmpegStatus：顶栏版本 chip
// ============================================================

const FFmpegStatus = (() => {
  function parseVersion(s) {
    if (!s) return "";
    const semver = s.match(/ffmpeg version (\d+(?:\.\d+)*)/i);
    if (semver) return semver[1];
    const any = s.match(/ffmpeg version (\S+)/i);
    return any ? any[1] : "";
  }

  async function load() {
    const chip = $("ffmpegStatus");
    try {
      const s = await Http.fetchJSON("/api/ffmpeg/status");
      if (s.available) {
        const v = parseVersion(s.version);
        const where = s.embedded ? "嵌入" : "系统";
        chip.textContent = v ? `FFmpeg ${v} · ${where}` : `FFmpeg 可用（${where}）`;
        chip.className = "status-chip ok clickable";
        chip.title = (s.version || "") + "\n\n点击打开 FFmpeg 所在文件夹";
      } else {
        chip.textContent = "FFmpeg 未安装";
        chip.className = "status-chip err";
        chip.title = "";
      }
    } catch {
      chip.textContent = "状态检测失败";
      chip.className = "status-chip err";
      chip.title = "";
    }
  }

  function init() {
    load();
    $("ffmpegStatus").addEventListener("click", async () => {
      const chip = $("ffmpegStatus");
      if (!chip.classList.contains("clickable")) return;
      try { await Http.postJSON("/api/ffmpeg/reveal", {}); }
      catch (e) { alert("打开失败: " + e.message); }
    });
  }
  return { init };
})();

// ============================================================
//  Picker：共享的文件/目录选择模态框
// ============================================================

const Picker = (() => {
  const el = {};
  const state = { mode: "dir", currentPath: "", selected: null, resolver: null };

  function open({ mode, title, startPath }) {
    state.mode = mode;
    state.selected = null;
    el.title.textContent = title;
    el.confirm.textContent = mode === "dir" ? "选择此目录" : "选择文件";
    el.hint.textContent = mode === "file" ? "选中一个文件后点击确认" : "";
    el.backdrop.classList.remove("hidden");
    return new Promise(async (resolve) => {
      state.resolver = resolve;
      let start = startPath || "";
      if (!start) {
        try { start = (await Http.fetchJSON("/api/fs/home")).home; } catch {}
      }
      loadPath(start);
    });
  }

  function close(result) {
    el.backdrop.classList.add("hidden");
    if (state.resolver) {
      const r = state.resolver;
      state.resolver = null;
      r(result);
    }
  }

  async function loadPath(path) {
    try {
      const q = path ? `?path=${encodeURIComponent(path)}` : "";
      const data = await Http.fetchJSON(`/api/fs/list${q}`);
      state.currentPath = data.path;
      state.selected = null;
      el.path.value = data.path;
      renderDrives(data.drives);
      renderEntries(data.entries);
    } catch (e) {
      el.list.innerHTML = `<li class="muted" style="padding:10px">加载失败: ${e.message}</li>`;
    }
  }

  function renderDrives(drives) {
    if (!drives || !drives.length) { el.drive.classList.add("hidden"); return; }
    el.drive.classList.remove("hidden");
    el.drive.innerHTML = drives.map(d => `<option value="${d}">${d}</option>`).join("");
    const cur = state.currentPath.toUpperCase();
    const match = drives.find(d => cur.startsWith(d.toUpperCase()));
    if (match) el.drive.value = match;
  }

  function renderEntries(entries) {
    el.list.innerHTML = "";
    if (!entries.length) {
      el.list.innerHTML = `<li class="muted" style="padding:10px">空目录</li>`;
      return;
    }
    for (const e of entries) {
      const li = document.createElement("li");
      li.innerHTML = `
        <span class="icon">${e.isDir ? "📁" : "📄"}</span>
        <span class="name">${e.name}</span>
        <span class="meta">${e.isDir ? "" : Fmt.human(e.size)}</span>
      `;
      li.addEventListener("click", () => {
        [...el.list.children].forEach(n => n.classList.remove("selected"));
        li.classList.add("selected");
        state.selected = e;
      });
      li.addEventListener("dblclick", () => {
        const full = Path.join(state.currentPath, e.name);
        if (e.isDir) loadPath(full);
        else if (state.mode === "file") close(full);
      });
      el.list.appendChild(li);
    }
  }

  function init() {
    Object.assign(el, {
      backdrop: $("pickerBackdrop"),
      title:    $("pickerTitle"),
      path:     $("pickerPath"),
      list:     $("pickerList"),
      drive:    $("pickerDrive"),
      hint:     $("pickerHint"),
      confirm:  $("pickerConfirm"),
      up:       $("pickerUp"),
      close:    $("pickerClose"),
      cancel:   $("pickerCancel"),
    });

    el.drive.addEventListener("change", () => loadPath(el.drive.value));
    el.up.addEventListener("click", () => {
      const cur = state.currentPath;
      if (!cur) return;
      const idx = cur.lastIndexOf("/");
      const parent = idx > 0 ? cur.slice(0, idx) : (cur.length > 3 ? cur.slice(0, 3) : cur);
      loadPath(parent);
    });
    el.path.addEventListener("keydown", (e) => {
      if (e.key === "Enter") loadPath(el.path.value);
    });
    el.confirm.addEventListener("click", () => {
      if (state.mode === "dir") {
        close(state.currentPath);
      } else if (state.selected && !state.selected.isDir) {
        close(Path.join(state.currentPath, state.selected.name));
      } else {
        el.hint.textContent = "请先选中一个文件";
      }
    });
    el.cancel.addEventListener("click", () => close(null));
    el.close.addEventListener("click", () => close(null));
    // Backdrop click no longer closes the picker — too easy to dismiss
    // by accident after navigating into a folder. × / 取消 / Esc are
    // the explicit dismissal paths.
  }

  return { init, open };
})();

// ============================================================
//  Confirm：自绘的弹窗，替代浏览器原生 window.confirm。
//  返回 Promise<boolean>，调用方可以直接 await。Esc / 点背景 = 取消，
//  Enter = 确认，焦点默认落在主按钮上。
// ============================================================

const Confirm = (() => {
  // Two distinct dialogs share the same controller — overwrite and
  // command — because both are "yes/no with some content"; multiplexing
  // them through one resolver/state is simpler than wiring two parallel
  // copies of the same listeners. `current` records which one is open
  // so the global keydown handler routes Esc/Enter to the right modal.
  let current = null;
  let pending = null;
  let lastFocused = null;

  // overwrite-modal refs
  let owBackdrop = null, owOk = null, owCancel = null, owClose = null, owPath = null;
  // command-modal refs
  let cmdBackdrop = null, cmdOk = null, cmdCancel = null, cmdClose = null,
      cmdText = null, cmdCopy = null, cmdHint = null;

  function init() {
    owBackdrop = $("confirmOverwriteBackdrop");
    owOk       = $("confirmOverwriteOk");
    owCancel   = $("confirmOverwriteCancel");
    owClose    = $("confirmOverwriteClose");
    owPath     = $("confirmOverwritePath");
    cmdBackdrop = $("confirmCommandBackdrop");
    cmdOk       = $("confirmCommandOk");
    cmdCancel   = $("confirmCommandCancel");
    cmdClose    = $("confirmCommandClose");
    cmdText     = $("confirmCommandText");
    cmdCopy     = $("confirmCommandCopy");
    cmdHint     = $("confirmCommandHint");
    if (owBackdrop) {
      owOk.addEventListener("click", () => settle(true));
      owCancel.addEventListener("click", () => settle(false));
      owClose.addEventListener("click", () => settle(false));
      // No backdrop-click-to-close: an accidental click outside the
      // dialog is too easy to do and would silently cancel an export
      // the user was about to confirm. Close button + Esc are the
      // explicit dismissal paths.
    }
    if (cmdBackdrop) {
      cmdOk.addEventListener("click", () => settle(true));
      cmdCancel.addEventListener("click", () => settle(false));
      cmdClose.addEventListener("click", () => settle(false));
      // The pre block is the primary copy affordance — clicking anywhere
      // on it copies the whole command. The button mirrors the action.
      cmdText.addEventListener("click", copyCurrentCommand);
      cmdCopy.addEventListener("click", copyCurrentCommand);
    }
    // Single global keydown listener: routes to whichever dialog is open.
    // Enter on the command pre would otherwise fight the OK button's
    // implicit submission; we own it explicitly.
    document.addEventListener("keydown", (e) => {
      if (!current) return;
      if (e.key === "Escape") { e.preventDefault(); settle(false); }
      else if (e.key === "Enter" && e.target !== cmdText) { e.preventDefault(); settle(true); }
    });
  }

  // Show the overwrite dialog and resolve to the user's choice.
  function overwrite(path) {
    if (!owBackdrop) return Promise.resolve(window.confirm("目标文件已存在，是否覆盖？\n\n" + (path || "")));
    if (pending) settle(false);
    owPath.textContent = path || "";
    owBackdrop.classList.remove("hidden");
    current = "overwrite";
    lastFocused = document.activeElement;
    requestAnimationFrame(() => owOk.focus());
    return new Promise((resolve) => { pending = resolve; });
  }

  // Show the command preview / confirmation dialog. Resolves true when
  // the user clicks "开始执行", false on cancel/Esc/backdrop click.
  function command(cmd) {
    if (!cmdBackdrop) return Promise.resolve(window.confirm("即将执行：\n\n" + (cmd || "")));
    if (pending) settle(false);
    cmdText.textContent = cmd || "";
    cmdHint.textContent = "点击命令框可复制";
    cmdHint.classList.remove("copied");
    cmdBackdrop.classList.remove("hidden");
    current = "command";
    lastFocused = document.activeElement;
    requestAnimationFrame(() => cmdOk.focus());
    return new Promise((resolve) => { pending = resolve; });
  }

  async function copyCurrentCommand() {
    const text = cmdText.textContent;
    let ok = false;
    try {
      if (navigator.clipboard && navigator.clipboard.writeText) {
        await navigator.clipboard.writeText(text);
        ok = true;
      }
    } catch {}
    if (!ok) {
      // Fallback for older runtimes / non-secure contexts.
      try {
        const ta = document.createElement("textarea");
        ta.value = text;
        ta.style.position = "fixed";
        ta.style.left = "-9999px";
        document.body.appendChild(ta);
        ta.select();
        document.execCommand("copy");
        document.body.removeChild(ta);
        ok = true;
      } catch {}
    }
    cmdHint.textContent = ok ? "✓ 已复制" : "✗ 复制失败（请手动选择）";
    cmdHint.classList.toggle("copied", ok);
  }

  function settle(value) {
    if (!current) return;
    if (current === "overwrite") owBackdrop.classList.add("hidden");
    if (current === "command")   cmdBackdrop.classList.add("hidden");
    current = null;
    if (lastFocused && typeof lastFocused.focus === "function") {
      try { lastFocused.focus(); } catch {}
    }
    lastFocused = null;
    const r = pending;
    pending = null;
    if (r) r(value);
  }

  return { init, overwrite, command };
})();

// ============================================================
//  JobBus：全局唯一的 SSE 连接，广播事件给订阅者
// ============================================================

const JobBus = (() => {
  const listeners = new Set();
  let es = null;

  function connect() {
    if (es) es.close();
    es = new EventSource("/api/convert/stream");
    es.onmessage = (msg) => {
      let ev; try { ev = JSON.parse(msg.data); } catch { return; }
      listeners.forEach(fn => { try { fn(ev); } catch {} });
    };
    es.onerror = () => setTimeout(connect, 1500);
  }

  function subscribe(fn) {
    listeners.add(fn);
    return () => listeners.delete(fn);
  }

  return { connect, subscribe };
})();

// ============================================================
//  createJobPanel：每个 Tab 独立持有的日志/动作/完成条控制器
//  只有"真正发起任务的 Panel"会响应 log/done/error/cancelled
// ============================================================

function createJobPanel({
  logEl, stateEl, startBtn, cancelBtn,
  finishBar, finishText, finishRevealBtn,
  progressWrap, progressFill, progressText,
  cancelUrl,
  runningLabel = "处理中...",
  idleLabel = "空闲",
  doneLabel = "✓ 完成",
  errorLabel = "✗ 失败",
  cancelledLabel = "! 已取消",
}) {
  const PROGRESS_RE = /^(frame=|size=|video:|Lsize=)/;
  // Total duration auto-detection: ffmpeg prints "Duration: HH:MM:SS.ms"
  // exactly once per input on stderr, before any progress lines.
  const DUR_RE  = /Duration:\s*(\d+):(\d+):([\d.]+)/;
  // time= appears in every progress line, monotonically increasing in
  // output time; this matches what we want even when the input is being
  // trimmed (output time, not source time).
  const TIME_RE = /time=(\d+):(\d+):([\d.]+)/;
  let owning = false;
  let lastOutputPath = null;
  // Total seconds for the current job. May be set via panel.start({totalDurationSec})
  // (preferred — known up-front, e.g. editor's program time) or auto-discovered
  // from "Duration:" lines (fallback for convert/audio).
  let totalSec = 0;

  function appendLog(text, cls) {
    // Drive the progress bar from the same lines we render in the log —
    // saves an extra event channel. Duration is parsed once (lazily, in
    // case totalSec was already passed in via start()), time= is parsed
    // every line so the bar tracks live progress.
    if (!cls) parseForProgress(text);
    const isProgress = !cls && PROGRESS_RE.test(text);
    if (isProgress) {
      const last = logEl.lastElementChild;
      if (last && last.classList.contains("progress")) {
        last.textContent = text;
        requestAnimationFrame(() => { logEl.scrollTop = logEl.scrollHeight; });
        return;
      }
      const line = document.createElement("span");
      line.className = "log-line progress";
      line.textContent = text;
      logEl.appendChild(line);
    } else {
      const line = document.createElement("span");
      line.className = "log-line" + (cls ? " " + cls : "");
      line.textContent = text;
      logEl.appendChild(line);
    }
    requestAnimationFrame(() => { logEl.scrollTop = logEl.scrollHeight; });
  }

  function parseForProgress(line) {
    if (!totalSec) {
      const m = DUR_RE.exec(line);
      if (m) totalSec = (+m[1]) * 3600 + (+m[2]) * 60 + parseFloat(m[3]);
    }
    const t = TIME_RE.exec(line);
    if (!t || !totalSec) return;
    const cur = (+t[1]) * 3600 + (+t[2]) * 60 + parseFloat(t[3]);
    setProgress(cur / totalSec);
  }

  function setProgress(ratio) {
    if (!progressFill || !progressText) return;
    const pct = Math.max(0, Math.min(100, ratio * 100));
    progressFill.style.width = pct.toFixed(1) + "%";
    progressText.textContent = pct.toFixed(1) + "%";
  }

  function showProgress(show) {
    if (!progressWrap) return;
    progressWrap.classList.toggle("hidden", !show);
  }

  function setRunning(running) {
    startBtn.disabled = running;
    cancelBtn.disabled = !running;
    stateEl.textContent = running ? runningLabel : idleLabel;
    stateEl.style.color = running ? "var(--accent)" : "var(--muted)";
  }

  function showFinish(kind, text, revealPath) {
    finishBar.classList.remove("hidden", "success", "error", "cancelled");
    finishBar.classList.add(kind);
    finishText.textContent = text;
    if (revealPath) {
      lastOutputPath = revealPath;
      finishRevealBtn.classList.remove("hidden");
    } else {
      finishRevealBtn.classList.add("hidden");
    }
  }

  function hideFinish() {
    finishBar.classList.add("hidden");
    finishRevealBtn.classList.add("hidden");
  }

  async function start({ url, body, outputPath, totalDurationSec }) {
    logEl.innerHTML = "";
    hideFinish();
    lastOutputPath = outputPath || null;
    // Reset progress for the new job. If the caller passes the program
    // duration explicitly we use it (editor knows its program time
    // exactly); otherwise we fall back to parsing "Duration:" out of
    // ffmpeg's startup log.
    totalSec = totalDurationSec && totalDurationSec > 0 ? totalDurationSec : 0;
    setProgress(0);

    // Preflight: ask the server to build the command without starting
    // ffmpeg, then show it in the confirm-command dialog. This lets the
    // user see exactly what would run before committing — and gives an
    // easy way to copy the command for offline use. The dry-run path
    // skips overwrite checks (those run on the real-run POST below).
    let previewCmd;
    try {
      const previewRes = await fetch(url, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ ...body, dryRun: true }),
      });
      const previewData = await previewRes.json().catch(() => ({}));
      if (!previewRes.ok) throw new Error(previewData.error || `HTTP ${previewRes.status}`);
      previewCmd = previewData.command || "";
    } catch (e) {
      showFinish("error", "✗ 启动失败: " + e.message, null);
      return;
    }
    if (!await Confirm.command(previewCmd)) {
      // User cancelled at the preview dialog — nothing started, no
      // progress bar to show.
      return;
    }
    showProgress(true);

    const doStart = async (b) => {
      const res = await fetch(url, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(b),
      });
      const data = await res.json().catch(() => ({}));
      if (res.status === 409 && data.existing) {
        if (!await Confirm.overwrite(data.path)) {
          showProgress(false);
          return;
        }
        return doStart({ ...b, overwrite: true });
      }
      if (!res.ok) throw new Error(data.error || `HTTP ${res.status}`);
      appendLog("> " + data.command, "info");
      owning = true;
      setRunning(true);
    };

    try { await doStart(body); }
    catch (e) {
      showProgress(false);
      showFinish("error", "✗ 启动失败: " + e.message, null);
    }
  }

  JobBus.subscribe((ev) => {
    if (!owning) return;
    switch (ev.type) {
      case "state":
        setRunning(!!ev.running);
        break;
      case "log":
        appendLog(ev.line);
        break;
      case "done":
        owning = false;
        setRunning(false);
        setProgress(1);
        // Leave the bar at 100% briefly, then hide. Avoids the bar
        // visibly jumping back to 0 when the next job starts.
        setTimeout(() => showProgress(false), 600);
        showFinish("success", doneLabel, lastOutputPath);
        break;
      case "error":
        owning = false;
        setRunning(false);
        showProgress(false);
        showFinish("error", `${errorLabel}: ${ev.message || ""}`, null);
        break;
      case "cancelled":
        owning = false;
        setRunning(false);
        showProgress(false);
        showFinish("cancelled", cancelledLabel, null);
        break;
    }
  });

  cancelBtn.addEventListener("click", async () => {
    try { await Http.postJSON(cancelUrl, {}); } catch {}
  });

  finishRevealBtn.addEventListener("click", async () => {
    if (!lastOutputPath) return;
    try { await Http.postJSON("/api/fs/reveal", { path: lastOutputPath }); }
    catch (e) { alert("打开失败: " + e.message); }
  });

  return { start };
}

// ============================================================
//  ConvertTab：视频转换
// ============================================================

const ConvertTab = (() => {
  const form = {};
  let panel;

  function readForm() {
    return {
      inputPath:    form.inputPath.value.trim(),
      outputDir:    form.outputDir.value.trim(),
      outputName:   form.outputName.value.trim(),
      videoEncoder: form.videoEncoder.value,
      audioEncoder: form.audioEncoder.value,
      format:       form.format.value,
    };
  }

  function normalizeVideo(v) {
    if (v === "h264") return "libx264";
    if (v === "h265") return "libx265";
    return v;
  }

  function updateCommandPreview() {
    const f = readForm();
    const pre = $("commandPreview");
    if (!f.inputPath || !f.outputDir || !f.outputName || !f.format) {
      pre.textContent = "ffmpeg ...";
      return;
    }
    const out = Path.join(f.outputDir, `${f.outputName}.${f.format}`);
    const vc = normalizeVideo(f.videoEncoder);
    const ac = f.audioEncoder;
    let cmd = `ffmpeg -y -i "${f.inputPath}"`;
    if (vc === "copy" && ac === "copy") cmd += ` -c copy`;
    else                                 cmd += ` -c:v ${vc} -c:a ${ac}`;
    cmd += ` "${out}"`;
    pre.textContent = cmd;
  }

  function refreshOpenOutputDirBtn() {
    $("openOutputDirBtn").disabled = !form.outputDir.value.trim();
  }

  function init() {
    Object.assign(form, {
      inputPath:    $("inputPath"),
      outputDir:    $("outputDir"),
      outputName:   $("outputName"),
      videoEncoder: $("videoEncoder"),
      audioEncoder: $("audioEncoder"),
      format:       $("format"),
    });

    panel = createJobPanel({
      logEl: $("log"),
      stateEl: $("jobState"),
      startBtn: $("startBtn"),
      cancelBtn: $("cancelBtn"),
      finishBar: $("finishBar"),
      finishText: $("finishText"),
      finishRevealBtn: $("finishRevealBtn"),
      progressWrap: $("progressWrap"),
      progressFill: $("progressFill"),
      progressText: $("progressText"),
      cancelUrl: "/api/convert/cancel",
      runningLabel: "转码中...",
      doneLabel: "✓ 转码完成",
      errorLabel: "✗ 转码失败",
      cancelledLabel: "! 转码已取消",
    });

    Object.values(form).forEach((el) => {
      el.addEventListener("input", updateCommandPreview);
      el.addEventListener("change", updateCommandPreview);
    });
    form.outputDir.addEventListener("input", refreshOpenOutputDirBtn);

    $("pickInputBtn").addEventListener("click", async () => {
      const start = form.inputPath.value || Dirs.get().inputDir || "";
      const p = await Picker.open({ mode: "file", title: "选择输入视频", startPath: start });
      if (!p) return;
      form.inputPath.value = p;
      const dir = Path.dirname(p);
      const base = Path.stripExt(Path.basename(p));
      if (!form.outputName.value) form.outputName.value = base + "_converted";
      if (dir) await Dirs.saveInput(dir).catch(() => {});
      updateCommandPreview();
    });

    $("pickOutputDirBtn").addEventListener("click", async () => {
      const start = form.outputDir.value || Dirs.get().outputDir || "";
      const p = await Picker.open({ mode: "dir", title: "选择输出目录", startPath: start });
      if (!p) return;
      form.outputDir.value = p;
      await Dirs.saveOutput(p).catch(() => {});
      updateCommandPreview();
      refreshOpenOutputDirBtn();
    });

    $("openOutputDirBtn").addEventListener("click", async () => {
      const path = form.outputDir.value.trim();
      if (!path) return;
      try { await Http.postJSON("/api/fs/reveal", { path }); }
      catch (e) { alert("打开失败: " + e.message); }
    });

    $("startBtn").addEventListener("click", async () => {
      const f = readForm();
      if (!f.inputPath)  return alert("请选择输入文件");
      if (!f.outputDir)  return alert("请选择输出目录");
      if (!f.outputName) return alert("请输入输出文件名");
      const outputPath = Path.join(f.outputDir, `${f.outputName}.${f.format}`);
      await panel.start({ url: "/api/convert/start", body: f, outputPath });
    });

    if (Dirs.get().outputDir) form.outputDir.value = Dirs.get().outputDir;
    refreshOpenOutputDirBtn();
    updateCommandPreview();
  }

  return { init };
})();

// ============================================================
//  AudioCodecs：容器 × 编码器 × 码率的共享知识
//  （AudioConvertMode 和 AudioExtractMode transcode 子表单共用）
// ============================================================

const AudioCodecs = (() => {
  const FORMAT_CODECS = {
    mp3:  [{ v: "libmp3lame", t: "libmp3lame (MP3)" }, { v: "copy", t: "copy" }],
    m4a:  [{ v: "aac",        t: "aac (AAC)" },        { v: "copy", t: "copy" }],
    flac: [{ v: "flac",       t: "flac (FLAC)" },      { v: "copy", t: "copy" }],
    wav:  [{ v: "pcm_s16le",  t: "pcm_s16le (16-bit)" }, { v: "pcm_s24le", t: "pcm_s24le (24-bit)" }, { v: "copy", t: "copy" }],
    ogg:  [{ v: "libvorbis",  t: "libvorbis (Vorbis)" }, { v: "libopus",    t: "libopus (Opus)" },    { v: "copy", t: "copy" }],
    opus: [{ v: "libopus",    t: "libopus (Opus)" },   { v: "copy", t: "copy" }],
  };
  const LOSSLESS_CONTAINERS = new Set(["flac", "wav"]);
  const CODEC_TO_CONTAINER = { aac: "m4a", mp3: "mp3", opus: "opus", vorbis: "ogg", flac: "flac" };

  return {
    codecsFor: (fmt) => FORMAT_CODECS[fmt] || [],
    isBitrateIgnored: (fmt, codec) =>
      LOSSLESS_CONTAINERS.has(fmt) || (codec || "").startsWith("pcm_") || codec === "copy",
    containerForCodec: (codec) => CODEC_TO_CONTAINER[(codec || "").toLowerCase()] || "mka",
  };
})();

// ============================================================
//  AudioConvertMode：格式转换 / 压缩
// ============================================================

const AudioConvertMode = (() => {
  const form = {};
  let onChange = () => {};

  function rebuildCodecOptions() {
    const fmt = form.format.value;
    const prev = form.codec.value;
    const list = AudioCodecs.codecsFor(fmt);
    form.codec.innerHTML = list.map(c => `<option value="${c.v}">${c.t}</option>`).join("");
    if (list.some(c => c.v === prev)) form.codec.value = prev;
    refreshBitrateState();
  }

  function refreshBitrateState() {
    const ignored = AudioCodecs.isBitrateIgnored(form.format.value, form.codec.value || "");
    form.bitrate.disabled = ignored;
    form.bitrate.title = ignored ? "当前格式/编码器无需码率" : "";
  }

  function readForm() {
    const bitrate = form.bitrate.disabled ? "" : form.bitrate.value;
    return {
      inputPath:  form.input.value.trim(),
      outputDir:  form.outDir.value.trim(),
      outputName: form.outName.value.trim(),
      format:     form.format.value,
      codec:      form.codec.value,
      bitrate:    bitrate,
      sampleRate: parseInt(form.sampleRate.value, 10) || 0,
      channels:   parseInt(form.channels.value, 10) || 0,
    };
  }

  function validate(body) {
    if (!body.inputPath)  throw new Error("请选择输入文件");
    if (!body.outputDir)  throw new Error("请选择输出目录");
    if (!body.outputName) throw new Error("请输入输出文件名");
    if (!body.format)     throw new Error("请选择输出格式");
  }

  function getOutputPath(body) {
    return Path.join(body.outputDir, `${body.outputName}.${body.format}`);
  }

  function buildPreview() {
    const body = readForm();
    if (!body.inputPath || !body.outputDir || !body.outputName) return "";
    const parts = [`ffmpeg -y -i "${body.inputPath}" -vn`];
    if (body.codec === "copy") {
      parts.push(`-c:a copy`);
    } else {
      parts.push(`-c:a ${body.codec}`);
      if (body.bitrate && body.bitrate !== "copy") parts.push(`-b:a ${body.bitrate}k`);
      if (body.sampleRate) parts.push(`-ar ${body.sampleRate}`);
      if (body.channels)   parts.push(`-ac ${body.channels}`);
    }
    parts.push(`"${getOutputPath(body)}"`);
    return parts.join(" ");
  }

  function refreshOpenOutDirBtn() {
    $("audioConvertOpenOutDir").disabled = !form.outDir.value.trim();
  }

  function init(opts = {}) {
    onChange = opts.onChange || (() => {});
    Object.assign(form, {
      input:      $("audioConvertInput"),
      outDir:     $("audioConvertOutDir"),
      outName:    $("audioConvertOutName"),
      format:     $("audioConvertFormat"),
      codec:      $("audioConvertCodec"),
      bitrate:    $("audioConvertBitrate"),
      sampleRate: $("audioConvertSampleRate"),
      channels:   $("audioConvertChannels"),
    });

    rebuildCodecOptions();

    Object.values(form).forEach(el => {
      el.addEventListener("input", onChange);
      el.addEventListener("change", onChange);
    });
    form.format.addEventListener("change", () => { rebuildCodecOptions(); onChange(); });
    form.codec.addEventListener("change", () => { refreshBitrateState(); onChange(); });
    form.outDir.addEventListener("input", refreshOpenOutDirBtn);

    $("audioConvertPickInput").addEventListener("click", async () => {
      const start = form.input.value || Dirs.get().inputDir || "";
      const p = await Picker.open({ mode: "file", title: "选择输入音频", startPath: start });
      if (!p) return;
      form.input.value = p;
      const dir = Path.dirname(p);
      const base = Path.stripExt(Path.basename(p));
      if (!form.outName.value) form.outName.value = base + "_converted";
      if (dir) await Dirs.saveInput(dir).catch(() => {});
      onChange();
    });

    $("audioConvertPickOutDir").addEventListener("click", async () => {
      const start = form.outDir.value || Dirs.get().outputDir || "";
      const p = await Picker.open({ mode: "dir", title: "选择输出目录", startPath: start });
      if (!p) return;
      form.outDir.value = p;
      await Dirs.saveOutput(p).catch(() => {});
      refreshOpenOutDirBtn();
      onChange();
    });

    $("audioConvertOpenOutDir").addEventListener("click", async () => {
      const path = form.outDir.value.trim();
      if (!path) return;
      try { await Http.postJSON("/api/fs/reveal", { path }); }
      catch (e) { alert("打开失败: " + e.message); }
    });

    if (Dirs.get().outputDir) form.outDir.value = Dirs.get().outputDir;
    refreshOpenOutDirBtn();

    return { readForm, validate, getOutputPath, buildPreview };
  }

  return { init };
})();

// ============================================================
//  AudioExtractMode：从视频提取音频
// ============================================================

const AudioExtractMode = (() => {
  const form = {};
  let onChange = () => {};
  let streams = [];        // 最近一次探测到的音轨
  let detectedFormat = "mka";

  function currentMethod() {
    const r = document.querySelector('input[name="audioExtractMethod"]:checked');
    return r ? r.value : "copy";
  }

  function refreshSubPanels() {
    const m = currentMethod();
    document.querySelectorAll('#panel-audio [data-extract-sub]').forEach(d => {
      d.classList.toggle("hidden", d.dataset.extractSub !== m);
    });
  }

  function renderStreamOptions() {
    const sel = form.stream;
    if (!streams.length) {
      sel.innerHTML = `<option value="">（请选择视频文件）</option>`;
      sel.disabled = true;
      return;
    }
    sel.disabled = false;
    sel.innerHTML = streams.map((s) => {
      const bits = [`#${s.index + 1}`];
      if (s.codecName) bits.push(s.codecName.toUpperCase());
      if (s.channels)  bits.push(`${s.channels}ch`);
      if (s.lang && s.lang !== "und") bits.push(s.lang);
      else if (s.title) bits.push(s.title);
      return `<option value="${s.index}">${bits.join(" · ")}</option>`;
    }).join("");
    sel.value = String(streams[0].index);
  }

  function updateDetectedFormat() {
    const idx = parseInt(form.stream.value, 10);
    const s = streams.find(x => x.index === idx);
    detectedFormat = s ? AudioCodecs.containerForCodec(s.codecName) : "mka";
  }

  async function probeInput(path) {
    form.streamHint.textContent = "探测中...";
    try {
      const res = await Http.postJSON("/api/audio/probe", { path });
      streams = res.streams || [];
      form.streamHint.textContent = streams.length
        ? `共 ${streams.length} 条音轨`
        : "未找到音频轨";
    } catch (e) {
      streams = [];
      form.streamHint.textContent = "探测失败: " + e.message;
    }
    renderStreamOptions();
    updateDetectedFormat();
    onChange();
  }

  function rebuildTranscodeCodecOptions() {
    const fmt = form.tFormat.value;
    const prev = form.tCodec.value;
    const list = AudioCodecs.codecsFor(fmt);
    form.tCodec.innerHTML = list.map(c => `<option value="${c.v}">${c.t}</option>`).join("");
    if (list.some(c => c.v === prev)) form.tCodec.value = prev;
    refreshTranscodeBitrateState();
  }

  function refreshTranscodeBitrateState() {
    const ignored = AudioCodecs.isBitrateIgnored(form.tFormat.value, form.tCodec.value || "");
    form.tBitrate.disabled = ignored;
    form.tBitrate.title = ignored ? "当前格式/编码器无需码率" : "";
  }

  function readForm() {
    const method = currentMethod();
    const base = {
      inputPath:        form.input.value.trim(),
      outputDir:        form.outDir.value.trim(),
      outputName:       form.outName.value.trim(),
      audioStreamIndex: parseInt(form.stream.value, 10) || 0,
      extractMethod:    method,
    };
    if (method === "copy") {
      return { ...base, format: detectedFormat, codec: "copy" };
    }
    const bitrate = form.tBitrate.disabled ? "" : form.tBitrate.value;
    return {
      ...base,
      format:     form.tFormat.value,
      codec:      form.tCodec.value,
      bitrate:    bitrate,
      sampleRate: parseInt(form.tSampleRate.value, 10) || 0,
      channels:   parseInt(form.tChannels.value, 10) || 0,
    };
  }

  function validate(body) {
    if (!body.inputPath)  throw new Error("请选择输入视频");
    if (!body.outputDir)  throw new Error("请选择输出目录");
    if (!body.outputName) throw new Error("请输入输出文件名");
    if (body.extractMethod === "copy" && !streams.length) {
      throw new Error("请等待音轨探测完成或选择有音轨的视频");
    }
  }

  function getOutputPath(body) {
    return Path.join(body.outputDir, `${body.outputName}.${body.format}`);
  }

  function buildPreview() {
    const body = readForm();
    if (!body.inputPath || !body.outputDir || !body.outputName) return "";
    const parts = [`ffmpeg -y -i "${body.inputPath}" -vn -map 0:a:${body.audioStreamIndex}`];
    if (body.extractMethod === "copy") {
      parts.push(`-c:a copy`);
    } else {
      parts.push(`-c:a ${body.codec}`);
      if (body.bitrate && body.bitrate !== "copy") parts.push(`-b:a ${body.bitrate}k`);
      if (body.sampleRate) parts.push(`-ar ${body.sampleRate}`);
      if (body.channels)   parts.push(`-ac ${body.channels}`);
    }
    parts.push(`"${getOutputPath(body)}"`);
    return parts.join(" ");
  }

  function refreshOpenOutDirBtn() {
    $("audioExtractOpenOutDir").disabled = !form.outDir.value.trim();
  }

  function init(opts = {}) {
    onChange = opts.onChange || (() => {});
    Object.assign(form, {
      input:        $("audioExtractInput"),
      stream:       $("audioExtractStream"),
      streamHint:   $("audioExtractStreamHint"),
      outDir:       $("audioExtractOutDir"),
      outName:      $("audioExtractOutName"),
      tFormat:      $("audioExtractFormat"),
      tCodec:       $("audioExtractCodec"),
      tBitrate:     $("audioExtractBitrate"),
      tSampleRate:  $("audioExtractSampleRate"),
      tChannels:    $("audioExtractChannels"),
    });

    rebuildTranscodeCodecOptions();
    refreshSubPanels();

    // 普通字段变化：刷新预览
    [form.input, form.outDir, form.outName,
     form.tFormat, form.tCodec, form.tBitrate, form.tSampleRate, form.tChannels]
      .forEach(el => {
        el.addEventListener("input", onChange);
        el.addEventListener("change", onChange);
      });
    form.stream.addEventListener("change", () => { updateDetectedFormat(); onChange(); });
    form.tFormat.addEventListener("change", () => { rebuildTranscodeCodecOptions(); onChange(); });
    form.tCodec.addEventListener("change", () => { refreshTranscodeBitrateState(); onChange(); });
    form.outDir.addEventListener("input", refreshOpenOutDirBtn);

    document.querySelectorAll('input[name="audioExtractMethod"]').forEach(r => {
      r.addEventListener("change", () => { refreshSubPanels(); onChange(); });
    });

    $("audioExtractPickInput").addEventListener("click", async () => {
      const start = form.input.value || Dirs.get().inputDir || "";
      const p = await Picker.open({ mode: "file", title: "选择输入视频", startPath: start });
      if (!p) return;
      form.input.value = p;
      const dir = Path.dirname(p);
      const base = Path.stripExt(Path.basename(p));
      if (!form.outName.value) form.outName.value = base + "_audio";
      if (dir) await Dirs.saveInput(dir).catch(() => {});
      await probeInput(p);
    });

    $("audioExtractPickOutDir").addEventListener("click", async () => {
      const start = form.outDir.value || Dirs.get().outputDir || "";
      const p = await Picker.open({ mode: "dir", title: "选择输出目录", startPath: start });
      if (!p) return;
      form.outDir.value = p;
      await Dirs.saveOutput(p).catch(() => {});
      refreshOpenOutDirBtn();
      onChange();
    });

    $("audioExtractOpenOutDir").addEventListener("click", async () => {
      const path = form.outDir.value.trim();
      if (!path) return;
      try { await Http.postJSON("/api/fs/reveal", { path }); }
      catch (e) { alert("打开失败: " + e.message); }
    });

    if (Dirs.get().outputDir) form.outDir.value = Dirs.get().outputDir;
    refreshOpenOutDirBtn();

    return { readForm, validate, getOutputPath, buildPreview };
  }

  return { init };
})();

// ============================================================
//  AudioMergeMode：多文件顺序拼接
// ============================================================

const AudioMergeMode = (() => {
  const form = {};
  let onChange = () => {};
  // items: [{ path, meta?: { codec, channels, sampleRate, bitRate, duration } }]
  let items = [];

  function currentStrategy() {
    const r = document.querySelector('input[name="audioMergeStrategy"]:checked');
    return r ? r.value : "auto";
  }

  function humanDuration(sec) {
    if (!sec) return "";
    sec = Math.round(sec);
    const h = Math.floor(sec / 3600), m = Math.floor((sec % 3600) / 60), s = sec % 60;
    if (h > 0) return `${h}:${String(m).padStart(2, "0")}:${String(s).padStart(2, "0")}`;
    return `${m}:${String(s).padStart(2, "0")}`;
  }

  function formatMeta(meta) {
    if (!meta) return "";
    const bits = [];
    if (meta.codec)    bits.push(meta.codec.toUpperCase());
    if (meta.channels) bits.push(`${meta.channels}ch`);
    if (meta.bitRate)  bits.push(`${Math.round(meta.bitRate / 1000)} kbps`);
    if (meta.duration) bits.push(humanDuration(meta.duration));
    return bits.join(" · ");
  }

  function renderList() {
    const ul = form.list;
    ul.innerHTML = "";
    items.forEach((it, i) => {
      const li = document.createElement("li");
      li.innerHTML = `
        <span class="drag-handle">☰</span>
        <span class="idx">${i + 1}</span>
        <span class="name" title="${it.path}">${Path.basename(it.path)}</span>
        <span class="meta">${formatMeta(it.meta)}</span>
        <button class="btn btn-ghost" data-action="up"   title="上移" ${i === 0 ? "disabled" : ""}>↑</button>
        <button class="btn btn-ghost" data-action="down" title="下移" ${i === items.length - 1 ? "disabled" : ""}>↓</button>
        <button class="btn btn-ghost" data-action="rm"   title="移除">🗑</button>
      `;
      li.querySelector('[data-action="up"]').addEventListener("click", () => move(i, -1));
      li.querySelector('[data-action="down"]').addEventListener("click", () => move(i, +1));
      li.querySelector('[data-action="rm"]').addEventListener("click", () => remove(i));
      ul.appendChild(li);
    });
    onChange();
  }

  function move(i, delta) {
    const j = i + delta;
    if (j < 0 || j >= items.length) return;
    [items[i], items[j]] = [items[j], items[i]];
    renderList();
  }

  function remove(i) {
    items.splice(i, 1);
    renderList();
  }

  async function addFile(path) {
    let meta = null;
    try {
      const res = await Http.postJSON("/api/audio/probe", { path });
      const s = res && res.streams && res.streams[0];
      if (s) {
        meta = {
          codec:      s.codecName,
          channels:   s.channels,
          sampleRate: s.sampleRate,
          bitRate:    s.bitRate,
          duration:   res.format ? res.format.duration : 0,
        };
      }
    } catch {}
    items.push({ path, meta });
    renderList();
  }

  function rebuildCodecOptions() {
    const fmt = form.format.value;
    const prev = form.codec.value;
    const list = AudioCodecs.codecsFor(fmt);
    form.codec.innerHTML = list.map(c => `<option value="${c.v}">${c.t}</option>`).join("");
    if (list.some(c => c.v === prev)) form.codec.value = prev;
    refreshBitrateState();
  }

  function refreshBitrateState() {
    const ignored = AudioCodecs.isBitrateIgnored(form.format.value, form.codec.value || "");
    form.bitrate.disabled = ignored;
    form.bitrate.title = ignored ? "当前格式/编码器无需码率" : "";
  }

  function readForm() {
    const bitrate = form.bitrate.disabled ? "" : form.bitrate.value;
    return {
      inputPaths:    items.map(it => it.path),
      outputDir:     form.outDir.value.trim(),
      outputName:    form.outName.value.trim(),
      format:        form.format.value,
      codec:         form.codec.value,
      bitrate:       bitrate,
      mergeStrategy: currentStrategy(),
    };
  }

  function validate(body) {
    if (body.inputPaths.length < 2) throw new Error("请至少添加 2 个输入文件");
    if (!body.outputDir)  throw new Error("请选择输出目录");
    if (!body.outputName) throw new Error("请输入输出文件名");
    if (!body.format)     throw new Error("请选择输出格式");
  }

  function getOutputPath(body) {
    return Path.join(body.outputDir, `${body.outputName}.${body.format}`);
  }

  function buildPreview() {
    const body = readForm();
    if (body.inputPaths.length < 2 || !body.outputDir || !body.outputName) return "";
    const out = getOutputPath(body);

    if (body.mergeStrategy === "copy") {
      return `ffmpeg -y -f concat -safe 0 -i <list.txt> -c copy "${out}"`;
    }

    const parts = ["ffmpeg -y"];
    body.inputPaths.forEach(p => parts.push(`-i "${p}"`));
    const filter = body.inputPaths.map((_, i) => `[${i}:a]`).join("")
                 + `concat=n=${body.inputPaths.length}:v=0:a=1[out]`;
    parts.push(`-filter_complex "${filter}"`);
    parts.push(`-map "[out]" -c:a ${body.codec || "aac"}`);
    if (body.bitrate && body.bitrate !== "copy") parts.push(`-b:a ${body.bitrate}k`);
    parts.push(`"${out}"`);
    if (body.mergeStrategy === "auto") parts.push("  # auto：编码一致时降级为快速拼接");
    return parts.join(" ");
  }

  function refreshOpenOutDirBtn() {
    $("audioMergeOpenOutDir").disabled = !form.outDir.value.trim();
  }

  function init(opts = {}) {
    onChange = opts.onChange || (() => {});
    Object.assign(form, {
      list:    $("audioMergeList"),
      outDir:  $("audioMergeOutDir"),
      outName: $("audioMergeOutName"),
      format:  $("audioMergeFormat"),
      codec:   $("audioMergeCodec"),
      bitrate: $("audioMergeBitrate"),
    });

    rebuildCodecOptions();

    [form.outDir, form.outName, form.format, form.codec, form.bitrate].forEach(el => {
      el.addEventListener("input", onChange);
      el.addEventListener("change", onChange);
    });
    form.format.addEventListener("change", () => { rebuildCodecOptions(); onChange(); });
    form.codec.addEventListener("change", () => { refreshBitrateState(); onChange(); });
    form.outDir.addEventListener("input", refreshOpenOutDirBtn);

    document.querySelectorAll('input[name="audioMergeStrategy"]').forEach(r => {
      r.addEventListener("change", onChange);
    });

    $("audioMergeAddBtn").addEventListener("click", async () => {
      const start = Dirs.get().inputDir || "";
      const p = await Picker.open({ mode: "file", title: "选择音频文件", startPath: start });
      if (!p) return;
      const dir = Path.dirname(p);
      if (dir) await Dirs.saveInput(dir).catch(() => {});
      await addFile(p);
    });

    $("audioMergePickOutDir").addEventListener("click", async () => {
      const start = form.outDir.value || Dirs.get().outputDir || "";
      const p = await Picker.open({ mode: "dir", title: "选择输出目录", startPath: start });
      if (!p) return;
      form.outDir.value = p;
      await Dirs.saveOutput(p).catch(() => {});
      refreshOpenOutDirBtn();
      onChange();
    });

    $("audioMergeOpenOutDir").addEventListener("click", async () => {
      const path = form.outDir.value.trim();
      if (!path) return;
      try { await Http.postJSON("/api/fs/reveal", { path }); }
      catch (e) { alert("打开失败: " + e.message); }
    });

    if (Dirs.get().outputDir) form.outDir.value = Dirs.get().outputDir;
    refreshOpenOutDirBtn();
    renderList();

    return { readForm, validate, getOutputPath, buildPreview };
  }

  return { init };
})();

// ============================================================
//  AudioTab：音频处理（三种模式共用日志 / 动作行）
// ============================================================

const AudioTab = (() => {
  const MODES = {};
  let activeMode = "convert";
  let panel;

  function updateCommandPreview() {
    const pre = $("audioCommandPreview");
    const m = MODES[activeMode];
    try {
      const cmd = m ? m.buildPreview() : "";
      pre.textContent = cmd || "ffmpeg ...";
    } catch {
      pre.textContent = "ffmpeg ...";
    }
  }

  function switchMode(name) {
    if (!MODES[name]) return;
    activeMode = name;
    document.querySelectorAll("#audioModeSwitch .seg").forEach(b => {
      b.classList.toggle("active", b.dataset.mode === name);
    });
    document.querySelectorAll("#panel-audio [data-mode-body]").forEach(d => {
      d.classList.toggle("hidden", d.dataset.modeBody !== name);
    });
    updateCommandPreview();
  }

  function init() {
    panel = createJobPanel({
      logEl: $("audioLog"),
      stateEl: $("audioJobState"),
      startBtn: $("audioStartBtn"),
      cancelBtn: $("audioCancelBtn"),
      finishBar: $("audioFinishBar"),
      finishText: $("audioFinishText"),
      finishRevealBtn: $("audioFinishRevealBtn"),
      progressWrap: $("audioProgressWrap"),
      progressFill: $("audioProgressFill"),
      progressText: $("audioProgressText"),
      cancelUrl: "/api/audio/cancel",
      runningLabel: "处理中...",
      doneLabel: "✓ 处理完成",
      errorLabel: "✗ 处理失败",
      cancelledLabel: "! 已取消",
    });

    MODES.convert = AudioConvertMode.init({ onChange: updateCommandPreview });
    MODES.extract = AudioExtractMode.init({ onChange: updateCommandPreview });
    MODES.merge   = AudioMergeMode.init({ onChange: updateCommandPreview });

    document.querySelectorAll("#audioModeSwitch .seg").forEach(b => {
      b.addEventListener("click", () => {
        if (b.disabled) return;
        if ($("audioStartBtn").disabled) return; // 运行中禁止切换
        if (!MODES[b.dataset.mode]) { alert("该模式尚未实现"); return; }
        switchMode(b.dataset.mode);
      });
    });

    $("audioStartBtn").addEventListener("click", async () => {
      const m = MODES[activeMode];
      if (!m) return;
      let body;
      try {
        body = m.readForm();
        m.validate(body);
      } catch (e) {
        alert(e.message);
        return;
      }
      const outputPath = m.getOutputPath(body);
      await panel.start({
        url: "/api/audio/start",
        body: { mode: activeMode, ...body },
        outputPath,
      });
    });

    switchMode("convert");
  }

  return { init };
})();

// ============================================================
//  Time：HH:MM:SS[.ms] 解析 / 格式化（供 EditorTab 等复用）
// ============================================================

const Time = (() => {
  // 严格接受 "HH:MM:SS" 或 "HH:MM:SS.mmm"；其他格式抛错。
  const RE = /^(\d{1,2}):(\d{1,2}):(\d{1,2})(?:\.(\d{1,3}))?$/;

  function parse(s) {
    const m = String(s || "").trim().match(RE);
    if (!m) throw new Error(`时间格式不合法，应为 HH:MM:SS 或 HH:MM:SS.mmm: "${s}"`);
    const h = parseInt(m[1], 10);
    const min = parseInt(m[2], 10);
    const sec = parseInt(m[3], 10);
    const ms = m[4] ? parseInt(m[4].padEnd(3, "0"), 10) : 0;
    if (min >= 60 || sec >= 60) throw new Error(`分/秒必须 < 60: "${s}"`);
    return h * 3600 + min * 60 + sec + ms / 1000;
  }

  function format(totalSec) {
    if (!isFinite(totalSec) || totalSec < 0) totalSec = 0;
    const h = Math.floor(totalSec / 3600);
    const m = Math.floor((totalSec % 3600) / 60);
    const s = Math.floor(totalSec % 60);
    const ms = Math.round((totalSec - Math.floor(totalSec)) * 1000);
    const pad = (n, w = 2) => String(n).padStart(w, "0");
    return `${pad(h)}:${pad(m)}:${pad(s)}.${pad(ms, 3)}`;
  }

  return { parse, format };
})();

// ============================================================
//  Tabs：Tab 切换
// ============================================================

const Tabs = (() => {
  let active = "convert";

  function switchTo(name) {
    document.querySelectorAll(".tab").forEach(t => {
      t.classList.toggle("active", t.dataset.tab === name);
    });
    document.querySelectorAll(".panel").forEach(p => {
      p.classList.toggle("hidden", p.id !== `panel-${name}`);
    });
    active = name;
  }

  function init() {
    document.querySelectorAll(".tab").forEach(btn => {
      btn.addEventListener("click", () => {
        if (btn.disabled) return;
        switchTo(btn.dataset.tab);
      });
    });
  }

  return { init, switchTo, getActive: () => active };
})();

// ============================================================
//  Quit：退出按钮
// ============================================================

const Quit = (() => {
  function init() {
    $("quitBtn").addEventListener("click", async () => {
      // No native confirm() — Wails WebView2 silently suppresses browser
      // dialogs in production, which would make this handler early-return
      // and leave the desktop window stuck open. Clicking "退出" is
      // already an explicit intent and no irreversible action follows.
      try { await fetch("/api/quit", { method: "POST" }); } catch {}
      document.body.innerHTML = `
        <div style="display:flex;align-items:center;justify-content:center;height:100vh;color:#9ca3af;flex-direction:column;gap:12px">
          <div style="font-size:42px">👋</div>
          <div>Easy FFmpeg 已退出，可关闭此页面。</div>
        </div>`;
    });
  }
  return { init };
})();

// ============================================================
//  Prepare：首次启动解压遮罩
// ============================================================

const Prepare = (() => {
  async function wait() {
    const backdrop = $("loadingBackdrop");
    const bar      = $("progressBar");
    const pctLabel = $("progressPercent");
    const fileLbl  = $("progressFile");
    const hint     = $("loadingHint");

    let overlayShown = false;
    while (true) {
      let p;
      try { p = await Http.fetchJSON("/api/prepare/status"); }
      catch { await new Promise(r => setTimeout(r, 500)); continue; }

      if (p.state === "ready") {
        if (overlayShown) {
          backdrop.classList.add("fading");
          setTimeout(() => backdrop.classList.add("hidden"), 300);
        }
        return;
      }
      if (p.state === "error") {
        if (!overlayShown) { backdrop.classList.remove("hidden"); overlayShown = true; }
        hint.textContent = "解压失败：" + (p.error || "未知错误");
        bar.style.background = "var(--danger)";
        return;
      }
      if (!overlayShown) { backdrop.classList.remove("hidden"); overlayShown = true; }
      const pct = p.percent || 0;
      bar.style.width = pct + "%";
      pctLabel.textContent = pct + "%";
      fileLbl.textContent = p.current || "";
      await new Promise(r => setTimeout(r, 300));
    }
  }
  return { wait };
})();

// ============================================================
//  init
// ============================================================

(async () => {
  await Prepare.wait();
  AppVersion.init();
  FFmpegStatus.init();
  await Dirs.load();
  Picker.init();
  ConvertTab.init();
  AudioTab.init();
  if (typeof EditorTab !== "undefined") EditorTab.init();
  Tabs.init();
  Quit.init();
  Confirm.init();
  JobBus.connect();
})();
