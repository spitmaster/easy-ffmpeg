package domain

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	common "easy-ffmpeg/editor/common/domain"
)

// BuildExportArgs translates a multitrack Project into a concrete ffmpeg
// argv plus the resolved output path. Pure function: no I/O, no globals.
//
// Strategy:
//   - Collect referenced sources (those any clip points at) in Sources order
//     and assign -i input indices. Unreferenced sources are skipped — no
//     point asking ffmpeg to demux files we never read.
//   - For video tracks: each track concats its clips (multi-source) into
//     [Vk]; gaps fill with `color`, sources scale+pad to canvas dims so
//     concat's homogeneous-input requirement holds across resolutions.
//     N=1 → the single track's outLabel is [V] directly. N≥2 → chain
//     overlay [V0][V1]→[Vmix1], [Vmix1][V2]→[Vmix2], … final [V].
//   - For audio tracks: each track concats clips into [Aj], optional
//     per-track volume (skipped at unity to keep filter byte-stable).
//     N=1 → [A1] aliased to [A] (or routed through [A_pre]→volume→[A] if
//     global volume non-unity). N≥2 → amix into [A_pre], then optional
//     global volume → [A].
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
	// filter graph — they'd contribute pure pad which is the same as not
	// being there at all.
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

	// Canvas dims: max across video sources actually referenced by some
	// video clip. Audio sources never participate. With no video tracks
	// the values are unused but kept defaulted so the helpers stay safe.
	canvasW, canvasH := 0, 0
	canvasFr := 0.0
	if hasVideo {
		usedByVideo := map[string]bool{}
		for _, vt := range vTracks {
			for _, c := range vt.vClips {
				usedByVideo[c.SourceID] = true
			}
		}
		for _, s := range p.Sources {
			if !usedByVideo[s.ID] {
				continue
			}
			if s.Width > canvasW {
				canvasW = s.Width
			}
			if s.Height > canvasH {
				canvasH = s.Height
			}
			if s.FrameRate > canvasFr {
				canvasFr = s.FrameRate
			}
		}
		if canvasW == 0 {
			canvasW = 1920
		}
		if canvasH == 0 {
			canvasH = 1080
		}
		if canvasFr == 0 {
			canvasFr = 30
		}
	}

	// Program duration: longest track, used to pad shorter tracks so the
	// muxed output's streams have matching lengths. Without this, browsers
	// cut playback at the shorter stream's EOF.
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

	// ---- video filter graph -----------------------------------------------
	if hasVideo {
		// trackOut: terminal label of each track's chain. When there's only
		// one track we set it directly to [V] and skip the overlay step;
		// emitting a no-op [V0] → [V] alias would just bloat the filter.
		trackOuts := make([]string, len(vTracks))
		if len(vTracks) == 1 {
			trackOuts[0] = "[V]"
		} else {
			for i := range vTracks {
				trackOuts[i] = fmt.Sprintf("[V%d]", i)
			}
		}
		for i, vt := range vTracks {
			built, err := BuildMultitrackVideoTrackFilter(
				vt.vClips, sourceInputIdx,
				trackOuts[i], programDur,
				canvasW, canvasH, canvasFr,
				fmt.Sprintf("v%d_", i),
			)
			if err != nil {
				return nil, "", err
			}
			parts = append(parts, built...)
		}
		// N≥2: chain overlay. [V0][V1]overlay=0:0[Vmix1]; [Vmix1][V2]overlay=...
		// Naming: final overlay step targets [V] directly; intermediate
		// rungs use [Vmix<k>]. Lower track index = bottom layer.
		if len(vTracks) >= 2 {
			cur := trackOuts[0]
			for k := 1; k < len(vTracks); k++ {
				next := "[V]"
				if k < len(vTracks)-1 {
					next = fmt.Sprintf("[Vmix%d]", k)
				}
				parts = append(parts, fmt.Sprintf(
					"%s%soverlay=0:0%s", cur, trackOuts[k], next,
				))
				cur = next
			}
		}
	}

	// ---- audio filter graph -----------------------------------------------
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
