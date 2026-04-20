"use strict";

// ---------------- helpers ----------------

const $ = (id) => document.getElementById(id);

async function fetchJSON(url, opts = {}) {
  const res = await fetch(url, opts);
  const data = await res.json().catch(() => ({}));
  if (!res.ok) throw new Error(data.error || `HTTP ${res.status}`);
  return data;
}

function human(size) {
  if (!size) return "";
  const units = ["B", "KB", "MB", "GB", "TB"];
  let i = 0, v = size;
  while (v >= 1024 && i < units.length - 1) { v /= 1024; i++; }
  return `${v.toFixed(i === 0 ? 0 : 1)} ${units[i]}`;
}

function joinPath(dir, name) {
  if (!dir) return name;
  if (dir.endsWith("/")) return dir + name;
  return dir + "/" + name;
}

// ---------------- ffmpeg status ----------------

function parseFFmpegVersion(s) {
  if (!s) return "";
  const semver = s.match(/ffmpeg version (\d+(?:\.\d+)*)/i);
  if (semver) return semver[1];
  const any = s.match(/ffmpeg version (\S+)/i);
  return any ? any[1] : "";
}

async function loadFFmpegStatus() {
  const chip = $("ffmpegStatus");
  try {
    const s = await fetchJSON("/api/ffmpeg/status");
    if (s.available) {
      const v = parseFFmpegVersion(s.version);
      const where = s.embedded ? "嵌入" : "系统";
      chip.textContent = v ? `FFmpeg ${v} · ${where}` : `FFmpeg 可用（${where}）`;
      chip.className = "status-chip ok clickable";
      chip.title = (s.version || "") + "\n\n点击打开 FFmpeg 所在文件夹";
    } else {
      chip.textContent = "FFmpeg 未安装";
      chip.className = "status-chip err";
      chip.title = "";
    }
  } catch (e) {
    chip.textContent = "状态检测失败";
    chip.className = "status-chip err";
    chip.title = "";
  }
}

$("ffmpegStatus").addEventListener("click", async () => {
  if (!$("ffmpegStatus").classList.contains("clickable")) return;
  try {
    await fetchJSON("/api/ffmpeg/reveal", { method: "POST" });
  } catch (e) {
    alert("打开失败: " + e.message);
  }
});

// ---------------- convert form ----------------

const form = {
  inputPath:    $("inputPath"),
  outputDir:    $("outputDir"),
  outputName:   $("outputName"),
  videoEncoder: $("videoEncoder"),
  audioEncoder: $("audioEncoder"),
  format:       $("format"),
};

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
  const out = joinPath(f.outputDir, `${f.outputName}.${f.format}`);
  const vc = normalizeVideo(f.videoEncoder);
  const ac = f.audioEncoder;
  let cmd = `ffmpeg -y -i "${f.inputPath}"`;
  if (vc === "copy" && ac === "copy") cmd += ` -c copy`;
  else                                 cmd += ` -c:v ${vc} -c:a ${ac}`;
  cmd += ` "${out}"`;
  pre.textContent = cmd;
}

Object.values(form).forEach((el) => el.addEventListener("input", updateCommandPreview));
Object.values(form).forEach((el) => el.addEventListener("change", updateCommandPreview));

// ---------------- file picker ----------------

const picker = {
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
};

let pickerState = {
  mode: "dir",        // "dir" | "file"
  currentPath: "",
  selected: null,     // { name, isDir, size } or null
  resolver: null,
};

function openPicker({ mode, title, startPath }) {
  pickerState.mode = mode;
  pickerState.selected = null;
  picker.title.textContent = title;
  picker.confirm.textContent = mode === "dir" ? "选择此目录" : "选择文件";
  picker.hint.textContent = mode === "file" ? "选中一个文件后点击确认" : "";
  picker.backdrop.classList.remove("hidden");
  return new Promise(async (resolve) => {
    pickerState.resolver = resolve;
    let start = startPath || "";
    if (!start) {
      try {
        const h = await fetchJSON("/api/fs/home");
        start = h.home;
      } catch {}
    }
    loadPickerPath(start);
  });
}

function closePicker(result) {
  picker.backdrop.classList.add("hidden");
  if (pickerState.resolver) {
    const r = pickerState.resolver;
    pickerState.resolver = null;
    r(result);
  }
}

