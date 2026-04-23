// editor.js — Video editor tab.
//
// Depends on utilities defined in app.js (Http, Picker, Dirs, Path, Time,
// JobBus, createJobPanel, $). These globals are resolved at function-call
// time, so script load order is not critical as long as app.js has finished
// parsing before any editor function is invoked.
//
// Public entry point: EditorTab.init(). Called from app.js's init sequence.

// ============================================================
//  Tracks：constants shared with the Go domain package
// ============================================================

const TRACK_VIDEO = "video";
const TRACK_AUDIO = "audio";

// ============================================================
//  EditorApi：HTTP wrappers for /api/editor/*
// ============================================================

const EditorApi = (() => {
  async function listProjects()     { return Http.getJSON("/api/editor/projects"); }
  async function createProject(body){ return Http.postJSON("/api/editor/projects", body); }
  async function getProject(id)     { return Http.getJSON(`/api/editor/projects/${encodeURIComponent(id)}`); }
  async function saveProject(p)     { return Http.putJSON(`/api/editor/projects/${encodeURIComponent(p.id)}`, p); }
  async function deleteProject(id)  { return Http.deleteJSON(`/api/editor/projects/${encodeURIComponent(id)}`); }
  async function probe(path)        { return Http.postJSON("/api/editor/probe", { path }); }
  async function startExport(body)  { return Http.postJSON("/api/editor/export", body); }
  async function cancelExport()     { return Http.postJSON("/api/editor/export/cancel", {}); }
  function sourceUrl(projectId)     { return `/api/editor/source?id=${encodeURIComponent(projectId)}`; }
  return { listProjects, createProject, getProject, saveProject, deleteProject, probe, startExport, cancelExport, sourceUrl };
})();

// ============================================================
//  TL：pure timeline math — operates on any []clip of one track
// ============================================================

const TL = (() => {
  function programToSource(clips, t) {
    let acc = 0;
    for (let i = 0; i < clips.length; i++) {
      const len = clips[i].sourceEnd - clips[i].sourceStart;
      if (t < acc + len) return { i, src: clips[i].sourceStart + (t - acc) };
      acc += len;
    }
    return null;
  }
  function clipProgramStart(clips, i) {
    let acc = 0;
    for (let k = 0; k < i; k++) acc += clips[k].sourceEnd - clips[k].sourceStart;
    return acc;
  }
  function programDuration(clips) {
    if (!clips) return 0;
    return clips.reduce((a, c) => a + (c.sourceEnd - c.sourceStart), 0);
  }
  function genClipId(track) {
    const p = track === TRACK_AUDIO ? "a" : "v";
    return p + Math.random().toString(36).slice(2, 6);
  }
  // total program length = max of the two tracks
  function totalDuration(project) {
    if (!project) return 0;
    return Math.max(programDuration(project.videoClips), programDuration(project.audioClips));
  }
  return { programToSource, clipProgramStart, programDuration, genClipId, totalDuration };
})();

// ============================================================
//  Selection helpers：selection is an array of {track, clipId}
// ============================================================

const Sel = (() => {
  function has(selection, track, clipId) {
    return selection.some(s => s.track === track && s.clipId === clipId);
  }
  function toggle(selection, track, clipId) {
    const i = selection.findIndex(s => s.track === track && s.clipId === clipId);
    if (i >= 0) {
      const out = selection.slice();
      out.splice(i, 1);
      return out;
    }
    return selection.concat([{ track, clipId }]);
  }
  function replace(track, clipId) {
    return [{ track, clipId }];
  }
  function inTrack(selection, track) {
    return selection.filter(s => s.track === track).map(s => s.clipId);
  }
  return { has, toggle, replace, inTrack };
})();

// ============================================================
//  EditorStore：state, subscribe, commit (auto-save)
// ============================================================

const EditorStore = (() => {
  let state = {
    project: null,           // domain Project or null
    dirty: false,
    selection: [],           // [{track, clipId}]
    splitScope: "both",      // "both" | "video" | "audio"
    playhead: 0,             // seconds, program time
    playing: false,
    pxPerSecond: 8,
  };
  const subs = new Set();

  function get() { return state; }

  function set(patch) {
    state = Object.assign({}, state, patch);
    notify();
  }

  function commit(projectPatch, opts) {
    if (!state.project) return;
    const nextProject = Object.assign({}, state.project, projectPatch);
    state = Object.assign({}, state, { project: nextProject, dirty: true });
    notify();
    if (!opts || opts.save !== false) scheduleSave();
  }

  function subscribe(fn) { subs.add(fn); return () => subs.delete(fn); }
  function notify()      { subs.forEach(fn => { try { fn(state); } catch (e) { console.error(e); } }); }

  let saveTimer = null;
  function scheduleSave() {
    if (saveTimer) clearTimeout(saveTimer);
    saveTimer = setTimeout(flushSave, 1500);
  }

  async function flushSave() {
    if (!state.project || !state.dirty) return;
    try {
      const saved = await EditorApi.saveProject(state.project);
      state = Object.assign({}, state, { project: saved, dirty: false });
      notify();
    } catch (e) {
      console.error("editor: save failed", e);
    }
  }

  return { get, set, commit, subscribe, flushSave };
})();

