// Package domain holds the multitrack editor's business logic. It composes
// the shared editing primitives in editor/common/domain (Clip,
// ExportSettings, ValidateClips) with multitrack-specific structures
// (multiple Sources, parallel VideoTrack/AudioTrack lists). Like the
// single-video editor, everything here is pure: no I/O, no globals.
package domain

import (
	"fmt"
	"time"

	common "easy-ffmpeg/editor/common/domain"
)

// SchemaVersion is the on-disk schema version for multitrack project JSON.
//
// v1: initial — Sources, VideoTracks, AudioTracks, AudioVolume, Export.
const SchemaVersion = 1

// Kind discriminator. Multitrack projects sit in a separate data directory
// from single-video projects, but the field guards against accidental
// cross-loading and lets future tooling distinguish them at a glance.
const KindMultitrack = "multitrack"

// ExportSettings is shared with single-video; aliased here so
// Project.Export keeps its current JSON shape across both editors.
type ExportSettings = common.ExportSettings

// Clip is defined in clip.go — multitrack extends common.Clip with a
// SourceID field so the same track can mix slices of different sources.

// SourceKind labels a source as carrying video (with optional audio) or
// audio only. Determines which track types can reference it.
const (
	SourceVideo = "video"
	SourceAudio = "audio"
)

// Source is one media file imported into the project. Multiple tracks may
// reference the same source by ID; clips on a track point at sub-ranges of
// the source they belong to (resolution rule lives in the export
// builder — clips don't carry the source id directly in v1, the track does).
type Source struct {
	ID         string  `json:"id"`
	Path       string  `json:"path"`
	Kind       string  `json:"kind"` // SourceVideo | SourceAudio
	Duration   float64 `json:"duration"`
	Width      int     `json:"width,omitempty"`
	Height     int     `json:"height,omitempty"`
	VideoCodec string  `json:"videoCodec,omitempty"`
	AudioCodec string  `json:"audioCodec,omitempty"`
	FrameRate  float64 `json:"frameRate,omitempty"`
	HasAudio   bool    `json:"hasAudio"`
}

// VideoTrack is a single row of video clips. Tracks render bottom-up at
// export: lower index = below, higher index = on top (overlay order).
type VideoTrack struct {
	ID     string `json:"id"`
	Locked bool   `json:"locked,omitempty"`
	Hidden bool   `json:"hidden,omitempty"`
	Clips  []Clip `json:"clips"`
}

// AudioTrack is a single row of audio clips. Tracks mix together with their
// own per-track volume on top of the project-level AudioVolume.
type AudioTrack struct {
	ID     string  `json:"id"`
	Locked bool    `json:"locked,omitempty"`
	Muted  bool    `json:"muted,omitempty"`
	Volume float64 `json:"volume"` // 0–2.0; 0 in JSON treated as unity by Migrate
	Clips  []Clip  `json:"clips"`
}

// Project is the single source of truth for one multitrack editing session.
// It is intentionally Kind-tagged so a single-video JSON file can never be
// mistaken for a multitrack one if they ever land in the same directory.
type Project struct {
	SchemaVersion int            `json:"schemaVersion"`
	Kind          string         `json:"kind"` // always KindMultitrack
	ID            string         `json:"id"`
	Name          string         `json:"name"`
	CreatedAt     time.Time      `json:"createdAt"`
	UpdatedAt     time.Time      `json:"updatedAt"`
	Sources       []Source       `json:"sources"`
	AudioVolume   float64        `json:"audioVolume,omitempty"` // 0–2.0; 0 → 1.0 by Migrate
	VideoTracks   []VideoTrack   `json:"videoTracks"`
	AudioTracks   []AudioTrack   `json:"audioTracks"`
	Export        ExportSettings `json:"export"`
}

// NewProject constructs a fresh, empty multitrack project. M5 only needs
// the empty case; later milestones add helpers to seed initial tracks
// when the user drags in the first source.
func NewProject(id, name string, now time.Time) *Project {
	return &Project{
		SchemaVersion: SchemaVersion,
		Kind:          KindMultitrack,
		ID:            id,
		Name:          name,
		CreatedAt:     now,
		UpdatedAt:     now,
		Sources:       []Source{},
		VideoTracks:   []VideoTrack{},
		AudioTracks:   []AudioTrack{},
		AudioVolume:   1.0,
		Export: ExportSettings{
			Format:     "mp4",
			VideoCodec: "h264",
			AudioCodec: "aac",
			OutputName: name,
		},
	}
}

// ProgramDuration is the longest track end across both video and audio.
// UI uses this for the timeline ruler and playhead range.
func (p *Project) ProgramDuration() float64 {
	var max float64
	for _, t := range p.VideoTracks {
		if d := common.TrackDuration(toCommonClips(t.Clips)); d > max {
			max = d
		}
	}
	for _, t := range p.AudioTracks {
		if d := common.TrackDuration(toCommonClips(t.Clips)); d > max {
			max = d
		}
	}
	return max
}

