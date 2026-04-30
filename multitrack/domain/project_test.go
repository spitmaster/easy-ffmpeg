package domain

import (
	"errors"
	"testing"
	"time"

	common "easy-ffmpeg/editor/common/domain"
)

// mkClip is a small helper so the test files don't have to spell out the
// embedded common.Clip every time.
func mkClip(id, sourceID string, start, end, prog float64) Clip {
	return Clip{
		Clip:     common.Clip{ID: id, SourceStart: start, SourceEnd: end, ProgramStart: prog},
		SourceID: sourceID,
	}
}

func TestNewProjectIsValid(t *testing.T) {
	now := time.Date(2026, 4, 30, 10, 0, 0, 0, time.UTC)
	p := NewProject("p1", "Demo", now)
	if p.SchemaVersion != SchemaVersion {
		t.Fatalf("schemaVersion = %d, want %d", p.SchemaVersion, SchemaVersion)
	}
	if p.Kind != KindMultitrack {
		t.Fatalf("kind = %q, want %q", p.Kind, KindMultitrack)
	}
	if p.AudioVolume != 1.0 {
		t.Fatalf("AudioVolume = %v, want 1.0", p.AudioVolume)
	}
	if errs := p.Validate(); len(errs) != 0 {
		t.Fatalf("validate empty project failed: %v", errs)
	}
	if p.ProgramDuration() != 0 {
		t.Fatalf("ProgramDuration on empty project = %v, want 0", p.ProgramDuration())
	}
	// JSON-friendly slices: must be non-nil so they encode as [].
	if p.Sources == nil || p.VideoTracks == nil || p.AudioTracks == nil {
		t.Fatalf("nil slice in fresh project")
	}
}

