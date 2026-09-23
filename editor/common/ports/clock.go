// Package ports defines abstractions shared by single-video and multitrack
// editors. Implementations are wired in by the composition root (server/).
//
// What lives here vs. the editor-specific ports/:
//   - Common: capabilities that both editors need verbatim (Clock, JobRunner,
//     PathResolver) — single-job execution, time injection, ffmpeg path resolution.
//   - editor/ports/: interfaces tied to the single-video Project shape
//     (ProjectRepository, VideoProber).
//   - multitrack/ports/: interfaces tied to the multitrack Project shape
//     (ProjectRepository, MediaProber).
package ports

import "time"

// Clock is the time source. In production it's wall-clock time; tests
// inject a fixed clock so project timestamps are deterministic.
type Clock interface {
	Now() time.Time
}
