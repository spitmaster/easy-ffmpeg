package server

import (
	"easy-ffmpeg/config"
	"easy-ffmpeg/internal/browser"
	"easy-ffmpeg/internal/embedded"
	"easy-ffmpeg/service"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
)

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// ---------------- ffmpeg status ----------------

func (s *Server) handleFFmpegStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"available": service.CheckFFmpeg(),
		"embedded":  service.IsEmbedded(),
		"version":   service.GetFFmpegVersion(),
	})
}

func (s *Server) handlePrepareStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, embedded.GetProgress())
}

func (s *Server) handleFFmpegReveal(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	dir, err := service.GetFFmpegDir()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := browser.Open(dir); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"path": filepath.ToSlash(dir)})
}

// ---------------- filesystem ----------------

type fsEntry struct {
	Name  string `json:"name"`
	IsDir bool   `json:"isDir"`
	Size  int64  `json:"size"`
}

type fsListResponse struct {
	Path    string    `json:"path"`
	Parent  string    `json:"parent"`
	Drives  []string  `json:"drives,omitempty"`
	Entries []fsEntry `json:"entries"`
}

func (s *Server) handleFsList(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Query().Get("path")
	if path == "" {
		home, _ := os.UserHomeDir()
		path = home
	}
	path = filepath.Clean(path)

	info, err := os.Stat(path)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	// If caller passed a file path (e.g. previously-picked input file),
	// fall back to listing its parent directory instead of erroring.
	if !info.IsDir() {
		path = filepath.Dir(path)
		info, err = os.Stat(path)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		if !info.IsDir() {
			writeErr(w, http.StatusBadRequest, "path is not a directory")
			return
		}
	}

	dirEntries, err := os.ReadDir(path)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}

	entries := make([]fsEntry, 0, len(dirEntries))
	for _, de := range dirEntries {
		name := de.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		var size int64
		if fi, err := de.Info(); err == nil {
			size = fi.Size()
		}
		entries = append(entries, fsEntry{
			Name:  name,
			IsDir: de.IsDir(),
			Size:  size,
		})
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].IsDir != entries[j].IsDir {
			return entries[i].IsDir
		}
		return strings.ToLower(entries[i].Name) < strings.ToLower(entries[j].Name)
	})

	parent := filepath.Dir(path)
	if parent == path {
		parent = ""
	}

	resp := fsListResponse{
		Path:    filepath.ToSlash(path),
		Parent:  filepath.ToSlash(parent),
		Entries: entries,
	}
	if runtime.GOOS == "windows" {
		resp.Drives = listWindowsDrives()
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleFsHome(w http.ResponseWriter, r *http.Request) {
	home, _ := os.UserHomeDir()
	writeJSON(w, http.StatusOK, map[string]string{
		"home": filepath.ToSlash(home),
	})
}

func (s *Server) handleFsReveal(w http.ResponseWriter, r *http.Request) {
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
	info, err := os.Stat(body.Path)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	target := body.Path
	if !info.IsDir() {
		target = filepath.Dir(body.Path)
	}
	if err := browser.Open(target); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func listWindowsDrives() []string {
	var drives []string
	for c := 'A'; c <= 'Z'; c++ {
		drive := string(c) + ":\\"
		if _, err := os.Stat(drive); err == nil {
			drives = append(drives, filepath.ToSlash(drive))
		}
	}
	return drives
}

// ---------------- dir persistence ----------------

func (s *Server) handleConfigDirs(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, map[string]string{
			"inputDir":  filepath.ToSlash(config.GetInputDir()),
			"outputDir": filepath.ToSlash(config.GetOutputDir()),
		})
	case http.MethodPost:
		var body struct {
			InputDir  string `json:"inputDir"`
			OutputDir string `json:"outputDir"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		if body.InputDir != "" {
			_ = config.SaveInputDir(body.InputDir)
		}
		if body.OutputDir != "" {
			_ = config.SaveOutputDir(body.OutputDir)
		}
		writeJSON(w, http.StatusOK, map[string]string{"ok": "true"})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// ---------------- convert ----------------

type convertRequest struct {
	InputPath    string `json:"inputPath"`
	OutputDir    string `json:"outputDir"`
	OutputName   string `json:"outputName"`
	VideoEncoder string `json:"videoEncoder"`
	AudioEncoder string `json:"audioEncoder"`
	Format       string `json:"format"`
	Overwrite    bool   `json:"overwrite"`
}

func buildFFmpegArgs(req convertRequest) []string {
	outputPath := filepath.Join(req.OutputDir, req.OutputName+"."+req.Format)
	args := []string{"-y", "-i", req.InputPath}

	videoCodec := normalizeVideoCodec(req.VideoEncoder)
	audioCodec := normalizeAudioCodec(req.AudioEncoder)

	if videoCodec == "copy" && audioCodec == "copy" {
		args = append(args, "-c", "copy")
	} else {
		args = append(args, "-c:v", videoCodec, "-c:a", audioCodec)
	}
	args = append(args, outputPath)
	return args
}

func normalizeVideoCodec(name string) string {
	switch strings.ToLower(name) {
	case "h264":
		return "libx264"
	case "h265":
		return "libx265"
	case "":
		return "libx264"
	default:
		return name
	}
}

func normalizeAudioCodec(name string) string {
	if name == "" {
		return "aac"
	}
	return name
}

func (s *Server) handleConvertStart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req convertRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.InputPath == "" || req.OutputDir == "" || req.OutputName == "" || req.Format == "" {
		writeErr(w, http.StatusBadRequest, "missing required fields")
		return
	}
	if err := os.MkdirAll(req.OutputDir, 0755); err != nil {
		writeErr(w, http.StatusBadRequest, "cannot create output dir: "+err.Error())
		return
	}

	// 目标文件已存在且未授权覆盖 → 返回 409 让前端弹确认框
	outputPath := filepath.Join(req.OutputDir, req.OutputName+"."+req.Format)
	if !req.Overwrite {
		if _, err := os.Stat(outputPath); err == nil {
			writeJSON(w, http.StatusConflict, map[string]interface{}{
				"error":    "file exists",
				"existing": true,
				"path":     filepath.ToSlash(outputPath),
			})
			return
		}
	}

	args := buildFFmpegArgs(req)
	if err := s.jobs.Start(service.GetFFmpegPath(), args); err != nil {
		writeErr(w, http.StatusConflict, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ok":      true,
		"command": "ffmpeg " + strings.Join(args, " "),
	})
}

func (s *Server) handleConvertCancel(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	s.jobs.Cancel()
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// ---------------- SSE stream ----------------

func (s *Server) handleConvertStream(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	events, unsub := s.jobs.Subscribe()
	defer unsub()

	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case ev, ok := <-events:
			if !ok {
				return
			}
			data, err := json.Marshal(ev)
			if err != nil {
				continue
			}
			fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()
		}
	}
}

// ---------------- quit ----------------

func (s *Server) handleQuit(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	go s.RequestShutdown()
}
