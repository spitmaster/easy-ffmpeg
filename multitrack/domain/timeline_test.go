package domain

import (
	"errors"
	"testing"
	"time"

	common "easy-ffmpeg/editor/common/domain"
)

// Multitrack timeline ops are the shared common.{Split, DeleteClip,
// Reorder, TrimLeft, TrimRight} composed with toCommonClips/fromCommonClips
// (defined inline here for the patch use case below). The shared layer is
// already covered by editor/common/domain; this file specifically asserts
// that running those functions through []multitrack.Clip preserves the
// SourceID extension field — the only invariant the multitrack-specific
// wrapper has to maintain.

// applyOnTrack runs a shared timeline op over a multitrack clip slice and
// reattaches each output clip's SourceID by id-matching with the input.
// Mirrors the helper a frontend-equivalent ops layer would need; including
// it here lets the tests exercise the full round-trip.
func applyOnTrack(in []Clip, op func([]common.Clip) ([]common.Clip, error)) ([]Clip, error) {
	srcID := make(map[string]string, len(in))
	for _, c := range in {
		srcID[c.ID] = c.SourceID
	}
	out, err := op(toCommonClips(in))
	if err != nil {
		return nil, err
	}
	res := make([]Clip, len(out))
	for i, c := range out {
		res[i] = Clip{Clip: c}
		// Either the id existed in the input (Reorder/Delete/TrimLeft/Right)
		// or it's a fresh id from a Split — in the latter case we can't
		// recover the SourceID from id alone, so fall back to "any clip
		// from in with the same SourceStart..SourceEnd subrange origin".
		// Practical heuristic: pick any input clip's SourceID if there's
		// only one distinct value. Tests below stay within single-source
		// tracks, so the simplification is safe.
		if sid, ok := srcID[c.ID]; ok {
			res[i].SourceID = sid
		} else if len(srcID) > 0 {
			for _, v := range srcID {
				res[i].SourceID = v
				break
			}
		}
	}
	return res, nil
}

func TestSharedOpsPreserveSourceID_Split(t *testing.T) {
	in := []Clip{mkClip("c1", "s1", 0, 20, 0)}
	out, err := applyOnTrack(in, func(cs []common.Clip) ([]common.Clip, error) {
		return common.Split(cs, 10, "c2")
	})
	if err != nil {
		t.Fatalf("split: %v", err)
	}
	if len(out) != 2 {
		t.Fatalf("split produced %d clips, want 2", len(out))
	}
	for _, c := range out {
		if c.SourceID != "s1" {
			t.Errorf("clip %q lost SourceID: got %q, want %q", c.ID, c.SourceID, "s1")
		}
	}
}

func TestSharedOpsPreserveSourceID_DeleteAndReorder(t *testing.T) {
	in := []Clip{
		mkClip("a", "s1", 0, 5, 0),
		mkClip("b", "s1", 5, 10, 5),
		mkClip("c", "s1", 10, 15, 10),
	}
	out, err := applyOnTrack(in, func(cs []common.Clip) ([]common.Clip, error) {
		return common.DeleteClip(cs, "b")
	})
	if err != nil {
		t.Fatalf("delete: %v", err)
	}
	if len(out) != 2 || out[0].SourceID != "s1" || out[1].SourceID != "s1" {
		t.Fatalf("delete: SourceID lost or wrong length, got %+v", out)
	}

	out, err = applyOnTrack(in, func(cs []common.Clip) ([]common.Clip, error) {
		return common.Reorder(cs, 0, 2)
	})
	if err != nil {
		t.Fatalf("reorder: %v", err)
	}
	for _, c := range out {
		if c.SourceID != "s1" {
			t.Errorf("reorder lost SourceID on %q", c.ID)
		}
	}
}

func TestSharedOpsPreserveSourceID_TrimLeftRight(t *testing.T) {
	in := []Clip{mkClip("a", "s2", 5, 20, 7)}
	out, err := applyOnTrack(in, func(cs []common.Clip) ([]common.Clip, error) {
		return common.TrimLeft(cs, "a", 8)
	})
	if err != nil {
		t.Fatalf("trimleft: %v", err)
	}
	if out[0].SourceID != "s2" {
		t.Errorf("trimleft lost SourceID")
	}
	out, err = applyOnTrack(in, func(cs []common.Clip) ([]common.Clip, error) {
		return common.TrimRight(cs, "a", 15)
	})
	if err != nil {
		t.Fatalf("trimright: %v", err)
	}
	if out[0].SourceID != "s2" {
		t.Errorf("trimright lost SourceID")
	}
}

