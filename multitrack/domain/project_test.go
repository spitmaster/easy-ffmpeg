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

// TestMigrateV1ToV2FillsCanvasAndTransform covers the v0.5.1 schema bump:
// a v1-shaped project (zero Canvas, zero per-clip Transform) must come out
// of Migrate with sensible defaults — Canvas derived from referenced video
// sources and every video clip's Transform filled to the full canvas.
func TestMigrateV1ToV2FillsCanvasAndTransform(t *testing.T) {
	p := &Project{
		ID:            "legacy",
		Kind:          KindMultitrack,
		SchemaVersion: 1,
		Sources: []Source{
			{ID: "s1", Path: "a.mp4", Kind: SourceVideo, Duration: 30, Width: 1920, Height: 1080, FrameRate: 24, HasAudio: true},
			{ID: "s2", Path: "b.mp4", Kind: SourceVideo, Duration: 30, Width: 1280, Height: 720, FrameRate: 30, HasAudio: true},
		},
		VideoTracks: []VideoTrack{
			{
				ID: "v1",
				Clips: []Clip{
					{Clip: common.Clip{ID: "c1", SourceStart: 0, SourceEnd: 5, ProgramStart: 0}, SourceID: "s1"},
					{Clip: common.Clip{ID: "c2", SourceStart: 0, SourceEnd: 5, ProgramStart: 5}, SourceID: "s2"},
				},
			},
		},
	}
	p.Migrate()
	// Canvas = max width × max height @ max frame rate across referenced video sources.
	want := Canvas{Width: 1920, Height: 1080, FrameRate: 30}
	if p.Canvas != want {
		t.Errorf("Canvas after Migrate = %+v, want %+v", p.Canvas, want)
	}
	for _, c := range p.VideoTracks[0].Clips {
		if c.Transform != (Transform{X: 0, Y: 0, W: 1920, H: 1080}) {
			t.Errorf("clip %q Transform = %+v, want full-canvas", c.ID, c.Transform)
		}
	}
	if p.SchemaVersion != SchemaVersion {
		t.Errorf("SchemaVersion = %d, want %d", p.SchemaVersion, SchemaVersion)
	}
}

// TestMigrateV1ToV2NoVideoSourcesUsesDefaultCanvas: when no video clips
// exist (audio-only project) Migrate falls back to 1920×1080@30.
func TestMigrateV1ToV2NoVideoSourcesUsesDefaultCanvas(t *testing.T) {
	p := &Project{
		ID: "audio-only", Kind: KindMultitrack, SchemaVersion: 1,
		Sources: []Source{{ID: "a1", Path: "a.wav", Kind: SourceAudio, Duration: 30, HasAudio: true}},
		AudioTracks: []AudioTrack{{
			ID: "at1", Volume: 1.0,
			Clips: []Clip{{Clip: common.Clip{ID: "c1", SourceStart: 0, SourceEnd: 10, ProgramStart: 0}, SourceID: "a1"}},
		}},
	}
	p.Migrate()
	if p.Canvas != (Canvas{Width: 1920, Height: 1080, FrameRate: 30}) {
		t.Errorf("audio-only Migrate Canvas = %+v, want default 1920x1080@30", p.Canvas)
	}
}

// TestMigrateIsIdempotent: running Migrate twice doesn't break a v0.5.1
// project — the second pass sees already-filled Canvas / Transform values
// and leaves them alone.
func TestMigrateIsIdempotent(t *testing.T) {
	p := NewProject("p1", "Demo", time.Now())
	p.Sources = []Source{{ID: "s1", Path: "a.mp4", Kind: SourceVideo, Duration: 10, Width: 1280, Height: 720, FrameRate: 24, HasAudio: true}}
	// Pre-set a non-default canvas + custom-transform clip to make sure
	// Migrate doesn't overwrite them.
	p.Canvas = Canvas{Width: 1280, Height: 720, FrameRate: 24}
	p.VideoTracks = []VideoTrack{{
		ID: "v1",
		Clips: []Clip{{
			Clip:      common.Clip{ID: "c1", SourceStart: 0, SourceEnd: 5, ProgramStart: 0},
			SourceID:  "s1",
			Transform: Transform{X: 10, Y: 20, W: 640, H: 360},
		}},
	}}
	p.Migrate()
	p.Migrate()
	if p.Canvas != (Canvas{Width: 1280, Height: 720, FrameRate: 24}) {
		t.Errorf("Migrate overwrote canvas: %+v", p.Canvas)
	}
	if got := p.VideoTracks[0].Clips[0].Transform; got != (Transform{X: 10, Y: 20, W: 640, H: 360}) {
		t.Errorf("Migrate overwrote clip transform: %+v", got)
	}
}

