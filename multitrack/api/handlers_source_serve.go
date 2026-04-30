package api

import (
	"errors"
	"net/http"
	"os"

	"easy-ffmpeg/multitrack/ports"
)

// SourceServeHandlers serves the raw bytes of any source registered on a
// project so the browser can stream + seek video / audio elements at it.
// Range support is provided for free by http.ServeContent.
//
// Like editor/api.SourceHandlers, the path is *never* taken from the URL —
// only the project + source id are. The actual on-disk path comes from the
// stored Source.Path, so this endpoint cannot be used to exfiltrate
// arbitrary files.
type SourceServeHandlers struct {
	repo ports.ProjectRepository
}

func NewSourceServeHandlers(repo ports.ProjectRepository) *SourceServeHandlers {
	return &SourceServeHandlers{repo: repo}
}

func (h *SourceServeHandlers) serve(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	projectID := r.URL.Query().Get("projectId")
	sourceID := r.URL.Query().Get("sourceId")
	if projectID == "" || sourceID == "" {
		http.Error(w, "missing projectId/sourceId", http.StatusBadRequest)
		return
	}
	p, err := h.repo.Get(r.Context(), projectID)
	if err != nil {
		if errors.Is(err, ports.ErrNotFound) {
			http.NotFound(w, r)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	var path string
	for _, s := range p.Sources {
		if s.ID == sourceID {
			path = s.Path
			break
		}
	}
	if path == "" {
		http.NotFound(w, r)
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
	http.ServeContent(w, r, stat.Name(), stat.ModTime(), f)
}
