package domain

import (
	common "easy-ffmpeg/editor/common/domain"
)

// Transform places a video clip on the project canvas (v0.5.1+). Coordinates
// and dimensions are integer pixels in canvas space; the source frame is
// scaled to (W, H) and laid down with its top-left at (X, Y). Out-of-bounds
// values are allowed (animation in/out semantics; UI flags them).
//
// Audio clips ignore Transform — its zero value still serializes, which is
// harmless and keeps a single Clip type usable across video/audio tracks.
type Transform struct {
	X int `json:"x"`
	Y int `json:"y"`
	W int `json:"w"` // > 0 (Validate enforces)
	H int `json:"h"` // > 0
}

// Clip is the multitrack-specific clip extension. The shared
// editor/common/domain.Clip is source-agnostic — single-video resolves the
// source from Project.Source, but multitrack clips on the same track may
// originate from different sources, so the source id has to live on the
// clip itself.
//
// Embedding common.Clip lets multitrack reuse every shared helper
// (Duration / ProgramEnd, ValidateClips, BuildVideoTrackFilter, …) without
// reimplementation: a slice copy via toCommonClips produces a []common.Clip
// the shared functions accept. Promoted fields keep field access
// (`clip.SourceStart`) transparent. JSON shape:
//
//	{ "id": "...", "sourceStart": ..., "sourceEnd": ..., "programStart": ...,
//	  "sourceId": "...", "transform": { "x": 0, "y": 0, "w": 1920, "h": 1080 } }
type Clip struct {
	common.Clip
	SourceID  string    `json:"sourceId"`
	Transform Transform `json:"transform"`
}

// toCommonClips copies a multitrack clip slice into a plain common.Clip
// slice for handing to shared helpers (validation, filter graph builders).
// Cheap because Clip is a small value type.
func toCommonClips(in []Clip) []common.Clip {
	out := make([]common.Clip, len(in))
	for i, c := range in {
		out[i] = c.Clip
	}
	return out
}
