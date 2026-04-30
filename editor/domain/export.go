package domain

import (
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// BuildExportArgs translates a Project into a concrete ffmpeg argv plus
// the resolved output path. Pure function: no I/O, no globals.
//
// Strategy:
//   * For each track, sort clips by ProgramStart, then build an alternating
//     sequence of "real" segments (trim+setpts from the source) and "gap"
//     segments (lavfi color/anullsrc) so the output starts at time 0 and has
//     no missing seconds.
//   * Gap video uses the source's width/height/frame-rate to match format.
//     Gap audio uses fixed 48000/stereo; source audio is aformat'd to match
//     so concat sees homogeneous streams.
//   * Video track and audio track are independent — tracks of different
//     length are muxed together; ffmpeg picks the shortest at container level.
func BuildExportArgs(p *Project) ([]string, string, error) {
	if p == nil {
		return nil, "", errors.New("project is nil")
	}
	hasVideo := len(p.VideoClips) > 0
	hasAudio := len(p.AudioClips) > 0 && p.Source.HasAudio
	if !hasVideo && !hasAudio {
		return nil, "", errors.New("project has no clips")
	}
	if p.Source.Path == "" {
		return nil, "", errors.New("source path is empty")
	}
	if p.Export.OutputDir == "" || p.Export.OutputName == "" || p.Export.Format == "" {
		return nil, "", errors.New("export: outputDir / outputName / format required")
	}
	// Leading-gap guard: only the **video** track must start at program
	// time 0 — a black-screen prefix on the exported file is almost always
	// a mistake. The audio track is allowed a leading gap (filled with
	// silence); legitimate use case: pre-roll silence before the first
	// spoken word. Mid-track gaps remain allowed on both tracks.
	if hasVideo {
		if t := earliestProgramStart(p.VideoClips); t > 1e-3 {
			return nil, "", fmt.Errorf("视频轨道开头必须有内容：第一个 clip 从 %.2fs 开始，请把它拖到 0 秒再导出", t)
		}
	}
	videoCodec := normalizeVideoCodec(p.Export.VideoCodec)
	audioCodec := normalizeAudioCodec(p.Export.AudioCodec)
	outPath := filepath.Join(p.Export.OutputDir, p.Export.OutputName+"."+p.Export.Format)

	// Both tracks are padded to programDur with synthetic black / silence
	// when shorter, so the output mp4's two streams have matching length.
	// Without this the muxer writes a 5s video + 10s audio into one file
	// and Chrome's <video> element (the Editor preview, and most native
	// players) cuts off at the shorter stream — ending playback at video
	// EOF even though the audio still has data.
	programDur := 0.0
	if v := trackDuration(p.VideoClips); v > programDur {
		programDur = v
	}
	if a := trackDuration(p.AudioClips); a > programDur {
		programDur = a
	}
	var parts []string
	if hasVideo {
		parts = append(parts, buildVideoTrackFilter(p.VideoClips, p.Source, programDur)...)
	}
	if hasAudio {
		parts = append(parts, buildAudioTrackFilter(p.AudioClips, p.AudioVolume, programDur)...)
	}
	filter := strings.Join(parts, ";")

	args := []string{"-y", "-i", p.Source.Path, "-filter_complex", filter}
	if hasVideo {
		args = append(args, "-map", "[v]", "-c:v", videoCodec)
	}
	if hasAudio {
		args = append(args, "-map", "[a]", "-c:a", audioCodec)
	}
	args = append(args, outPath)
	return args, outPath, nil
}

// earliestProgramStart returns the smallest ProgramStart in the slice, or 0
// for an empty track. Used by the export-time leading-gap guard.
func earliestProgramStart(clips []Clip) float64 {
	if len(clips) == 0 {
		return 0
	}
	min := clips[0].ProgramStart
	for _, c := range clips[1:] {
		if c.ProgramStart < min {
			min = c.ProgramStart
		}
	}
	return min
}

// segmentPlan describes one segment on a track — either a real cut of the
// source ("clip") or a synthetic fill ("gap"). The planner expands a track
// of clips-plus-implicit-gaps into a flat slice that the filter builder can
// iterate without branching on gap/clip logic per segment.
type segmentPlan struct {
	isGap       bool
	sourceStart float64 // when !isGap
	sourceEnd   float64 // when !isGap
	duration    float64 // when isGap
}

// planSegments lays out one track as an alternating sequence of clip and
// gap segments. When totalDur > track length, a trailing gap is appended
// so the rendered stream matches the overall program duration — this is
// what keeps the muxed mp4's two streams equal-length when one track was
// edited shorter than the other.
func planSegments(clips []Clip, totalDur float64) []segmentPlan {
	if len(clips) == 0 {
		// Pure trailing gap: caller decides whether to emit it (an empty
		// track shouldn't get padded — it just isn't output at all).
		return nil
	}
	sorted := append([]Clip(nil), clips...)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].ProgramStart < sorted[j].ProgramStart })

	var plan []segmentPlan
	var cursor float64
	for _, c := range sorted {
		if c.ProgramStart > cursor+1e-6 {
			plan = append(plan, segmentPlan{isGap: true, duration: c.ProgramStart - cursor})
		}
		plan = append(plan, segmentPlan{
			sourceStart: c.SourceStart,
			sourceEnd:   c.SourceEnd,
			duration:    c.Duration(),
		})
		cursor = c.ProgramStart + c.Duration()
	}
	if totalDur > cursor+1e-3 {
		plan = append(plan, segmentPlan{isGap: true, duration: totalDur - cursor})
	}
	return plan
}

