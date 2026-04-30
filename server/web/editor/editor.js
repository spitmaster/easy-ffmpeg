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
  function clipDur(c)     { return c.sourceEnd - c.sourceStart; }
  function clipProgEnd(c) { return c.programStart + clipDur(c); }

  // Find which clip occupies program time t. Returns { i, src } or null when
  // t falls in a gap between clips (or past the end).
  function programToSource(clips, t) {
    if (!clips) return null;
    for (let i = 0; i < clips.length; i++) {
      const c = clips[i];
      if (t >= c.programStart && t < clipProgEnd(c)) {
        return { i, src: c.sourceStart + (t - c.programStart) };
      }
    }
    return null;
  }
  // Duration of one track = largest program-end across its clips.
  // A leading gap counts toward this length.
  function programDuration(clips) {
    if (!clips || !clips.length) return 0;
    let max = 0;
    for (const c of clips) {
      const e = clipProgEnd(c);
      if (e > max) max = e;
    }
    return max;
  }
  function genClipId(track) {
    const p = track === TRACK_AUDIO ? "a" : "v";
    return p + Math.random().toString(36).slice(2, 6);
  }
  function totalDuration(project) {
    if (!project) return 0;
    return Math.max(programDuration(project.videoClips), programDuration(project.audioClips));
  }
  // Nearest clip boundary on a track (used for ⏮/⏭ and potentially snaps).
  function collectBoundaries(clips) {
    const xs = [0];
    if (!clips) return xs;
    for (const c of clips) {
      xs.push(c.programStart);
      xs.push(clipProgEnd(c));
    }
    return Array.from(new Set(xs)).sort((a, b) => a - b);
  }
  return { programToSource, programDuration, genClipId, totalDuration, clipDur, clipProgEnd, collectBoundaries };
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
    // {start, end} program-time range (start <= end) painted across the
    // ruler + both tracks. Set by right-drag on the ruler; consumed by
    // split / delete; cleared by Esc, by starting a new right-drag, or by
    // loading a different project.
    rangeSelection: null,
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
//  Preview：separate <video> + <audio>, independent per-track playback.
//
//  The video element is muted and owns the picture; the audio element owns
//  the sound. Both load the same source URL and are seek'd independently
//  according to videoClips / audioClips respectively. The video element
//  is the master clock — its timeupdate drives the program playhead, and
//  the audio element is re-synced on every tick.
//
//  Why two elements instead of one <video> + WebAudio:
//  — Cheapest way to get independent seeks on the same source
//  — Browser handles decoding, range requests, buffering
//  — The caller can't introduce clock drift between picture and sound
//    beyond a few tens of ms, which is good enough for editing preview
// ============================================================

