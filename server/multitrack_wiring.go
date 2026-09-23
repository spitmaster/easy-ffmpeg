package server

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"easy-ffmpeg/multitrack"
	mtports "easy-ffmpeg/multitrack/ports"
	"easy-ffmpeg/service"
)

// multitrack_wiring.go contains the small adapters that bridge the main
// app's concrete services (service.*, internal/job) to the abstract ports
// the multitrack editor module depends on. Like editor_wiring.go this is
// where the dependency inversion is realized — multitrack/ stays unaware
// of the main app.

// mediaProberAdapter adapts service.ProbeVideo to ports.MediaProber.
// ffprobe naturally surfaces both audio-only and video files; the adapter
// flags Kind based on whether the probe found a video stream. Audio-only
// files report zero Width/Height/FrameRate, which matches multitrack's
// "video tracks may not reference SourceAudio" rule (enforced at the
// timeline / drag layer in M6+).
type mediaProberAdapter struct{}

func (mediaProberAdapter) ProbeMedia(_ context.Context, path string) (*mtports.MediaInfo, error) {
	res, err := service.ProbeVideo(path)
	if err != nil {
		return nil, err
	}
	info := &mtports.MediaInfo{
		Duration: res.Format.Duration,
	}
	if res.Audio != nil {
		info.AudioCodec = res.Audio.CodecName
		info.HasAudio = true
	}
	// VideoProbeResult.Video is a value type — an empty CodecName means
	// "no video stream", which is how ffprobe reports a pure-audio file.
	if res.Video.CodecName != "" {
		info.Kind = "video"
		info.Width = res.Video.Width
		info.Height = res.Video.Height
		info.VideoCodec = res.Video.CodecName
		info.FrameRate = res.Video.FrameRate
	} else {
		info.Kind = "audio"
	}
	return info, nil
}

// buildMultitrackModule constructs the multitrack Module with production
// deps. Reuses jobRunnerAdapter / pathResolverAdapter from
// editor_wiring.go — the JobRunner instance is shared so multitrack
// export can never run alongside convert/audio/single-video export.
func (s *Server) buildMultitrackModule() (*multitrack.Module, string, error) {
	dataDir, err := multitrackDataDir()
	if err != nil {
		return nil, "", fmt.Errorf("multitrack: resolve data dir: %w", err)
	}
	mod, err := multitrack.NewModule(multitrack.Deps{
		Prober:  mediaProberAdapter{},
		Runner:  jobRunnerAdapter{m: s.jobs},
		Paths:   pathResolverAdapter{},
		DataDir: dataDir,
	})
	if err != nil {
		return nil, dataDir, err
	}
	return mod, dataDir, nil
}

// multitrackDataDir returns the directory where multitrack project JSON
// files live. Sits under ~/.easy-ffmpeg/, separate from the single-video
// editor's projects/ — the kind discriminator on Project guards against
// cross-loading even if a file is misplaced, but the directory split
// makes accidental collisions structurally impossible.
func multitrackDataDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".easy-ffmpeg", "multitrack"), nil
}
