package ports

// JobRunner is the abstraction over the long-running ffmpeg subprocess. The
// editor uses it to Start an export job and to Cancel in-flight jobs. The
// actual SSE streaming of log output is served by the main app and is not
// part of this interface — the editor simply kicks off the job.
//
// Implementations must enforce single-job semantics; Start while Running()
// is true should return an error rather than spawning a second process.
type JobRunner interface {
	Start(binary string, args []string) error
	Cancel()
	Running() bool
}
