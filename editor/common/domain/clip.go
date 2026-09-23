// Package domain holds the shared editing primitives used by both the
// single-video editor (editor/domain/) and the multitrack editor
// (multitrack/domain/, future). Everything here is pure: no I/O, no
// globals, no dependencies on other easy-ffmpeg packages, no third-party
// libraries beyond the standard library.
//
// Boundary rule: a type or function belongs here only if it is meaningful
// for *both* editors as-is. Anything that mentions Project / Source /
// Material / Migrate stays in the editor-specific package.
package domain

import (
	"errors"
	"fmt"
)

// SnapEpsilon is the slack used by clip-boundary checks. Values within
// SnapEpsilon of a boundary are treated as on the boundary (and rejected
// for splits). Floats accumulate noise across edits, so an exact equality
// test is too strict.
const SnapEpsilon = 1e-6

// ErrClipNotFound is returned when an operation targets a clip id that is
// not present in the timeline, or a program time that falls in a gap.
// Callers compare with errors.Is.
var ErrClipNotFound = errors.New("clip not found")

// Clip is a sub-range of some source positioned on its track at
// ProgramStart. The source the clip belongs to is implicit at the
// editor-specific level — single-video has one source, multitrack
// resolves it via TrackData.SourceID. Clips on a track may have gaps
// between them; gaps render as black video / silent audio on export.
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

// TrackDuration returns the track's program length: the largest ProgramEnd
// across its clips. A track with a single clip at ProgramStart=5 lasting 10s
// is 15s long (not 10s) — the leading gap counts.
func TrackDuration(clips []Clip) float64 {
	var max float64
	for _, c := range clips {
		if e := c.ProgramEnd(); e > max {
			max = e
		}
	}
	return max
}

// EarliestProgramStart returns the smallest ProgramStart in the slice, or
// 0 for an empty track. Callers use it to detect leading gaps.
func EarliestProgramStart(clips []Clip) float64 {
	if len(clips) == 0 {
		return 0
	}
	min := clips[0].ProgramStart
	for _, c := range clips[1:] {
		if c.ProgramStart < min {
			min = c.ProgramStart
		}
	}
	return min
}

// ValidateClips returns invariant violations for a list of clips. The
// label is interpolated into error messages so multiple track validations
// in one Project.Validate produce distinguishable errors. When
// sourceDuration > 0 the function also enforces SourceEnd <= sourceDuration
// (with SnapEpsilon slack); pass 0 to skip — multitrack uses a per-clip
// source duration that is checked separately.
func ValidateClips(clips []Clip, label string, sourceDuration float64) []error {
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
		if sourceDuration > 0 && c.SourceEnd > sourceDuration+SnapEpsilon {
			errs = append(errs, fmt.Errorf("%s[%d]: sourceEnd > source.duration", label, i))
		}
		if c.ProgramStart < 0 {
			errs = append(errs, fmt.Errorf("%s[%d]: programStart < 0", label, i))
		}
	}
	return errs
}