// ============================================================
//  History：undo / redo stack of (videoClips, audioClips) snapshots
// ============================================================

const History = (() => {
  const MAX = 100;
  let stack = [];
  let cursor = -1;

  function snapshot(project) {
    return {
      videoClips: (project.videoClips || []).map(c => Object.assign({}, c)),
      audioClips: (project.audioClips || []).map(c => Object.assign({}, c)),
    };
  }

  function push(project) {
    stack = stack.slice(0, cursor + 1);
    stack.push(snapshot(project));
    if (stack.length > MAX) stack = stack.slice(stack.length - MAX);
    cursor = stack.length - 1;
    notify();
  }

  function reset(project) {
    stack = [snapshot(project)];
    cursor = 0;
    notify();
  }

  function undo() {
    if (cursor <= 0) return null;
    cursor--;
    notify();
    return stack[cursor];
  }

  function redo() {
    if (cursor >= stack.length - 1) return null;
    cursor++;
    notify();
    return stack[cursor];
  }

  function canUndo() { return cursor > 0; }
  function canRedo() { return cursor < stack.length - 1; }

  const subs = new Set();
  function subscribe(fn) { subs.add(fn); return () => subs.delete(fn); }
  function notify() { subs.forEach(fn => fn()); }

  return { push, reset, undo, redo, canUndo, canRedo, subscribe };
})();

// ============================================================
//  Preview：<video> + program↔source mapping
//
//  Single <video> element. Preview follows the VIDEO track as the master:
//  when the program time crosses a video-clip boundary, seek the video to
//  the next clip's sourceStart. Independent audio edits are export-only —
//  preview always uses the source's raw audio aligned to video time.
// ============================================================

const Preview = (() => {
  let video = null;
  let activeIndex = -1;

  function init(videoEl) {
    video = videoEl;
    video.addEventListener("timeupdate", onSourceTimeUpdate);
    video.addEventListener("ended", () => EditorStore.set({ playing: false }));
    video.addEventListener("loadedmetadata", () => {
      const st = EditorStore.get();
      if (st.project) applySourceForProgramTime(st.playhead);
    });
  }

  function loadProject(project) {
    if (!project || !video) return;
    const url = EditorApi.sourceUrl(project.id);
    if (video.src !== location.origin + url && video.src !== url) {
      video.src = url;
    }
    activeIndex = -1;
    seek(0);
  }

  function play() {
    const st = EditorStore.get();
    if (!st.project || !video) return;
    if (video.paused) {
      applySourceForProgramTime(st.playhead);
      video.play().catch(() => {}).then(() => EditorStore.set({ playing: true }));
    }
  }

  function pause() {
    if (!video) return;
    video.pause();
    EditorStore.set({ playing: false });
  }

  function toggle() {
    const st = EditorStore.get();
    if (st.playing) pause(); else play();
  }

  function seek(programTime) {
    const st = EditorStore.get();
    if (!st.project) return;
    const total = TL.totalDuration(st.project);
    const clamped = Math.max(0, Math.min(programTime, total));
    EditorStore.set({ playhead: clamped });
    applySourceForProgramTime(clamped);
  }

  // previous / next clip boundary on the video track
  function seekToClipStart(direction) {
    const st = EditorStore.get();
    if (!st.project) return;
    const clips = st.project.videoClips || [];
    const cur = st.playhead;
    const boundaries = [0];
    let acc = 0;
    for (const c of clips) { acc += c.sourceEnd - c.sourceStart; boundaries.push(acc); }
    if (direction < 0) {
      for (let k = boundaries.length - 1; k >= 0; k--) {
        if (boundaries[k] < cur - 0.05) { seek(boundaries[k]); return; }
      }
      seek(0);
    } else {
      for (const b of boundaries) { if (b > cur + 0.05) { seek(b); return; } }
      seek(boundaries[boundaries.length - 1]);
    }
  }

  function applySourceForProgramTime(t) {
    const st = EditorStore.get();
    const clips = st.project && st.project.videoClips;
    if (!clips || !clips.length || !video) return;
    const pos = TL.programToSource(clips, t);
    if (!pos) {
      const last = clips[clips.length - 1];
      if (video.readyState > 0) video.currentTime = last.sourceEnd - 0.01;
      pause();
      return;
    }
    activeIndex = pos.i;
    if (video.readyState > 0 && Math.abs(video.currentTime - pos.src) > 0.05) {
      video.currentTime = pos.src;
    }
  }

  function onSourceTimeUpdate() {
    const st = EditorStore.get();
    const clips = st.project && st.project.videoClips;
    if (!clips || !clips.length || !video || activeIndex < 0) return;
    const c = clips[activeIndex];
    if (!c) return;

    if (video.currentTime >= c.sourceEnd - 0.01) {
      const next = activeIndex + 1;
      if (next >= clips.length) {
        pause();
        EditorStore.set({ playhead: TL.programDuration(clips) });
        return;
      }
      activeIndex = next;
      video.currentTime = clips[next].sourceStart;
      const newProgram = TL.clipProgramStart(clips, next);
      EditorStore.set({ playhead: newProgram });
      return;
    }
    const programStart = TL.clipProgramStart(clips, activeIndex);
    const delta = video.currentTime - c.sourceStart;
    EditorStore.set({ playhead: programStart + Math.max(0, delta) });
  }

  return { init, loadProject, play, pause, toggle, seek, seekToClipStart };
})();

