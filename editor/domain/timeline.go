package domain

import (
	"errors"
	"fmt"
)

// Timeline ops are all pure functions over []Clip. They return a new slice
// and never mutate the input — callers use them to transition Project state.

// ErrClipNotFound is returned when an operation targets a clip id that is
// not present in the timeline.
var ErrClipNotFound = errors.New("clip not found")

const snapEpsilon = 1e-6

// Split divides the clip containing programTime into two clips at that
// point on the program timeline. The second half gets the given newID.
// Returns ErrClipNotFound if programTime lands in a gap or is on a clip
// boundary (splitting at a boundary would produce a zero-length clip).
func Split(clips []Clip, programTime float64, newID string) ([]Clip, error) {
	if newID == "" {
		return nil, errors.New("newID is empty")
	}
	if programTime < 0 {
		return nil, fmt.Errorf("programTime %v < 0", programTime)
	}
	idx, sourceAt, ok := clipAtProgramTime(clips, programTime)
	if !ok {
		return nil, ErrClipNotFound
	}
	left := clips[idx]
	if sourceAt-left.SourceStart < snapEpsilon || left.SourceEnd-sourceAt < snapEpsilon {
		return nil, fmt.Errorf("split point on clip boundary")
	}
	leftDur := sourceAt - left.SourceStart
	out := make([]Clip, 0, len(clips)+1)
	out = append(out, clips[:idx]...)
	out = append(out,
		Clip{ID: left.ID, SourceStart: left.SourceStart, SourceEnd: sourceAt, ProgramStart: left.ProgramStart},
		Clip{ID: newID, SourceStart: sourceAt, SourceEnd: left.SourceEnd, ProgramStart: left.ProgramStart + leftDur},
	)
	out = append(out, clips[idx+1:]...)
	return out, nil
}

// DeleteClip removes the clip with the given id, leaving subsequent clips
// to shift left on the program timeline.
func DeleteClip(clips []Clip, id string) ([]Clip, error) {
	idx := indexOf(clips, id)
	if idx < 0 {
		return nil, ErrClipNotFound
	}
	out := make([]Clip, 0, len(clips)-1)
	out = append(out, clips[:idx]...)
	out = append(out, clips[idx+1:]...)
	return out, nil
}

// Reorder moves the clip at fromIdx to toIdx, shifting the intermediate
// clips. Indices are 0-based and must be in [0, len).
func Reorder(clips []Clip, fromIdx, toIdx int) ([]Clip, error) {
	if fromIdx < 0 || fromIdx >= len(clips) {
		return nil, fmt.Errorf("fromIdx %d out of range", fromIdx)
	}
	if toIdx < 0 || toIdx >= len(clips) {
		return nil, fmt.Errorf("toIdx %d out of range", toIdx)
	}
	if fromIdx == toIdx {
		return append([]Clip(nil), clips...), nil
	}
	out := append([]Clip(nil), clips...)
	c := out[fromIdx]
	out = append(out[:fromIdx], out[fromIdx+1:]...)
	out = append(out[:toIdx], append([]Clip{c}, out[toIdx:]...)...)
	return out, nil
}

// TrimLeft updates the clip's sourceStart. The new value must be >= 0 and
// < current sourceEnd. ProgramStart moves by the same delta so the clip's
// right edge on the track stays put — intuitive trim-handle behaviour.
func TrimLeft(clips []Clip, id string, newSourceStart float64) ([]Clip, error) {
	idx := indexOf(clips, id)
	if idx < 0 {
		return nil, ErrClipNotFound
	}
	if newSourceStart < 0 {
		return nil, fmt.Errorf("newSourceStart < 0")
	}
	if newSourceStart >= clips[idx].SourceEnd {
		return nil, fmt.Errorf("newSourceStart >= sourceEnd")
	}
	out := append([]Clip(nil), clips...)
	delta := newSourceStart - out[idx].SourceStart
	out[idx].SourceStart = newSourceStart
	newProg := out[idx].ProgramStart + delta
	if newProg < 0 {
		newProg = 0
	}
	out[idx].ProgramStart = newProg
	return out, nil
}

// TrimRight updates the clip's sourceEnd. The new value must be > current
// sourceStart. Callers are responsible for clamping to Source.Duration.
func TrimRight(clips []Clip, id string, newSourceEnd float64) ([]Clip, error) {
	idx := indexOf(clips, id)
	if idx < 0 {
		return nil, ErrClipNotFound
	}
	if newSourceEnd <= clips[idx].SourceStart {
		return nil, fmt.Errorf("newSourceEnd <= sourceStart")
	}
	out := append([]Clip(nil), clips...)
	out[idx].SourceEnd = newSourceEnd
	return out, nil
}

func indexOf(clips []Clip, id string) int {
	for i, c := range clips {
		if c.ID == id {
			return i
		}
	}
	return -1
}

// clipAtProgramTime locates the clip containing the given program time and
// returns its index, the corresponding source time, and ok=true. Returns
// ok=false when t falls in a gap or outside the program range.
func clipAtProgramTime(clips []Clip, t float64) (idx int, sourceAt float64, ok bool) {
	for i, c := range clips {
		if t >= c.ProgramStart && t < c.ProgramEnd() {
			return i, c.SourceStart + (t - c.ProgramStart), true
		}
	}
	return 0, 0, false
}

// SetProgramStart updates the on-track position of a clip. Callers use this
// for drag-to-reposition; no overlap or ordering is enforced here — the UI
// enforces its own constraints (snap, clamp ≥ 0, etc.).
func SetProgramStart(clips []Clip, id string, newStart float64) ([]Clip, error) {
	idx := indexOf(clips, id)
	if idx < 0 {
		return nil, ErrClipNotFound
	}
	if newStart < 0 {
		newStart = 0
	}
	out := append([]Clip(nil), clips...)
	out[idx].ProgramStart = newStart
	return out, nil
}
