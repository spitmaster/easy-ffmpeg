package ports

import "context"

// VideoInfo is the subset of ffprobe output the editor cares about. It is
// intentionally *not* the same type as domain.Source — the port describes a
// contract with the outside world, domain describes our internal model.
// The api layer converts between them.
type VideoInfo struct {
	Duration   float64
	Width      int
	Height     int
	VideoCodec string
	AudioCodec string
	FrameRate  float64
	HasAudio   bool
}

// VideoProber runs ffprobe on a local file and returns the subset of
// fields the editor needs. Implementations typically shell out to ffprobe.
type VideoProber interface {
	Probe(ctx context.Context, path string) (*VideoInfo, error)
}
