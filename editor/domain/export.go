package domain

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	commondomain "easy-ffmpeg/editor/common/domain"
)

// BuildExportArgs translates a Project into a concrete ffmpeg argv plus
// the resolved output path. Pure function: no I/O, no globals.
//
// Strategy:
//   * Each track delegates to common.BuildVideoTrackFilter /
//     BuildAudioTrackFilter for its filter chain — those handle clip
//     trim/concat, gap fill (color/anullsrc), and trailing pad to
//     program duration.
//   * Single-video assembles the two chains into one -filter_complex,
//     mapping [v]/[a] to the encoder. Both tracks are padded to
//     programDur so the muxed mp4's two streams share length and
//     Chrome/native players don't truncate at the shorter stream.
//   * The leading-gap guard is single-video specific: a black-screen
//     prefix on the exported video is almost always a mistake.
//     Multitrack will revisit this rule per-track.
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
	if err := commondomain.ValidateExportSettings(p.Export); err != nil {
		return nil, "", err
	}
	// Leading-gap guard: only the **video** track must start at program
	// time 0 — a black-screen prefix on the exported file is almost always
	// a mistake. The audio track is allowed a leading gap (filled with
	// silence); legitimate use case: pre-roll silence before the first
	// spoken word. Mid-track gaps remain allowed on both tracks.
	if hasVideo {
		if t := commondomain.EarliestProgramStart(p.VideoClips); t > 1e-3 {
			return nil, "", fmt.Errorf("视频轨道开头必须有内容：第一个 clip 从 %.2fs 开始，请把它拖到 0 秒再导出", t)
		}
	}
	videoCodec := commondomain.NormalizeVideoCodec(p.Export.VideoCodec)
	audioCodec := commondomain.NormalizeAudioCodec(p.Export.AudioCodec)
	outPath := filepath.Join(p.Export.OutputDir, p.Export.OutputName+"."+p.Export.Format)

	// Both tracks are padded to programDur with synthetic black / silence
	// when shorter, so the output mp4's two streams have matching length.
	// Without this the muxer writes a 5s video + 10s audio into one file
	// and Chrome's <video> element (the Editor preview, and most native
	// players) cuts off at the shorter stream — ending playback at video
	// EOF even though the audio still has data.
	programDur := 0.0
	if v := commondomain.TrackDuration(p.VideoClips); v > programDur {
		programDur = v
	}
	if a := commondomain.TrackDuration(p.AudioClips); a > programDur {
		programDur = a
	}
	var parts []string
	if hasVideo {
		parts = append(parts, commondomain.BuildVideoTrackFilter(
			p.VideoClips, "[0:v]", "[v]", programDur,
			p.Source.Width, p.Source.Height, p.Source.FrameRate,
		)...)
	}
	if hasAudio {
		parts = append(parts, commondomain.BuildAudioTrackFilter(
			p.AudioClips, "[0:a]", "[a]", "[a_pre]", p.AudioVolume, programDur,
		)...)
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
