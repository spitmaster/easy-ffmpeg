// Package domain holds the editor's pure business logic.
//
// This package MUST NOT import any other easy-ffmpeg package, nor any
// third-party library. All I/O (files, network, ffmpeg subprocess) lives
// outside — domain types and functions are pure and directly unit-testable.
package domain

import (
	"fmt"
	"time"
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
	AudioVolume   float64        `json:"audioVolume,omitempty"`
	Export        ExportSettings `json:"export"`

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

// Clip is a sub-range of the source positioned on its track at ProgramStart.
// Clips no longer need to be contiguous — gaps between clips are preserved
// and render as black video / silent audio on export.
type Clip struct {
	ID           string  `json:"id"`
	SourceStart  float64 `json:"sourceStart"`  // seconds into the source, inclusive
	SourceEnd    float64 `json:"sourceEnd"`    // seconds into the source, exclusive
	ProgramStart float64 `json:"programStart"` // seconds on the track timeline
}

// Duration returns the clip's duration in seconds (on both source and track).
func (c Clip) Duration() float64 { return c.SourceEnd - c.SourceStart }

// ProgramEnd returns the clip's track end time.
func (c Clip) ProgramEnd() float64 { return c.ProgramStart + c.Duration() }

// ExportSettings carry the user's export preferences. Persisted alongside
// the project so next export starts with the same choices.
type ExportSettings struct {
	Format     string `json:"format"`
	VideoCodec string `json:"videoCodec"`
	AudioCodec string `json:"audioCodec"`
	OutputDir  string `json:"outputDir"`
	OutputName string `json:"outputName"`
}

// trackDuration returns the track's program length: the largest ProgramEnd
// across its clips. A track with a single clip at ProgramStart=5 lasting 10s
// is 15s long (not 10s) — the leading gap counts.
func trackDuration(clips []Clip) float64 {
	var max float64
	for _, c := range clips {
		if e := c.ProgramEnd(); e > max {
			max = e
		}
	}
	return max
}

// VideoDuration / AudioDuration give the program length of each track in
// isolation. They can diverge after independent edits — ffmpeg pads or
// truncates at export according to ffmpeg's own rules.
func (p *Project) VideoDuration() float64 { return trackDuration(p.VideoClips) }
func (p *Project) AudioDuration() float64 { return trackDuration(p.AudioClips) }

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
	errs = append(errs, validateClips(p.VideoClips, "video", p.Source.Duration)...)
	errs = append(errs, validateClips(p.AudioClips, "audio", p.Source.Duration)...)
	return errs
}

func validateClips(clips []Clip, label string, sourceDuration float64) []error {
	var errs []error
	seen := map[string]bool{}
	for i, c := range clips {
		if c.ID == "" {
			errs = append(errs, fmt.Errorf("%s[%d]: id is empty", label, i))
		}
		if seen[c.ID] {
			errs = append(errs, fmt.Errorf("%s[%d]: duplicate id %q", label, i, c.ID))
		}
		seen[c.ID] = true
		if c.SourceStart < 0 {
			errs = append(errs, fmt.Errorf("%s[%d]: sourceStart < 0", label, i))
		}
		if c.SourceEnd <= c.SourceStart {
			errs = append(errs, fmt.Errorf("%s[%d]: sourceEnd must be > sourceStart", label, i))
		}
		if sourceDuration > 0 && c.SourceEnd > sourceDuration+1e-6 {
			errs = append(errs, fmt.Errorf("%s[%d]: sourceEnd > source.duration", label, i))
		}
		if c.ProgramStart < 0 {
			errs = append(errs, fmt.Errorf("%s[%d]: programStart < 0", label, i))
		}
	}
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