func TestValidateRejectsLeadingVideoGap(t *testing.T) {
	now := time.Now()
	p := NewProject("p1", "Demo", now)
	p.Sources = []Source{{ID: "s1", Path: "a.mp4", Kind: SourceVideo, Duration: 10}}
	p.VideoTracks = []VideoTrack{{
		ID:    "v1",
		Clips: []Clip{mkClip("c1", "s1", 0, 5, 1.0)}, // leading gap
	}}
	errs := p.Validate()
	found := false
	for _, e := range errs {
		if msg := e.Error(); msg != "" && containsLeadingGap(msg) {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected leading-gap error, got %v", errs)
	}
}

func TestValidateAllowsLeadingAudioGap(t *testing.T) {
	now := time.Now()
	p := NewProject("p1", "Demo", now)
	p.Sources = []Source{{ID: "s1", Path: "a.wav", Kind: SourceAudio, Duration: 10, HasAudio: true}}
	p.AudioTracks = []AudioTrack{{
		ID:     "a1",
		Volume: 1.0,
		Clips:  []Clip{mkClip("c1", "s1", 0, 5, 2.0)},
	}}
	if errs := p.Validate(); len(errs) != 0 {
		t.Fatalf("audio leading gap should be allowed, got %v", errs)
	}
}

func TestValidateDuplicateTrackIDs(t *testing.T) {
	now := time.Now()
	p := NewProject("p1", "Demo", now)
	p.VideoTracks = []VideoTrack{{ID: "dup"}, {ID: "dup"}}
	p.Migrate()
	errs := p.Validate()
	if len(errs) == 0 {
		t.Fatalf("expected duplicate-id error, got none")
	}
}

func TestValidateRejectsClipWithMissingSourceID(t *testing.T) {
	p := NewProject("p1", "Demo", time.Now())
	p.Sources = []Source{{ID: "s1", Path: "a.mp4", Kind: SourceVideo, Duration: 10}}
	p.VideoTracks = []VideoTrack{{
		ID:    "v1",
		Clips: []Clip{mkClip("c1", "", 0, 5, 0)}, // empty SourceID
	}}
	errs := p.Validate()
	if len(errs) == 0 {
		t.Fatalf("expected error for empty sourceId")
	}
}

func TestValidateRejectsClipWithUnknownSourceID(t *testing.T) {
	p := NewProject("p1", "Demo", time.Now())
	p.Sources = []Source{{ID: "s1", Path: "a.mp4", Kind: SourceVideo, Duration: 10}}
	p.VideoTracks = []VideoTrack{{
		ID:    "v1",
		Clips: []Clip{mkClip("c1", "ghost", 0, 5, 0)}, // not in Sources
	}}
	errs := p.Validate()
	found := false
	for _, e := range errs {
		if msg := e.Error(); msg != "" && containsUnknownSource(msg) {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected unknown-source error, got %v", errs)
	}
}

func TestValidateRejectsAudioSourceOnVideoTrack(t *testing.T) {
	p := NewProject("p1", "Demo", time.Now())
	p.Sources = []Source{{ID: "a1", Path: "a.mp3", Kind: SourceAudio, Duration: 10, HasAudio: true}}
	p.VideoTracks = []VideoTrack{{
		ID:    "v1",
		Clips: []Clip{mkClip("c1", "a1", 0, 5, 0)}, // audio source on video track
	}}
	errs := p.Validate()
	if len(errs) == 0 {
		t.Fatalf("expected error: audio source on video track")
	}
}

func TestProgramDurationTakesMaxAcrossTracks(t *testing.T) {
	p := NewProject("p1", "Demo", time.Now())
	p.Sources = []Source{
		{ID: "s1", Path: "a.mp4", Kind: SourceVideo, Duration: 20},
		{ID: "s2", Path: "b.mp3", Kind: SourceAudio, Duration: 20, HasAudio: true},
	}
	p.VideoTracks = []VideoTrack{{
		ID:    "v1",
		Clips: []Clip{mkClip("c1", "s1", 0, 5, 0)},
	}}
	p.AudioTracks = []AudioTrack{{
		ID:     "a1",
		Volume: 1.0,
		Clips:  []Clip{mkClip("c1", "s2", 0, 8, 0)},
	}}
	if d := p.ProgramDuration(); d != 8 {
		t.Fatalf("ProgramDuration = %v, want 8 (max)", d)
	}
}

func TestMigrateNormalizesZeroAudioVolume(t *testing.T) {
	p := &Project{
		ID:          "p1",
		Kind:        "", // legacy file without kind
		Sources:     nil,
		VideoTracks: []VideoTrack{{ID: "v1", Clips: nil}},
		AudioTracks: []AudioTrack{{ID: "a1", Volume: 0, Clips: nil}},
	}
	p.Migrate()
	if p.Kind != KindMultitrack {
		t.Fatalf("Migrate didn't set Kind: %q", p.Kind)
	}
	if p.AudioVolume != 1.0 {
		t.Fatalf("Migrate didn't set AudioVolume: %v", p.AudioVolume)
	}
	if p.AudioTracks[0].Volume != 1.0 {
		t.Fatalf("Migrate didn't set track Volume: %v", p.AudioTracks[0].Volume)
	}
	if p.Sources == nil || p.VideoTracks[0].Clips == nil || p.AudioTracks[0].Clips == nil {
		t.Fatalf("Migrate left nil slices that should be empty")
	}
	if p.SchemaVersion != SchemaVersion {
		t.Fatalf("SchemaVersion = %d, want %d", p.SchemaVersion, SchemaVersion)
	}
}

func TestMigrateRejectsForeignKind(t *testing.T) {
	// Migrate doesn't enforce — Validate does. Make sure a wrong-Kind
	// file at least surfaces as an error from Validate.
	p := &Project{ID: "p1", Kind: "single-video"}
	p.Migrate()
	errs := p.Validate()
	if len(errs) == 0 {
		t.Fatalf("expected kind mismatch error")
	}
}

func TestAddSourceAppendsAndReplaces(t *testing.T) {
	p := NewProject("p1", "Demo", time.Now())
	p1 := AddSource(p, Source{ID: "s1", Path: "a.mp4", Kind: SourceVideo, Duration: 10})
	if len(p1.Sources) != 1 || p1.Sources[0].Duration != 10 {
		t.Fatalf("AddSource append: got %+v", p1.Sources)
	}
	// Same id, new duration → replace, not append.
	p2 := AddSource(p1, Source{ID: "s1", Path: "a.mp4", Kind: SourceVideo, Duration: 99})
	if len(p2.Sources) != 1 || p2.Sources[0].Duration != 99 {
		t.Fatalf("AddSource replace: got %+v", p2.Sources)
	}
	// Original p1 must not be mutated.
	if p1.Sources[0].Duration != 10 {
		t.Fatalf("AddSource mutated input: p1=%+v", p1.Sources)
	}
}

func TestRemoveSourceRejectsInUse(t *testing.T) {
	p := NewProject("p1", "Demo", time.Now())
	p.Sources = []Source{{ID: "s1", Path: "a.mp4", Kind: SourceVideo, Duration: 10}}
	p.VideoTracks = []VideoTrack{{
		ID:    "v1",
		Clips: []Clip{mkClip("c1", "s1", 0, 5, 0)},
	}}
	if _, err := RemoveSource(p, "s1"); !errors.Is(err, ErrSourceInUse) {
		t.Fatalf("RemoveSource: want ErrSourceInUse, got %v", err)
	}
}

func TestRemoveSourceNotFound(t *testing.T) {
	p := NewProject("p1", "Demo", time.Now())
	if _, err := RemoveSource(p, "ghost"); !errors.Is(err, ErrSourceNotFound) {
		t.Fatalf("RemoveSource: want ErrSourceNotFound, got %v", err)
	}
}

func TestRemoveSourceSucceedsWhenUnreferenced(t *testing.T) {
	p := NewProject("p1", "Demo", time.Now())
	p.Sources = []Source{
		{ID: "s1", Path: "a.mp4", Kind: SourceVideo, Duration: 10},
		{ID: "s2", Path: "b.mp4", Kind: SourceVideo, Duration: 5},
	}
	p2, err := RemoveSource(p, "s1")
	if err != nil {
		t.Fatalf("RemoveSource: %v", err)
	}
	if len(p2.Sources) != 1 || p2.Sources[0].ID != "s2" {
		t.Fatalf("RemoveSource: got %+v", p2.Sources)
	}
	// Input must not be mutated.
	if len(p.Sources) != 2 {
		t.Fatalf("RemoveSource mutated input")
	}
}

func TestAddVideoTrackAppendsWithFreshID(t *testing.T) {
	p := NewProject("p1", "Demo", time.Now())
	p, id1 := AddVideoTrack(p)
	if id1 == "" || len(p.VideoTracks) != 1 || p.VideoTracks[0].ID != id1 {
		t.Fatalf("AddVideoTrack first: id=%q tracks=%+v", id1, p.VideoTracks)
	}
	p, id2 := AddVideoTrack(p)
	if id2 == id1 {
		t.Fatalf("AddVideoTrack didn't produce a fresh id: %q", id2)
	}
	if len(p.VideoTracks) != 2 {
		t.Fatalf("AddVideoTrack didn't append: %+v", p.VideoTracks)
	}
	if p.VideoTracks[1].Clips == nil {
		t.Fatalf("AddVideoTrack: new track has nil Clips slice")
	}
}

func TestAddAudioTrackUsesUnityVolume(t *testing.T) {
	p := NewProject("p1", "Demo", time.Now())
	p, id := AddAudioTrack(p)
	if id == "" {
		t.Fatalf("AddAudioTrack returned empty id")
	}
	if got := p.AudioTracks[0].Volume; got != 1.0 {
		t.Fatalf("AddAudioTrack volume = %v, want 1.0", got)
	}
}

func containsLeadingGap(s string) bool {
	return contains(s, "leading gap")
}

func containsUnknownSource(s string) bool {
	return contains(s, "not found in sources")
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
