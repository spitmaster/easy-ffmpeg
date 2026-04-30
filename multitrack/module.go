// Package multitrack is the entry point for the multitrack editor module.
// Like editor/, it is composable: the main app wires it in alongside other
// modules, and a future standalone exe could compile just this module
// with its own thin main.
//
// Only this file (and the sub-packages' exported types) form the public
// surface. Consumers depend on Module + Deps + NewModule; they should
// not reach into api/domain/storage directly.
package multitrack

import (
	"fmt"
	"net/http"
	"time"

	commonports "easy-ffmpeg/editor/common/ports"
	"easy-ffmpeg/multitrack/api"
	"easy-ffmpeg/multitrack/ports"
	"easy-ffmpeg/multitrack/storage"
)

// Deps bundles all external capabilities the multitrack editor needs.
// Clock / Runner / Paths are shared with the single-video editor (same
// JobRunner instance enforces the global single-job invariant). Prober
// is multitrack-specific (MediaProber, super-set of VideoProber).
//
// In M5 only Clock + DataDir are actually exercised. Prober / Runner /
// Paths are validated at NewModule time so missing deps surface
// immediately rather than the first time M6/M8 routes light up — wiring
// regressions are easier to debug at startup than under load.
type Deps struct {
	Prober  ports.MediaProber
	Runner  commonports.JobRunner
	Paths   commonports.PathResolver
	Clock   commonports.Clock // nil → wallClock
	DataDir string            // where multitrack/<id>.json files are stored
}

// Module is a constructed multitrack editor ready to be mounted on an
// http.ServeMux.
type Module struct {
	router *api.Router
	repo   ports.ProjectRepository
}

// NewModule validates deps, opens the JSON repository and wires every
// handler. Returns an error only for setup-time failures (e.g. cannot
// create the data directory). Once constructed, Module is safe for
// concurrent use.
func NewModule(d Deps) (*Module, error) {
	if d.Prober == nil {
		return nil, fmt.Errorf("multitrack: Deps.Prober is required")
	}
	if d.Runner == nil {
		return nil, fmt.Errorf("multitrack: Deps.Runner is required")
	}
	if d.Paths == nil {
		return nil, fmt.Errorf("multitrack: Deps.Paths is required")
	}
	if d.DataDir == "" {
		return nil, fmt.Errorf("multitrack: Deps.DataDir is required")
	}
	clock := d.Clock
	if clock == nil {
		clock = wallClock{}
	}
	repo, err := storage.NewJSONRepo(d.DataDir)
	if err != nil {
		return nil, fmt.Errorf("multitrack: init repo: %w", err)
	}
	router := api.NewRouter(api.Config{
		Repo:   repo,
		Prober: d.Prober,
		Runner: d.Runner,
		Paths:  d.Paths,
		Clock:  clock,
	})
	return &Module{router: router, repo: repo}, nil
}

// Register mounts the multitrack editor's HTTP routes under prefix.
func (m *Module) Register(mux *http.ServeMux, prefix string) {
	m.router.Register(mux, prefix)
}

// wallClock is the default production clock.
type wallClock struct{}

func (wallClock) Now() time.Time { return time.Now() }
