package api

import "easy-ffmpeg/multitrack/domain"

// createProjectRequest is the JSON body for POST /projects. Multitrack
// creates *empty* projects (no source) — sources are imported separately
// via POST /projects/:id/sources (M6+).
type createProjectRequest struct {
	Name string `json:"name"`
}

// importSourcesRequest is the JSON body for POST /projects/:id/sources.
// Each path is probed with ffprobe, converted to a domain.Source, and
// appended (or replaced if a source with the same path already exists).
type importSourcesRequest struct {
	Paths []string `json:"paths"`
}

// importSourcesResponse returns just the newly added (or refreshed)
// sources, plus the updated project so the frontend can swap state in
// one round-trip without an extra GET.
type importSourcesResponse struct {
	Sources []domain.Source   `json:"sources"`
	Project *domain.Project   `json:"project"`
	Errors  []importErrorItem `json:"errors,omitempty"`
}

// importErrorItem is one path that failed to probe. The handler returns
// 200 even when some paths fail so the user gets partial success
// information instead of an all-or-nothing 500.
type importErrorItem struct {
	Path  string `json:"path"`
	Error string `json:"error"`
}

// exportRequest is the JSON body for POST /export. Mirrors the editor
// shape — same overwrite-confirmation flow, same dryRun semantics — so
// the frontend client and the modals/showOverwrite path stay uniform
// across both editors.
type exportRequest struct {
	ProjectID string                 `json:"projectId"`
	Export    *domain.ExportSettings `json:"export"`    // optional override; if nil, use project.Export
	Overwrite bool                   `json:"overwrite"` // false + existing file → 409
	DryRun    bool                   `json:"dryRun"`    // true → return the would-be command without running ffmpeg
}
