package api

import (
	"net/http"
	"strings"

	commonports "easy-ffmpeg/editor/common/ports"
	"easy-ffmpeg/multitrack/ports"
)

// Config bundles all the dependencies Router needs to build its handlers.
// Mirrors editor/api.Config; multitrack-specific Prober / Runner / Paths
// are accepted as deps even though Runner / Paths are exercised only by
// the export route added in M8.
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
	cfg     Config
	prefix  string
	proj    *ProjectHandlers
	sources *SourceHandlers
	serve   *SourceServeHandlers
}

func NewRouter(cfg Config) *Router {
	return &Router{cfg: cfg}
}

// Register mounts all multitrack routes under prefix (typically
// "/api/multitrack"). The prefix is also passed to the per-id handlers so
// /:id parsing works correctly regardless of where the module is mounted.
func (r *Router) Register(mux *http.ServeMux, prefix string) {
	prefix = strings.TrimRight(prefix, "/")
	r.prefix = prefix
	r.proj = NewProjectHandlers(r.cfg.Repo, r.cfg.Clock, prefix)
	r.sources = NewSourceHandlers(r.cfg.Repo, r.cfg.Prober, r.cfg.Clock, prefix)
	r.serve = NewSourceServeHandlers(r.cfg.Repo)

	mux.HandleFunc(prefix+"/projects", r.proj.listOrCreate)
	// /projects/ catches both project CRUD (/:id) and source CRUD
	// (/:id/sources, /:id/sources/:sid). handleProjectsTree picks one.
	mux.HandleFunc(prefix+"/projects/", r.handleProjectsTree)
	mux.HandleFunc(prefix+"/source", r.serve.serve)
	// M8+ routes:
	//   POST /export, POST /export/cancel
}

// handleProjectsTree disambiguates between project-id and source-tree URLs
// for the same path prefix. net/http's ServeMux doesn't pattern-match
// nested segments, so we read the path here. Anything that looks like
// /projects/:id/sources(/:sid) goes to the sources handler; everything
// else falls back to the per-id project handler (GET / PUT / DELETE).
func (r *Router) handleProjectsTree(w http.ResponseWriter, req *http.Request) {
	rest := strings.TrimPrefix(req.URL.Path, r.prefix+"/projects/")
	rest = strings.Trim(rest, "/")
	parts := strings.Split(rest, "/")
	if len(parts) >= 2 && parts[1] == "sources" {
		r.sources.dispatch(w, req)
		return
	}
	r.proj.getUpdateDelete(w, req)
}
