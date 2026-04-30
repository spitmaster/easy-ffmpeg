// Package domain holds the single-video editor's business logic. The
// pure clip-level primitives (Clip / Split / TrimLeft / planSegments /
// single-track filter graph builders) live in editor/common/domain;
// this package re-exports them as type aliases so existing callers and
// JSON wire-formats stay byte-for-byte identical, and adds the
// single-video-only Project / Source / Migrate machinery on top.
package domain

import (
	"fmt"
	"time"

	commondomain "easy-ffmpeg/editor/common/domain"
)

// SchemaVersion is the current on-disk schema version for Project JSON.
//
// v1: single Clips []Clip covering both video and audio together
// v2: split into VideoClips + AudioClips so each track is independently
//     edited (split / trim / reorder / delete).
// v3: Clip gains ProgramStart — clips now have an explicit position on the
//     track instead of being stacked in array order, enabling gaps and
//     free placement. Migration auto-fills ProgramStart by accumulation.
const SchemaVersion = 3

// Track identifiers used across domain, api, and UI layers.
const (
	TrackVideo = "video"
	TrackAudio = "audio"
)

// Clip is the shared clip primitive. Aliased here so editor/domain.Clip
// keeps working at every existing call site (handlers, storage, tests)
// while the actual definition lives once in editor/common/domain.
type Clip = commondomain.Clip

// ExportSettings is shared with multitrack and lives in common; aliased
// here so domain.Project.Export keeps its current JSON shape.
type ExportSettings = commondomain.ExportSettings

// Project is the single source of truth for one editing session.
type Project struct {
	SchemaVersion int            `json:"schemaVersion"`
	ID            string         `json:"id"`
	Name          string         `json:"name"`
	CreatedAt     time.Time      `json:"createdAt"`
	UpdatedAt     time.Time      `json:"updatedAt"`
	Source        Source         `json:"source"`
	VideoClips    []Clip         `json:"videoClips,omitempty"`
	AudioClips    []Clip         `json:"audioClips,omitempty"`
	// AudioVolume is a linear gain applied to the entire audio track
	// (0.0 = silent, 1.0 = unity). Drives both preview playback and
	// export. 0 in JSON means "missing" — Migrate() upgrades it to 1.0.
	AudioVolume float64        `json:"audioVolume,omitempty"`
	Export      ExportSettings `json:"export"`

	// LegacyClips is a v1 field kept for migration only. When the repo
	// reads a v1 file this slice carries the old data; Migrate() copies
	// it into VideoClips/AudioClips and nils it out before the first save.
	LegacyClips []Clip `json:"clips,omitempty"`
}

// Source describes the single video file the project is editing.
type Source struct {
	Path       string  `json:"path"`
	Duration   float64 `json:"duration"` // seconds
	Width      int     `json:"width"`
	Height     int     `json:"height"`
	VideoCodec string  `json:"videoCodec"`
	AudioCodec string  `json:"audioCodec"`
	FrameRate  float64 `json:"frameRate"`
	HasAudio   bool    `json:"hasAudio"`
}

// VideoDuration / AudioDuration give the program length of each track in
// isolation. They can diverge after independent edits — ffmpeg pads or
// truncates at export according to ffmpeg's own rules.
func (p *Project) VideoDuration() float64 { return commondomain.TrackDuration(p.VideoClips) }
func (p *Project) AudioDuration() float64 { return commondomain.TrackDuration(p.AudioClips) }

// ProgramDuration is the length of the composite program: the longer of
// the two tracks. UI uses this for the timeline ruler and playhead range.
func (p *Project) ProgramDuration() float64 {
	v, a := p.VideoDuration(), p.AudioDuration()
	if v >= a {
		return v
	}
	return a
}

