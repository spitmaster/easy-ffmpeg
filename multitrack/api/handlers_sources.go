package api

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	commonports "easy-ffmpeg/editor/common/ports"
	"easy-ffmpeg/multitrack/domain"
	"easy-ffmpeg/multitrack/ports"
)

// SourceHandlers serves source import / removal under
// /projects/:id/sources(/:sid). Probing happens here (not in the
// repository) so the JSON repo stays a pure persistence layer; ffprobe
// I/O lives next to the rest of the HTTP-layer code.
type SourceHandlers struct {
	repo   ports.ProjectRepository
	prober ports.MediaProber
	clock  commonports.Clock
	prefix string // mount prefix, used to parse /:id and /:sid out of the URL
}

func NewSourceHandlers(repo ports.ProjectRepository, prober ports.MediaProber, clock commonports.Clock, prefix string) *SourceHandlers {
	return &SourceHandlers{repo: repo, prober: prober, clock: clock, prefix: prefix}
}

// dispatch routes the two URL shapes:
//
//	POST   /projects/:id/sources       → import
//	DELETE /projects/:id/sources/:sid  → remove
//
// Single handler so we can mount one ServeMux entry and read the path
// segments ourselves — net/http's ServeMux doesn't do path patterns we'd
// need here without bringing in extra dependencies.
func (h *SourceHandlers) dispatch(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, h.prefix+"/projects/")
	rest = strings.Trim(rest, "/")
	parts := strings.Split(rest, "/")
	// Expected: ["<id>", "sources"] or ["<id>", "sources", "<sid>"].
	if len(parts) < 2 || parts[1] != "sources" {
		http.NotFound(w, r)
		return
	}
	projectID := parts[0]
	if projectID == "" {
		writeErr(w, http.StatusBadRequest, "missing project id")
		return
	}
	switch {
	case len(parts) == 2 && r.Method == http.MethodPost:
		h.importSources(w, r, projectID)
	case len(parts) == 3 && r.Method == http.MethodDelete:
		h.removeSource(w, r, projectID, parts[2])
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (h *SourceHandlers) importSources(w http.ResponseWriter, r *http.Request, projectID string) {
	var req importSourcesRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json: "+err.Error())
		return
	}
	if len(req.Paths) == 0 {
		writeErr(w, http.StatusBadRequest, "no paths supplied")
		return
	}
	p, err := h.repo.Get(r.Context(), projectID)
	if err != nil {
		if errors.Is(err, ports.ErrNotFound) {
			http.NotFound(w, r)
			return
		}
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}

	resp := importSourcesResponse{Sources: []domain.Source{}}
	for _, path := range req.Paths {
		path = strings.TrimSpace(path)
		if path == "" {
			continue
		}
		// If we already have a source for this exact path, reuse its id —
		// that lets re-importing the same file refresh metadata in place
		// instead of producing a duplicate entry.
		existingID := ""
		for _, s := range p.Sources {
			if s.Path == path {
				existingID = s.ID
				break
			}
		}
		info, err := h.prober.ProbeMedia(r.Context(), path)
		if err != nil {
			resp.Errors = append(resp.Errors, importErrorItem{Path: path, Error: err.Error()})
			continue
		}
		src := domain.Source{
			ID:         pickSourceID(existingID, p),
			Path:       path,
			Kind:       info.Kind,
			Duration:   info.Duration,
			Width:      info.Width,
			Height:     info.Height,
			VideoCodec: info.VideoCodec,
			AudioCodec: info.AudioCodec,
			FrameRate:  info.FrameRate,
			HasAudio:   info.HasAudio,
		}
		p = domain.AddSource(p, src)
		resp.Sources = append(resp.Sources, src)
	}

	if len(resp.Sources) > 0 {
		p.UpdatedAt = h.clock.Now()
		if err := h.repo.Save(r.Context(), p); err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
	}
	resp.Project = p
	writeJSON(w, http.StatusOK, resp)
}

func (h *SourceHandlers) removeSource(w http.ResponseWriter, r *http.Request, projectID, sourceID string) {
	p, err := h.repo.Get(r.Context(), projectID)
	if err != nil {
		if errors.Is(err, ports.ErrNotFound) {
			http.NotFound(w, r)
			return
		}
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	p2, err := domain.RemoveSource(p, sourceID)
	if err != nil {
		switch {
		case errors.Is(err, domain.ErrSourceNotFound):
			http.NotFound(w, r)
		case errors.Is(err, domain.ErrSourceInUse):
			writeErr(w, http.StatusConflict, err.Error())
		default:
			writeErr(w, http.StatusInternalServerError, err.Error())
		}
		return
	}
	p2.UpdatedAt = h.clock.Now()
	if err := h.repo.Save(r.Context(), p2); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, p2)
}

// pickSourceID returns the existing id when re-probing an existing path,
// otherwise mints a fresh one that does not collide with the project's
// current source ids.
func pickSourceID(existing string, p *domain.Project) string {
	if existing != "" {
		return existing
	}
	for {
		id := newSourceID()
		taken := false
		for _, s := range p.Sources {
			if s.ID == id {
				taken = true
				break
			}
		}
		if !taken {
			return id
		}
	}
}

// newSourceID mints a 6-hex random id. Short, URL-safe, and very unlikely
// to collide within a single project.
func newSourceID() string {
	var b [3]byte
	if _, err := rand.Read(b[:]); err != nil {
		// Fall back to a deterministic but unique-enough id from the clock.
		// Empty string would force pickSourceID into an infinite loop, which
		// is worse than an unusual id.
		return "src" + hex.EncodeToString(b[:])
	}
	return hex.EncodeToString(b[:])
}

