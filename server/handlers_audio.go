package server

import (
	"easy-ffmpeg/service"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// handleAudioProbe returns ffprobe-derived audio stream metadata for a local file.
// Used by the audio tab to list tracks (for extract mode) and check codec consistency (for merge mode).
func (s *Server) handleAudioProbe(w http.ResponseWriter, r *http.Request) {
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
	res, err := service.ProbeAudio(body.Path)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, res)
}

// handleAudioStart dispatches to the mode-specific builder and launches ffmpeg via the shared Job manager.
// Mirrors handleConvertStart (same 409 overwrite flow) but uses BuildAudioArgs.
func (s *Server) handleAudioStart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req AudioRequest
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

	// Merge "auto" strategy: probe inputs here and resolve to copy/reencode
	// before dispatching to the pure builder.
	if req.Mode == "merge" && req.MergeStrategy == "auto" {
		req.MergeStrategy = resolveMergeStrategy(req.InputPaths)
	}

	result, err := BuildAudioArgs(req)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}

	if !req.Overwrite {
		if _, err := os.Stat(result.OutputPath); err == nil {
			if result.Cleanup != nil {
				result.Cleanup()
			}
			writeJSON(w, http.StatusConflict, map[string]interface{}{
				"error":    "file exists",
				"existing": true,
				"path":     filepath.ToSlash(result.OutputPath),
			})
			return
		}
	}

	if err := s.jobs.Start(service.GetFFmpegPath(), result.Args); err != nil {
		if result.Cleanup != nil {
			result.Cleanup()
		}
		writeErr(w, http.StatusConflict, err.Error())
		return
	}

	// Run cleanup after the job finishes, if the builder requested one.
	// The shared JobManager broadcasts a terminal event (done/error/cancelled)
	// which we subscribe to here and unsubscribe on first match.
	if result.Cleanup != nil {
		s.scheduleCleanup(result.Cleanup)
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ok":      true,
		"command": "ffmpeg " + strings.Join(result.Args, " "),
	})
}

// handleAudioCancel stops the current job (shared with the video convert tab).
func (s *Server) handleAudioCancel(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	s.jobs.Cancel()
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// resolveMergeStrategy probes every input; returns "copy" when codec, sample rate
// and channels match across all inputs (and bit rate is within 10%), otherwise "reencode".
// Any probe failure short-circuits to "reencode" — safer than risking a broken copy.
func resolveMergeStrategy(paths []string) string {
	if len(paths) < 2 {
		return "reencode"
	}
	var ref *service.AudioStream
	for _, p := range paths {
		res, err := service.ProbeAudio(p)
		if err != nil || len(res.Streams) == 0 {
			return "reencode"
		}
		s := res.Streams[0]
		if ref == nil {
			ref = &s
			continue
		}
		if s.CodecName != ref.CodecName ||
			s.SampleRate != ref.SampleRate ||
			s.Channels != ref.Channels {
			return "reencode"
		}
		if ref.BitRate > 0 && s.BitRate > 0 {
			diff := float64(s.BitRate-ref.BitRate) / float64(ref.BitRate)
			if diff < 0 {
				diff = -diff
			}
			if diff > 0.1 {
				return "reencode"
			}
		}
	}
	return "copy"
}

// scheduleCleanup runs fn after the next terminal job event (done/error/cancelled).
// Used for tempfile cleanup in merge mode.
func (s *Server) scheduleCleanup(fn func()) {
	events, unsub := s.jobs.Subscribe()
	go func() {
		defer unsub()
		for ev := range events {
			switch ev.Type {
			case "done", "error", "cancelled":
				fn()
				return
			}
		}
	}()
}
