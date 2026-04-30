package domain

import (
	"fmt"
	"strconv"
	"strings"
)

// AudioFormatExpr is the aformat filter snippet appended to every audio
// segment so that real source slices and synthetic anullsrc gaps share
// sample format / rate / channel layout — concat refuses heterogeneous
// inputs otherwise.
const AudioFormatExpr = "aformat=sample_fmts=fltp:sample_rates=48000:channel_layouts=stereo"

// BuildVideoTrackFilter produces the filter graph for one video track,
// including synthetic black-frame segments for gaps and a trailing
// black pad when the track is shorter than totalDur.
//
// srcLabel is the input pad reference, e.g. "[0:v]" for the first
// input's video stream. outLabel is the chain's terminal pad, e.g.
// "[v]" or "[v0]". w / h / fr describe the gap's pixel format and
// frame rate; multitrack uses the project's canvas dimensions, single-
// video uses the source's own dimensions so the gap matches the clips
// byte-for-byte.
//
// The output is a slice of filter expressions; callers join them with
// ";" when assembling -filter_complex.
func BuildVideoTrackFilter(clips []Clip, srcLabel, outLabel string, totalDur float64, w, h int, fr float64) []string {
	if w <= 0 {
		w = 1920
	}
	if h <= 0 {
		h = 1080
	}
	if fr <= 0 {
		fr = 30
	}
	segs := PlanSegments(clips, totalDur)
	var parts []string
	var refs []string
	for i, seg := range segs {
		label := fmt.Sprintf("[v%d]", i)
		if seg.IsGap {
			// `color` generates a constant-color source for `duration`
			// seconds; format=yuv420p aligns pixel format with typical
			// source streams so concat sees homogeneous inputs.
			parts = append(parts, fmt.Sprintf(
				"color=c=black:s=%dx%d:r=%s:d=%s,format=yuv420p%s",
				w, h, FormatFloat(fr), FormatFloat(seg.Duration), label,
			))
		} else {
			// `trim` + `setpts=PTS-STARTPTS` extracts the slice and rebases
			// its timestamps to zero so concat can stack it without gaps.
			parts = append(parts, fmt.Sprintf(
				"%strim=start=%s:end=%s,setpts=PTS-STARTPTS,format=yuv420p%s",
				srcLabel, FormatFloat(seg.SourceStart), FormatFloat(seg.SourceEnd), label,
			))
		}
		refs = append(refs, label)
	}
	parts = append(parts, fmt.Sprintf(
		"%sconcat=n=%d:v=1:a=0%s",
		strings.Join(refs, ""), len(segs), outLabel,
	))
	return parts
}

// BuildAudioTrackFilter is the audio twin of BuildVideoTrackFilter.
// anullsrc fills gaps; AudioFormatExpr normalises both real and gap
// segments to 48k/stereo/fltp.
//
// srcLabel: input pad reference, e.g. "[0:a]".
// outLabel: chain terminal pad, e.g. "[a]" or "[a0]".
// preLabel: intermediate pad used only when volume != 1.0; the chain
// concats into preLabel and then routes through volume into outLabel.
// When volume == 1.0 the volume filter is omitted entirely (no rename),
// so the filter graph for unity-gain projects stays byte-for-byte
// identical to a no-volume build — important for filter-graph test
// stability across feature additions.
func BuildAudioTrackFilter(clips []Clip, srcLabel, outLabel, preLabel string, volume, totalDur float64) []string {
	segs := PlanSegments(clips, totalDur)
	var parts []string
	var refs []string
	for i, seg := range segs {
		label := fmt.Sprintf("[a%d]", i)
		if seg.IsGap {
			parts = append(parts, fmt.Sprintf(
				"anullsrc=r=48000:cl=stereo:d=%s,%s%s",
				FormatFloat(seg.Duration), AudioFormatExpr, label,
			))
		} else {
			parts = append(parts, fmt.Sprintf(
				"%satrim=start=%s:end=%s,asetpts=PTS-STARTPTS,%s%s",
				srcLabel, FormatFloat(seg.SourceStart), FormatFloat(seg.SourceEnd), AudioFormatExpr, label,
			))
		}
		refs = append(refs, label)
	}
	useVolume := volume > 0 && (volume < 0.999 || volume > 1.001)
	concatOut := outLabel
	if useVolume {
		concatOut = preLabel
	}
	parts = append(parts, fmt.Sprintf(
		"%sconcat=n=%d:v=0:a=1%s",
		strings.Join(refs, ""), len(segs), concatOut,
	))
	if useVolume {
		parts = append(parts, fmt.Sprintf("%svolume=%s%s", preLabel, FormatFloat(volume), outLabel))
	}
	return parts
}

// FormatFloat writes a seconds value without scientific notation and
// with up to 6 decimals, trimmed. ffmpeg accepts both "12" and "12.345".
func FormatFloat(v float64) string {
	s := strconv.FormatFloat(v, 'f', 6, 64)
	s = strings.TrimRight(s, "0")
	s = strings.TrimRight(s, ".")
	if s == "" || s == "-" {
		s = "0"
	}
	return s
}
