package api

import (
	"crypto/rand"
	"encoding/hex"
	"time"

	"easy-ffmpeg/editor/domain"
	"easy-ffmpeg/editor/ports"
)

// createProjectRequest is the JSON body for POST /projects.
type createProjectRequest struct {
	SourcePath string `json:"sourcePath"`
	Name       string `json:"name"` // optional
}

// exportRequest is the JSON body for POST /export.
type exportRequest struct {
	ProjectID string                 `json:"projectId"`
	Export    *domain.ExportSettings `json:"export"`    // optional override; if nil, use project.Export
	Overwrite bool                   `json:"overwrite"` // if false and outputPath exists, server returns 409
	// DryRun returns the would-be command without starting ffmpeg or
	// checking overwrite. Front-end uses it to populate the
	// pre-execution confirmation dialog.
	DryRun bool `json:"dryRun"`
}

// probeRequest is the JSON body for POST /probe.
type probeRequest struct {
	Path string `json:"path"`
}

// probeResponse mirrors ports.VideoInfo but is shaped for the frontend.
type probeResponse struct {
	Duration   float64 `json:"duration"`
	Width      int     `json:"width"`
	Height     int     `json:"height"`
	VideoCodec string  `json:"videoCodec"`
	AudioCodec string  `json:"audioCodec"`
	FrameRate  float64 `json:"frameRate"`
	HasAudio   bool    `json:"hasAudio"`
}

func probeToResponse(v *ports.VideoInfo) probeResponse {
	if v == nil {
		return probeResponse{}
	}
	return probeResponse{
		Duration:   v.Duration,
		Width:      v.Width,
		Height:     v.Height,
		VideoCodec: v.VideoCodec,
		AudioCodec: v.AudioCodec,
		FrameRate:  v.FrameRate,
		HasAudio:   v.HasAudio,
	}
}

func probeToSource(path string, v *ports.VideoInfo) domain.Source {
	if v == nil {
		return domain.Source{Path: path}
	}
	return domain.Source{
		Path:       path,
		Duration:   v.Duration,
		Width:      v.Width,
		Height:     v.Height,
		VideoCodec: v.VideoCodec,
		AudioCodec: v.AudioCodec,
		FrameRate:  v.FrameRate,
		HasAudio:   v.HasAudio,
	}
}

// newID returns a random 8-hex id for a project or other entity.
func newID() string {
	var b [4]byte
	if _, err := rand.Read(b[:]); err != nil {
		// fall back to time-based id if crypto/rand ever fails
		return "id" + time.Now().UTC().Format("150405")
	}
	return hex.EncodeToString(b[:])
}