async function loadPickerPath(path) {
  try {
    const q = path ? `?path=${encodeURIComponent(path)}` : "";
    const data = await fetchJSON(`/api/fs/list${q}`);
    pickerState.currentPath = data.path;
    pickerState.selected = null;
    picker.path.value = data.path;
    renderDrives(data.drives);
    renderEntries(data.entries);
  } catch (e) {
    picker.list.innerHTML = `<li class="muted" style="padding:10px">加载失败: ${e.message}</li>`;
  }
}

function renderDrives(drives) {
  if (!drives || !drives.length) {
    picker.drive.classList.add("hidden");
    return;
  }
  picker.drive.classList.remove("hidden");
  picker.drive.innerHTML = drives
    .map((d) => `<option value="${d}">${d}</option>`)
    .join("");
  // select closest drive to currentPath
  const cur = pickerState.currentPath.toUpperCase();
  const match = drives.find((d) => cur.startsWith(d.toUpperCase()));
  if (match) picker.drive.value = match;
}
picker.drive.addEventListener("change", () => loadPickerPath(picker.drive.value));

function renderEntries(entries) {
  const mode = pickerState.mode;
  picker.list.innerHTML = "";
  if (!entries.length) {
    picker.list.innerHTML = `<li class="muted" style="padding:10px">空目录</li>`;
    return;
  }
  for (const e of entries) {
    const li = document.createElement("li");
    li.innerHTML = `
      <span class="icon">${e.isDir ? "📁" : "📄"}</span>
      <span class="name">${e.name}</span>
      <span class="meta">${e.isDir ? "" : human(e.size)}</span>
    `;
    li.addEventListener("click", () => {
      [...picker.list.children].forEach((n) => n.classList.remove("selected"));
      li.classList.add("selected");
      pickerState.selected = e;
    });
    li.addEventListener("dblclick", () => {
      const full = joinPath(pickerState.currentPath, e.name);
      if (e.isDir) loadPickerPath(full);
      else if (mode === "file") closePicker(full);
    });
    picker.list.appendChild(li);
  }
}

picker.up.addEventListener("click", () => {
  const cur = pickerState.currentPath;
  if (!cur) return;
  const idx = cur.lastIndexOf("/");
  const parent = idx > 0 ? cur.slice(0, idx) : (cur.length > 3 ? cur.slice(0, 3) : cur);
  loadPickerPath(parent);
});

picker.path.addEventListener("keydown", (e) => {
  if (e.key === "Enter") loadPickerPath(picker.path.value);
});

picker.confirm.addEventListener("click", () => {
  const mode = pickerState.mode;
  if (mode === "dir") {
    closePicker(pickerState.currentPath);
  } else {
    if (pickerState.selected && !pickerState.selected.isDir) {
      closePicker(joinPath(pickerState.currentPath, pickerState.selected.name));
    } else {
      picker.hint.textContent = "请先选中一个文件";
    }
  }
});

picker.cancel.addEventListener("click", () => closePicker(null));
picker.close.addEventListener("click", () => closePicker(null));
picker.backdrop.addEventListener("click", (e) => {
  if (e.target === picker.backdrop) closePicker(null);
});

// ---------------- wire up input / output pickers ----------------

$("pickInputBtn").addEventListener("click", async () => {
  const dirs = await fetchJSON("/api/config/dirs").catch(() => ({}));
  const start = form.inputPath.value || dirs.inputDir || "";
  const p = await openPicker({ mode: "file", title: "选择输入视频", startPath: start });
  if (!p) return;
  form.inputPath.value = p;
  const slash = p.lastIndexOf("/");
  const dir = slash >= 0 ? p.slice(0, slash) : "";
  const name = slash >= 0 ? p.slice(slash + 1) : p;
  const dot = name.lastIndexOf(".");
  const base = dot > 0 ? name.slice(0, dot) : name;
  if (!form.outputName.value) form.outputName.value = base + "_converted";
  if (dir) await fetch("/api/config/dirs", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ inputDir: dir }),
  });
  updateCommandPreview();
});

$("pickOutputDirBtn").addEventListener("click", async () => {
  const dirs = await fetchJSON("/api/config/dirs").catch(() => ({}));
  const start = form.outputDir.value || dirs.outputDir || "";
  const p = await openPicker({ mode: "dir", title: "选择输出目录", startPath: start });
  if (!p) return;
  form.outputDir.value = p;
  await fetch("/api/config/dirs", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ outputDir: p }),
  });
  updateCommandPreview();
  refreshOpenOutputDirBtn();
});

function refreshOpenOutputDirBtn() {
  $("openOutputDirBtn").disabled = !form.outputDir.value.trim();
}
form.outputDir.addEventListener("input", refreshOpenOutputDirBtn);

