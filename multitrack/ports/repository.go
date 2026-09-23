// Package ports defines the abstractions the multitrack editor needs from
// the outside world. Implementations are wired in by the composition root
// (server/). Mirrors editor/ports/ but typed against the multitrack
// Project shape — Clock / JobRunner / PathResolver are shared via
// editor/common/ports because they are identical for both editors.
package ports

import (
	"context"
	"errors"

	"easy-ffmpeg/multitrack/domain"
)

// ErrNotFound is returned by ProjectRepository when the requested id does
// not exist. Callers compare with errors.Is.
var ErrNotFound = errors.New("multitrack project not found")

// ProjectSummary is the lightweight row used for the project list panel.
// Kept here (not in domain) because it's an output-only shape for the
// list endpoint. Detail differs from single-video — multitrack's primary
// detail line is the source count rather than a single source path.
type ProjectSummary struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	SourceCount int    `json:"sourceCount"`
	CreatedAt   string `json:"createdAt"` // RFC3339
	UpdatedAt   string `json:"updatedAt"`
}

// ProjectRepository is the persistence port for multitrack projects.
// Implementations might be JSON files on disk, sqlite, or an in-memory
// fake for tests. Implementations must be safe to call concurrently.
type ProjectRepository interface {
	List(ctx context.Context) ([]ProjectSummary, error)
	Get(ctx context.Context, id string) (*domain.Project, error)
	Save(ctx context.Context, p *domain.Project) error
	Delete(ctx context.Context, id string) error
}