// ============================================================
//  Timeline：render + interaction
//
//  Two independent tracks: video row + audio row. Each track is a DOM
//  container with absolute-positioned clip blocks. A "big" playhead spans
//  both tracks (mode = split-all); a "small" playhead sits inside one
//  track (mode = split-that-track).
// ============================================================

const Timeline = (() => {
  let els = null;
  let ghost = null;

  function init(refs) {
    els = refs;
    els.ruler.addEventListener("mousedown", onRulerMouseDown);
    els.videoTrack.addEventListener("mousedown", (e) => onTrackMouseDown(e, TRACK_VIDEO));
    els.audioTrack.addEventListener("mousedown", (e) => onTrackMouseDown(e, TRACK_AUDIO));
  }

  function render(state) {
    if (!els) return;
    renderRuler(state);
    renderTrack(state, els.videoTrack, TRACK_VIDEO);
    renderTrack(state, els.audioTrack, TRACK_AUDIO);
    renderPlayhead(state);
  }

  function renderRuler(state) {
    els.ruler.innerHTML = "";
    if (!state.project) return;
    const total = TL.totalDuration(state.project);
    const ppS = state.pxPerSecond;
    const step = pickStep(ppS);
    for (let t = 0; t <= total + 0.01; t += step) {
      const x = t * ppS;
      const tick = document.createElement("div");
      tick.className = "tick";
      tick.style.left = x + "px";
      els.ruler.appendChild(tick);
      const label = document.createElement("div");
      label.className = "tick-label";
      label.style.left = x + "px";
      label.textContent = fmtShort(t);
      els.ruler.appendChild(label);
    }
    const w = Math.max(total * ppS + 40, 400);
    els.ruler.style.width = w + "px";
    els.videoTrack.style.width = w + "px";
    els.audioTrack.style.width = w + "px";
  }

  function pickStep(ppS) {
    if (ppS <= 3)   return 30;
    if (ppS <= 6)   return 15;
    if (ppS <= 12)  return 10;
    if (ppS <= 20)  return 5;
    return 1;
  }

  function fmtShort(sec) {
    const s = Math.round(sec);
    const m = Math.floor(s / 60);
    const ss = s % 60;
    return `${m}:${ss.toString().padStart(2, "0")}`;
  }

  function renderTrack(state, trackEl, trackId) {
    Array.from(trackEl.querySelectorAll(".clip")).forEach(n => n.remove());
    if (!state.project) return;
    const ppS = state.pxPerSecond;
    const clips = (trackId === TRACK_VIDEO ? state.project.videoClips : state.project.audioClips) || [];
    let x = 0;
    clips.forEach((c, i) => {
      const w = (c.sourceEnd - c.sourceStart) * ppS;
      const el = document.createElement("div");
      el.className = "clip";
      if (Sel.has(state.selection, trackId, c.id)) el.classList.add("selected");
      el.style.left = x + "px";
      el.style.width = Math.max(8, w) + "px";
      el.dataset.clipId = c.id;
      el.dataset.clipIndex = String(i);
      el.dataset.track = trackId;
      el.innerHTML = `
        <span class="clip-label">${fmtShort(c.sourceStart)} - ${fmtShort(c.sourceEnd)}</span>
        <div class="clip-handle left"  data-handle="left"></div>
        <div class="clip-handle right" data-handle="right"></div>
      `;
      trackEl.appendChild(el);
      x += w;
    });
  }

  function renderPlayhead(state) {
    // Big playhead spans both tracks when splitScope === "both"
    // Small playhead sits in one track when splitScope === "video" or "audio"
    if (!state.project) {
      els.playheadBig.style.display = "none";
      els.playheadVideo.style.display = "none";
      els.playheadAudio.style.display = "none";
      return;
    }
    const x = (state.playhead * state.pxPerSecond) + "px";
    const show = (el, visible) => { el.style.display = visible ? "block" : "none"; el.style.left = x; };
    show(els.playheadBig,   state.splitScope === "both");
    show(els.playheadVideo, state.splitScope === TRACK_VIDEO);
    show(els.playheadAudio, state.splitScope === TRACK_AUDIO);
  }

  // ---- Ruler / track click handlers -------------------------------------

  function onRulerMouseDown(ev) {
    const rect = els.ruler.getBoundingClientRect();
    const x = ev.clientX - rect.left;
    const t = Math.max(0, x / EditorStore.get().pxPerSecond);
    Preview.seek(t);
    EditorStore.set({ splitScope: "both", selection: [] });
  }

  function onTrackMouseDown(ev, trackId) {
    const clipEl = ev.target.closest(".clip");
    if (!clipEl) {
      // empty track area click → seek + narrow split scope to this track
      const rect = ev.currentTarget.getBoundingClientRect();
      const x = ev.clientX - rect.left;
      const t = Math.max(0, x / EditorStore.get().pxPerSecond);
      Preview.seek(t);
      EditorStore.set({ splitScope: trackId, selection: [] });
      return;
    }
    const handle = ev.target.closest(".clip-handle");
    const clipId = clipEl.dataset.clipId;
    const multi = ev.shiftKey || ev.ctrlKey || ev.metaKey;
    const cur = EditorStore.get().selection;
    const nextSelection = multi ? Sel.toggle(cur, trackId, clipId) : Sel.replace(trackId, clipId);
    EditorStore.set({ selection: nextSelection, splitScope: trackId });

    if (handle) {
      startTrimDrag(ev, trackId, clipId, handle.dataset.handle);
    } else if (!multi) {
      startReorderDrag(ev, trackId, clipId);
    }
  }

  // ---- Trim drag (one clip's start or end) ------------------------------

  function startTrimDrag(ev, trackId, clipId, side) {
    ev.preventDefault();
    const state = EditorStore.get();
    const project = state.project;
    const clipsKey = trackClipsKey(trackId);
    const original = (project[clipsKey] || []).map(c => Object.assign({}, c));
    const idx = original.findIndex(c => c.id === clipId);
    if (idx < 0) return;
    const ppS = state.pxPerSecond;
    const startX = ev.clientX;

    function onMove(e) {
      const dx = e.clientX - startX;
      const ds = dx / ppS;
      const clips = original.map(c => Object.assign({}, c));
      const c = clips[idx];
      if (side === "left") {
        const newStart = Math.max(0, Math.min(c.sourceEnd - 0.05, c.sourceStart + ds));
        c.sourceStart = newStart;
      } else {
        const maxEnd = (project.source && project.source.duration) ? project.source.duration : c.sourceEnd + 600;
        const newEnd = Math.max(c.sourceStart + 0.05, Math.min(maxEnd, c.sourceEnd + ds));
        c.sourceEnd = newEnd;
      }
      EditorStore.commit({ [clipsKey]: clips }, { save: false });
    }
    function onUp() {
      document.removeEventListener("mousemove", onMove);
      document.removeEventListener("mouseup", onUp);
      History.push(EditorStore.get().project);
      EditorStore.commit({}, { save: true });
    }
    document.addEventListener("mousemove", onMove);
    document.addEventListener("mouseup", onUp);
  }

  // ---- Reorder drag (with cursor-following ghost) -----------------------

  function startReorderDrag(ev, trackId, clipId) {
    ev.preventDefault();
    const state = EditorStore.get();
    const project = state.project;
    const clipsKey = trackClipsKey(trackId);
    const original = (project[clipsKey] || []).map(c => Object.assign({}, c));
    const fromIdx = original.findIndex(c => c.id === clipId);
    if (fromIdx < 0) return;
    const ppS = state.pxPerSecond;
    const clipWidth = (original[fromIdx].sourceEnd - original[fromIdx].sourceStart) * ppS;
    const startX = ev.clientX;
    const startY = ev.clientY;
    const grabbedRect = ev.currentTarget.getBoundingClientRect();
    const grabOffsetX = ev.clientX - grabbedRect.left;
    let lastTargetIdx = fromIdx;
    let ghostBorn = false;

    function createGhost() {
      ghost = document.createElement("div");
      ghost.className = "clip-ghost " + (trackId === TRACK_VIDEO ? "ghost-video" : "ghost-audio");
      ghost.style.width = clipWidth + "px";
      ghost.style.height = (trackId === TRACK_VIDEO ? 40 : 30) + "px";
      document.body.appendChild(ghost);
    }
    function moveGhost(e) {
      if (!ghost) return;
      ghost.style.left = (e.clientX - grabOffsetX) + "px";
      ghost.style.top  = (e.clientY - 16) + "px";
    }

    function onMove(e) {
      // Start ghost after the pointer moves a few pixels so a simple click
      // doesn't leave a ghost behind.
      if (!ghostBorn && (Math.abs(e.clientX - startX) > 3 || Math.abs(e.clientY - startY) > 3)) {
        createGhost();
        ghostBorn = true;
      }
      if (ghostBorn) moveGhost(e);

      const dx = e.clientX - startX;
      const originalStart = TL.clipProgramStart(original, fromIdx);
      const centerProgram = originalStart + (clipWidth / 2) / ppS + dx / ppS;
      let acc = 0;
      let targetIdx = 0;
      for (let i = 0; i < original.length; i++) {
        const d = original[i].sourceEnd - original[i].sourceStart;
        const mid = acc + d / 2;
        if (centerProgram < mid) { targetIdx = i; break; }
        targetIdx = i + 1;
        acc += d;
      }
      if (targetIdx > fromIdx) targetIdx--;
      targetIdx = Math.max(0, Math.min(original.length - 1, targetIdx));
      if (targetIdx !== lastTargetIdx) {
        lastTargetIdx = targetIdx;
        const reordered = reorderArray(original, fromIdx, targetIdx);
        EditorStore.commit({ [clipsKey]: reordered }, { save: false });
      }
    }
    function onUp() {
      document.removeEventListener("mousemove", onMove);
      document.removeEventListener("mouseup", onUp);
      if (ghost) { ghost.remove(); ghost = null; }
      History.push(EditorStore.get().project);
      EditorStore.commit({}, { save: true });
    }
    document.addEventListener("mousemove", onMove);
    document.addEventListener("mouseup", onUp);
  }

  function reorderArray(arr, from, to) {
    const out = arr.slice();
    const [c] = out.splice(from, 1);
    out.splice(to, 0, c);
    return out;
  }

  function trackClipsKey(trackId) {
    return trackId === TRACK_VIDEO ? "videoClips" : "audioClips";
  }

  return { init, render };
})();