$("openOutputDirBtn").addEventListener("click", async () => {
  const path = form.outputDir.value.trim();
  if (!path) return;
  try {
    await fetchJSON("/api/fs/reveal", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ path }),
    });
  } catch (e) {
    alert("打开失败: " + e.message);
  }
});

// ---------------- SSE + convert ----------------

const logEl   = $("log");
const stateEl = $("jobState");
const startBtn  = $("startBtn");
const cancelBtn = $("cancelBtn");

const PROGRESS_RE = /^(frame=|size=|video:|Lsize=)/;

function appendLog(text, cls) {
  const isProgress = !cls && PROGRESS_RE.test(text);
  if (isProgress) {
    const last = logEl.lastElementChild;
    if (last && last.classList.contains("progress")) {
      last.textContent = text;
      logEl.scrollTop = logEl.scrollHeight;
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
  logEl.scrollTop = logEl.scrollHeight;
}

function setRunning(running) {
  startBtn.disabled  = running;
  cancelBtn.disabled = !running;
  stateEl.textContent = running ? "转码中..." : "空闲";
  stateEl.style.color = running ? "var(--accent)" : "var(--muted)";
}

let es = null;
function connectStream() {
  if (es) es.close();
  es = new EventSource("/api/convert/stream");
  es.onmessage = (msg) => {
    let ev;
    try { ev = JSON.parse(msg.data); } catch { return; }
    switch (ev.type) {
      case "state":
        setRunning(!!ev.running);
        break;
      case "log":
        appendLog(ev.line);
        break;
      case "done":
        setRunning(false);
        appendLog("✓ 转码完成", "success");
        break;
      case "error":
        setRunning(false);
        appendLog("✗ 转码失败: " + (ev.message || ""), "error");
        break;
      case "cancelled":
        setRunning(false);
        appendLog("! 转码已取消", "cancelled");
        break;
    }
  };
  es.onerror = () => {
    setTimeout(connectStream, 1500);
  };
}

startBtn.addEventListener("click", async () => {
  const f = readForm();
  if (!f.inputPath)   return alert("请选择输入文件");
  if (!f.outputDir)   return alert("请选择输出目录");
  if (!f.outputName)  return alert("请输入输出文件名");
  logEl.innerHTML = "";
  try {
    const res = await fetchJSON("/api/convert/start", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(f),
    });
    appendLog("> " + res.command, "info");
    setRunning(true);
  } catch (e) {
    appendLog("✗ 启动失败: " + e.message, "error");
  }
});

cancelBtn.addEventListener("click", async () => {
  try { await fetchJSON("/api/convert/cancel", { method: "POST" }); } catch {}
});

// ---------------- quit ----------------

$("quitBtn").addEventListener("click", async () => {
  if (!confirm("确定退出 Easy FFmpeg 吗？")) return;
  try { await fetch("/api/quit", { method: "POST" }); } catch {}
  document.body.innerHTML = `
    <div style="display:flex;align-items:center;justify-content:center;height:100vh;color:#9ca3af;flex-direction:column;gap:12px">
      <div style="font-size:42px">👋</div>
      <div>Easy FFmpeg 已退出，可关闭此页面。</div>
    </div>`;
});

// ---------------- first-run extraction wait ----------------

async function waitForPrepare() {
  const backdrop = $("loadingBackdrop");
  const bar      = $("progressBar");
  const pctLabel = $("progressPercent");
  const fileLbl  = $("progressFile");
  const hint     = $("loadingHint");

  let overlayShown = false;
  while (true) {
    let p;
    try {
      p = await fetchJSON("/api/prepare/status");
    } catch {
      await new Promise((r) => setTimeout(r, 500));
      continue;
    }

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

    // extracting or idle
    if (!overlayShown) { backdrop.classList.remove("hidden"); overlayShown = true; }
    const pct = p.percent || 0;
    bar.style.width = pct + "%";
    pctLabel.textContent = pct + "%";
    fileLbl.textContent = p.current || "";
    await new Promise((r) => setTimeout(r, 300));
  }
}

// ---------------- init ----------------

(async () => {
  await waitForPrepare();
  await loadFFmpegStatus();
  const dirs = await fetchJSON("/api/config/dirs").catch(() => ({}));
  if (dirs.outputDir) form.outputDir.value = dirs.outputDir;
  refreshOpenOutputDirBtn();
  connectStream();
  updateCommandPreview();
})();
