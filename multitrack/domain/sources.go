package domain

import (
	"errors"
	"fmt"
)

// ErrSourceInUse is returned by RemoveSource when at least one clip still
// references the source. Callers compare with errors.Is.
var ErrSourceInUse = errors.New("source is still referenced by a clip")

// ErrSourceNotFound is returned by RemoveSource when the id does not match
// any source in the project. Callers compare with errors.Is.
var ErrSourceNotFound = errors.New("source not found")

// AddSource returns a copy of p with src appended. Replaces an existing
// entry if the id matches — useful for re-probing the same path after the
// file changed on disk. The returned Project is a fresh value; the caller
// must persist it (the api layer calls repo.Save). Sources is the only
// slice copied; tracks are shared because nothing here mutates them.
func AddSource(p *Project, src Source) *Project {
	out := *p
	out.Sources = make([]Source, 0, len(p.Sources)+1)
	replaced := false
	for _, s := range p.Sources {
		if s.ID == src.ID {
			out.Sources = append(out.Sources, src)
			replaced = true
			continue
		}
		out.Sources = append(out.Sources, s)
	}
	if !replaced {
		out.Sources = append(out.Sources, src)
	}
	return &out
}

// RemoveSource returns a copy of p with the source removed. Returns
// ErrSourceNotFound if the id is not present, or ErrSourceInUse if any
// clip still references it (the UI is expected to confirm + cascade
// before calling, or surface the error).
func RemoveSource(p *Project, sourceID string) (*Project, error) {
	idx := -1
	for i, s := range p.Sources {
		if s.ID == sourceID {
			idx = i
			break
		}
	}
	if idx < 0 {
		return nil, fmt.Errorf("%w: %q", ErrSourceNotFound, sourceID)
	}
	for _, t := range p.VideoTracks {
		for _, c := range t.Clips {
			if c.SourceID == sourceID {
				return nil, fmt.Errorf("%w: %q", ErrSourceInUse, sourceID)
			}
		}
	}
	for _, t := range p.AudioTracks {
		for _, c := range t.Clips {
			if c.SourceID == sourceID {
				return nil, fmt.Errorf("%w: %q", ErrSourceInUse, sourceID)
			}
		}
	}
	out := *p
	out.Sources = make([]Source, 0, len(p.Sources)-1)
	out.Sources = append(out.Sources, p.Sources[:idx]...)
	out.Sources = append(out.Sources, p.Sources[idx+1:]...)
	return &out, nil
}

// AddVideoTrack appends an empty video track and returns the new project
// plus the new track id. ID is derived from the position in the slice
// (`v1`, `v2`, …) — callers don't need to invent ids and the value stays
// stable across saves.
func AddVideoTrack(p *Project) (*Project, string) {
	out := *p
	id := nextTrackID("v", len(p.VideoTracks)+1, func(s string) bool {
		for _, t := range p.VideoTracks {
			if t.ID == s {
				return true
			}
		}
		return false
	})
	out.VideoTracks = append([]VideoTrack(nil), p.VideoTracks...)
	out.VideoTracks = append(out.VideoTracks, VideoTrack{ID: id, Clips: []Clip{}})
	return &out, id
}

// AddAudioTrack appends an empty audio track at unity volume and returns
// the new project plus the new track id.
func AddAudioTrack(p *Project) (*Project, string) {
	out := *p
	id := nextTrackID("a", len(p.AudioTracks)+1, func(s string) bool {
		for _, t := range p.AudioTracks {
			if t.ID == s {
				return true
			}
		}
		return false
	})
	out.AudioTracks = append([]AudioTrack(nil), p.AudioTracks...)
	out.AudioTracks = append(out.AudioTracks, AudioTrack{ID: id, Volume: 1.0, Clips: []Clip{}})
	return &out, id
}

// nextTrackID picks the smallest <prefix><n> not already taken. Callers
// pass a starting hint (typically len+1) and a "taken" predicate so we
// don't cap at the slice length — gaps from earlier deletions are fine
// to leave behind.
func nextTrackID(prefix string, start int, taken func(string) bool) string {
	for n := start; ; n++ {
		id := fmt.Sprintf("%s%d", prefix, n)
		if !taken(id) {
			return id
		}
	}
}