// buildVideoTrackFilter produces the filter graph for the video track,
// including synthetic black-frame segments for gaps between clips and a
// trailing black pad when the video track is shorter than totalDur.
func buildVideoTrackFilter(clips []Clip, src Source, totalDur float64) []string {
	w, h, fr := src.Width, src.Height, src.FrameRate
	if w <= 0 {
		w = 1920
	}
	if h <= 0 {
		h = 1080
	}
	if fr <= 0 {
		fr = 30
	}
	segs := planSegments(clips, totalDur)
	var parts []string
	var refs []string
	for i, seg := range segs {
		label := fmt.Sprintf("[v%d]", i)
		if seg.isGap {
			// `color` generates a constant color source for `duration`
			// seconds; format=yuv420p aligns pixel format with typical
			// source streams so concat sees homogeneous inputs.
			parts = append(parts, fmt.Sprintf(
				"color=c=black:s=%dx%d:r=%s:d=%s,format=yuv420p%s",
				w, h, formatFloat(fr), formatFloat(seg.duration), label,
			))
		} else {
			// `trim` + `setpts=PTS-STARTPTS` extracts the slice and rebases
			// its timestamps to zero so concat can stack it without gaps.
			parts = append(parts, fmt.Sprintf(
				"[0:v]trim=start=%s:end=%s,setpts=PTS-STARTPTS,format=yuv420p%s",
				formatFloat(seg.sourceStart), formatFloat(seg.sourceEnd), label,
			))
		}
		refs = append(refs, label)
	}
	parts = append(parts, fmt.Sprintf(
		"%sconcat=n=%d:v=1:a=0[v]",
		strings.Join(refs, ""), len(segs),
	))
	return parts
}

// buildAudioTrackFilter is the audio twin of buildVideoTrackFilter.
// anullsrc fills gaps; aformat normalises both real and gap segments to
// 48k/stereo/fltp so concat has matching inputs.
//
// volume is a linear gain (1.0 = unity). When != 1.0, a trailing
// `volume=X` filter is appended after concat by routing the concat
// output through an intermediate label [a_pre] then into [a]. We avoid
// the rename when volume == 1.0 so the filter graph for existing
// projects stays byte-for-byte identical (test stability + readability).
func buildAudioTrackFilter(clips []Clip, volume float64, totalDur float64) []string {
	const audioFormat = "aformat=sample_fmts=fltp:sample_rates=48000:channel_layouts=stereo"
	segs := planSegments(clips, totalDur)
	var parts []string
	var refs []string
	for i, seg := range segs {
		label := fmt.Sprintf("[a%d]", i)
		if seg.isGap {
			parts = append(parts, fmt.Sprintf(
				"anullsrc=r=48000:cl=stereo:d=%s,%s%s",
				formatFloat(seg.duration), audioFormat, label,
			))
		} else {
			parts = append(parts, fmt.Sprintf(
				"[0:a]atrim=start=%s:end=%s,asetpts=PTS-STARTPTS,%s%s",
				formatFloat(seg.sourceStart), formatFloat(seg.sourceEnd), audioFormat, label,
			))
		}
		refs = append(refs, label)
	}
	useVolume := volume > 0 && (volume < 0.999 || volume > 1.001)
	concatOut := "[a]"
	if useVolume {
		concatOut = "[a_pre]"
	}
	parts = append(parts, fmt.Sprintf(
		"%sconcat=n=%d:v=0:a=1%s",
		strings.Join(refs, ""), len(segs), concatOut,
	))
	if useVolume {
		parts = append(parts, fmt.Sprintf("[a_pre]volume=%s[a]", formatFloat(volume)))
	}
	return parts
}

// normalizeVideoCodec mirrors the mapping used elsewhere in the app: UI
// names like "h264" become the actual ffmpeg encoder name. Unknown names
// pass through, so users can type raw encoder names if they want.
func normalizeVideoCodec(name string) string {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "", "h264":
		return "libx264"
	case "h265":
		return "libx265"
	default:
		return name
	}
}

func normalizeAudioCodec(name string) string {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "":
		return "aac"
	default:
		return name
	}
}

// formatFloat writes a seconds value without scientific notation and with
// up to 6 decimals, trimmed. ffmpeg accepts both "12" and "12.345".
func formatFloat(v float64) string {
	s := strconv.FormatFloat(v, 'f', 6, 64)
	s = strings.TrimRight(s, "0")
	s = strings.TrimRight(s, ".")
	if s == "" || s == "-" {
		s = "0"
	}
	return s
}
