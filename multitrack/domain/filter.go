package domain

import (
	"fmt"
	"sort"
	"strings"

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

// BuildMultitrackVideoTrackFilter is the multi-source twin of the shared
// editor/common.BuildVideoTrackFilter. The clip slice can mix slices of
// different sources, so each segment names its own input pad rather than a
// single srcLabel; gaps and clips are scale+pad+setsar normalised to the
// canvas dimensions before concat so concat sees homogeneous sizes —
// without that, mixing 1080p and 720p sources fails the concat filter
// outright.
//
// Args:
//   - clips: the track's clips (need not be sorted).
//   - sourceInputIdx: maps clip.SourceID → ffmpeg -i input index. Caller
//     is expected to have populated entries for every source referenced
//     by clips; an unknown id returns an error.
//   - sourceDims: per-source dimensions. Only used to know whether a clip
//     even *has* dimensions (zero-value sources fail validation upstream),
//     not to emit per-source scaling — the scale step always normalises
//     to canvasW/H regardless of the source's natural size.
//   - outLabel: terminal pad, e.g. "[V0]" or "[V]" (when the track is the
//     only one and overlay is skipped).
//   - totalDur: program duration; if greater than the track's length a
//     trailing black pad fills the difference.
//   - canvasW / canvasH / canvasFr: output canvas (max W × max H × max FR
//     across video sources).
//   - labelPrefix: prefixes each intermediate label so multiple tracks'
//     filter chains don't collide (e.g. "v0_", "v1_").
//
// Returns the filter parts; callers join them with ";" when assembling
// -filter_complex. Returns an error if a clip references a missing source.
func BuildMultitrackVideoTrackFilter(
	clips []Clip,
	sourceInputIdx map[string]int,
	outLabel string,
	totalDur float64,
	canvasW, canvasH int,
	canvasFr float64,
	labelPrefix string,
) ([]string, error) {
	if canvasW <= 0 {
		canvasW = 1920
	}
	if canvasH <= 0 {
		canvasH = 1080
	}
	if canvasFr <= 0 {
		canvasFr = 30
	}
	// Sort by ProgramStart so the concat order matches the timeline. Clips
	// arrive in arbitrary order from drag/drop, and the planner-style sort
	// is what makes "out of order" inputs render the same way as sorted ones.
	sorted := append([]Clip(nil), clips...)
	sort.SliceStable(sorted, func(i, j int) bool {
		return sorted[i].ProgramStart < sorted[j].ProgramStart
	})

	scalePadFmt := fmt.Sprintf(
		"scale=%d:%d:force_original_aspect_ratio=decrease,pad=%d:%d:(ow-iw)/2:(oh-ih)/2:black,setsar=1,format=yuv420p",
		canvasW, canvasH, canvasW, canvasH,
	)

	var parts []string
	var refs []string
	var cursor float64
	segIdx := 0
	emitGap := func(dur float64) {
		label := fmt.Sprintf("[%sv%d]", labelPrefix, segIdx)
		// color already produces canvas-sized frames at canvasFr — no scale/pad
		// needed, just format=yuv420p so the pixel format matches clip segments.
		parts = append(parts, fmt.Sprintf(
			"color=c=black:s=%dx%d:r=%s:d=%s,format=yuv420p%s",
			canvasW, canvasH, common.FormatFloat(canvasFr), common.FormatFloat(dur), label,
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
		label := fmt.Sprintf("[%sv%d]", labelPrefix, segIdx)
		// trim + setpts rebases clip timestamps to zero; scalePadFmt unifies
		// to canvas dims so concat across heterogeneous sources is legal.
		parts = append(parts, fmt.Sprintf(
			"[%d:v]trim=start=%s:end=%s,setpts=PTS-STARTPTS,%s%s",
			idx,
			common.FormatFloat(c.SourceStart), common.FormatFloat(c.SourceEnd),
			scalePadFmt, label,
		))
		refs = append(refs, label)
		segIdx++
		cursor = c.ProgramStart + c.Duration()
	}
	// Trailing pad to programDur — ensures all video tracks render the same
	// length so overlay / muxer don't truncate at the shorter input.
	if totalDur > cursor+1e-3 {
		emitGap(totalDur - cursor)
	}
	parts = append(parts, fmt.Sprintf(
		"%sconcat=n=%d:v=1:a=0%s",
		strings.Join(refs, ""), len(refs), outLabel,
	))
	return parts, nil
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
	sort.SliceStable(sorted, func(i, j int) bool {
		return sorted[i].ProgramStart < sorted[j].ProgramStart
	})

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
		strings.Join(refs, ""), len(refs), concatOut,
	))
	if useVolume {
		parts = append(parts, fmt.Sprintf("%svolume=%s%s", preLabel, common.FormatFloat(volume), outLabel))
	}
	return parts, nil
}