// ============================================================
//  TimelineOps：split / delete / undo / redo
// ============================================================

const TimelineOps = (() => {
  function splitTrack(project, trackId, programTime) {
    const key = trackId === TRACK_VIDEO ? "videoClips" : "audioClips";
    const clips = project[key] || [];
    if (!clips.length) return null;
    const pos = TL.programToSource(clips, programTime);
    if (!pos) return null;
    const clip = clips[pos.i];
    if (pos.src - clip.sourceStart < 0.05 || clip.sourceEnd - pos.src < 0.05) return null;
    const next = clips.slice();
    const left  = Object.assign({}, clip, { sourceEnd: pos.src });
    const right = Object.assign({}, clip, { id: TL.genClipId(trackId), sourceStart: pos.src });
    next.splice(pos.i, 1, left, right);
    return { [key]: next };
  }

  function splitAtPlayhead() {
    const st = EditorStore.get();
    if (!st.project) return;
    const t = st.playhead;
    let patch = {};
    if (st.splitScope === "both" || st.splitScope === TRACK_VIDEO) {
      const p = splitTrack(st.project, TRACK_VIDEO, t);
      if (p) Object.assign(patch, p);
    }
    if (st.splitScope === "both" || st.splitScope === TRACK_AUDIO) {
      const p = splitTrack(st.project, TRACK_AUDIO, t);
      if (p) Object.assign(patch, p);
    }
    if (!Object.keys(patch).length) return;
    EditorStore.commit(patch);
    History.push(EditorStore.get().project);
  }

  function deleteSelection() {
    const st = EditorStore.get();
    if (!st.project || !st.selection.length) return;
    const vIds = new Set(Sel.inTrack(st.selection, TRACK_VIDEO));
    const aIds = new Set(Sel.inTrack(st.selection, TRACK_AUDIO));
    const patch = {};
    if (vIds.size) patch.videoClips = (st.project.videoClips || []).filter(c => !vIds.has(c.id));
    if (aIds.size) patch.audioClips = (st.project.audioClips || []).filter(c => !aIds.has(c.id));
    if (!Object.keys(patch).length) return;
    EditorStore.commit(patch);
    EditorStore.set({ selection: [] });
    History.push(EditorStore.get().project);
  }

  function applySnapshot(snap) {
    EditorStore.commit({ videoClips: snap.videoClips, audioClips: snap.audioClips });
  }

  function undo() { const s = History.undo(); if (s) applySnapshot(s); }
  function redo() { const s = History.redo(); if (s) applySnapshot(s); }

  return { splitAtPlayhead, deleteSelection, undo, redo };
})();

