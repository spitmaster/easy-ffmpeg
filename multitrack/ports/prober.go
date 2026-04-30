package ports

import "context"

// MediaInfo is the subset of ffprobe output the multitrack editor cares
// about. It is a superset of editor/ports.VideoInfo: pure-audio sources
// are first-class via Kind, with video fields left zero. Like the
// single-video VideoInfo, it is intentionally *not* the same type as
// domain.Source — the port describes a contract with the outside world,
// domain describes our internal model. The api layer converts between them.
type MediaInfo struct {
	Kind       string  // "video" | "audio"
	Duration   float64
	Width      int
	Height     int
	VideoCodec string
	AudioCodec string
	FrameRate  float64
	HasAudio   bool
}

// MediaProber runs ffprobe on a local file and returns the subset of
// fields the multitrack editor needs. Implementations typically shell out
// to ffprobe. Multitrack must accept pure-audio inputs (Kind="audio")
// where the single-video VideoProber would error on missing video.
type MediaProber interface {
	ProbeMedia(ctx context.Context, path string) (*MediaInfo, error)
}
