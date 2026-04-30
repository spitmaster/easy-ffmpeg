package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"easy-ffmpeg/editor/domain"
	"easy-ffmpeg/editor/ports"
)

// ProjectHandlers serves the /projects CRUD endpoints. It only depends on
// the ProjectRepository + VideoProber + Clock ports; nothing else.
type ProjectHandlers struct {
	repo   ports.ProjectRepository
	prober ports.VideoProber
	clock  ports.Clock
}

func NewProjectHandlers(repo ports.ProjectRepository, prober ports.VideoProber, clock ports.Clock) *ProjectHandlers {
	return &ProjectHandlers{repo: repo, prober: prober, clock: clock}
}

// listOrCreate routes GET vs POST at /projects.
func (h *ProjectHandlers) listOrCreate(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		h.list(w, r)
	case http.MethodPost:
		h.create(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// getUpdateDelete routes GET/PUT/DELETE at /projects/<id>.
func (h *ProjectHandlers) getUpdateDelete(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/api/editor/projects/")
	id = strings.TrimSuffix(id, "/")
	if id == "" {
		http.Error(w, "missing project id", http.StatusBadRequest)
		return
	}
	switch r.Method {
	case http.MethodGet:
		h.get(w, r, id)
	case http.MethodPut:
		h.update(w, r, id)
	case http.MethodDelete:
		h.delete(w, r, id)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (h *ProjectHandlers) list(w http.ResponseWriter, r *http.Request) {
	rows, err := h.repo.List(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, rows)
}

func (h *ProjectHandlers) create(w http.ResponseWriter, r *http.Request) {
	var req createProjectRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json: "+err.Error())
		return
	}
	if strings.TrimSpace(req.SourcePath) == "" {
		writeErr(w, http.StatusBadRequest, "sourcePath is empty")
		return
	}
	info, err := h.prober.Probe(r.Context(), req.SourcePath)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "probe failed: "+err.Error())
		return
	}
	if info == nil || info.Duration <= 0 {
		writeErr(w, http.StatusBadRequest, "probe returned no duration")
		return
	}
	now := h.clock.Now()
	name := strings.TrimSpace(req.Name)
	if name == "" {
		name = fmt.Sprintf("未命名工程 %s", now.Format("2006-01-02 15:04"))
	}
	p := domain.NewProject(newID(), name, probeToSource(req.SourcePath, info), now)
	if err := h.repo.Save(r.Context(), p); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, p)
}

func (h *ProjectHandlers) get(w http.ResponseWriter, r *http.Request, id string) {
	p, err := h.repo.Get(r.Context(), id)
	if err != nil {
		h.writeRepoErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, p)
}

func (h *ProjectHandlers) update(w http.ResponseWriter, r *http.Request, id string) {
	var p domain.Project
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json: "+err.Error())
		return
	}
	if p.ID != id {
		writeErr(w, http.StatusBadRequest, "project id in body does not match url")
		return
	}
	if errs := p.Validate(); len(errs) > 0 {
		msgs := make([]string, 0, len(errs))
		for _, e := range errs {
			msgs = append(msgs, e.Error())
		}
		writeErr(w, http.StatusBadRequest, strings.Join(msgs, "; "))
		return
	}
	p.UpdatedAt = h.clock.Now()
	if err := h.repo.Save(r.Context(), &p); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, &p)
}

func (h *ProjectHandlers) delete(w http.ResponseWriter, r *http.Request, id string) {
	if err := h.repo.Delete(r.Context(), id); err != nil {
		h.writeRepoErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (h *ProjectHandlers) writeRepoErr(w http.ResponseWriter, err error) {
	if errors.Is(err, ports.ErrNotFound) {
		writeErr(w, http.StatusNotFound, err.Error())
		return
	}
	writeErr(w, http.StatusInternalServerError, err.Error())
}