// Validate returns all invariant violations. Returning a slice (vs. first
// error) lets the UI surface the full list so the user can fix everything.
func (p *Project) Validate() []error {
	var errs []error
	if p.ID == "" {
		errs = append(errs, fmt.Errorf("project id is empty"))
	}
	if p.Source.Path == "" {
		errs = append(errs, fmt.Errorf("source.path is empty"))
	}
	if p.Source.Duration <= 0 {
		errs = append(errs, fmt.Errorf("source.duration must be > 0"))
	}
	errs = append(errs, commondomain.ValidateClips(p.VideoClips, "video", p.Source.Duration)...)
	errs = append(errs, commondomain.ValidateClips(p.AudioClips, "audio", p.Source.Duration)...)
	return errs
}

// NewProject constructs a fresh project covering the entire source as one
// clip per track. The caller provides id / name / now, so NewProject stays
// pure (no globals, no time.Now()).
func NewProject(id, name string, src Source, now time.Time) *Project {
	p := &Project{
		SchemaVersion: SchemaVersion,
		ID:            id,
		Name:          name,
		CreatedAt:     now,
		UpdatedAt:     now,
		Source:        src,
		VideoClips: []Clip{
			{ID: "v1", SourceStart: 0, SourceEnd: src.Duration, ProgramStart: 0},
		},
		Export: ExportSettings{
			Format:     "mp4",
			VideoCodec: "h264",
			AudioCodec: "aac",
			OutputName: name,
		},
		AudioVolume: 1.0,
	}
	if src.HasAudio {
		p.AudioClips = []Clip{
			{ID: "a1", SourceStart: 0, SourceEnd: src.Duration, ProgramStart: 0},
		}
	}
	return p
}

// Migrate brings an on-disk project up to the current schema.
// Safe to call multiple times; a no-op on already-current projects.
//
// v1 → v2: the single Clips slice (now LegacyClips for decode) is
// duplicated into both VideoClips and AudioClips (if the source has
// audio). Audio clip ids are derived by prefixing "a" so both tracks
// stay unique if later merged.
//
// v2 → v3: Clip.ProgramStart is filled in by accumulation from 0, so old
// stacked-clip projects render identically to before.
//
// AudioVolume default: missing (0 from JSON unmarshal) → 1.0 (unity gain).
// Done unconditionally so even already-v3 projects without the field get
// upgraded transparently.
func (p *Project) Migrate() {
	if p.AudioVolume <= 0 {
		p.AudioVolume = 1.0
	}
	if p.SchemaVersion >= SchemaVersion {
		p.LegacyClips = nil
		return
	}
	// v1 → v2 shape: move legacy Clips into VideoClips/AudioClips.
	if len(p.VideoClips) == 0 && len(p.LegacyClips) > 0 {
		p.VideoClips = append([]Clip(nil), p.LegacyClips...)
		if p.Source.HasAudio && len(p.AudioClips) == 0 {
			p.AudioClips = make([]Clip, len(p.LegacyClips))
			for i, c := range p.LegacyClips {
				p.AudioClips[i] = Clip{
					ID:          fmt.Sprintf("a%d", i+1),
					SourceStart: c.SourceStart,
					SourceEnd:   c.SourceEnd,
				}
			}
		}
	}
	p.LegacyClips = nil
	// v2 → v3: fill ProgramStart by accumulation so old projects look the
	// same on the timeline. We trust any nonzero ProgramStart already there
	// (a belt-and-braces move: a half-migrated file won't be clobbered).
	fillProgramStarts(p.VideoClips)
	fillProgramStarts(p.AudioClips)
	p.SchemaVersion = SchemaVersion
}

func fillProgramStarts(clips []Clip) {
	if len(clips) == 0 {
		return
	}
	// If any clip already has a nonzero ProgramStart, assume the file has
	// already been through the v3 migration (or the user explicitly placed
	// gaps) and leave everything alone.
	for _, c := range clips {
		if c.ProgramStart > 0 {
			return
		}
	}
	var acc float64
	for i := range clips {
		clips[i].ProgramStart = acc
		acc += clips[i].Duration()
	}
}
