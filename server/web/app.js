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
    el.backdrop.addEventListener("click", (e) => {
      if (e.target === el.backdrop) close(null);
    });
  }

  return { init, open };
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
  cancelUrl,
  runningLabel = "处理中...",
  idleLabel = "空闲",
  doneLabel = "✓ 完成",
  errorLabel = "✗ 失败",
  cancelledLabel = "! 已取消",
}) {
  const PROGRESS_RE = /^(frame=|size=|video:|Lsize=)/;
  let owning = false;
  let lastOutputPath = null;

  function appendLog(text, cls) {
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

  async function start({ url, body, outputPath }) {
    logEl.innerHTML = "";
    hideFinish();
    lastOutputPath = outputPath || null;

    const doStart = async (b) => {
      const res = await fetch(url, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(b),
      });
      const data = await res.json().catch(() => ({}));
      if (res.status === 409 && data.existing) {
        if (!confirm(`目标文件已存在，是否覆盖？\n\n${data.path}`)) return;
        return doStart({ ...b, overwrite: true });
      }
      if (!res.ok) throw new Error(data.error || `HTTP ${res.status}`);
      appendLog("> " + data.command, "info");
      owning = true;
      setRunning(true);
    };

    try { await doStart(body); }
    catch (e) { showFinish("error", "✗ 启动失败: " + e.message, null); }
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
        showFinish("success", doneLabel, lastOutputPath);
        break;
      case "error":
        owning = false;
        setRunning(false);
        showFinish("error", `${errorLabel}: ${ev.message || ""}`, null);
        break;
      case "cancelled":
        owning = false;
        setRunning(false);
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
//  Time：HH:MM:SS[.ms] 解析 / 格式化（供 TrimTab 与后续功能复用）
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
//  TrimTab：视频裁剪（时间 / 空间 / 分辨率 三组独立开关）
// ============================================================

const TrimTab = (() => {
  const form = {};
  let panel;
  let probe = null; // 最近一次 /api/trim/probe 响应

  // 分辨率预设；custom 表示手动输入
  const SCALE_PRESETS = {
    source: null,                       // 由 probe 填充
    "480p":  { w: 854,  h: 480  },
    "720p":  { w: 1280, h: 720  },
    "1080p": { w: 1920, h: 1080 },
    "4k":    { w: 3840, h: 2160 },
    custom:  null,
  };

  function normalizeVideo(v) {
    if (v === "h264") return "libx264";
    if (v === "h265") return "libx265";
    return v;
  }

  function isEnabled(block) {
    return block.dataset.enabled === "true";
  }

  function setEnabled(block, enabled) {
    block.dataset.enabled = enabled ? "true" : "false";
  }

  function readForm() {
    return {
      inputPath:    form.input.value.trim(),
      outputDir:    form.outDir.value.trim(),
      outputName:   form.outName.value.trim(),
      format:       form.format.value,
      videoEncoder: form.videoEnc.value,
      audioEncoder: form.audioEnc.value,
      trim: {
        enabled: form.timeEnable.checked,
        start:   form.startTime.value.trim(),
        end:     form.endTime.value.trim(),
      },
      crop: {
        enabled: form.cropEnable.checked,
        x: parseInt(form.cropX.value, 10) || 0,
        y: parseInt(form.cropY.value, 10) || 0,
        w: parseInt(form.cropW.value, 10) || 0,
        h: parseInt(form.cropH.value, 10) || 0,
      },
      scale: {
        enabled: form.scaleEnable.checked,
        keepRatio: form.scaleKeep.checked,
        w: parseInt(form.scaleW.value, 10) || 0,
        h: parseInt(form.scaleH.value, 10) || 0,
      },
    };
  }

  function validate(body) {
    if (!body.inputPath)  throw new Error("请选择输入视频");
    if (!body.outputDir)  throw new Error("请选择输出目录");
    if (!body.outputName) throw new Error("请输入输出文件名");
    if (!body.trim.enabled && !body.crop.enabled && !body.scale.enabled) {
      throw new Error("请至少启用一项操作（时间裁剪 / 空间裁剪 / 分辨率缩放）");
    }

    if (body.trim.enabled) {
      const a = Time.parse(body.trim.start);
      const b = Time.parse(body.trim.end);
      if (a >= b) throw new Error("时间裁剪：起始必须早于结束");
    }

    if (body.crop.enabled) {
      if (body.crop.w <= 0 || body.crop.h <= 0) {
        throw new Error("空间裁剪：宽与高必须大于 0");
      }
      if (probe && probe.video && probe.video.width) {
        if (body.crop.x + body.crop.w > probe.video.width ||
            body.crop.y + body.crop.h > probe.video.height) {
          throw new Error("空间裁剪：矩形超出源画面范围");
        }
      }
    }

    if (body.scale.enabled) {
      const w = body.scale.w, h = body.scale.h;
      if (body.scale.keepRatio) {
        if (w <= 0 && h <= 0) throw new Error("分辨率缩放：保持比例时至少填一维");
      } else if (w <= 0 || h <= 0) {
        throw new Error("分辨率缩放：宽与高必须大于 0");
      }
    }
  }

  function getOutputPath(body) {
    return Path.join(body.outputDir, `${body.outputName}.${body.format}`);
  }

  function buildPreview() {
    const body = readForm();
    if (!body.inputPath || !body.outputDir || !body.outputName) return "";
    const parts = [`ffmpeg -y -i "${body.inputPath}"`];
    if (body.trim.enabled) {
      parts.push(`-ss ${body.trim.start || "00:00:00"}`);
      parts.push(`-to ${body.trim.end || "00:00:00"}`);
    }
    const filters = [];
    if (body.crop.enabled) {
      filters.push(`crop=${body.crop.w}:${body.crop.h}:${body.crop.x}:${body.crop.y}`);
    }
    if (body.scale.enabled) {
      const w = body.scale.keepRatio && body.scale.w <= 0 ? -2 : body.scale.w;
      const h = body.scale.keepRatio && body.scale.h <= 0 ? -2 : body.scale.h;
      filters.push(`scale=${w}:${h}`);
    }
    if (filters.length) parts.push(`-vf ${filters.join(",")}`);
    parts.push(`-c:v ${normalizeVideo(body.videoEncoder)} -c:a ${body.audioEncoder}`);
    parts.push(`"${getOutputPath(body)}"`);
    return parts.join(" ");
  }

  function updateCommandPreview() {
    const pre = $("trimCommandPreview");
    try {
      const cmd = buildPreview();
      pre.textContent = cmd || "ffmpeg ...";
    } catch {
      pre.textContent = "ffmpeg ...";
    }
  }

  function refreshOpenOutDirBtn() {
    $("trimOpenOutDir").disabled = !form.outDir.value.trim();
  }

  function applyScalePreset() {
    const v = form.scalePreset.value;
    if (v === "custom") return;
    const preset = SCALE_PRESETS[v];
    if (!preset) return; // "source" 还没 probe
    form.scaleW.value = preset.w;
    form.scaleH.value = preset.h;
  }

  function renderProbeStatus() {
    if (!probe) return;
    const bits = [];
    if (probe.format && probe.format.duration) {
      bits.push(Time.format(probe.format.duration).replace(/\.000$/, ""));
    }
    if (probe.video && probe.video.width) {
      bits.push(`${probe.video.width}×${probe.video.height}`);
    }
    if (probe.video && probe.video.codecName) bits.push(probe.video.codecName);
    if (probe.video && probe.video.frameRate) {
      bits.push(`${probe.video.frameRate.toFixed(2)} fps`);
    }
    form.probeStatus.textContent = bits.join(" · ") || "无可用信息";
  }

  function applyProbeDefaults() {
    if (!probe) return;
    if (probe.format && probe.format.duration) {
      form.endTime.value = Time.format(probe.format.duration);
    }
    const v = probe.video;
    if (v && v.width && v.height) {
      form.cropW.value = v.width;
      form.cropH.value = v.height;
      SCALE_PRESETS.source = { w: v.width, h: v.height };
      if (form.scalePreset.value === "source") {
        form.scaleW.value = v.width;
        form.scaleH.value = v.height;
      }
    }
  }

  async function probeInput(path) {
    form.probeStatus.textContent = "探测中...";
    probe = null;
    try {
      probe = await Http.postJSON("/api/trim/probe", { path });
      renderProbeStatus();
      applyProbeDefaults();
    } catch (e) {
      form.probeStatus.textContent = "探测失败: " + e.message;
    }
    updateCommandPreview();
  }

  function init() {
    Object.assign(form, {
      input:        $("trimInput"),
      probeStatus:  $("trimProbeStatus"),

      timeBlock:    $("trimTimeBlock"),
      timeEnable:   $("trimTimeEnable"),
      startTime:    $("trimStart"),
      endTime:      $("trimEnd"),

      cropBlock:    $("trimCropBlock"),
      cropEnable:   $("trimCropEnable"),
      cropX:        $("trimCropX"),
      cropY:        $("trimCropY"),
      cropW:        $("trimCropW"),
      cropH:        $("trimCropH"),

      scaleBlock:   $("trimScaleBlock"),
      scaleEnable:  $("trimScaleEnable"),
      scalePreset:  $("trimScalePreset"),
      scaleKeep:    $("trimScaleKeepRatio"),
      scaleW:       $("trimScaleW"),
      scaleH:       $("trimScaleH"),

      outDir:       $("trimOutDir"),
      outName:      $("trimOutName"),
      videoEnc:     $("trimVideoEncoder"),
      audioEnc:     $("trimAudioEncoder"),
      format:       $("trimFormat"),
    });

    panel = createJobPanel({
      logEl: $("trimLog"),
      stateEl: $("trimJobState"),
      startBtn: $("trimStartBtn"),
      cancelBtn: $("trimCancelBtn"),
      finishBar: $("trimFinishBar"),
      finishText: $("trimFinishText"),
      finishRevealBtn: $("trimFinishRevealBtn"),
      cancelUrl: "/api/trim/cancel",
      runningLabel: "裁剪中...",
      doneLabel: "✓ 裁剪完成",
      errorLabel: "✗ 裁剪失败",
      cancelledLabel: "! 裁剪已取消",
    });

    // 启用开关
    [
      [form.timeEnable,  form.timeBlock],
      [form.cropEnable,  form.cropBlock],
      [form.scaleEnable, form.scaleBlock],
    ].forEach(([cb, block]) => {
      cb.addEventListener("change", () => {
        setEnabled(block, cb.checked);
        updateCommandPreview();
      });
    });

    // 命令预览实时刷新
    [
      form.startTime, form.endTime,
      form.cropX, form.cropY, form.cropW, form.cropH,
      form.scalePreset, form.scaleKeep, form.scaleW, form.scaleH,
      form.outDir, form.outName,
      form.videoEnc, form.audioEnc, form.format,
    ].forEach(el => {
      el.addEventListener("input", updateCommandPreview);
      el.addEventListener("change", updateCommandPreview);
    });
    form.outDir.addEventListener("input", refreshOpenOutDirBtn);

    // 预设联动：切预设自动填宽高（包括 source / 480p / 1080p 等）；手动改宽高 → preset 变 custom
    form.scalePreset.addEventListener("change", () => {
      applyScalePreset();
      updateCommandPreview();
    });
    [form.scaleW, form.scaleH].forEach(el => {
      el.addEventListener("input", () => {
        if (form.scalePreset.value !== "custom") form.scalePreset.value = "custom";
      });
    });

    // Pickers
    $("trimPickInput").addEventListener("click", async () => {
      const start = form.input.value || Dirs.get().inputDir || "";
      const p = await Picker.open({ mode: "file", title: "选择输入视频", startPath: start });
      if (!p) return;
      form.input.value = p;
      const dir = Path.dirname(p);
      const base = Path.stripExt(Path.basename(p));
      if (!form.outName.value) form.outName.value = base + "_trimmed";
      if (dir) await Dirs.saveInput(dir).catch(() => {});
      await probeInput(p);
    });

    $("trimPickOutDir").addEventListener("click", async () => {
      const start = form.outDir.value || Dirs.get().outputDir || "";
      const p = await Picker.open({ mode: "dir", title: "选择输出目录", startPath: start });
      if (!p) return;
      form.outDir.value = p;
      await Dirs.saveOutput(p).catch(() => {});
      refreshOpenOutDirBtn();
      updateCommandPreview();
    });

    $("trimOpenOutDir").addEventListener("click", async () => {
      const path = form.outDir.value.trim();
      if (!path) return;
      try { await Http.postJSON("/api/fs/reveal", { path }); }
      catch (e) { alert("打开失败: " + e.message); }
    });

    $("trimStartBtn").addEventListener("click", async () => {
      let body;
      try {
        body = readForm();
        validate(body);
      } catch (e) {
        alert(e.message);
        return;
      }
      const outputPath = getOutputPath(body);
      await panel.start({ url: "/api/trim/start", body, outputPath });
    });

    if (Dirs.get().outputDir) form.outDir.value = Dirs.get().outputDir;
    refreshOpenOutDirBtn();
    updateCommandPreview();
  }

  return { init };
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
      if (!confirm("确定退出 Easy FFmpeg 吗？")) return;
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
  FFmpegStatus.init();
  await Dirs.load();
  Picker.init();
  ConvertTab.init();
  AudioTab.init();
  TrimTab.init();
  Tabs.init();
  Quit.init();
  JobBus.connect();
})();