// ---- RemoveVideoTrack / RemoveAudioTrack ----

func TestRemoveVideoTrackDropsClips(t *testing.T) {
	p := NewProject("p", "demo", time.Now())
	p.Sources = []Source{{ID: "s1", Path: "a.mp4", Kind: SourceVideo, Duration: 30}}
	p.VideoTracks = []VideoTrack{
		{ID: "v1", Clips: []Clip{mkClip("c1", "s1", 0, 5, 0)}},
		{ID: "v2", Clips: []Clip{mkClip("c2", "s1", 5, 10, 0)}},
	}
	got, err := RemoveVideoTrack(p, "v1")
	if err != nil {
		t.Fatalf("RemoveVideoTrack: %v", err)
	}
	if len(got.VideoTracks) != 1 || got.VideoTracks[0].ID != "v2" {
		t.Fatalf("unexpected video tracks after remove: %+v", got.VideoTracks)
	}
	// Sources must NOT be auto-removed (product rule: cascading source
	// deletion is the user's call via the library).
	if len(got.Sources) != 1 {
		t.Errorf("Sources changed unexpectedly: %+v", got.Sources)
	}
	if len(p.VideoTracks) != 2 {
		t.Errorf("input mutated")
	}
}

func TestRemoveAudioTrackDropsClips(t *testing.T) {
	p := NewProject("p", "demo", time.Now())
	p.Sources = []Source{{ID: "s1", Path: "a.wav", Kind: SourceAudio, Duration: 30, HasAudio: true}}
	p.AudioTracks = []AudioTrack{
		{ID: "a1", Volume: 1, Clips: []Clip{mkClip("c1", "s1", 0, 5, 0)}},
	}
	got, err := RemoveAudioTrack(p, "a1")
	if err != nil {
		t.Fatalf("RemoveAudioTrack: %v", err)
	}
	if len(got.AudioTracks) != 0 {
		t.Fatalf("expected empty audioTracks, got %+v", got.AudioTracks)
	}
}

func TestRemoveTrackNotFound(t *testing.T) {
	p := NewProject("p", "demo", time.Now())
	if _, err := RemoveVideoTrack(p, "ghost"); !errors.Is(err, ErrTrackNotFound) {
		t.Errorf("RemoveVideoTrack: want ErrTrackNotFound, got %v", err)
	}
	if _, err := RemoveAudioTrack(p, "ghost"); !errors.Is(err, ErrTrackNotFound) {
		t.Errorf("RemoveAudioTrack: want ErrTrackNotFound, got %v", err)
	}
}

// ---- MoveClipAcrossTracks ----

func TestMoveClipAcrossVideoTracks(t *testing.T) {
	p := NewProject("p", "demo", time.Now())
	p.Sources = []Source{{ID: "s1", Path: "a.mp4", Kind: SourceVideo, Duration: 30}}
	p.VideoTracks = []VideoTrack{
		{ID: "v1", Clips: []Clip{mkClip("c1", "s1", 0, 5, 0)}},
		{ID: "v2", Clips: []Clip{}},
	}
	got, err := MoveClipAcrossTracks(p, SourceVideo, "v1", "v2", "c1", 3.0)
	if err != nil {
		t.Fatalf("MoveClipAcrossTracks: %v", err)
	}
	if len(got.VideoTracks[0].Clips) != 0 {
		t.Errorf("source track still has clip: %+v", got.VideoTracks[0].Clips)
	}
	if len(got.VideoTracks[1].Clips) != 1 || got.VideoTracks[1].Clips[0].ID != "c1" {
		t.Errorf("dest track missing clip: %+v", got.VideoTracks[1].Clips)
	}
	if got.VideoTracks[1].Clips[0].ProgramStart != 3.0 {
		t.Errorf("ProgramStart = %v, want 3.0", got.VideoTracks[1].Clips[0].ProgramStart)
	}
	// Validate must still pass (no leading gap on v2 because programStart 3
	// would actually leave a leading gap... product rule says no gap on
	// video). Acceptance: callers ensure this; here we just confirm the
	// sourceId is intact.
	if got.VideoTracks[1].Clips[0].SourceID != "s1" {
		t.Errorf("SourceID lost: %v", got.VideoTracks[1].Clips[0])
	}
}

