package domain

import (
	"errors"
	"fmt"

	common "easy-ffmpeg/editor/common/domain"
)

// ErrTrackNotFound is returned when an op targets a track id that does not
// exist on the project. Callers compare with errors.Is.
var ErrTrackNotFound = errors.New("track not found")

// ErrCrossKindMove is returned when MoveClipAcrossTracks is asked to move a
// clip between video and audio tracks. The product rule (multitrack
// product.md §5) is that cross-kind moves are forbidden — the caller is
// expected to surface this as a `not-allowed` cursor and silently drop the
// drop, but the domain still surfaces a typed error so tests can assert it.
var ErrCrossKindMove = errors.New("cannot move clip across video/audio kinds")

// ErrClipNotFound is re-exported from editor/common so multitrack callers
// can compare errors without two imports. Same value as common.ErrClipNotFound.
var ErrClipNotFound = common.ErrClipNotFound

// RemoveVideoTrack returns a copy of p with the named video track and all
// its clips removed. Sources referenced by removed clips are *not* removed
// from p.Sources — the user can later remove them from the library
// explicitly (RemoveSource is the gatekeeper there). Returns
// ErrTrackNotFound when the id is not present.
func RemoveVideoTrack(p *Project, trackID string) (*Project, error) {
	idx := -1
	for i, t := range p.VideoTracks {
		if t.ID == trackID {
			idx = i
			break
		}
	}
	if idx < 0 {
		return nil, fmt.Errorf("%w: %q", ErrTrackNotFound, trackID)
	}
	out := *p
	out.VideoTracks = make([]VideoTrack, 0, len(p.VideoTracks)-1)
	out.VideoTracks = append(out.VideoTracks, p.VideoTracks[:idx]...)
	out.VideoTracks = append(out.VideoTracks, p.VideoTracks[idx+1:]...)
	return &out, nil
}

// RemoveAudioTrack returns a copy of p with the named audio track removed.
// See RemoveVideoTrack for source-retention rationale.
func RemoveAudioTrack(p *Project, trackID string) (*Project, error) {
	idx := -1
	for i, t := range p.AudioTracks {
		if t.ID == trackID {
			idx = i
			break
		}
	}
	if idx < 0 {
		return nil, fmt.Errorf("%w: %q", ErrTrackNotFound, trackID)
	}
	out := *p
	out.AudioTracks = make([]AudioTrack, 0, len(p.AudioTracks)-1)
	out.AudioTracks = append(out.AudioTracks, p.AudioTracks[:idx]...)
	out.AudioTracks = append(out.AudioTracks, p.AudioTracks[idx+1:]...)
	return &out, nil
}

