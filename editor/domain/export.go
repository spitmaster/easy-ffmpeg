package domain

import (
	"errors"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
)

// BuildExportArgs translates a Project into a concrete ffmpeg argv plus
// the resolved output path. Pure function: no I/O, no globals.
//
// Strategy: build one filter_complex with:
//   * a per-clip trim+setpts chain for each VideoClip, concatenated to [v]
//   * a per-clip atrim+asetpts chain for each AudioClip, concatenated to [a]
//
// Video track and audio track are independent — the video-only timeline
// and the audio-only timeline can have different lengths after independent
// edits. ffmpeg muxes them into one output; the container length follows
// the longer stream.
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
	videoCodec := normalizeVideoCodec(p.Export.VideoCodec)
	audioCodec := normalizeAudioCodec(p.Export.AudioCodec)
	outPath := filepath.Join(p.Export.OutputDir, p.Export.OutputName+"."+p.Export.Format)

	var parts []string
	if hasVideo {
		parts = append(parts, buildTrackFilter(p.VideoClips, "v", "trim", "setpts")...)
	}
	if hasAudio {
		parts = append(parts, buildTrackFilter(p.AudioClips, "a", "atrim", "asetpts")...)
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

// buildTrackFilter produces the per-clip trim+concat filter chain for one
// track. streamKind is "v" or "a"; trimOp is "trim" or "atrim"; setptsOp
// is "setpts" or "asetpts". Returns filter statements ending in a concat
// that exposes a named label "[v]" or "[a]".
func buildTrackFilter(clips []Clip, streamKind, trimOp, setptsOp string) []string {
	var parts []string
	var refs []string
	for i, c := range clips {
		label := fmt.Sprintf("[%s%d]", streamKind, i)
		parts = append(parts, fmt.Sprintf(
			"[0:%s]%s=start=%s:end=%s,%s=PTS-STARTPTS%s",
			streamKind, trimOp,
			formatFloat(c.SourceStart), formatFloat(c.SourceEnd),
			setptsOp, label,
		))
		refs = append(refs, label)
	}
	videoFlag, audioFlag := 1, 0
	outLabel := "[v]"
	if streamKind == "a" {
		videoFlag, audioFlag = 0, 1
		outLabel = "[a]"
	}
	parts = append(parts, fmt.Sprintf(
		"%sconcat=n=%d:v=%d:a=%d%s",
		strings.Join(refs, ""), len(clips), videoFlag, audioFlag, outLabel,
	))
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
