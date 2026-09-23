// Package editor is the entry point for the video editor module. It is
// designed to be composable — the main app wires it in alongside other
// modules, and a future standalone exe can compile just this module with
// its own thin main.
//
// Only this file (and the sub-packages' exported types) form the public
// surface. Consumers depend on Module + Deps + NewModule; they should not
// reach into api/domain/storage directly.
package editor

import (
	"fmt"
	"net/http"
	"time"

	"easy-ffmpeg/editor/api"
	commonports "easy-ffmpeg/editor/common/ports"
	"easy-ffmpeg/editor/ports"
	"easy-ffmpeg/editor/storage"
)

// Deps bundles all external capabilities the editor needs. Supplying
// different implementations (real vs. fake) is how tests and the
// standalone exe parameterize the module.
//
// Clock / Runner / Paths come from editor/common/ports — multitrack uses
// the same interfaces. Prober is single-video specific (multitrack will
// add its own MediaProber port).
type Deps struct {
	Prober  ports.VideoProber
	Runner  commonports.JobRunner
	Paths   commonports.PathResolver
	Clock   commonports.Clock // nil → wallClock
	DataDir string            // where projects/<id>.json are stored
}

// Module is a constructed editor ready to be mounted on an http.ServeMux.
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
		return nil, fmt.Errorf("editor: Deps.Prober is required")
	}
	if d.Runner == nil {
		return nil, fmt.Errorf("editor: Deps.Runner is required")
	}
	if d.Paths == nil {
		return nil, fmt.Errorf("editor: Deps.Paths is required")
	}
	if d.DataDir == "" {
		return nil, fmt.Errorf("editor: Deps.DataDir is required")
	}
	clock := d.Clock
	if clock == nil {
		clock = wallClock{}
	}
	repo, err := storage.NewJSONRepo(d.DataDir)
	if err != nil {
		return nil, fmt.Errorf("editor: init repo: %w", err)
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

// Register mounts the editor's HTTP routes under prefix.
func (m *Module) Register(mux *http.ServeMux, prefix string) {
	m.router.Register(mux, prefix)
}

// wallClock is the default production clock.
type wallClock struct{}

func (wallClock) Now() time.Time { return time.Now() }
