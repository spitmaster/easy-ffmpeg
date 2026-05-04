package domain

import (
	"fmt"

	common "easy-ffmpeg/editor/common/domain"
)

// SourceDims describes the resolution / frame-rate of one video source as
// surfaced by ffprobe at import time. Audio-only sources don't carry these
// and are never asked for here — the filter builder is video-track only.
type SourceDims struct {
	W  int
	H  int
	Fr float64
}

// BuildVideoSegment emits the per-clip filter chain for a single video clip
// in the v0.5.1 true-composition export path. Each clip becomes one segment
// pad ([seg_k]) that is later overlaid onto a base canvas.
//
// The chain shape is:
//
//	[i:v]trim=start=S:end=E,setpts=PTS-STARTPTS+programStart/TB,
//	     scale=W:H,setsar=1,fps=FR,format=yuva420p[label]
//
// Why each step matters:
//   - trim selects the source sub-range
//   - setpts shifts the segment's PTS zero to programStart, so overlay's
//     time-based gating + auto-positioning lands the segment in the right
//     window (using only PTS-STARTPTS would stack every segment at t=0)
//   - scale resizes the source frame to the clip's transform.W × transform.H
//     (no pad — overlay positions us; transparent edges come from yuva420p)
//   - setsar=1 keeps a square sample aspect ratio so downstream filters
//     don't accidentally double-scale via SAR
//   - fps=canvasFR normalises the segment to the canvas frame rate so the
//     overlay step doesn't produce duplicated/dropped frames at boundaries
//   - format=yuva420p adds an alpha plane: outside the segment's pixel
//     extent the segment is transparent, letting underlying layers show
//     through. Final encode strips alpha (mp4 doesn't carry it), but the
//     intermediate composite is alpha-aware.
//
// Args:
//   - c: the clip; reads SourceID, SourceStart, SourceEnd, ProgramStart, Transform.
//   - inputIdx: ffmpeg -i index for c.SourceID; the caller resolves the map.
//   - canvasFr: project canvas frame rate; segments are forced to this rate.
//   - label: terminal pad name without brackets (e.g. "seg_0"); the function
//     wraps it with `[ ]` so callers can stay symmetrical with overlay refs.
//
// Returns the filter chain as a single string; callers join multiple
// segments with ";". Returns an error if Transform is invalid (W or H <= 0
// would otherwise produce a malformed scale filter that ffmpeg rejects late).
func BuildVideoSegment(c Clip, inputIdx int, canvasFr float64, label string) (string, error) {
	if c.Transform.W <= 0 || c.Transform.H <= 0 {
		return "", fmt.Errorf("clip %q: transform W/H must be > 0 (got %dx%d)", c.ID, c.Transform.W, c.Transform.H)
	}
	if canvasFr <= 0 {
		canvasFr = 30
	}
	return fmt.Sprintf(
		"[%d:v]trim=start=%s:end=%s,setpts=PTS-STARTPTS+%s/TB,scale=%d:%d,setsar=1,fps=%s,format=yuva420p[%s]",
		inputIdx,
		common.FormatFloat(c.SourceStart), common.FormatFloat(c.SourceEnd),
		common.FormatFloat(c.ProgramStart),
		c.Transform.W, c.Transform.H,
		common.FormatFloat(canvasFr),
		label,
	), nil
}

// BuildMultitrackAudioTrackFilter is the multi-source audio twin. Mirrors
// common.BuildAudioTrackFilter shape but per-segment input index. Volume
// at unity is omitted (no volume filter) so byte-for-byte filter graphs
// stay stable when track volume hasn't been touched.
//
// outLabel: terminal pad, e.g. "[A0]" or "[A]" (single-track case).
// preLabel: intermediate pad used only when volume != 1.0; the chain
// concats into preLabel, then routes through volume into outLabel. When
// volume == 1.0 preLabel is unused and the concat outputs directly to
// outLabel.
func BuildMultitrackAudioTrackFilter(
	clips []Clip,
	sourceInputIdx map[string]int,
	outLabel, preLabel string,
	volume, totalDur float64,
	labelPrefix string,
) ([]string, error) {
	sorted := append([]Clip(nil), clips...)
	sortClipsByProgramStart(sorted)

	var parts []string
	var refs []string
	var cursor float64
	segIdx := 0
	emitGap := func(dur float64) {
		label := fmt.Sprintf("[%sa%d]", labelPrefix, segIdx)
		parts = append(parts, fmt.Sprintf(
			"anullsrc=r=48000:cl=stereo:d=%s,%s%s",
			common.FormatFloat(dur), common.AudioFormatExpr, label,
		))
		refs = append(refs, label)
		segIdx++
	}
	for _, c := range sorted {
		if c.ProgramStart > cursor+common.SnapEpsilon {
			emitGap(c.ProgramStart - cursor)
		}
		idx, ok := sourceInputIdx[c.SourceID]
		if !ok {
			return nil, fmt.Errorf("multitrack export: source %q not in input map", c.SourceID)
		}
		label := fmt.Sprintf("[%sa%d]", labelPrefix, segIdx)
		parts = append(parts, fmt.Sprintf(
			"[%d:a]atrim=start=%s:end=%s,asetpts=PTS-STARTPTS,%s%s",
			idx,
			common.FormatFloat(c.SourceStart), common.FormatFloat(c.SourceEnd),
			common.AudioFormatExpr, label,
		))
		refs = append(refs, label)
		segIdx++
		cursor = c.ProgramStart + c.Duration()
	}
	if totalDur > cursor+1e-3 {
		emitGap(totalDur - cursor)
	}
	useVolume := volume > 0 && (volume < 0.999 || volume > 1.001)
	concatOut := outLabel
	if useVolume {
		concatOut = preLabel
	}
	parts = append(parts, fmt.Sprintf(
		"%sconcat=n=%d:v=0:a=1%s",
		joinRefs(refs), len(refs), concatOut,
	))
	if useVolume {
		parts = append(parts, fmt.Sprintf("%svolume=%s%s", preLabel, common.FormatFloat(volume), outLabel))
	}
	return parts, nil
}

// sortClipsByProgramStart is the in-place stable sort used by audio path.
// Pulled out so filter.go doesn't need a "sort" import twice; export.go
// has its own bigger sort step for the video z-list.
func sortClipsByProgramStart(clips []Clip) {
	for i := 1; i < len(clips); i++ {
		for j := i; j > 0 && clips[j-1].ProgramStart > clips[j].ProgramStart; j-- {
			clips[j-1], clips[j] = clips[j], clips[j-1]
		}
	}
}

// joinRefs concatenates label refs (which already include their `[]`
// brackets) without inserting a separator — what concat / amix expect.
func joinRefs(refs []string) string {
	out := ""
	for _, r := range refs {
		out += r
	}
	return out
}
