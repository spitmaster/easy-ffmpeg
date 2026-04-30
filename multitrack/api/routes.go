package api

import (
	"net/http"
	"strings"

	commonports "easy-ffmpeg/editor/common/ports"
	"easy-ffmpeg/multitrack/ports"
)

// Config bundles all the dependencies Router needs to build its handlers.
// Mirrors editor/api.Config; multitrack-specific Prober / Runner / Paths
// are accepted but currently only Repo + Clock are touched in M5.
type Config struct {
	Repo   ports.ProjectRepository
	Prober ports.MediaProber
	Runner commonports.JobRunner
	Paths  commonports.PathResolver
	Clock  commonports.Clock
}

// Router constructs every handler once and exposes a Register method.
// It never holds mutable state itself — handlers do.
type Router struct {
	cfg  Config
	proj *ProjectHandlers
}

func NewRouter(cfg Config) *Router {
	return &Router{cfg: cfg}
}

// Register mounts all multitrack routes under prefix (typically
// "/api/multitrack"). The prefix is also passed to ProjectHandlers so
// /:id parsing works correctly regardless of where the module is mounted.
func (r *Router) Register(mux *http.ServeMux, prefix string) {
	prefix = strings.TrimRight(prefix, "/")
	r.proj = NewProjectHandlers(r.cfg.Repo, r.cfg.Clock, prefix)
	mux.HandleFunc(prefix+"/projects", r.proj.listOrCreate)
	mux.HandleFunc(prefix+"/projects/", r.proj.getUpdateDelete)
	// M6+ routes:
	//   POST   /sources           — import media into a project
	//   POST   /export            — start export
	//   POST   /export/cancel     — cancel running export
	//   GET    /source            — Range-served file for <video>/<audio>
}
