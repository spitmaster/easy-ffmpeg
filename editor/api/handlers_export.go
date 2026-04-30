package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"easy-ffmpeg/editor/domain"
	"easy-ffmpeg/editor/ports"
)

// ExportHandlers kicks off and cancels the export ffmpeg job. It also
// tracks the latest export's output path so an authorized source-serve
// endpoint can access intermediate files if needed in the future (not
// used today; reserved).
type ExportHandlers struct {
	repo   ports.ProjectRepository
	runner ports.JobRunner
	paths  ports.PathResolver

	mu          sync.Mutex
	lastCommand string
}

func NewExportHandlers(repo ports.ProjectRepository, runner ports.JobRunner, paths ports.PathResolver) *ExportHandlers {
	return &ExportHandlers{repo: repo, runner: runner, paths: paths}
}

func (h *ExportHandlers) start(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req exportRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json: "+err.Error())
		return
	}
	if strings.TrimSpace(req.ProjectID) == "" {
		writeErr(w, http.StatusBadRequest, "projectId is empty")
		return
	}
	p, err := h.repo.Get(r.Context(), req.ProjectID)
	if err != nil {
		if errors.Is(err, ports.ErrNotFound) {
			writeErr(w, http.StatusNotFound, err.Error())
			return
		}
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	// Merge in request-side export overrides (if any), but don't persist.
	if req.Export != nil {
		p.Export = *req.Export
	}
	if p.Export.OutputDir != "" && !req.DryRun {
		if err := os.MkdirAll(p.Export.OutputDir, 0o755); err != nil {
			writeErr(w, http.StatusBadRequest, "cannot create output dir: "+err.Error())
			return
		}
	}
	args, outPath, err := domain.BuildExportArgs(p)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	command := "ffmpeg " + strings.Join(args, " ")
	// DryRun: hand back the command without starting ffmpeg or checking
	// for an existing output. Front-end uses this to show the user the
	// exact command that would run before they commit. Overwrite is
	// re-checked on the real-run POST.
	if req.DryRun {
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"ok":         true,
			"dryRun":     true,
			"command":    command,
			"outputPath": filepath.ToSlash(outPath),
		})
		return
	}
	// Overwrite guard: if the destination already exists and the client
	// hasn't authorized overwrite, return 409 with `existing:true` so the
	// frontend can surface a confirmation dialog. Same shape as
	// /api/convert/start and /api/audio/start.
	if !req.Overwrite {
		if _, statErr := os.Stat(outPath); statErr == nil {
			writeJSON(w, http.StatusConflict, map[string]interface{}{
				"error":    "file exists",
				"existing": true,
				"path":     filepath.ToSlash(outPath),
			})
			return
		}
	}
	if err := h.runner.Start(h.paths.FFmpegPath(), args); err != nil {
		writeErr(w, http.StatusConflict, err.Error())
		return
	}
	h.mu.Lock()
	h.lastCommand = command
	h.mu.Unlock()
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ok":         true,
		"command":    command,
		"outputPath": filepath.ToSlash(outPath),
	})
}

func (h *ExportHandlers) cancel(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	h.runner.Cancel()
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
