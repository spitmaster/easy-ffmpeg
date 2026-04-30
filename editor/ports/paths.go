package ports

// PathResolver tells the editor where ffmpeg and ffprobe live. The concrete
// implementation in the main app returns the embedded binaries (or falls
// back to system PATH); a standalone editor exe could return "ffmpeg".
type PathResolver interface {
	FFmpegPath() string
	FFprobePath() string
}
