package api

import (
	"errors"
	"net/http"
	"os"

	"easy-ffmpeg/editor/ports"
)

// SourceHandlers serves the raw video file referenced by a project's
// Source.Path so <video> can stream + seek it. Range support is handled
// automatically by http.ServeContent.
//
// The path is *never* taken from the URL — only indirectly via project id,
// so a caller cannot use this endpoint to exfiltrate arbitrary files.
type SourceHandlers struct {
	repo ports.ProjectRepository
}

func NewSourceHandlers(repo ports.ProjectRepository) *SourceHandlers {
	return &SourceHandlers{repo: repo}
}

func (h *SourceHandlers) serve(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	id := r.URL.Query().Get("id")
	if id == "" {
		http.Error(w, "missing id", http.StatusBadRequest)
		return
	}
	p, err := h.repo.Get(r.Context(), id)
	if err != nil {
		if errors.Is(err, ports.ErrNotFound) {
			http.NotFound(w, r)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	path := p.Source.Path
	if path == "" {
		http.Error(w, "project has no source", http.StatusBadRequest)
		return
	}
	f, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			http.NotFound(w, r)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer f.Close()
	stat, err := f.Stat()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	// ServeContent handles Range, If-Modified-Since, and Content-Type detection.
	http.ServeContent(w, r, stat.Name(), stat.ModTime(), f)
}