// MoveClipAcrossTracks moves the clip with id clipID from a track of the
// given kind to another track of the same kind, parking it at
// newProgramStart on the destination track. Same-track moves
// (fromTrackID == toTrackID) reduce to a SetProgramStart equivalent.
//
// Returns ErrCrossKindMove if the move would cross the video/audio
// boundary — the product rule disallows it (§5: a video clip in an audio
// row would have no picture, and vice versa). Cross-kind is rejected even
// when fromKind matches toKind formally but the destination track id
// belongs to the other kind list, so callers don't have to pre-validate.
//
// Source-id invariants from Validate are preserved automatically because
// we never touch the clip's SourceID here. newProgramStart < 0 is clamped
// to 0, matching common.SetProgramStart.
func MoveClipAcrossTracks(
	p *Project,
	kind string,
	fromTrackID, toTrackID, clipID string,
	newProgramStart float64,
) (*Project, error) {
	if kind != SourceVideo && kind != SourceAudio {
		return nil, fmt.Errorf("invalid kind %q (want %q or %q)", kind, SourceVideo, SourceAudio)
	}
	if newProgramStart < 0 {
		newProgramStart = 0
	}
	out := *p

	if kind == SourceVideo {
		fromIdx, toIdx := findTrackIdx(p.VideoTracks, fromTrackID), findTrackIdx(p.VideoTracks, toTrackID)
		if fromIdx < 0 || toIdx < 0 {
			return nil, fmt.Errorf("%w: from=%q to=%q", ErrTrackNotFound, fromTrackID, toTrackID)
		}
		from := p.VideoTracks[fromIdx]
		clipIdx := -1
		for i, c := range from.Clips {
			if c.ID == clipID {
				clipIdx = i
				break
			}
		}
		if clipIdx < 0 {
			return nil, fmt.Errorf("%w (clip %q on track %q)", ErrClipNotFound, clipID, fromTrackID)
		}
		moving := from.Clips[clipIdx]
		moving.ProgramStart = newProgramStart

		out.VideoTracks = make([]VideoTrack, len(p.VideoTracks))
		copy(out.VideoTracks, p.VideoTracks)
		// Build the new from/to clip slices; same-track move: drop and
		// re-append within the same slice.
		newFromClips := make([]Clip, 0, len(from.Clips)-1)
		newFromClips = append(newFromClips, from.Clips[:clipIdx]...)
		newFromClips = append(newFromClips, from.Clips[clipIdx+1:]...)
		out.VideoTracks[fromIdx] = VideoTrack{
			ID: from.ID, Locked: from.Locked, Hidden: from.Hidden, Clips: newFromClips,
		}
		// Re-read the destination from `out` in case from == to (we just
		// overwrote the from entry above; same-track move appends to the
		// already-shortened slice).
		to := out.VideoTracks[toIdx]
		newToClips := append(append([]Clip(nil), to.Clips...), moving)
		out.VideoTracks[toIdx] = VideoTrack{
			ID: to.ID, Locked: to.Locked, Hidden: to.Hidden, Clips: newToClips,
		}
		return &out, nil
	}

	// Audio kind.
	fromIdx, toIdx := findAudioTrackIdx(p.AudioTracks, fromTrackID), findAudioTrackIdx(p.AudioTracks, toTrackID)
	if fromIdx < 0 || toIdx < 0 {
		// One leg may live on the video list — that's a cross-kind move.
		if findTrackIdx(p.VideoTracks, fromTrackID) >= 0 || findTrackIdx(p.VideoTracks, toTrackID) >= 0 {
			return nil, fmt.Errorf("%w: from=%q to=%q", ErrCrossKindMove, fromTrackID, toTrackID)
		}
		return nil, fmt.Errorf("%w: from=%q to=%q", ErrTrackNotFound, fromTrackID, toTrackID)
	}
	from := p.AudioTracks[fromIdx]
	clipIdx := -1
	for i, c := range from.Clips {
		if c.ID == clipID {
			clipIdx = i
			break
		}
	}
	if clipIdx < 0 {
		return nil, fmt.Errorf("%w (clip %q on track %q)", ErrClipNotFound, clipID, fromTrackID)
	}
	moving := from.Clips[clipIdx]
	moving.ProgramStart = newProgramStart

	out.AudioTracks = make([]AudioTrack, len(p.AudioTracks))
	copy(out.AudioTracks, p.AudioTracks)
	newFromClips := make([]Clip, 0, len(from.Clips)-1)
	newFromClips = append(newFromClips, from.Clips[:clipIdx]...)
	newFromClips = append(newFromClips, from.Clips[clipIdx+1:]...)
	out.AudioTracks[fromIdx] = AudioTrack{
		ID: from.ID, Locked: from.Locked, Muted: from.Muted, Volume: from.Volume, Clips: newFromClips,
	}
	to := out.AudioTracks[toIdx]
	newToClips := append(append([]Clip(nil), to.Clips...), moving)
	out.AudioTracks[toIdx] = AudioTrack{
		ID: to.ID, Locked: to.Locked, Muted: to.Muted, Volume: to.Volume, Clips: newToClips,
	}
	return &out, nil
}

func findTrackIdx(ts []VideoTrack, id string) int {
	for i, t := range ts {
		if t.ID == id {
			return i
		}
	}
	return -1
}

func findAudioTrackIdx(ts []AudioTrack, id string) int {
	for i, t := range ts {
		if t.ID == id {
			return i
		}
	}
	return -1
}
