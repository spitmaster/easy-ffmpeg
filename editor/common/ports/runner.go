package ports

// JobRunner is the abstraction over the long-running ffmpeg subprocess.
// Editors use it to Start an export job and to Cancel in-flight jobs. The
// actual SSE streaming of log output is served by the main app and is not
// part of this interface — editors simply kick off the job.
//
// Implementations must enforce single-job semantics across the whole app:
// Start while Running() is true should return an error rather than spawning
// a second process. The single instance is shared between convert / audio /
// editor / multitrack export so only one ffmpeg job ever runs at a time.
type JobRunner interface {
	Start(binary string, args []string) error
	Cancel()
	Running() bool
}
