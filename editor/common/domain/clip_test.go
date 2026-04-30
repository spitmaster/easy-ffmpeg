package domain

import "testing"

func TestClipDurationProgramEnd(t *testing.T) {
	c := Clip{SourceStart: 5, SourceEnd: 12, ProgramStart: 30}
	if c.Duration() != 7 {
		t.Errorf("Duration = %v, want 7", c.Duration())
	}
	if c.ProgramEnd() != 37 {
		t.Errorf("ProgramEnd = %v, want 37", c.ProgramEnd())
	}
}

func TestTrackDuration(t *testing.T) {
	cases := []struct {
		name string
		in   []Clip
		want float64
	}{
		{"empty", nil, 0},
		{"single contig", []Clip{{SourceStart: 0, SourceEnd: 10}}, 10},
		{"with gap", []Clip{
			{SourceStart: 0, SourceEnd: 5, ProgramStart: 0},
			{SourceStart: 0, SourceEnd: 5, ProgramStart: 20},
		}, 25},
		{"out of order", []Clip{
			{SourceStart: 0, SourceEnd: 5, ProgramStart: 20},
			{SourceStart: 0, SourceEnd: 5, ProgramStart: 0},
		}, 25},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := TrackDuration(c.in); got != c.want {
				t.Errorf("got %v, want %v", got, c.want)
			}
		})
	}
}

func TestEarliestProgramStart(t *testing.T) {
	if EarliestProgramStart(nil) != 0 {
		t.Error("empty should return 0")
	}
	clips := []Clip{
		{ProgramStart: 5},
		{ProgramStart: 1.5},
		{ProgramStart: 10},
	}
	if got := EarliestProgramStart(clips); got != 1.5 {
		t.Errorf("got %v, want 1.5", got)
	}
}

func TestValidateClips(t *testing.T) {
	cases := []struct {
		name           string
		clips          []Clip
		sourceDuration float64
		wantErrs       int
	}{
		{"happy", []Clip{{ID: "a", SourceStart: 0, SourceEnd: 10}}, 100, 0},
		{"missing id", []Clip{{ID: "", SourceStart: 0, SourceEnd: 10}}, 100, 1},
		{"dup id", []Clip{
			{ID: "x", SourceStart: 0, SourceEnd: 5},
			{ID: "x", SourceStart: 5, SourceEnd: 10},
		}, 100, 1},
		{"negative source start", []Clip{{ID: "a", SourceStart: -1, SourceEnd: 5}}, 100, 1},
		{"inverted times", []Clip{{ID: "a", SourceStart: 10, SourceEnd: 5}}, 100, 1},
		{"past source end", []Clip{{ID: "a", SourceStart: 0, SourceEnd: 200}}, 100, 1},
		{"negative program start", []Clip{{ID: "a", SourceStart: 0, SourceEnd: 5, ProgramStart: -1}}, 100, 1},
		// sourceDuration=0 disables the source-duration check; multitrack
		// callers use this when each clip points at a different material.
		{"sourceDuration=0 skips source check", []Clip{{ID: "a", SourceStart: 0, SourceEnd: 99999}}, 0, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := ValidateClips(c.clips, "track", c.sourceDuration)
			if len(got) != c.wantErrs {
				t.Errorf("got %d errors, want %d: %v", len(got), c.wantErrs, got)
			}
		})
	}
}
