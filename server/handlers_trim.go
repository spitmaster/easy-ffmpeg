package server

import (
	"easy-ffmpeg/service"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// handleTrimProbe returns ffprobe-derived video + audio metadata for a local file.
// Used by the trim tab to auto-fill duration / dimensions / defaults.
func (s *Server) handleTrimProbe(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		Path string `json:"path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if body.Path == "" {
		writeErr(w, http.StatusBadRequest, "path is empty")
		return
	}
	res, err := service.ProbeVideo(body.Path)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, res)
}

// handleTrimStart launches ffmpeg with trim / crop / scale arguments.
// Mirrors handleConvertStart (same 409 overwrite flow) via the shared Job manager.
func (s *Server) handleTrimStart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req TrimRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.OutputDir != "" {
		if err := os.MkdirAll(req.OutputDir, 0755); err != nil {
			writeErr(w, http.StatusBadRequest, "cannot create output dir: "+err.Error())
			return
		}
	}

	result, err := BuildTrimArgs(req)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}

	if !req.Overwrite {
		if _, err := os.Stat(result.OutputPath); err == nil {
			writeJSON(w, http.StatusConflict, map[string]interface{}{
				"error":    "file exists",
				"existing": true,
				"path":     filepath.ToSlash(result.OutputPath),
			})
			return
		}
	}

	if err := s.jobs.Start(service.GetFFmpegPath(), result.Args); err != nil {
		writeErr(w, http.StatusConflict, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ok":      true,
		"command": "ffmpeg " + strings.Join(result.Args, " "),
	})
}

// handleTrimCancel stops the current job (shared with convert / audio tabs).
func (s *Server) handleTrimCancel(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	s.jobs.Cancel()
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
