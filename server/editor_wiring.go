package server

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"easy-ffmpeg/editor"
	"easy-ffmpeg/editor/ports"
	"easy-ffmpeg/internal/job"
	"easy-ffmpeg/service"
)

// editor_wiring.go contains the small adapters that bridge the main app's
// concrete services (service.*, internal/job) to the abstract ports the
// editor module depends on. Keeping adapters here means editor/ itself
// never has to know about the main app — reversing the dependency.

// proberAdapter adapts service.ProbeVideo to ports.VideoProber.
type proberAdapter struct{}

func (proberAdapter) Probe(_ context.Context, path string) (*ports.VideoInfo, error) {
	res, err := service.ProbeVideo(path)
	if err != nil {
		return nil, err
	}
	info := &ports.VideoInfo{
		Duration:   res.Format.Duration,
		Width:      res.Video.Width,
		Height:     res.Video.Height,
		VideoCodec: res.Video.CodecName,
		FrameRate:  res.Video.FrameRate,
	}
	if res.Audio != nil {
		info.AudioCodec = res.Audio.CodecName
		info.HasAudio = true
	}
	return info, nil
}

// jobRunnerAdapter adapts *job.Manager to ports.JobRunner. Sharing the
// same manager instance with convert/audio ensures only one ffmpeg job
// runs at a time across the whole app.
type jobRunnerAdapter struct{ m *job.Manager }

func (a jobRunnerAdapter) Start(binary string, args []string) error {
	return a.m.Start(binary, args)
}
func (a jobRunnerAdapter) Cancel()       { a.m.Cancel() }
func (a jobRunnerAdapter) Running() bool { return a.m.Running() }

// pathResolverAdapter adapts service.GetFFmpegPath / GetFFprobePath to
// ports.PathResolver.
type pathResolverAdapter struct{}

func (pathResolverAdapter) FFmpegPath() string  { return service.GetFFmpegPath() }
func (pathResolverAdapter) FFprobePath() string { return service.GetFFprobePath() }

// buildEditorModule constructs the editor Module with production deps.
// Returns the module (or error) and the path used as the data dir, so the
// caller can log it.
func (s *Server) buildEditorModule() (*editor.Module, string, error) {
	dataDir, err := editorDataDir()
	if err != nil {
		return nil, "", fmt.Errorf("editor: resolve data dir: %w", err)
	}
	mod, err := editor.NewModule(editor.Deps{
		Prober:  proberAdapter{},
		Runner:  jobRunnerAdapter{m: s.jobs},
		Paths:   pathResolverAdapter{},
		DataDir: dataDir,
	})
	if err != nil {
		return nil, dataDir, err
	}
	return mod, dataDir, nil
}

// editorDataDir returns the directory where project JSON files live. It
// sits under the same ~/.easy-ffmpeg/ tree as the embedded ffmpeg cache.
func editorDataDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".easy-ffmpeg", "projects"), nil
}