const Preview = (() => {
  let video = null;
  let audio = null;
  let activeVideoIndex = -1;
  let activeAudioIndex = -1;
  // WebAudio routing for the audio track. Going through a GainNode lets
  // us boost above 1.0 (HTMLMediaElement.volume hard-caps at 1.0) so
  // preview can match the export's `volume=` filter at any setting.
  // audioCtx is created lazily on first applyAudioVolume call so we
  // don't trip browser autoplay policies before the user gestures.
  let audioCtx = null;
  let gainNode = null;
  // "Gap clock": when the program time is sitting in a video-track gap
  // the <video> element has no frames to deliver, so we drive the
  // playhead via requestAnimationFrame instead. Anchored to a real-time
  // sample so leaving a gap and re-entering it stays linear.
  let gapClockId = null;
  let gapAnchorReal = 0;       // performance.now() at clock start
  let gapAnchorPlayhead = 0;   // program time at clock start

  function init(videoEl, audioEl) {
    video = videoEl;
    audio = audioEl;

    // Video carries no sound — the <audio> element does. Keeping volume=0
    // too is belt-and-braces against some browsers that interpret `muted`
    // as "mute the track" but still play audible audio at low volume.
    video.muted = true;
    video.volume = 0;
    if (audio) {
      audio.muted = false;
      // Initial volume comes from project on first loadProject; until
      // then keep it at unity so silent-by-default isn't surprising.
      audio.volume = 1;
    }

    video.addEventListener("timeupdate", onVideoTimeUpdate);
    // When the <video> element naturally hits source EOF (e.g. the
    // user's video clip extends to the end of the source file and our
    // timeupdate-based detector didn't catch the boundary first), don't
    // stop everything if there's still audio program time remaining —
    // hand off to the gap clock so the audio plays through to its end.
    video.addEventListener("ended", () => {
      const st = EditorStore.get();
      if (!st.project || !st.playing) {
        EditorStore.set({ playing: false });
        return;
      }
      const total = TL.totalDuration(st.project);
      if (st.playhead < total - 0.01) {
        activeVideoIndex = -1;
        video.classList.add("in-gap");
        startGapClock();
        return;
      }
      EditorStore.set({ playing: false });
    });
    video.addEventListener("loadedmetadata", () => {
      const st = EditorStore.get();
      if (st.project) {
        applyVideoFor(st.playhead);
        applyAudioFor(st.playhead);
      }
    });
    video.addEventListener("error", () => {
      console.error("[editor] video error:", video.error);
    });
    // Watchdog: if anything flips the video element off mute (stale HTML
    // cache, browser sync on src change, user typing M in the OS key queue,
    // etc.) slam it back. Sound must always come from the <audio> element.
    video.addEventListener("volumechange", () => {
      if (!video.muted || video.volume !== 0) {
        video.muted = true;
        video.volume = 0;
      }
    });

    if (audio) {
      audio.addEventListener("timeupdate", onAudioTimeUpdate);
      audio.addEventListener("error", () => {
        console.error("[editor] audio error:", audio.error);
      });
    }
  }

  // Audio volume comes from project.audioVolume (a per-project track
  // property), not from a transient UI control. applyAudioVolume is
  // called on project load, on store changes, and after the <audio>
  // element rebinds, so what plays back stays in lockstep with the
  // project state and (more importantly) with what export will emit.
  //
  // Routing: HTMLMediaElement.volume is capped at 1.0, so values above
  // 100% (boost) need WebAudio. We lazy-init a GainNode on first call
  // and route the <audio> element through it; from then on audio.volume
  // is irrelevant and the gain node is the single source of truth.
  // Falls back to setting audio.volume directly if WebAudio isn't
  // available — preview will silently cap at 100% in that case.
  function applyAudioVolume() {
    if (!audio) return;
    const p = EditorStore.get().project;
    const v = Math.max(0, p && p.audioVolume != null ? p.audioVolume : 1);
    if (!gainNode) initGainNode();
    if (gainNode) {
      gainNode.gain.value = v;
      audio.volume = 1; // gain node is in charge
      if (audioCtx && audioCtx.state === "suspended") {
        audioCtx.resume().catch(() => {});
      }
    } else {
      audio.volume = Math.min(1, v);
    }
  }

  function initGainNode() {
    if (!audio || gainNode) return;
    const Ctor = window.AudioContext || window.webkitAudioContext;
    if (!Ctor) return;
    try {
      audioCtx = new Ctor();
      const src = audioCtx.createMediaElementSource(audio);
      gainNode = audioCtx.createGain();
      gainNode.gain.value = 1;
      src.connect(gainNode).connect(audioCtx.destination);
    } catch (e) {
      // createMediaElementSource throws if called twice on the same
      // element, or if the element is in a state WebAudio rejects. We
      // log once and fall back to plain audio.volume.
      console.warn("[editor] WebAudio gain unavailable; preview volume capped at 100%:", e);
      audioCtx = null;
      gainNode = null;
    }
  }

  function loadProject(project) {
    if (!project || !video) return;
    const url = EditorApi.sourceUrl(project.id);
    if (!sameSrc(video, url)) video.src = url;
    if (audio && !sameSrc(audio, url)) audio.src = url;
    activeVideoIndex = -1;
    activeAudioIndex = -1;
    stopGapClock();
    video.classList.remove("in-gap");
    applyAudioVolume();
    seek(0);
  }

  function sameSrc(el, url) {
    return el.src === url || el.src === location.origin + url;
  }

  function play() {
    const st = EditorStore.get();
    if (!st.project || !video) return;
    video.muted = true;
    if (audio) audio.muted = false;
    applyVideoFor(st.playhead);
    applyAudioFor(st.playhead);
    // Once playback starts, the cursor becomes a global program-time
    // indicator and stays that way after pause too — flipping back to a
    // single-track playhead post-playback feels like the cursor is
    // "regressing". Promote splitScope so visual + cut semantics agree.
    EditorStore.set({ playing: true, splitScope: "both" });
    // Pick a clock: real <video> when there's a clip to decode, gap
    // clock when there isn't. Never both — they'd race on `playhead`.
    if (activeVideoIndex >= 0) {
      stopGapClock();
      if (video.paused) video.play().catch(() => {});
    } else {
      startGapClock();
    }
    if (audio && audio.paused && activeAudioIndex >= 0) {
      audio.play().catch(() => {});
    }
  }

  function pause() {
    if (video) video.pause();
    if (audio) audio.pause();
    stopGapClock();
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
    applyVideoFor(clamped);
    applyAudioFor(clamped);
    // Mid-playback seeks may cross between gap and clip — re-pick the
    // clock so we don't end up stuck (video paused but no gap clock).
    if (st.playing) {
      if (activeVideoIndex >= 0) {
        stopGapClock();
        if (video.paused) video.play().catch(() => {});
      } else {
        // Re-anchor the gap clock at the new playhead so elapsed-time
        // math stays linear after the seek.
        stopGapClock();
        startGapClock();
      }
    }
  }

  // ---- Gap clock --------------------------------------------------------
  //
  // Advances the program playhead by real elapsed time while the video
  // track has no clip under the cursor. Each tick re-checks whether the
  // playhead has crossed into a video clip; when it has, we hand control
  // back to the <video> element. End of program ⇒ pause everything.

  function startGapClock() {
    if (gapClockId !== null) return;
    gapAnchorReal = performance.now();
    gapAnchorPlayhead = EditorStore.get().playhead;
    gapClockId = requestAnimationFrame(gapTick);
  }

  function stopGapClock() {
    if (gapClockId !== null) {
      cancelAnimationFrame(gapClockId);
      gapClockId = null;
    }
  }

  function gapTick() {
    gapClockId = null;
    const st = EditorStore.get();
    if (!st.playing || !st.project) return;
    const total = TL.totalDuration(st.project);
    const elapsed = (performance.now() - gapAnchorReal) / 1000;
    const newPlayhead = gapAnchorPlayhead + elapsed;
    if (newPlayhead >= total - 1e-3) {
      EditorStore.set({ playhead: total });
      pause();
      return;
    }
    EditorStore.set({ playhead: newPlayhead });
    const videoClips = (st.project.videoClips) || [];
    const pos = TL.programToSource(videoClips, newPlayhead);
    if (pos) {
      // Crossed into a video clip — wake the <video> element up. From
      // here, timeupdate drives the clock; gap clock stops.
      activeVideoIndex = pos.i;
      video.classList.remove("in-gap");
      if (video.readyState > 0) video.currentTime = pos.src;
      if (video.paused) video.play().catch(() => {});
      keepAudioInSync(newPlayhead);
      return;
    }
    keepAudioInSync(newPlayhead);
    gapClockId = requestAnimationFrame(gapTick);
  }

  // Previous / next boundary on the video track (Arrow keys / ⏮ ⏭).
  function seekToClipStart(direction) {
    const st = EditorStore.get();
    if (!st.project) return;
    const boundaries = TL.collectBoundaries(st.project.videoClips);
    const cur = st.playhead;
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

  // ---- video track ------------------------------------------------------

  function applyVideoFor(t) {
    if (!video) return;
    const clips = (EditorStore.get().project && EditorStore.get().project.videoClips) || [];
    const pos = TL.programToSource(clips, t);
    if (!pos) {
      // Gap (or empty track): show black instead of freezing on the last
      // frame, and mark the element so CSS can hide it.
      activeVideoIndex = -1;
      if (!video.paused) video.pause();
      video.classList.add("in-gap");
      return;
    }
    activeVideoIndex = pos.i;
    video.classList.remove("in-gap");
    if (video.readyState > 0 && Math.abs(video.currentTime - pos.src) > 0.05) {
      video.currentTime = pos.src;
    }
  }

  function onVideoTimeUpdate() {
    const st = EditorStore.get();
    const clips = (st.project && st.project.videoClips) || [];
    if (!clips.length || !video || activeVideoIndex < 0) return;
    const c = clips[activeVideoIndex];
    if (!c) return;

    // End of current video clip. If the next clip is back-to-back we seek
    // straight into it; otherwise a gap follows (or no more clips) and the
    // gap clock takes over so the playhead keeps advancing while the
    // preview shows black.
    if (video.currentTime >= c.sourceEnd - 0.01) {
      const sorted = clips.slice().sort((a, b) => a.programStart - b.programStart);
      const curIdx = sorted.findIndex(x => x.id === c.id);
      const nextClip = sorted[curIdx + 1];
      const programEnd = c.programStart + (c.sourceEnd - c.sourceStart);
      if (nextClip && nextClip.programStart - programEnd < 0.01) {
        // Back-to-back — no visible gap, no need for the gap clock.
        activeVideoIndex = clips.findIndex(x => x.id === nextClip.id);
        video.currentTime = nextClip.sourceStart;
        EditorStore.set({ playhead: nextClip.programStart });
        applyAudioFor(nextClip.programStart);
        return;
      }
      // Gap follows (or program ended on this track) — hand off to gap clock.
      EditorStore.set({ playhead: programEnd });
      activeVideoIndex = -1;
      video.classList.add("in-gap");
      if (!video.paused) video.pause();
      keepAudioInSync(programEnd);
      // Re-read playing from the store rather than the entry snapshot
      // (`st`) — defensive against any subscriber side-effect that may
      // have flipped state between function entry and here.
      if (EditorStore.get().playing) startGapClock();
      return;
    }

    const delta = video.currentTime - c.sourceStart;
    const newPlayhead = c.programStart + Math.max(0, delta);
    EditorStore.set({ playhead: newPlayhead });
    keepAudioInSync(newPlayhead);
  }

  // ---- audio track ------------------------------------------------------

  function applyAudioFor(t) {
    if (!audio) return;
    const clips = (EditorStore.get().project && EditorStore.get().project.audioClips) || [];
    const pos = TL.programToSource(clips, t);
    if (!pos) {
      // Audio gap (or track empty) — silence until next clip arrives.
      activeAudioIndex = -1;
      if (!audio.paused) audio.pause();
      return;
    }
    activeAudioIndex = pos.i;
    if (audio.readyState > 0 && Math.abs(audio.currentTime - pos.src) > 0.05) {
      audio.currentTime = pos.src;
    }
    // Resume audio if we were playing but paused for a prior gap.
    if (EditorStore.get().playing && audio.paused) {
      audio.play().catch(() => {});
    }
  }

  // Called from onVideoTimeUpdate to keep audio continuously aligned.
  // Drift up to ~150ms is tolerated without hard-seeking, which would
  // otherwise cause an audible click every 250ms (typical video timeupdate
  // cadence in Chrome).
  function keepAudioInSync(programTime) {
    if (!audio) return;
    const clips = (EditorStore.get().project && EditorStore.get().project.audioClips) || [];
    const pos = TL.programToSource(clips, programTime);
    if (!pos) {
      if (activeAudioIndex !== -1 || !audio.paused) {
        activeAudioIndex = -1;
        if (!audio.paused) audio.pause();
      }
      return;
    }
    if (pos.i !== activeAudioIndex) {
      // Crossed an audio clip boundary — hard seek into the new clip.
      activeAudioIndex = pos.i;
      if (audio.readyState > 0) audio.currentTime = pos.src;
      if (EditorStore.get().playing && audio.paused) audio.play().catch(() => {});
      return;
    }
    if (audio.readyState > 0 && Math.abs(audio.currentTime - pos.src) > 0.15) {
      audio.currentTime = pos.src;
    }
    if (EditorStore.get().playing && audio.paused) {
      audio.play().catch(() => {});
    }
  }

  function onAudioTimeUpdate() {
    // Audio is not the master clock — we only need this to detect the end
    // of an audio clip early and skip to the next one, so the user doesn't
    // briefly hear the last frames of audio that got trimmed out.
    const st = EditorStore.get();
    const clips = (st.project && st.project.audioClips) || [];
    if (!clips.length || !audio || activeAudioIndex < 0) return;
    const c = clips[activeAudioIndex];
    if (!c) return;
    if (audio.currentTime >= c.sourceEnd - 0.01) {
      applyAudioFor(st.playhead);
    }
  }

  return { init, loadProject, play, pause, toggle, seek, seekToClipStart, applyAudioVolume };
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

  // PX_MIN is deliberately tiny: with a 2h+ source and a 1000px viewport
  // the fit value can legitimately reach ~0.14 px/s. Earlier we clamped at
  // 0.5, which made the default-fit view of a long video unable to zoom
  // out any further — user perceived it as "ticks collide, slider dead".
  const PX_MIN = 0.05;
  const PX_MAX = 80;

  function init(refs) {
    els = refs;
    els.ruler.addEventListener("mousedown", onRulerMouseDown);
    els.videoTrack.addEventListener("mousedown", (e) => onTrackMouseDown(e, TRACK_VIDEO));
    els.audioTrack.addEventListener("mousedown", (e) => onTrackMouseDown(e, TRACK_AUDIO));
    els.playheadBig.addEventListener("mousedown", onPlayheadMouseDown);
    els.playheadVideo.addEventListener("mousedown", onPlayheadMouseDown);
    els.playheadAudio.addEventListener("mousedown", onPlayheadMouseDown);
    if (els.scroll) els.scroll.addEventListener("wheel", onWheel, { passive: false });
  }

  // Fit-to-width: pick the largest pxPerSecond that shows the whole program
  // inside the visible scroll width (clamped to the slider's range).
  function fitPxPerSecond(project) {
    if (!els || !els.scroll) return 8;
    const total = TL.totalDuration(project);
    if (total <= 0) return 8;
    const viewW = Math.max(100, els.scroll.clientWidth - 24);
    return Math.max(PX_MIN, Math.min(PX_MAX, viewW / total));
  }

  // Ctrl+Wheel zooms around the cursor; plain wheel scrolls horizontally.
  // We swap deltaY → scrollLeft because timelines have no vertical overflow,
  // so a bare mouse wheel would otherwise do nothing useful.
  function onWheel(ev) {
    if (ev.ctrlKey || ev.metaKey) {
      ev.preventDefault();
      const st = EditorStore.get();
      if (!st.project) return;
      const rect = els.scroll.getBoundingClientRect();
      const anchorX = ev.clientX - rect.left + els.scroll.scrollLeft;
      const anchorTime = anchorX / st.pxPerSecond;
      // Exponential zoom — constant ratio per wheel notch feels smoother
      // than linear across a wide (0.5–80 px/s) range.
      const factor = Math.exp(-ev.deltaY * 0.0015);
      const next = Math.max(PX_MIN, Math.min(PX_MAX, st.pxPerSecond * factor));
      EditorStore.set({ pxPerSecond: next });
      syncZoomSlider(next);
      // Keep the time under the cursor stationary on screen after zoom.
      const newAnchorX = anchorTime * next;
      els.scroll.scrollLeft = newAnchorX - (ev.clientX - rect.left);
    } else if (ev.deltaY !== 0 && ev.deltaX === 0) {
      // Plain wheel → horizontal scroll (only if the browser isn't already
      // sending a horizontal event, e.g. from a trackpad with two-finger pan).
      ev.preventDefault();
      els.scroll.scrollLeft += ev.deltaY;
    }
  }

  function syncZoomSlider(px) {
    if (els && els.zoom) els.zoom.value = String(px);
  }

  // Exposed so EditorTab.loadProject can set the initial fit.
  function applyFit(project) {
    const px = fitPxPerSecond(project);
    EditorStore.set({ pxPerSecond: px });
    syncZoomSlider(px);
  }

  function render(state) {
    if (!els) return;
    renderRuler(state);
    renderTrack(state, els.videoTrack, TRACK_VIDEO);
    renderTrack(state, els.audioTrack, TRACK_AUDIO);
    renderPlayhead(state);
    renderRangeSelection(state);
  }

  function renderRangeSelection(state) {
    const el = els.rangeSel;
    if (!el) return;
    if (!state.project || !state.rangeSelection) {
      el.style.display = "none";
      return;
    }
    const a = Math.min(state.rangeSelection.start, state.rangeSelection.end);
    const b = Math.max(state.rangeSelection.start, state.rangeSelection.end);
    const ppS = state.pxPerSecond;
    el.style.display = "block";
    el.style.left  = (a * ppS) + "px";
    el.style.width = Math.max(1, (b - a) * ppS) + "px";
  }

  function renderRuler(state) {
    els.ruler.innerHTML = "";
    if (!state.project) return;
    const total = TL.totalDuration(state.project);
    const ppS = state.pxPerSecond;
    const step = pickStep(ppS);
    // Use a numeric loop counter (not float addition) so tiny FP errors
    // don't compound into missing ticks at the end of long tracks.
    const count = Math.floor(total / step) + 1;
    for (let i = 0; i <= count; i++) {
      const t = i * step;
      if (t > total + 0.01) break;
      const x = t * ppS;
      const tick = document.createElement("div");
      tick.className = "tick";
      tick.style.left = x + "px";
      els.ruler.appendChild(tick);
      const label = document.createElement("div");
      label.className = "tick-label";
      label.style.left = x + "px";
      label.textContent = fmtTick(t, step);
      els.ruler.appendChild(label);
    }
    const w = Math.max(total * ppS + 40, 400);
    els.ruler.style.width = w + "px";
    els.videoTrack.style.width = w + "px";
    els.audioTrack.style.width = w + "px";
  }

  // Pick a label step from the "nice number" ladder so adjacent labels are
  // at least TARGET_PX apart. Using a ladder (vs e.g. step = ideal rounded
  // to 1 sig fig) keeps the ticks at human-friendly values — 0.2, 0.5, 1,
  // 2, 5… — which read much better than 0.23s or 1.7s.
  const STEPS = [0.05, 0.1, 0.2, 0.5, 1, 2, 5, 10, 15, 30, 60, 120, 300, 600, 900, 1800, 3600, 7200, 14400];
  // Target spacing between ticks. Labels at ≥1h look like "1:30:00" (~48px)
  // so 90px keeps at least ~40px of breathing room between adjacent labels.
  const TARGET_PX = 90;

  function pickStep(ppS) {
    const ideal = TARGET_PX / ppS;
    for (const s of STEPS) if (s >= ideal) return s;
    return STEPS[STEPS.length - 1];
  }

  // Format a ruler label at the current step granularity. Sub-second steps
  // show decimals; ≥1h tracks gain an hour field.
  function fmtTick(sec, step) {
    const decimals = step >= 1 ? 0 : (step >= 0.1 ? 1 : 2);
    if (sec >= 3600) {
      const h = Math.floor(sec / 3600);
      const m = Math.floor((sec % 3600) / 60);
      const s = (sec % 60).toFixed(decimals);
      const pad = decimals > 0 ? decimals + 3 : 2;
      return `${h}:${String(m).padStart(2, "0")}:${s.padStart(pad, "0")}`;
    }
    const m = Math.floor(sec / 60);
    const s = (sec % 60).toFixed(decimals);
    const pad = decimals > 0 ? decimals + 3 : 2;
    return `${m}:${s.padStart(pad, "0")}`;
  }

  // Short mm:ss for clip badges — always integer seconds, step-agnostic.
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
    clips.forEach((c, i) => {
      const w = TL.clipDur(c) * ppS;
      const el = document.createElement("div");
      el.className = "clip";
      if (Sel.has(state.selection, trackId, c.id)) el.classList.add("selected");
      // Clips are now positioned absolutely by their ProgramStart, not by
      // accumulating earlier clips' widths — this is what lets gaps exist.
      el.style.left = (c.programStart * ppS) + "px";
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
    });
  }

  function renderPlayhead(state) {
    // splitScope drives the playhead form: "both" → cross-track big
    // playhead; "video" / "audio" → single-track playhead inside that
    // track. Hitting play() promotes splitScope to "both" (Preview.play),
    // and it sticks after pause — so the cursor never regresses from
    // global to single-track on its own.
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

  // Continuously seek to the cursor's X position on the ruler. Used by
  // ruler click-drag and by grabbing a playhead handle directly. Pauses
  // playback during the drag and resumes if it was playing, so audio
  // doesn't keep ticking against the seeking video.
  function startScrubDrag(ev) {
    ev.preventDefault();
    const wasPlaying = EditorStore.get().playing;
    if (wasPlaying) Preview.pause();
    const seekFromClientX = (clientX) => {
      const rect = els.ruler.getBoundingClientRect();
      const x = clientX - rect.left;
      const t = Math.max(0, x / EditorStore.get().pxPerSecond);
      Preview.seek(t);
    };
    seekFromClientX(ev.clientX);
    function onMove(e) { seekFromClientX(e.clientX); }
    function onUp() {
      document.removeEventListener("mousemove", onMove);
      document.removeEventListener("mouseup", onUp);
      if (wasPlaying) Preview.play();
    }
    document.addEventListener("mousemove", onMove);
    document.addEventListener("mouseup", onUp);
  }

  function onRulerMouseDown(ev) {
    if (ev.button === 2) {
      // Right-click drag → range select. Range and clip selections are
      // mutually exclusive, so wipe the clip selection here.
      startRangeSelect(ev);
      return;
    }
    // Any non-range left-click on the timeline cancels the range — the
    // user is starting a new action, the highlight shouldn't linger.
    EditorStore.set({ splitScope: "both", selection: [], rangeSelection: null });
    startScrubDrag(ev);
  }

  // Right-drag on the ruler defines a [start, end] program-time selection.
  // While dragging we keep both endpoints in store order (start = anchor,
  // end = cursor) so the visual mirrors the gesture; we normalize on mouseup
  // so consumers (split/delete) can assume start <= end.
  function startRangeSelect(ev) {
    ev.preventDefault();
    const project = EditorStore.get().project;
    if (!project) return;
    const total = TL.totalDuration(project);
    const ppS = EditorStore.get().pxPerSecond;
    const rect = els.ruler.getBoundingClientRect();
    const toTime = (clientX) =>
      Math.max(0, Math.min(total, (clientX - rect.left) / ppS));
    const anchor = toTime(ev.clientX);
    // Clip selection and range selection are mutually exclusive — drop
    // any selected clips so the toolbar reflects "range mode". Also force
    // splitScope back to "both": the range overlay paints across ruler +
    // both tracks, so any inherited single-track scope (left over from
    // clicking a video/audio clip earlier) would silently make split /
    // delete only touch one track and confuse the user.
    EditorStore.set({
      rangeSelection: { start: anchor, end: anchor },
      selection: [],
      splitScope: "both",
    });
    function onMove(e) {
      EditorStore.set({ rangeSelection: { start: anchor, end: toTime(e.clientX) } });
    }
    function onUp() {
      document.removeEventListener("mousemove", onMove);
      document.removeEventListener("mouseup", onUp);
      const r = EditorStore.get().rangeSelection;
      if (!r) return;
      const a = Math.min(r.start, r.end);
      const b = Math.max(r.start, r.end);
      // A bare right-click (no real drag) clears the range — gives the user
      // an explicit way to dismiss it without reaching for the keyboard.
      if (b - a < 0.05) EditorStore.set({ rangeSelection: null });
      else EditorStore.set({ rangeSelection: { start: a, end: b } });
    }
    document.addEventListener("mousemove", onMove);
    document.addEventListener("mouseup", onUp);
  }

  // Grab handle on any of the three playheads — drag-scrubs without
  // changing splitScope. Cancels the range selection, since starting any
  // other timeline gesture means the user has moved on from "range mode".
  function onPlayheadMouseDown(ev) {
    if (ev.button !== 0) return;
    ev.stopPropagation();
    if (EditorStore.get().rangeSelection) EditorStore.set({ rangeSelection: null });
    startScrubDrag(ev);
  }

  function onTrackMouseDown(ev, trackId) {
    // Right-click on a track is a no-op — range select only triggers on
    // the ruler. We just swallow the event so contextmenu suppression at
    // the panel level can do its job without surprising selection changes.
    if (ev.button !== 0) return;
    // Any track interaction cancels the range — clip-mode and range-mode
    // are mutually exclusive.
    if (EditorStore.get().rangeSelection) EditorStore.set({ rangeSelection: null });

    const clipEl = ev.target.closest(".clip");
    if (!clipEl) {
      // empty track area click → narrow split scope to this track, then
      // start scrub-drag so the user can swipe to a precise time.
      EditorStore.set({ splitScope: trackId, selection: [] });
      startScrubDrag(ev);
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
  //
  // Left trim: SourceStart moves; ProgramStart moves by the same delta so
  //   the clip's right edge on the track stays put (intuitive — the handle
  //   under the cursor is the one moving).
  // Right trim: only SourceEnd moves; ProgramStart does not change.

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
    const origClip = Object.assign({}, original[idx]);

    function onMove(e) {
      const dx = e.clientX - startX;
      const ds = dx / ppS;
      const clips = original.map(c => Object.assign({}, c));
      const c = clips[idx];
      if (side === "left") {
        const newStart = Math.max(0, Math.min(origClip.sourceEnd - 0.05, origClip.sourceStart + ds));
        const delta = newStart - origClip.sourceStart;
        c.sourceStart = newStart;
        c.programStart = Math.max(0, origClip.programStart + delta);
      } else {
        const maxEnd = (project.source && project.source.duration) ? project.source.duration : origClip.sourceEnd + 600;
        const newEnd = Math.max(origClip.sourceStart + 0.05, Math.min(maxEnd, origClip.sourceEnd + ds));
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

  // ---- Position drag (free placement + magnetic snap) -------------------
  //
  // Gaps are first-class now: a clip can be dragged to any ProgramStart >= 0,
  // including positions that leave a hole before it or between it and its
  // neighbours. As the drag crosses within SNAP_PX of another clip's edge
  // (or time 0 / the playhead), it snaps to that point so the user can
  // butt clips up against each other without sub-pixel fiddling.

  const SNAP_PX = 8;

  function startReorderDrag(ev, trackId, clipId) {
    ev.preventDefault();
    const state = EditorStore.get();
    const project = state.project;
    const clipsKey = trackClipsKey(trackId);
    const original = (project[clipsKey] || []).map(c => Object.assign({}, c));
    const idx = original.findIndex(c => c.id === clipId);
    if (idx < 0) return;
    const ppS = state.pxPerSecond;
    const startX = ev.clientX;
    const origProgramStart = original[idx].programStart;
    const clipDur = TL.clipDur(original[idx]);

    // Snap anchors = all OTHER clip edges on the same track + playhead + 0.
    // We exclude the dragged clip's own edges so it never magnetises to
    // its own start position.
    const snapPoints = [0, state.playhead];
    original.forEach((c, i) => {
      if (i === idx) return;
      snapPoints.push(c.programStart);
      snapPoints.push(c.programStart + TL.clipDur(c));
    });

    function snapToNearest(candidateStart) {
      const candidateEnd = candidateStart + clipDur;
      const snapSec = SNAP_PX / ppS;
      let bestDelta = Infinity;
      let bestStart = candidateStart;
      for (const p of snapPoints) {
        // Try aligning either the left or the right edge of the clip to
        // each anchor. Whichever is closest wins.
        const dL = Math.abs(candidateStart - p);
        if (dL < bestDelta && dL <= snapSec) { bestDelta = dL; bestStart = p; }
        const dR = Math.abs(candidateEnd - p);
        if (dR < bestDelta && dR <= snapSec) { bestDelta = dR; bestStart = p - clipDur; }
      }
      return Math.max(0, bestStart);
    }

    function onMove(e) {
      const dx = e.clientX - startX;
      const raw = Math.max(0, origProgramStart + dx / ppS);
      const snapped = snapToNearest(raw);
      const clips = original.map(c => Object.assign({}, c));
      clips[idx].programStart = snapped;
      EditorStore.commit({ [clipsKey]: clips }, { save: false });
    }
    function onUp() {
      document.removeEventListener("mousemove", onMove);
      document.removeEventListener("mouseup", onUp);
      // Only snapshot if the clip actually moved — a plain click shouldn't
      // pollute the undo stack.
      const finalClip = (EditorStore.get().project[clipsKey] || []).find(c => c.id === clipId);
      if (finalClip && Math.abs(finalClip.programStart - origProgramStart) > 1e-6) {
        History.push(EditorStore.get().project);
      }
      EditorStore.commit({}, { save: true });
    }
    document.addEventListener("mousemove", onMove);
    document.addEventListener("mouseup", onUp);
  }

  function trackClipsKey(trackId) {
    return trackId === TRACK_VIDEO ? "videoClips" : "audioClips";
  }

  return { init, render, applyFit };
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
    if (!pos) return null; // split point in a gap → no-op
    const clip = clips[pos.i];
    if (pos.src - clip.sourceStart < 0.05 || clip.sourceEnd - pos.src < 0.05) return null;
    const next = clips.slice();
    const leftDur = pos.src - clip.sourceStart;
    // Left inherits the original ProgramStart; right starts where left ends.
    const left  = Object.assign({}, clip, { sourceEnd: pos.src });
    const right = Object.assign({}, clip, {
      id: TL.genClipId(trackId),
      sourceStart: pos.src,
      programStart: clip.programStart + leftDur,
    });
    next.splice(pos.i, 1, left, right);
    return { [key]: next };
  }

  // splitAtPlayhead now also serves the "range select" case: if a range
  // selection exists, split each in-scope track at both range edges (two
  // cuts). Otherwise fall back to the original single-cut at playhead.
  function splitAtPlayhead() {
    const st = EditorStore.get();
    if (!st.project) return;
    const r = st.rangeSelection;
    const cuts = (r && (r.end - r.start) > 0.05) ? [r.start, r.end] : [st.playhead];
    const tracks = [];
    if (st.splitScope === "both" || st.splitScope === TRACK_VIDEO) tracks.push(TRACK_VIDEO);
    if (st.splitScope === "both" || st.splitScope === TRACK_AUDIO) tracks.push(TRACK_AUDIO);

    let cur = st.project;
    let changed = false;
    for (const trackId of tracks) {
      // Splitting the same track at two times in a row needs the second
      // call to see the updated clip array, otherwise the second cut
      // targets the pre-split clip and lands in the wrong place.
      for (const t of cuts) {
        const p = splitTrack(cur, trackId, t);
        if (p) { cur = Object.assign({}, cur, p); changed = true; }
      }
    }
    if (!changed) return;
    const patch = {};
    if (cur.videoClips !== st.project.videoClips) patch.videoClips = cur.videoClips;
    if (cur.audioClips !== st.project.audioClips) patch.audioClips = cur.audioClips;
    EditorStore.commit(patch);
    if (r) EditorStore.set({ rangeSelection: null });
    History.push(EditorStore.get().project);
  }

  // Trim a track's clips so [rangeStart, rangeEnd] becomes empty space.
  // Returns a new clips array; gaps are first-class so we never reflow.
  //   - clip wholly outside        → kept as-is
  //   - clip wholly inside         → dropped
  //   - clip spans the whole range → split into two (left + right shoulder)
  //   - clip overlaps left edge    → trimmed on the right
  //   - clip overlaps right edge   → trimmed on the left
  function carveRange(clips, trackId, rangeStart, rangeEnd) {
    const out = [];
    for (const c of clips) {
      const ps = c.programStart;
      const pe = c.programStart + (c.sourceEnd - c.sourceStart);
      if (pe <= rangeStart + 1e-6 || ps >= rangeEnd - 1e-6) { out.push(c); continue; }
      if (ps >= rangeStart - 1e-6 && pe <= rangeEnd + 1e-6) { continue; }
      if (ps < rangeStart && pe > rangeEnd) {
        const leftDur = rangeStart - ps;
        out.push(Object.assign({}, c, { sourceEnd: c.sourceStart + leftDur }));
        out.push(Object.assign({}, c, {
          id: TL.genClipId(trackId),
          sourceStart: c.sourceStart + (rangeEnd - ps),
          programStart: rangeEnd,
        }));
        continue;
      }
      if (ps < rangeStart) {
        out.push(Object.assign({}, c, { sourceEnd: c.sourceStart + (rangeStart - ps) }));
        continue;
      }
      // ps >= rangeStart, pe > rangeEnd: trim left edge of clip
      out.push(Object.assign({}, c, {
        sourceStart: c.sourceStart + (rangeEnd - ps),
        programStart: rangeEnd,
      }));
    }
    return out;
  }

  // deleteSelection covers two distinct intents now:
  //   1. range selection set → carve [start, end] out of each in-scope
  //      track, leaving a hole. Selection of clips is ignored.
  //   2. otherwise → drop the clips named in `selection` (legacy behavior).
  function deleteSelection() {
    const st = EditorStore.get();
    if (!st.project) return;
    const r = st.rangeSelection;
    if (r && (r.end - r.start) > 0.05) {
      const tracks = [];
      if (st.splitScope === "both" || st.splitScope === TRACK_VIDEO) tracks.push(TRACK_VIDEO);
      if (st.splitScope === "both" || st.splitScope === TRACK_AUDIO) tracks.push(TRACK_AUDIO);
      const patch = {};
      for (const trackId of tracks) {
        const key = trackId === TRACK_VIDEO ? "videoClips" : "audioClips";
        const clips = st.project[key] || [];
        const next = carveRange(clips, trackId, r.start, r.end);
        if (next.length !== clips.length || next.some((c, i) => c !== clips[i])) {
          patch[key] = next;
        }
      }
      if (!Object.keys(patch).length) {
        // Nothing actually changed — still clear the range so the UI
        // doesn't keep dangling a stale highlight.
        EditorStore.set({ rangeSelection: null });
        return;
      }
      EditorStore.commit(patch);
      EditorStore.set({ rangeSelection: null, selection: [] });
      History.push(EditorStore.get().project);
      return;
    }
    if (!st.selection.length) return;
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
    // Backdrop click no longer closes the modal — too easy to dismiss
    // by accident while reaching for a button. The × close button and
    // Esc key are the explicit dismissal paths.
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
      progressWrap:    $("edProgressWrap"),
      progressFill:    $("edProgressFill"),
      progressText:    $("edProgressText"),
      cancelUrl:       "/api/editor/export/cancel",
      runningLabel:    "导出中...",
      doneLabel:       "✓ 导出完成",
      errorLabel:      "✗ 导出失败",
      cancelledLabel:  "! 导出已取消",
    });

    $("edExportClose").addEventListener("click", close);
    $("edExportCancel").addEventListener("click", close);
    // Backdrop click no longer closes the export config dialog — too
    // easy to lose carefully-tuned export settings by an off-target
    // click. × close, "取消" button, and Esc are the explicit ways out.

    // Close button on the right-hand log sidebar. If a job is still
    // running, closing the panel implicitly cancels it — leaving an
    // orphan ffmpeg process running in the background while its only UI
    // surface is gone would be confusing. Confirm first so a misclick
    // doesn't throw away a long export. After cancel, terminal JobBus
    // events (`cancelled`) clear .exporting; we still hide the sidebar
    // and shrink main here so the close feels immediate either way.
    $("edExportClosePanel").addEventListener("click", async () => {
      const running = !$("edExportCancelBtn").disabled;
      if (running) {
        if (!confirm("导出仍在进行中，关闭面板将取消导出。确认关闭？")) return;
        try { await Http.postJSON("/api/editor/export/cancel", {}); } catch {}
      }
      statusEl.classList.add("hidden");
      document.body.classList.remove("editor-export-active");
      setExporting(false);
    });

    // Track the export lifecycle so we can block / unblock the editor.
    // The blocker is a CSS overlay tied to .exporting on the workspace —
    // we set it the moment we issue the start request and clear it on
    // any terminal event so the user never gets stuck.
    if (typeof JobBus !== "undefined" && JobBus.subscribe) {
      JobBus.subscribe((ev) => {
        if (statusEl.classList.contains("hidden")) return;
        if (ev.type === "done" || ev.type === "error" || ev.type === "cancelled") {
          setExporting(false);
        }
      });
    }

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
      const vClips = (st.project.videoClips || []);
      const aClips = (st.project.audioClips || []);
      if (!vClips.length && !aClips.length) { alert("时间轴为空，无法导出"); return; }
      // Leading-gap guard: only the video track must start at 0 (a
      // black-screen prefix is almost always a mistake). Audio is free
      // to start late — pre-roll silence before the first dialogue is a
      // legitimate use case. Backend mirrors this rule.
      if (vClips.length) {
        const t = vClips.reduce((m, c) => Math.min(m, c.programStart), Infinity);
        if (t > 0.001) {
          alert("视频轨道开头必须有内容：第一个 clip 从 " + t.toFixed(2) + "s 开始。\n请把它拖到 0 秒再导出。");
          return;
        }
      }
      await EditorStore.flushSave();
      close();
      statusEl.classList.remove("hidden");
      // Widen <main> so the editor doesn't shrink — see CSS rule on
      // body.editor-export-active. Stays on after the job finishes
      // (sidebar still showing the success log) until the user clicks
      // the sidebar's × close button.
      document.body.classList.add("editor-export-active");
      setExporting(true);
      await panel.start({
        url: "/api/editor/export",
        body,
        outputPath: Path.join(body.export.outputDir, body.export.outputName + "." + body.export.format),
        // Editor knows the program duration exactly — pass it so the
        // progress bar starts updating from the very first time= line
        // rather than waiting for ffmpeg's "Duration:" startup print.
        totalDurationSec: TL.totalDuration(st.project),
      });
      // panel.start swallows POST failures into a finish-bar error and
      // returns without firing JobBus events, so the lifecycle subscriber
      // would never run for that branch. setRunning(true) fires only on
      // successful start; check it to know whether to keep the editor
      // blocked or release it now.
      if ($("edExportCancelBtn").disabled) setExporting(false);
    });
  }

  // Toggle the editor-blocking overlay. Pulled out of the click handler
  // so the JobBus subscription can clear it from any terminal state.
  function setExporting(running) {
    const content = document.querySelector("#panel-editor .editor-content");
    if (!content) return;
    content.classList.toggle("exporting", !!running);
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

// ============================================================
//  BrowserCompat：detect codecs the browser can't preview, so the user
//  isn't surprised by a silent or broken preview. Export is unaffected —
//  ffmpeg handles everything regardless.
//
//  Strategy: ALLOWLIST (whitelist) over blocklist. Browsers ship a small,
//  stable set of codecs; everything else is suspect. This may produce a
//  false positive on the rare codec a particular browser does support, but
//  the alert is non-fatal and tells the user how to proceed, so being a
//  little over-eager is the right tradeoff.
// ============================================================

const BrowserCompat = (() => {
  // Video codecs every modern Chromium-based browser plays out of the box.
  // HEVC is intentionally NOT here — its support varies by OS+GPU; we probe
  // for it at runtime via canPlayType.
  const VIDEO_ALLOW = new Set([
    "h264", "avc1", "vp8", "vp9", "av1", "av01",
  ]);
  const AUDIO_ALLOW = new Set([
    "aac", "mp3", "opus", "vorbis", "flac",
    // Linear PCM in WAV / MP4 — most variants are fine.
    "pcm_s16le", "pcm_s16be", "pcm_s24le", "pcm_s24be",
    "pcm_f32le", "pcm_f32be", "pcm_u8",
  ]);

  // Pretty name for each known-bad codec. Falls back to the raw name when
  // we have no friendly label.
  const PRETTY = {
    ac3:        "AC-3 / Dolby Digital",
    eac3:       "E-AC-3 / Dolby Digital Plus",
    dts:        "DTS",
    dtshd:      "DTS-HD",
    truehd:     "Dolby TrueHD",
    mlp:        "MLP",
    mpeg2video: "MPEG-2 视频",
    mpeg4:      "MPEG-4 Part 2 (Xvid / DivX)",
    wmv3:       "WMV9",
    vc1:        "VC-1",
    hevc:       "HEVC / H.265",
    h265:       "HEVC / H.265",
    prores:     "Apple ProRes",
    cinepak:    "Cinepak",
    rv40:       "RealVideo 4",
  };

  function hevcSupported() {
    const v = document.createElement("video");
    const a = v.canPlayType('video/mp4; codecs="hev1.1.6.L93.B0"');
    const b = v.canPlayType('video/mp4; codecs="hvc1.1.6.L93.B0"');
    return !!(a || b);
  }

  function isVideoSupported(codec) {
    if (!codec) return true; // unknown — don't false-positive
    if (VIDEO_ALLOW.has(codec)) return true;
    if (codec === "hevc" || codec === "h265") return hevcSupported();
    return false;
  }
  function isAudioSupported(codec) {
    if (!codec) return true;
    return AUDIO_ALLOW.has(codec);
  }

  function check(project) {
    const issues = [];
    const src = (project && project.source) || {};
    const vc = (src.videoCodec || "").toLowerCase();
    const ac = (src.audioCodec || "").toLowerCase();
    if (vc && !isVideoSupported(vc)) {
      issues.push({ kind: "video", codec: vc, label: PRETTY[vc] || vc });
    }
    if (src.hasAudio && ac && !isAudioSupported(ac)) {
      issues.push({ kind: "audio", codec: ac, label: PRETTY[ac] || ac });
    }
    return issues;
  }

  // Show a confirm dialog. "去转码" jumps to the convert tab and prefills
  // the input path; "继续编辑" lets the user proceed knowing preview will
  // be degraded.
  function alertIfIncompatible(project) {
    const issues = check(project);
    if (!issues.length) return;
    const lines = ["⚠ 当前浏览器无法预览此视频："];
    for (const i of issues) {
      const tag = i.kind === "video" ? "视频" : "音频";
      const effect = i.kind === "video" ? "画面无法显示" : "听不到声音";
      lines.push(`  • ${tag}编码 ${i.label}（${i.codec}）— ${effect}`);
    }
    lines.push("");
    lines.push("建议先用「视频转换」把它转成 H.264 + AAC 再剪辑，预览体验会好很多。");
    lines.push("（导出本身不受影响，ffmpeg 能正常处理；这只是预览的问题）");
    lines.push("");
    lines.push("点「确定」跳转到「视频转换」并自动填入当前文件；点「取消」继续在此剪辑。");
    if (confirm(lines.join("\n"))) {
      jumpToConvert(project);
    }
  }

  function jumpToConvert(project) {
    const src = (project && project.source) || {};
    if (typeof Tabs !== "undefined" && Tabs.switchTo) {
      Tabs.switchTo("convert");
    }
    // Prefill the convert tab's input path so the user just has to press
    // 转码. Suggest an output name based on the source name to nudge them
    // away from overwriting the original.
    const ip = document.getElementById("inputPath");
    if (ip && src.path) {
      ip.value = src.path;
      ip.dispatchEvent(new Event("input", { bubbles: true }));
    }
    const onName = document.getElementById("outputName");
    if (onName && (project.name || src.path)) {
      const base = project.name || (src.path.split(/[\\/]/).pop() || "").replace(/\.[^.]+$/, "");
      if (base && !onName.value) {
        onName.value = base + "_h264";
        onName.dispatchEvent(new Event("input", { bubbles: true }));
      }
    }
  }

  return { check, alertIfIncompatible };
})();

const EditorTab = (() => {
  function init() {
    const refs = {
      tabBtn:        document.querySelector('[data-tab="editor"]'),
      empty:         $("edEmpty"),
      workspace:     $("edWorkspace"),
      video:         $("edVideo"),
      audio:         $("edAudio"),
      ruler:         $("edRuler"),
      scroll:        $("edTimelineScroll"),
      videoTrack:    $("edVideoTrack"),
      audioTrack:    $("edAudioTrack"),
      playheadBig:   $("edPlayheadBig"),
      playheadVideo: $("edPlayheadVideo"),
      playheadAudio: $("edPlayheadAudio"),
      rangeSel:      $("edRangeSel"),
      playPause:     $("edPlayPause"),
      prevClip:      $("edPrevClip"),
      nextClip:      $("edNextClip"),
      timecode:      $("edTimecode"),
      audioVolume:        $("edAudioVolume"),
      audioVolumePct:     $("edAudioVolumePct"),
      audioVolumeBtn:     $("edAudioVolumeBtn"),
      audioVolumePopover: $("edAudioVolumePopover"),
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

    // Right-click is repurposed (range select on the ruler) — kill the
    // browser's default context menu anywhere inside the editor panel so a
    // stray right-click doesn't pop the OS menu over our timeline.
    const panel = document.getElementById("panel-editor");
    if (panel) panel.addEventListener("contextmenu", (e) => e.preventDefault());

    Preview.init(refs.video, refs.audio);
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
    // Audio track volume — chevron button on the audio label opens a
    // small fixed-position popover with a vertical slider (0–200%).
    // Drag → commit project.audioVolume → preview gain & export filter
    // both pick up the new value. Outside-click / Esc / second click
    // closes the popover.
    refs.audioVolume.addEventListener("input", () => {
      if (!EditorStore.get().project) return;
      const v = parseFloat(refs.audioVolume.value);
      EditorStore.commit({ audioVolume: v });
      Preview.applyAudioVolume();
    });
    refs.audioVolumeBtn.addEventListener("click", (e) => {
      e.stopPropagation();
      toggleAudioVolumePopover(refs);
    });
    refs.audioVolumePopover.addEventListener("mousedown", (e) => e.stopPropagation());
    document.addEventListener("mousedown", () => {
      if (!refs.audioVolumePopover.classList.contains("hidden")) {
        closeAudioVolumePopover(refs);
      }
    });

    // Toolbar
    refs.splitBtn.addEventListener("click", TimelineOps.splitAtPlayhead);
    refs.deleteBtn.addEventListener("click", TimelineOps.deleteSelection);
    refs.undoBtn.addEventListener("click", TimelineOps.undo);
    refs.redoBtn.addEventListener("click", TimelineOps.redo);
    refs.zoom.addEventListener("input", () => EditorStore.set({ pxPerSecond: parseFloat(refs.zoom.value) }));

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
        case "Escape":
          if (!refs.audioVolumePopover.classList.contains("hidden")) {
            closeAudioVolumePopover(refs);
          } else if (EditorStore.get().rangeSelection) {
            EditorStore.set({ rangeSelection: null });
          }
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

  // Position the volume popover under the chevron button. position:fixed
  // means we can ignore .editor-timeline's overflow:hidden — the popover
  // floats at the viewport level. We anchor to the button's bottom-left,
  // then nudge inward if the popover would clip off the right edge of
  // the viewport (cheap fallback; the popover is small enough that this
  // is rarely needed in practice).
  function toggleAudioVolumePopover(refs) {
    if (refs.audioVolumePopover.classList.contains("hidden")) {
      openAudioVolumePopover(refs);
    } else {
      closeAudioVolumePopover(refs);
    }
  }
  function openAudioVolumePopover(refs) {
    const btnRect = refs.audioVolumeBtn.getBoundingClientRect();
    refs.audioVolumePopover.classList.remove("hidden");
    const popRect = refs.audioVolumePopover.getBoundingClientRect();
    // Horizontal: anchor under the button; if it would clip off the
    // right edge, slide left until it fits.
    let left = btnRect.left;
    if (left + popRect.width > window.innerWidth - 8) {
      left = window.innerWidth - popRect.width - 8;
    }
    if (left < 8) left = 8;
    // Vertical: prefer below the button, but the audio-volume button
    // sits near the bottom of the editor (timeline is the bottommost
    // section), so "below" almost always clips. Flip above whenever
    // below doesn't fit, with the same below-button gap.
    const gap = 6;
    const fitsBelow = btnRect.bottom + gap + popRect.height <= window.innerHeight - 8;
    const top = fitsBelow
      ? btnRect.bottom + gap
      : Math.max(8, btnRect.top - popRect.height - gap);
    refs.audioVolumePopover.style.left = left + "px";
    refs.audioVolumePopover.style.top  = top + "px";
    refs.audioVolumeBtn.classList.add("is-open");
    refs.audioVolumeBtn.setAttribute("aria-expanded", "true");
  }
  function closeAudioVolumePopover(refs) {
    refs.audioVolumePopover.classList.add("hidden");
    refs.audioVolumeBtn.classList.remove("is-open");
    refs.audioVolumeBtn.setAttribute("aria-expanded", "false");
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
    EditorStore.set({ project, selection: [], playhead: 0, playing: false, dirty: false, splitScope: "both", rangeSelection: null });
    History.reset(project);
    Preview.loadProject(project);
    // Fit-to-width must run after the workspace is visible, otherwise
    // clientWidth reads as 0 and we'd pick the min zoom.
    requestAnimationFrame(() => Timeline.applyFit(project));
    // Defer compat alert so the workspace renders first — otherwise the
    // user sees a blank screen behind the modal which is mildly disorienting.
    setTimeout(() => BrowserCompat.alertIfIncompatible(project), 100);
  }

  function render(state) {
    const refs = collectRefs();
    const hasProject = !!state.project;
    refs.empty.classList.toggle("hidden", hasProject);
    refs.workspace.classList.toggle("hidden", !hasProject);
    const total = state.project ? TL.totalDuration(state.project) : 0;
    refs.exportBtn.disabled = !hasProject || total <= 0;
    refs.deleteBtn.disabled = !state.selection.length && !state.rangeSelection;
    refs.undoBtn.disabled = !History.canUndo();
    refs.redoBtn.disabled = !History.canRedo();
    refs.playPause.textContent = state.playing ? "⏸" : "▶";
    if (refs.scopeLabel) refs.scopeLabel.textContent = scopeLabelText(state.splitScope);
    if (!hasProject) return;
    refs.projectName.value = state.project.name || "";
    refs.projectName.classList.toggle("dirty", state.dirty);
    refs.timecode.textContent = `${Time.format(state.playhead)} / ${Time.format(total)}`;
    // Audio volume slider mirrors project.audioVolume; default to unity
    // when missing (very old / pre-feature projects). Skip writing back
    // while the user is dragging — would yank the slider mid-gesture.
    {
      const v = state.project.audioVolume != null ? state.project.audioVolume : 1;
      const pct = Math.round(v * 100) + "%";
      // Skip writing the slider value while the user is dragging it;
      // otherwise the assignment would yank the thumb out from under
      // the cursor mid-gesture.
      if (refs.audioVolume && document.activeElement !== refs.audioVolume) {
        refs.audioVolume.value = String(v);
      }
      if (refs.audioVolumePct) refs.audioVolumePct.textContent = pct;
      // Button doubles as a live readout. Prefix "音量:" so the number
      // reads as audio volume, not some random percentage on the track
      // (without it users had to guess what the digits meant).
      if (refs.audioVolumeBtn) refs.audioVolumeBtn.textContent = "音量: " + pct;
    }
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
      empty:              $("edEmpty"),
      workspace:          $("edWorkspace"),
      exportBtn:          $("edExport"),
      deleteBtn:          $("edDelete"),
      undoBtn:            $("edUndo"),
      redoBtn:            $("edRedo"),
      playPause:          $("edPlayPause"),
      projectName:        $("edProjectName"),
      timecode:           $("edTimecode"),
      scopeLabel:         $("edSplitScopeLabel"),
      // Volume controls — earlier omitted from collectRefs, which made
      // the per-render audioVolumePct sync silently no-op (the popover
      // readout stayed at 100% no matter the slider position).
      audioVolume:        $("edAudioVolume"),
      audioVolumePct:     $("edAudioVolumePct"),
      audioVolumeBtn:     $("edAudioVolumeBtn"),
      audioVolumePopover: $("edAudioVolumePopover"),
    };
  }

  return { init };
})();
