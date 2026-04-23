package api

import (
	"encoding/json"
	"net/http"

	"easy-ffmpeg/editor/ports"
)

// ProbeHandlers exposes the video metadata endpoint. It is a thin adapter
// around ports.VideoProber — it exists so the editor can probe without
// coupling to the main app's /api/audio/probe (which returns audio-focused
// shape).
type ProbeHandlers struct {
	prober ports.VideoProber
}

func NewProbeHandlers(prober ports.VideoProber) *ProbeHandlers {
	return &ProbeHandlers{prober: prober}
}

func (h *ProbeHandlers) probe(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req probeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json: "+err.Error())
		return
	}
	if req.Path == "" {
		writeErr(w, http.StatusBadRequest, "path is empty")
		return
	}
	info, err := h.prober.Probe(r.Context(), req.Path)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, probeToResponse(info))
}