// ============================================================
//  ProjectsModal：剪辑记录
// ============================================================

const ProjectsModal = (() => {
  let backdrop, listEl, emptyEl, onLoad;

  function init({ onProjectLoad }) {
    backdrop = $("edProjectsBackdrop");
    listEl   = $("edProjectList");
    emptyEl  = $("edProjectEmpty");
    onLoad   = onProjectLoad;
    $("edProjectsClose").addEventListener("click", close);
    backdrop.addEventListener("click", (ev) => { if (ev.target === backdrop) close(); });
  }

  async function open() {
    try {
      const items = await EditorApi.listProjects();
      renderList(items || []);
    } catch (e) {
      alert("加载剪辑记录失败: " + e.message);
      return;
    }
    backdrop.classList.remove("hidden");
  }

  function close() { backdrop.classList.add("hidden"); }

  function renderList(items) {
    listEl.innerHTML = "";
    if (!items.length) { emptyEl.classList.remove("hidden"); return; }
    emptyEl.classList.add("hidden");
    items.forEach(it => {
      const li = document.createElement("li");
      li.className = "row-item";
      li.innerHTML = `
        <div class="info">
          <div class="title"></div>
          <div class="meta"></div>
        </div>
        <div class="actions">
          <button class="btn btn-ghost" data-action="load">打开</button>
          <button class="btn btn-ghost" data-action="delete">🗑</button>
        </div>`;
      li.querySelector(".title").textContent = it.name || "(未命名)";
      li.querySelector(".meta").textContent = `${it.sourcePath} · 更新于 ${fmtDate(it.updatedAt)}`;
      li.querySelector("[data-action=load]").addEventListener("click", () => {
        close();
        onLoad && onLoad(it.id);
      });
      li.querySelector("[data-action=delete]").addEventListener("click", async () => {
        if (!confirm(`删除工程 "${it.name}"？`)) return;
        try { await EditorApi.deleteProject(it.id); } catch (e) { alert("删除失败: " + e.message); return; }
        li.remove();
        if (!listEl.children.length) emptyEl.classList.remove("hidden");
      });
      listEl.appendChild(li);
    });
  }

  function fmtDate(s) {
    if (!s) return "";
    return s.replace("T", " ").slice(0, 16);
  }

  return { init, open, close };
})();