// TestValidateRejectsTinyCanvas: Validate enforces W/H ≥ 16. A 4×4 canvas
// is too small to be useful and ffmpeg's color filter would still accept
// it, so the domain layer is the right place to surface the error.
func TestValidateRejectsTinyCanvas(t *testing.T) {
	p := NewProject("p1", "Demo", time.Now())
	p.Canvas = Canvas{Width: 4, Height: 4, FrameRate: 30}
	errs := p.Validate()
	if len(errs) == 0 {
		t.Fatalf("expected canvas-too-small error")
	}
	found := false
	for _, e := range errs {
		if contains(e.Error(), "太小") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected '太小' in error, got %v", errs)
	}
}

// TestValidateRejectsCanvasFrameRateOutOfRange: FR must be in (0, 240].
// Zero/negative FR breaks the base canvas duration computation; >240 fps
// is a clear user error (we cap at common cinematic ceiling).
func TestValidateRejectsCanvasFrameRateOutOfRange(t *testing.T) {
	for _, fr := range []float64{0, -1, 241, 1000} {
		p := NewProject("p1", "Demo", time.Now())
		p.Canvas.FrameRate = fr
		errs := p.Validate()
		if len(errs) == 0 {
			t.Errorf("fr=%v should error", fr)
		}
	}
}

// TestValidateRejectsClipWithNonPositiveTransform: video clips must carry
// a positive W and H (Migrate fills defaults; the only way this fires is
// via direct mutation in tests or programmatic edits).
func TestValidateRejectsClipWithNonPositiveTransform(t *testing.T) {
	p := NewProject("p1", "Demo", time.Now())
	p.Sources = []Source{{ID: "s1", Path: "a.mp4", Kind: SourceVideo, Duration: 10}}
	p.VideoTracks = []VideoTrack{{
		ID: "v1",
		Clips: []Clip{{
			Clip:     common.Clip{ID: "c1", SourceStart: 0, SourceEnd: 5, ProgramStart: 0},
			SourceID: "s1",
			// Transform deliberately zero.
		}},
	}}
	errs := p.Validate()
	if len(errs) == 0 {
		t.Fatalf("zero Transform should error")
	}
	found := false
	for _, e := range errs {
		if contains(e.Error(), "transform W/H") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected transform W/H error, got %v", errs)
	}
}

// TestValidateAllowsTransformOutOfBounds: a clip whose transform falls
// outside the canvas is intentionally legal (animation in/out semantics).
// The UI annotates these clips, but Validate does not reject them.
func TestValidateAllowsTransformOutOfBounds(t *testing.T) {
	p := NewProject("p1", "Demo", time.Now())
	p.Sources = []Source{{ID: "s1", Path: "a.mp4", Kind: SourceVideo, Duration: 10}}
	p.VideoTracks = []VideoTrack{{
		ID: "v1",
		Clips: []Clip{{
			Clip:      common.Clip{ID: "c1", SourceStart: 0, SourceEnd: 5, ProgramStart: 0},
			SourceID:  "s1",
			Transform: Transform{X: 5000, Y: -200, W: 200, H: 200}, // far OOB
		}},
	}}
	for _, e := range p.Validate() {
		if contains(e.Error(), "transform") {
			t.Errorf("Validate rejected OOB transform: %v", e)
		}
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
