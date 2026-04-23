package domain

import (
	"testing"
	"time"
)

func TestNewProject(t *testing.T) {
	now := time.Date(2026, 4, 23, 14, 0, 0, 0, time.UTC)
	src := Source{Path: "in.mp4", Duration: 60, Width: 1920, Height: 1080, HasAudio: true}

	p := NewProject("abc12345", "Hello", src, now)

	if p.SchemaVersion != SchemaVersion {
		t.Errorf("SchemaVersion = %d, want %d", p.SchemaVersion, SchemaVersion)
	}
	if p.ID != "abc12345" || p.Name != "Hello" {
		t.Errorf("unexpected id/name: %q / %q", p.ID, p.Name)
	}
	if !p.CreatedAt.Equal(now) || !p.UpdatedAt.Equal(now) {
		t.Errorf("timestamps not seeded from now")
	}
	if len(p.VideoClips) != 1 {
		t.Fatalf("want 1 initial video clip, got %d", len(p.VideoClips))
	}
	if p.VideoClips[0].SourceStart != 0 || p.VideoClips[0].SourceEnd != 60 {
		t.Errorf("initial video clip should cover full source, got %v", p.VideoClips[0])
	}
	if len(p.AudioClips) != 1 {
		t.Fatalf("want 1 initial audio clip when HasAudio, got %d", len(p.AudioClips))
	}
	if p.Export.OutputName != "Hello" {
		t.Errorf("Export.OutputName = %q, want %q", p.Export.OutputName, "Hello")
	}
}

func TestNewProjectNoAudio(t *testing.T) {
	src := Source{Path: "in.mp4", Duration: 60, Width: 1920, Height: 1080, HasAudio: false}
	p := NewProject("id1", "noaudio", src, time.Now())
	if len(p.AudioClips) != 0 {
		t.Errorf("expected 0 audio clips when source has no audio, got %d", len(p.AudioClips))
	}
	if len(p.VideoClips) != 1 {
		t.Errorf("expected 1 video clip, got %d", len(p.VideoClips))
	}
}

func TestProgramDuration(t *testing.T) {
	cases := []struct {
		name  string
		video []Clip
		audio []Clip
		want  float64
	}{
		{"empty", nil, nil, 0},
		{"video only", []Clip{{SourceStart: 0, SourceEnd: 10}}, nil, 10},
		{"audio only", nil, []Clip{{SourceStart: 0, SourceEnd: 12}}, 12},
		{"video longer", []Clip{{SourceStart: 0, SourceEnd: 15}}, []Clip{{SourceStart: 0, SourceEnd: 8}}, 15},
		{"audio longer", []Clip{{SourceStart: 0, SourceEnd: 5}}, []Clip{{SourceStart: 0, SourceEnd: 20}}, 20},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p := &Project{VideoClips: c.video, AudioClips: c.audio}
			if got := p.ProgramDuration(); got != c.want {
				t.Errorf("got %v, want %v", got, c.want)
			}
		})
	}
}

func TestValidate(t *testing.T) {
	base := func() *Project {
		return &Project{
			ID:         "x",
			Source:     Source{Path: "a.mp4", Duration: 100, HasAudio: true},
			VideoClips: []Clip{{ID: "v1", SourceStart: 0, SourceEnd: 10}},
			AudioClips: []Clip{{ID: "a1", SourceStart: 0, SourceEnd: 10}},
		}
	}
	cases := []struct {
		name     string
		mutate   func(*Project)
		wantErrs int
	}{
		{"happy path", func(*Project) {}, 0},
		{"empty id", func(p *Project) { p.ID = "" }, 1},
		{"empty source path", func(p *Project) { p.Source.Path = "" }, 1},
		{"zero duration", func(p *Project) { p.Source.Duration = 0 }, 1},
		{"video clip missing id", func(p *Project) { p.VideoClips[0].ID = "" }, 1},
		{"audio clip missing id", func(p *Project) { p.AudioClips[0].ID = "" }, 1},
		{"video clip inverted times", func(p *Project) { p.VideoClips[0].SourceStart = 20 }, 1},
		{"audio clip past source end", func(p *Project) { p.AudioClips[0].SourceEnd = 200 }, 1},
		{"duplicate video id", func(p *Project) {
			p.VideoClips = append(p.VideoClips, Clip{ID: "v1", SourceStart: 20, SourceEnd: 30})
		}, 1},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p := base()
			c.mutate(p)
			got := p.Validate()
			if len(got) != c.wantErrs {
				t.Errorf("got %d errors, want %d: %v", len(got), c.wantErrs, got)
			}
		})
	}
}

func TestMigrateV1ToV2(t *testing.T) {
	// Simulate a v1 project loaded from disk: SchemaVersion=1, legacy Clips
	// set, VideoClips/AudioClips empty.
	p := &Project{
		SchemaVersion: 1,
		ID:            "old",
		Source:        Source{Path: "a.mp4", Duration: 50, HasAudio: true},
		LegacyClips: []Clip{
			{ID: "c1", SourceStart: 0, SourceEnd: 10},
			{ID: "c2", SourceStart: 20, SourceEnd: 30},
		},
	}
	p.Migrate()
	if p.SchemaVersion != SchemaVersion {
		t.Errorf("SchemaVersion not bumped: %d", p.SchemaVersion)
	}
	if len(p.VideoClips) != 2 || p.VideoClips[0].ID != "c1" {
		t.Errorf("VideoClips not populated from legacy: %v", p.VideoClips)
	}
	if len(p.AudioClips) != 2 || p.AudioClips[0].ID != "a1" {
		t.Errorf("AudioClips not populated when HasAudio: %v", p.AudioClips)
	}
	if p.LegacyClips != nil {
		t.Errorf("LegacyClips should be nil after migration, got %v", p.LegacyClips)
	}
}

func TestMigrateV1NoAudio(t *testing.T) {
	p := &Project{
		SchemaVersion: 1,
		Source:        Source{Path: "a.mp4", Duration: 50, HasAudio: false},
		LegacyClips:   []Clip{{ID: "c1", SourceStart: 0, SourceEnd: 10}},
	}
	p.Migrate()
	if len(p.VideoClips) != 1 {
		t.Errorf("VideoClips should be populated, got %v", p.VideoClips)
	}
	if len(p.AudioClips) != 0 {
		t.Errorf("AudioClips should stay empty when source has no audio, got %v", p.AudioClips)
	}
}

func TestMigrateIdempotent(t *testing.T) {
	p := &Project{
		SchemaVersion: SchemaVersion,
		VideoClips:    []Clip{{ID: "v1", SourceStart: 0, SourceEnd: 10}},
	}
	p.Migrate()
	if len(p.VideoClips) != 1 || p.VideoClips[0].ID != "v1" {
		t.Errorf("Migrate mutated an already-current project: %v", p.VideoClips)
	}
}