// ============================================================
//  ExportModal：导出对话框 + 日志
// ============================================================

const ExportModal = (() => {
  let backdrop, panel, statusEl;

  function init() {
    backdrop = $("edExportBackdrop");
    statusEl = $("edExportStatus");
    panel = createJobPanel({
      logEl:           $("edExportLog"),
      stateEl:         $("edExportState"),
      startBtn:        $("edExportStart"),
      cancelBtn:       $("edExportCancelBtn"),
      finishBar:       $("edExportFinishBar"),
      finishText:      $("edExportFinishText"),
      finishRevealBtn: $("edExportFinishReveal"),
      cancelUrl:       "/api/editor/export/cancel",
      runningLabel:    "导出中...",
      doneLabel:       "✓ 导出完成",
      errorLabel:      "✗ 导出失败",
      cancelledLabel:  "! 导出已取消",
    });

    $("edExportClose").addEventListener("click", close);
    $("edExportCancel").addEventListener("click", close);
    backdrop.addEventListener("click", (ev) => { if (ev.target === backdrop) close(); });

    $("edExportPickDir").addEventListener("click", async () => {
      const start = $("edExportOutDir").value || (Dirs.get() || {}).outputDir || "";
      const p = await Picker.open({ mode: "dir", title: "选择输出目录", startPath: start });
      if (!p) return;
      $("edExportOutDir").value = p;
      await Dirs.saveOutput(p).catch(() => {});
    });

    $("edExportStart").addEventListener("click", async () => {
      const st = EditorStore.get();
      if (!st.project) return;
      const body = {
        projectId: st.project.id,
        export: {
          format:     $("edExportFormat").value,
          videoCodec: $("edExportVideoCodec").value,
          audioCodec: $("edExportAudioCodec").value,
          outputDir:  $("edExportOutDir").value.trim(),
          outputName: $("edExportOutName").value.trim() || "edit",
        },
      };
      if (!body.export.outputDir) { alert("请选择输出目录"); return; }
      const vCount = (st.project.videoClips || []).length;
      const aCount = (st.project.audioClips || []).length;
      if (!vCount && !aCount) { alert("时间轴为空，无法导出"); return; }
      await EditorStore.flushSave();
      close();
      statusEl.classList.remove("hidden");
      await panel.start({
        url: "/api/editor/export",
        body,
        outputPath: Path.join(body.export.outputDir, body.export.outputName + "." + body.export.format),
      });
    });
  }

  function open() {
    const st = EditorStore.get();
    if (!st.project) return;
    const e = st.project.export || {};
    $("edExportFormat").value     = e.format     || "mp4";
    $("edExportVideoCodec").value = e.videoCodec || "h264";
    $("edExportAudioCodec").value = e.audioCodec || "aac";
    $("edExportOutDir").value     = e.outputDir  || (Dirs.get() || {}).outputDir || "";
    $("edExportOutName").value    = e.outputName || (st.project.name || "edit");
    backdrop.classList.remove("hidden");
  }

  function close() { backdrop.classList.add("hidden"); }

  return { init, open, close };
})();