// Validate returns all invariant violations. Returning a slice (vs. first
// error) lets the UI surface the full list in one pass.
//
// Beyond per-clip checks delegated to common.ValidateClips, multitrack
// enforces:
//   - Kind == KindMultitrack
//   - Track ids unique within their kind
//   - Every clip has a non-empty SourceID that resolves in p.Sources
//   - Video-track clips must point at a SourceVideo; audio-track clips may
//     point at either kind (an audio track can mix audio-only sources with
//     audio extracted from video sources).
//   - Video tracks have no leading gap (program rule from product.md);
//     audio tracks may start with a gap (anullsrc fills it on export).
func (p *Project) Validate() []error {
	var errs []error
	if p.ID == "" {
		errs = append(errs, fmt.Errorf("project id is empty"))
	}
	if p.Kind != "" && p.Kind != KindMultitrack {
		errs = append(errs, fmt.Errorf("project kind = %q (want %q)", p.Kind, KindMultitrack))
	}

	// Build a quick id -> kind map for clip-source lookups.
	srcKind := make(map[string]string, len(p.Sources))
	seenSrc := map[string]bool{}
	for i, s := range p.Sources {
		if s.ID == "" {
			errs = append(errs, fmt.Errorf("sources[%d]: id is empty", i))
		}
		if seenSrc[s.ID] {
			errs = append(errs, fmt.Errorf("sources[%d]: duplicate id %q", i, s.ID))
		}
		seenSrc[s.ID] = true
		if s.Kind != SourceVideo && s.Kind != SourceAudio {
			errs = append(errs, fmt.Errorf("sources[%d]: invalid kind %q", i, s.Kind))
		}
		srcKind[s.ID] = s.Kind
	}

	seenVideo := map[string]bool{}
	for i, t := range p.VideoTracks {
		if t.ID == "" {
			errs = append(errs, fmt.Errorf("videoTracks[%d]: id is empty", i))
		}
		if seenVideo[t.ID] {
			errs = append(errs, fmt.Errorf("videoTracks[%d]: duplicate id %q", i, t.ID))
		}
		seenVideo[t.ID] = true
		shared := toCommonClips(t.Clips)
		errs = append(errs, common.ValidateClips(shared, fmt.Sprintf("videoTracks[%d]", i), 0)...)
		if len(shared) > 0 && common.EarliestProgramStart(shared) > common.SnapEpsilon {
			errs = append(errs, fmt.Errorf("videoTracks[%d]: leading gap not allowed on video", i))
		}
		for j, c := range t.Clips {
			if c.SourceID == "" {
				errs = append(errs, fmt.Errorf("videoTracks[%d][%d]: sourceId is empty", i, j))
				continue
			}
			k, ok := srcKind[c.SourceID]
			if !ok {
				errs = append(errs, fmt.Errorf("videoTracks[%d][%d]: sourceId %q not found in sources", i, j, c.SourceID))
				continue
			}
			if k != SourceVideo {
				errs = append(errs, fmt.Errorf("videoTracks[%d][%d]: sourceId %q is %s, video track requires video", i, j, c.SourceID, k))
			}
		}
	}

	seenAudio := map[string]bool{}
	for i, t := range p.AudioTracks {
		if t.ID == "" {
			errs = append(errs, fmt.Errorf("audioTracks[%d]: id is empty", i))
		}
		if seenAudio[t.ID] {
			errs = append(errs, fmt.Errorf("audioTracks[%d]: duplicate id %q", i, t.ID))
		}
		seenAudio[t.ID] = true
		errs = append(errs, common.ValidateClips(toCommonClips(t.Clips), fmt.Sprintf("audioTracks[%d]", i), 0)...)
		for j, c := range t.Clips {
			if c.SourceID == "" {
				errs = append(errs, fmt.Errorf("audioTracks[%d][%d]: sourceId is empty", i, j))
				continue
			}
			if _, ok := srcKind[c.SourceID]; !ok {
				errs = append(errs, fmt.Errorf("audioTracks[%d][%d]: sourceId %q not found in sources", i, j, c.SourceID))
			}
		}
	}
	return errs
}

// Migrate brings an on-disk multitrack project up to the current schema.
// Safe to call multiple times; a no-op on already-current projects.
//
// In v1 the only normalizations needed are defaults for fields that
// JSON-decode to zero values:
//   - AudioVolume <= 0 → 1.0 (unity)
//   - per-track Volume <= 0 → 1.0
//   - missing Kind → KindMultitrack (a file produced before Kind was added)
//   - nil track / sources slices → empty slices, so JSON re-encodes as []
//     not null (matches the wire shape the frontend expects).
func (p *Project) Migrate() {
	if p.Kind == "" {
		p.Kind = KindMultitrack
	}
	if p.AudioVolume <= 0 {
		p.AudioVolume = 1.0
	}
	if p.Sources == nil {
		p.Sources = []Source{}
	}
	if p.VideoTracks == nil {
		p.VideoTracks = []VideoTrack{}
	}
	if p.AudioTracks == nil {
		p.AudioTracks = []AudioTrack{}
	}
	for i := range p.AudioTracks {
		if p.AudioTracks[i].Volume <= 0 {
			p.AudioTracks[i].Volume = 1.0
		}
		if p.AudioTracks[i].Clips == nil {
			p.AudioTracks[i].Clips = []Clip{}
		}
	}
	for i := range p.VideoTracks {
		if p.VideoTracks[i].Clips == nil {
			p.VideoTracks[i].Clips = []Clip{}
		}
	}
	p.SchemaVersion = SchemaVersion
}
