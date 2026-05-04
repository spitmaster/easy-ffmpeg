package domain

import (
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	common "easy-ffmpeg/editor/common/domain"
)

// BuildExportArgs translates a multitrack Project into a concrete ffmpeg
// argv plus the resolved output path. Pure function: no I/O, no globals.
//
// v0.5.1 video strategy (true compositing):
//   - Start a base canvas of project.Canvas dims:
//     `color=c=black:s=CWxCH:r=FR:d=programDur,format=yuv420p[base]`
//   - Collect every video clip across all tracks into one z-list, sorted by
//     (trackIndex asc, programStart asc). Lower track index = bottom layer.
//   - Each clip becomes one segment via BuildVideoSegment: trim + setpts
//     (PTS shifted to programStart) + scale to the clip's transform.W×H +
//     fps + yuva420p (alpha-aware).
//   - Flatten into a single overlay chain over the base. Every overlay is
//     gated by `enable='between(t,start,end)'` and uses `eof_action=pass`
//     so a finished segment doesn't keep painting its last frame on top.
//
// Audio path is unchanged from v0.5.0: per-track concat (filling silence
// for gaps) → optional per-track volume → amix (when ≥ 2 tracks) →
// optional global volume → [A].
//
// Strategy:
//   - Collect referenced sources (those any clip points at) in Sources order
//     and assign -i input indices. Unreferenced sources are skipped — no
//     point asking ffmpeg to demux files we never read.
//   - Map [V] / [A] only for the streams that exist; codec args are paired
//     with each map.
func BuildExportArgs(p *Project) ([]string, string, error) {
	if p == nil {
		return nil, "", errors.New("project is nil")
	}
	if err := common.ValidateExportSettings(p.Export); err != nil {
		return nil, "", err
	}

	// Gather non-empty tracks. Empty tracks are silently dropped from the
	// filter graph — they'd contribute nothing to a base+overlay composite
	// and their absence keeps the segment list tight.
	var vTracks, aTracks []trackWithIndex
	for i, t := range p.VideoTracks {
		if len(t.Clips) > 0 {
			vTracks = append(vTracks, trackWithIndex{idx: i, vClips: t.Clips})
		}
	}
	for i, t := range p.AudioTracks {
		if len(t.Clips) > 0 {
			aTracks = append(aTracks, trackWithIndex{
				idx:    i,
				aClips: t.Clips,
				volume: t.Volume,
			})
		}
	}
	hasVideo := len(vTracks) > 0
	hasAudio := len(aTracks) > 0
	if !hasVideo && !hasAudio {
		return nil, "", errors.New("project has no clips")
	}

	// Leading-gap guard, per video track. Audio is allowed to lead with
	// silence (a valid edit — pre-roll before voice). The error names the
	// track index so the UI can highlight which track is offending.
	for _, vt := range vTracks {
		if t := common.EarliestProgramStart(toCommonClips(vt.vClips)); t > common.SnapEpsilon {
			return nil, "", fmt.Errorf("视频轨道 videoTracks[%d] 开头必须有内容：第一个 clip 从 %.2fs 开始，请把它拖到 0 秒再导出", vt.idx, t)
		}
	}

	// Build the source-id → input-index map, in Sources order, only for
	// sources actually used by some clip. Unused sources are not added as
	// -i because demuxing them would burn CPU/IO for nothing.
	used := map[string]bool{}
	for _, vt := range vTracks {
		for _, c := range vt.vClips {
			used[c.SourceID] = true
		}
	}
	for _, at := range aTracks {
		for _, c := range at.aClips {
			used[c.SourceID] = true
		}
	}
	var inputs []Source
	sourceInputIdx := map[string]int{}
	for _, s := range p.Sources {
		if used[s.ID] {
			sourceInputIdx[s.ID] = len(inputs)
			inputs = append(inputs, s)
		}
	}
	for sid := range used {
		if _, ok := sourceInputIdx[sid]; !ok {
			return nil, "", fmt.Errorf("clip references unknown source %q", sid)
		}
	}

	// Canvas: prefer the project's stored Canvas (v0.5.1+). For files that
	// somehow reach export with a zero canvas (Migrate didn't run, or a
	// programmatic caller skipped it), fall back to the v0.5.0 derivation
	// so we never produce a malformed base.
	canvasW, canvasH, canvasFr := p.Canvas.Width, p.Canvas.Height, p.Canvas.FrameRate
	if hasVideo && (canvasW <= 0 || canvasH <= 0 || canvasFr <= 0) {
		w, h, fr := deriveDefaultCanvas(p)
		if canvasW <= 0 {
			canvasW = w
		}
		if canvasH <= 0 {
			canvasH = h
		}
		if canvasFr <= 0 {
			canvasFr = fr
		}
	}

	// Program duration: longest track, used to size the base canvas. The
	// base spans the whole program; segments above it are gated by their
	// own enable= windows so the composite plays for exactly programDur.
	programDur := 0.0
	for _, vt := range vTracks {
		if d := common.TrackDuration(toCommonClips(vt.vClips)); d > programDur {
			programDur = d
		}
	}
	for _, at := range aTracks {
		if d := common.TrackDuration(toCommonClips(at.aClips)); d > programDur {
			programDur = d
		}
	}

	var parts []string

	// ---- video filter graph (base + flat overlay chain) -----------------
	if hasVideo {
		// Base canvas: a single black picture at canvas dims and frame rate
		// for programDur. format=yuv420p so the muxer-bound output stays in
		// the standard non-alpha pixel format; segments above carry alpha
		// (yuva420p) so they composite cleanly.
		parts = append(parts, fmt.Sprintf(
			"color=c=black:s=%dx%d:r=%s:d=%s,format=yuv420p[base]",
			canvasW, canvasH,
			common.FormatFloat(canvasFr),
			common.FormatFloat(programDur),
		))

		// Flatten all video clips into a z-list. The pair (track index,
		// programStart) is enough to pin both layer order and a stable
		// in-track order (clips on the same track don't overlap visually,
		// so their listing order doesn't change pixel output, but stable
		// sort keeps filter strings deterministic across runs).
		type segRef struct {
			c       Clip
			trackIx int
			label   string
		}
		var segs []segRef
		for _, vt := range vTracks {
			for _, c := range vt.vClips {
				segs = append(segs, segRef{c: c, trackIx: vt.idx})
			}
		}
		sort.SliceStable(segs, func(i, j int) bool {
			if segs[i].trackIx != segs[j].trackIx {
				return segs[i].trackIx < segs[j].trackIx
			}
			return segs[i].c.ProgramStart < segs[j].c.ProgramStart
		})

		// Emit one segment chain per clip.
		for k := range segs {
			label := fmt.Sprintf("seg_%d", k)
			segs[k].label = label
			idx, ok := sourceInputIdx[segs[k].c.SourceID]
			if !ok {
				return nil, "", fmt.Errorf("multitrack export: source %q not in input map", segs[k].c.SourceID)
			}
			s, err := BuildVideoSegment(segs[k].c, idx, canvasFr, label)
			if err != nil {
				return nil, "", err
			}
			parts = append(parts, s)
		}

		// Flatten the overlay chain. base is the bottom; each segment is
		// composited in z-order. The intermediate pads `[v_k]` keep labels
		// stable across runs, and the final overlay output is `[V]`.
		cur := "[base]"
		for k := range segs {
			next := "[V]"
			if k < len(segs)-1 {
				next = fmt.Sprintf("[v_%d]", k)
			}
			pStart := segs[k].c.ProgramStart
			pEnd := segs[k].c.ProgramStart + segs[k].c.Duration()
			parts = append(parts, fmt.Sprintf(
				"%s[%s]overlay=x=%d:y=%d:enable='between(t,%s,%s)':eof_action=pass%s",
				cur, segs[k].label,
				segs[k].c.Transform.X, segs[k].c.Transform.Y,
				common.FormatFloat(pStart), common.FormatFloat(pEnd),
				next,
			))
			cur = next
		}
	}

	// ---- audio filter graph (unchanged from v0.5.0) ---------------------
	globalVol := p.AudioVolume
	if globalVol <= 0 {
		globalVol = 1.0
	}
	useGlobalVol := globalVol < 0.999 || globalVol > 1.001
	if hasAudio {
		// Per-track filter chains. Single-track case routes either directly
		// to [A] (when no global volume to apply afterwards) or to
		// [A_pre]→volume→[A]. Multi-track case always emits [A0]...[An]
		// then amixes them.
		trackOuts := make([]string, len(aTracks))
		switch {
		case len(aTracks) == 1 && !useGlobalVol:
			trackOuts[0] = "[A]"
		case len(aTracks) == 1 && useGlobalVol:
			trackOuts[0] = "[A_pre]"
		default:
			for i := range aTracks {
				trackOuts[i] = fmt.Sprintf("[A%d]", i)
			}
		}
		for i, at := range aTracks {
			vol := at.volume
			if vol <= 0 {
				vol = 1.0
			}
			// preLabel: per-track scratch pad used only when track volume
			// is non-unity. Distinct prefix per track so labels don't clash.
			built, err := BuildMultitrackAudioTrackFilter(
				at.aClips, sourceInputIdx,
				trackOuts[i],
				fmt.Sprintf("[A%d_pre]", i),
				vol, programDur,
				fmt.Sprintf("a%d_", i),
			)
			if err != nil {
				return nil, "", err
			}
			parts = append(parts, built...)
		}
		// Multi-track: amix into [A_pre] (or [A] if no global volume), then
		// optional global volume → [A].
		if len(aTracks) >= 2 {
			amixOut := "[A_pre]"
			if !useGlobalVol {
				amixOut = "[A]"
			}
			refs := strings.Join(trackOuts, "")
			parts = append(parts, fmt.Sprintf(
				"%samix=inputs=%d:duration=longest:dropout_transition=0%s",
				refs, len(aTracks), amixOut,
			))
		}
		if useGlobalVol {
			parts = append(parts, fmt.Sprintf(
				"[A_pre]volume=%s[A]", common.FormatFloat(globalVol),
			))
		}
	}

	// ---- assemble argv ----------------------------------------------------
	args := []string{"-y"}
	for _, s := range inputs {
		args = append(args, "-i", s.Path)
	}
	if len(parts) > 0 {
		args = append(args, "-filter_complex", strings.Join(parts, ";"))
	}
	if hasVideo {
		args = append(args, "-map", "[V]", "-c:v", common.NormalizeVideoCodec(p.Export.VideoCodec))
	}
	if hasAudio {
		args = append(args, "-map", "[A]", "-c:a", common.NormalizeAudioCodec(p.Export.AudioCodec))
	}
	outPath := filepath.Join(p.Export.OutputDir, p.Export.OutputName+"."+p.Export.Format)
	args = append(args, outPath)
	return args, outPath, nil
}

