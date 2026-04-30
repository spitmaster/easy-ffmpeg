package domain

import (
	common "easy-ffmpeg/editor/common/domain"
)

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
//	{ "id": "...", "sourceStart": ..., "sourceEnd": ..., "programStart": ..., "sourceId": "..." }
type Clip struct {
	common.Clip
	SourceID string `json:"sourceId"`
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