// ============================================================
//  EditorTab：top-level init + render glue
// ============================================================

const EditorTab = (() => {
  function init() {
    const refs = {
      tabBtn:        document.querySelector('[data-tab="editor"]'),
      empty:         $("edEmpty"),
      workspace:     $("edWorkspace"),
      video:         $("edVideo"),
      ruler:         $("edRuler"),
      videoTrack:    $("edVideoTrack"),
      audioTrack:    $("edAudioTrack"),
      playheadBig:   $("edPlayheadBig"),
      playheadVideo: $("edPlayheadVideo"),
      playheadAudio: $("edPlayheadAudio"),
      playPause:     $("edPlayPause"),
      prevClip:      $("edPrevClip"),
      nextClip:      $("edNextClip"),
      timecode:      $("edTimecode"),
      volume:        $("edVolume"),
      splitBtn:      $("edSplit"),
      deleteBtn:     $("edDelete"),
      undoBtn:       $("edUndo"),
      redoBtn:       $("edRedo"),
      zoom:          $("edZoom"),
      projectName:   $("edProjectName"),
      openVideoBtn:  $("edOpenVideo"),
      projectsBtn:   $("edProjects"),
      exportBtn:     $("edExport"),
      scopeLabel:    $("edSplitScopeLabel"),
    };

    if (refs.tabBtn) refs.tabBtn.disabled = false;

    Preview.init(refs.video);
    Timeline.init(refs);
    ProjectsModal.init({ onProjectLoad: loadProjectById });
    ExportModal.init();

    // Top-bar
    refs.openVideoBtn.addEventListener("click", openVideo);
    refs.projectsBtn.addEventListener("click", () => ProjectsModal.open());
    refs.exportBtn.addEventListener("click", () => ExportModal.open());
    refs.projectName.addEventListener("input", () => {
      if (!EditorStore.get().project) return;
      EditorStore.commit({ name: refs.projectName.value });
    });

    // Playbar
    refs.playPause.addEventListener("click", () => Preview.toggle());
    refs.prevClip.addEventListener("click", () => Preview.seekToClipStart(-1));
    refs.nextClip.addEventListener("click", () => Preview.seekToClipStart(1));
    refs.volume.addEventListener("input", () => { refs.video.volume = parseFloat(refs.volume.value); });

    // Toolbar
    refs.splitBtn.addEventListener("click", TimelineOps.splitAtPlayhead);
    refs.deleteBtn.addEventListener("click", TimelineOps.deleteSelection);
    refs.undoBtn.addEventListener("click", TimelineOps.undo);
    refs.redoBtn.addEventListener("click", TimelineOps.redo);
    refs.zoom.addEventListener("input", () => EditorStore.set({ pxPerSecond: parseInt(refs.zoom.value, 10) }));

    // Keyboard shortcuts
    document.addEventListener("keydown", (e) => {
      if (!isEditorActive() || isEditableFocus()) return;
      switch (e.key) {
        case " ":           e.preventDefault(); Preview.toggle(); break;
        case "s": case "S": TimelineOps.splitAtPlayhead(); break;
        case "Delete":
        case "Backspace":   TimelineOps.deleteSelection(); break;
        case "ArrowLeft":   Preview.seekToClipStart(-1); break;
        case "ArrowRight":  Preview.seekToClipStart(1);  break;
        case "z": case "Z":
          if (e.ctrlKey || e.metaKey) { e.preventDefault(); e.shiftKey ? TimelineOps.redo() : TimelineOps.undo(); }
          break;
        case "y": case "Y":
          if (e.ctrlKey || e.metaKey) { e.preventDefault(); TimelineOps.redo(); }
          break;
      }
    });

    // Single render function, fed by both publishers.
    const rerender = () => render(EditorStore.get());
    EditorStore.subscribe(rerender);
    History.subscribe(rerender);

    renderEmpty(refs);
  }

  function isEditorActive() {
    const panel = $("panel-editor");
    return panel && !panel.classList.contains("hidden");
  }

  function isEditableFocus() {
    const a = document.activeElement;
    if (!a) return false;
    const tag = a.tagName;
    return tag === "INPUT" || tag === "TEXTAREA" || tag === "SELECT" || a.isContentEditable;
  }

  async function openVideo() {
    const start = (Dirs.get() || {}).inputDir || "";
    const p = await Picker.open({ mode: "file", title: "选择要剪辑的视频", startPath: start });
    if (!p) return;
    try {
      const project = await EditorApi.createProject({ sourcePath: p, name: Path.stripExt(Path.basename(p)) });
      if (Path.dirname(p)) Dirs.saveInput(Path.dirname(p)).catch(() => {});
      loadProject(project);
    } catch (e) {
      alert("创建工程失败: " + e.message);
    }
  }

  async function loadProjectById(id) {
    try {
      const project = await EditorApi.getProject(id);
      loadProject(project);
    } catch (e) {
      alert("加载工程失败: " + e.message);
    }
  }

  function loadProject(project) {
    EditorStore.set({ project, selection: [], playhead: 0, playing: false, dirty: false, splitScope: "both" });
    History.reset(project);
    Preview.loadProject(project);
  }

  function render(state) {
    const refs = collectRefs();
    const hasProject = !!state.project;
    refs.empty.classList.toggle("hidden", hasProject);
    refs.workspace.classList.toggle("hidden", !hasProject);
    const total = state.project ? TL.totalDuration(state.project) : 0;
    refs.exportBtn.disabled = !hasProject || total <= 0;
    refs.deleteBtn.disabled = !state.selection.length;
    refs.undoBtn.disabled = !History.canUndo();
    refs.redoBtn.disabled = !History.canRedo();
    refs.playPause.textContent = state.playing ? "⏸" : "▶";
    if (refs.scopeLabel) refs.scopeLabel.textContent = scopeLabelText(state.splitScope);
    if (!hasProject) return;
    refs.projectName.value = state.project.name || "";
    refs.projectName.classList.toggle("dirty", state.dirty);
    refs.timecode.textContent = `${Time.format(state.playhead)} / ${Time.format(total)}`;
    Timeline.render(state);
  }

  function scopeLabelText(scope) {
    if (scope === TRACK_VIDEO) return "当前分割范围：视频轨";
    if (scope === TRACK_AUDIO) return "当前分割范围：音频轨";
    return "当前分割范围：两轨一起";
  }

  function renderEmpty(refs) {
    refs.workspace.classList.add("hidden");
    refs.empty.classList.remove("hidden");
    refs.exportBtn.disabled = true;
  }

  function collectRefs() {
    return {
      empty:       $("edEmpty"),
      workspace:   $("edWorkspace"),
      exportBtn:   $("edExport"),
      deleteBtn:   $("edDelete"),
      undoBtn:     $("edUndo"),
      redoBtn:     $("edRedo"),
      playPause:   $("edPlayPause"),
      projectName: $("edProjectName"),
      timecode:    $("edTimecode"),
      scopeLabel:  $("edSplitScopeLabel"),
    };
  }

  return { init };
})();