// deriveDefaultCanvas computes the v0.5.0 fallback canvas dims: max width,
// max height, and max frame rate across video sources actually referenced
// by some video clip. Returns sane defaults (1920×1080@30) when no video
// sources are referenced. Used by Migrate to fill v1 projects' Canvas
// field so they export visually identical to v0.5.0, and as a defensive
// fallback in BuildExportArgs in case Migrate didn't run.
func deriveDefaultCanvas(p *Project) (int, int, float64) {
	usedByVideo := map[string]bool{}
	for _, t := range p.VideoTracks {
		for _, c := range t.Clips {
			usedByVideo[c.SourceID] = true
		}
	}
	w, h := 0, 0
	fr := 0.0
	for _, s := range p.Sources {
		if !usedByVideo[s.ID] {
			continue
		}
		if s.Width > w {
			w = s.Width
		}
		if s.Height > h {
			h = s.Height
		}
		if s.FrameRate > fr {
			fr = s.FrameRate
		}
	}
	if w <= 0 {
		w = 1920
	}
	if h <= 0 {
		h = 1080
	}
	if fr <= 0 {
		fr = 30
	}
	return w, h, fr
}

// trackWithIndex carries a track's content alongside its position in the
// Project's track slice. The index is what error messages refer to so the
// UI can pinpoint which track is offending without us renumbering after
// dropping empty tracks.
type trackWithIndex struct {
	idx    int
	vClips []Clip
	aClips []Clip
	volume float64
}
