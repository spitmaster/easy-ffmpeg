// Package ports defines the abstractions the editor needs from the outside
// world. Implementations are wired in by the composition root (main / server).
// This enforces the dependency inversion: editor/api depends on these
// interfaces, not on concrete implementations.
package ports

import (
	"context"
	"errors"

	"easy-ffmpeg/editor/domain"
)

// ErrNotFound is returned by ProjectRepository when the requested id does
// not exist. Callers compare with errors.Is.
var ErrNotFound = errors.New("project not found")

// ProjectSummary is the lightweight row used for the project list panel.
// Kept here (not in domain) because it's an output-only shape for the
// list endpoint.
type ProjectSummary struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	SourcePath string `json:"sourcePath"`
	CreatedAt  string `json:"createdAt"` // RFC3339
	UpdatedAt  string `json:"updatedAt"`
}

// ProjectRepository is the persistence port. Implementations might be
// JSON files on disk, sqlite, or an in-memory fake for tests.
//
// Implementations must be safe to call concurrently.
type ProjectRepository interface {
	List(ctx context.Context) ([]ProjectSummary, error)
	Get(ctx context.Context, id string) (*domain.Project, error)
	Save(ctx context.Context, p *domain.Project) error
	Delete(ctx context.Context, id string) error
}
