package api

import (
	"net/http"
	"strings"

	commonports "easy-ffmpeg/editor/common/ports"
	"easy-ffmpeg/editor/ports"
)

// Config bundles all the dependencies Router needs to build its handlers.
// Keeping them in one struct makes Module.NewModule wiring explicit.
type Config struct {
	Repo   ports.ProjectRepository
	Prober ports.VideoProber
	Runner commonports.JobRunner
	Paths  commonports.PathResolver
	Clock  commonports.Clock
}

// Router constructs every handler once and exposes a Register method.
// It never holds mutable state itself — handlers do.
type Router struct {
	proj   *ProjectHandlers
	probe  *ProbeHandlers
	export *ExportHandlers
	source *SourceHandlers
}

func NewRouter(cfg Config) *Router {
	return &Router{
		proj:   NewProjectHandlers(cfg.Repo, cfg.Prober, cfg.Clock),
		probe:  NewProbeHandlers(cfg.Prober),
		export: NewExportHandlers(cfg.Repo, cfg.Runner, cfg.Paths),
		source: NewSourceHandlers(cfg.Repo),
	}
}

// Register mounts all editor routes under prefix (typically "/api/editor").
// Using a prefix lets the same module work in the main app and in a future
// standalone exe — the caller decides the URL space.
func (r *Router) Register(mux *http.ServeMux, prefix string) {
	prefix = strings.TrimRight(prefix, "/")
	mux.HandleFunc(prefix+"/projects", r.proj.listOrCreate)
	mux.HandleFunc(prefix+"/projects/", r.proj.getUpdateDelete)
	mux.HandleFunc(prefix+"/probe", r.probe.probe)
	mux.HandleFunc(prefix+"/export", r.export.start)
	mux.HandleFunc(prefix+"/export/cancel", r.export.cancel)
	mux.HandleFunc(prefix+"/source", r.source.serve)
}
