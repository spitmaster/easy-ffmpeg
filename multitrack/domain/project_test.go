package domain

import (
	"testing"
	"time"
)

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
		ID: "v1",
		Clips: []Clip{
			{ID: "c1", SourceStart: 0, SourceEnd: 5, ProgramStart: 1.0}, // leading gap
		},
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
		Clips: []Clip{
			{ID: "c1", SourceStart: 0, SourceEnd: 5, ProgramStart: 2.0},
		},
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

func TestProgramDurationTakesMaxAcrossTracks(t *testing.T) {
	p := NewProject("p1", "Demo", time.Now())
	p.VideoTracks = []VideoTrack{{
		ID: "v1",
		Clips: []Clip{
			{ID: "c1", SourceStart: 0, SourceEnd: 5, ProgramStart: 0},
		},
	}}
	p.AudioTracks = []AudioTrack{{
		ID:     "a1",
		Volume: 1.0,
		Clips: []Clip{
			{ID: "c1", SourceStart: 0, SourceEnd: 8, ProgramStart: 0},
		},
	}}
	if d := p.ProgramDuration(); d != 8 {
		t.Fatalf("ProgramDuration = %v, want 8 (max)", d)
	}
}

func TestMigrateNormalizesZeroAudioVolume(t *testing.T) {
	p := &Project{
		ID:         "p1",
		Kind:       "", // legacy file without kind
		Sources:    nil,
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

func containsLeadingGap(s string) bool {
	for i := 0; i+len("leading gap") <= len(s); i++ {
		if s[i:i+len("leading gap")] == "leading gap" {
			return true
		}
	}
	return false
}
