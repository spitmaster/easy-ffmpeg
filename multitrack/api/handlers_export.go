package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"

	commonports "easy-ffmpeg/editor/common/ports"
	"easy-ffmpeg/multitrack/domain"
	"easy-ffmpeg/multitrack/ports"
)

// ExportHandlers serves POST /export and POST /export/cancel for the
// multitrack editor. Mirrors editor/api.ExportHandlers — same overwrite
// 409 flow, same dryRun semantics — so the frontend's ExportDialog and
// modals.showOverwrite path are reusable byte-for-byte.
//
// The runner is the global single-job JobRunner shared with single-video
// editor, audio, and convert. Concurrency is therefore enforced at the
// runner: a 409 from runner.Start means another job is already running,
// not that the multitrack export specifically clashed.
type ExportHandlers struct {
	repo   ports.ProjectRepository
	runner commonports.JobRunner
	paths  commonports.PathResolver

	mu          sync.Mutex
	lastCommand string
}

func NewExportHandlers(repo ports.ProjectRepository, runner commonports.JobRunner, paths commonports.PathResolver) *ExportHandlers {
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
	// Apply request-side export overrides ephemerally (no save). The
	// frontend sends the dialog form values here so the user can preview
	// or run an export without first persisting unsaved settings tweaks.
	if req.Export != nil {
		p.Export = *req.Export
	}
	// Pre-create the output directory on real runs so ffmpeg doesn't
	// fail with ENOENT. Skipped on dryRun — preview should be a pure
	// no-side-effect operation.
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
	if req.DryRun {
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"ok":         true,
			"dryRun":     true,
			"command":    command,
			"outputPath": filepath.ToSlash(outPath),
		})
		return
	}
	// Overwrite guard: same shape as /api/editor/export and
	// /api/convert/start so the frontend's modals.showOverwrite +
	// re-submit-with-overwrite=true loop works without per-module
	// conditionals.
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