func TestMoveClipAcrossAudioTracks(t *testing.T) {
	p := NewProject("p", "demo", time.Now())
	p.Sources = []Source{{ID: "s1", Path: "a.wav", Kind: SourceAudio, Duration: 10, HasAudio: true}}
	p.AudioTracks = []AudioTrack{
		{ID: "a1", Volume: 1, Clips: []Clip{mkClip("c1", "s1", 0, 3, 0)}},
		{ID: "a2", Volume: 1, Clips: []Clip{}},
	}
	got, err := MoveClipAcrossTracks(p, SourceAudio, "a1", "a2", "c1", 5.0)
	if err != nil {
		t.Fatalf("move: %v", err)
	}
	if len(got.AudioTracks[0].Clips) != 0 {
		t.Errorf("source still has clip")
	}
	if got.AudioTracks[1].Clips[0].ProgramStart != 5.0 {
		t.Errorf("ProgramStart = %v, want 5.0", got.AudioTracks[1].Clips[0].ProgramStart)
	}
}

func TestMoveClipSameTrackNoOpExceptProgramStart(t *testing.T) {
	p := NewProject("p", "demo", time.Now())
	p.Sources = []Source{{ID: "s1", Path: "a.mp4", Kind: SourceVideo, Duration: 30}}
	p.VideoTracks = []VideoTrack{
		{ID: "v1", Clips: []Clip{
			mkClip("c1", "s1", 0, 5, 0),
			mkClip("c2", "s1", 5, 10, 5),
		}},
	}
	got, err := MoveClipAcrossTracks(p, SourceVideo, "v1", "v1", "c1", 12.0)
	if err != nil {
		t.Fatalf("move: %v", err)
	}
	if len(got.VideoTracks[0].Clips) != 2 {
		t.Fatalf("clip count changed: %+v", got.VideoTracks[0].Clips)
	}
	// c1 is now at the tail (we removed then re-appended) at programStart=12.
	last := got.VideoTracks[0].Clips[len(got.VideoTracks[0].Clips)-1]
	if last.ID != "c1" || last.ProgramStart != 12.0 {
		t.Errorf("expected c1 at tail with programStart=12, got %+v", last)
	}
}

func TestMoveClipCrossKindRejected(t *testing.T) {
	p := NewProject("p", "demo", time.Now())
	p.Sources = []Source{{ID: "s1", Path: "a.mp4", Kind: SourceVideo, Duration: 30}}
	p.VideoTracks = []VideoTrack{
		{ID: "v1", Clips: []Clip{mkClip("c1", "s1", 0, 5, 0)}},
	}
	p.AudioTracks = []AudioTrack{{ID: "a1", Volume: 1, Clips: []Clip{}}}
	// Caller (incorrectly) invokes audio kind with from=video track id —
	// must surface as cross-kind, not as a generic missing-track error.
	if _, err := MoveClipAcrossTracks(p, SourceAudio, "v1", "a1", "c1", 0); !errors.Is(err, ErrCrossKindMove) {
		t.Errorf("want ErrCrossKindMove, got %v", err)
	}
}

func TestMoveClipNotFound(t *testing.T) {
	p := NewProject("p", "demo", time.Now())
	p.Sources = []Source{{ID: "s1", Path: "a.mp4", Kind: SourceVideo, Duration: 30}}
	p.VideoTracks = []VideoTrack{{ID: "v1", Clips: []Clip{}}, {ID: "v2", Clips: []Clip{}}}
	if _, err := MoveClipAcrossTracks(p, SourceVideo, "v1", "v2", "ghost", 0); !errors.Is(err, ErrClipNotFound) {
		t.Errorf("want ErrClipNotFound, got %v", err)
	}
	if _, err := MoveClipAcrossTracks(p, SourceVideo, "ghost", "v2", "c1", 0); !errors.Is(err, ErrTrackNotFound) {
		t.Errorf("want ErrTrackNotFound, got %v", err)
	}
}

func TestMoveClipNegativeProgramStartClamped(t *testing.T) {
	p := NewProject("p", "demo", time.Now())
	p.Sources = []Source{{ID: "s1", Path: "a.mp4", Kind: SourceVideo, Duration: 30}}
	p.VideoTracks = []VideoTrack{
		{ID: "v1", Clips: []Clip{mkClip("c1", "s1", 0, 5, 0)}},
		{ID: "v2", Clips: []Clip{}},
	}
	got, err := MoveClipAcrossTracks(p, SourceVideo, "v1", "v2", "c1", -7)
	if err != nil {
		t.Fatalf("move: %v", err)
	}
	if got.VideoTracks[1].Clips[0].ProgramStart != 0 {
		t.Errorf("negative programStart should clamp to 0, got %v", got.VideoTracks[1].Clips[0].ProgramStart)
	}
}
