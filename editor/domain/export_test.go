package domain

import (
	"path/filepath"
	"strings"
	"testing"
)

func baseProject() *Project {
	return &Project{
		ID: "a1b2c3d4",
		Source: Source{
			Path:     "/tmp/in.mp4",
			Duration: 60,
			HasAudio: true,
		},
		VideoClips: []Clip{
			{ID: "v1", SourceStart: 0, SourceEnd: 10},
			{ID: "v2", SourceStart: 20, SourceEnd: 25},
		},
		AudioClips: []Clip{
			{ID: "a1", SourceStart: 0, SourceEnd: 10},
			{ID: "a2", SourceStart: 20, SourceEnd: 25},
		},
		Export: ExportSettings{
			Format:     "mp4",
			VideoCodec: "h264",
			AudioCodec: "aac",
			OutputDir:  "/tmp/out",
			OutputName: "result",
		},
	}
}

func TestBuildExportArgs_TwoClipsWithAudio(t *testing.T) {
	p := baseProject()
	args, outPath, err := BuildExportArgs(p)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if outPath != filepath.Join("/tmp/out", "result.mp4") {
		t.Errorf("outPath = %q", outPath)
	}
	filter := args[indexOfStr(args, "-filter_complex")+1]
	wantSnippets := []string{
		"[0:v]trim=start=0:end=10",
		"[0:a]atrim=start=0:end=10",
		"[0:v]trim=start=20:end=25",
		"[0:a]atrim=start=20:end=25",
		"concat=n=2:v=1:a=0[v]",
		"concat=n=2:v=0:a=1[a]",
	}
	for _, w := range wantSnippets {
		if !strings.Contains(filter, w) {
			t.Errorf("filter missing %q\nfilter: %s", w, filter)
		}
	}
	if !sliceHasPair(args, "-map", "[v]") {
		t.Error("missing -map [v]")
	}
	if !sliceHasPair(args, "-map", "[a]") {
		t.Error("missing -map [a]")
	}
	if !sliceHasPair(args, "-c:v", "libx264") {
		t.Errorf("want libx264 normalized, got args=%v", args)
	}
	if !sliceHasPair(args, "-c:a", "aac") {
		t.Errorf("want -c:a aac, got args=%v", args)
	}
}

func TestBuildExportArgs_NoAudio(t *testing.T) {
	p := baseProject()
	p.Source.HasAudio = false
	p.AudioClips = nil
	args, _, err := BuildExportArgs(p)
	if err != nil {
		t.Fatal(err)
	}
	filter := args[indexOfStr(args, "-filter_complex")+1]
	if strings.Contains(filter, "atrim") || strings.Contains(filter, "[0:a]") {
		t.Errorf("filter should have no audio ops: %s", filter)
	}
	if !strings.Contains(filter, "concat=n=2:v=1:a=0[v]") {
		t.Errorf("expected video-only concat, got %s", filter)
	}
	if sliceHasPair(args, "-map", "[a]") {
		t.Error("should not map [a] when source has no audio")
	}
	if sliceHasPair(args, "-c:a", "aac") {
		t.Error("should not emit -c:a when no audio")
	}
}

func TestBuildExportArgs_IndependentTrackLengths(t *testing.T) {
	// Video track has 2 clips, audio track has 3 — tracks are independent.
	p := baseProject()
	p.AudioClips = []Clip{
		{ID: "a1", SourceStart: 0, SourceEnd: 10},
		{ID: "a2", SourceStart: 15, SourceEnd: 20},
		{ID: "a3", SourceStart: 40, SourceEnd: 50},
	}
	args, _, err := BuildExportArgs(p)
	if err != nil {
		t.Fatal(err)
	}
	filter := args[indexOfStr(args, "-filter_complex")+1]
	if !strings.Contains(filter, "concat=n=2:v=1:a=0[v]") {
		t.Error("video concat should be n=2")
	}
	if !strings.Contains(filter, "concat=n=3:v=0:a=1[a]") {
		t.Error("audio concat should be n=3")
	}
}

func TestBuildExportArgs_VideoOnlyNoAudioTrack(t *testing.T) {
	// Source has audio, but user deleted all audio clips from the timeline.
	p := baseProject()
	p.AudioClips = nil
	args, _, err := BuildExportArgs(p)
	if err != nil {
		t.Fatal(err)
	}
	if sliceHasPair(args, "-map", "[a]") {
		t.Error("no audio clips → no -map [a]")
	}
}

func TestBuildExportArgs_Errors(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*Project)
	}{
		{"no clips at all", func(p *Project) { p.VideoClips = nil; p.AudioClips = nil }},
		{"no source path", func(p *Project) { p.Source.Path = "" }},
		{"no output dir", func(p *Project) { p.Export.OutputDir = "" }},
		{"no output name", func(p *Project) { p.Export.OutputName = "" }},
		{"no format", func(p *Project) { p.Export.Format = "" }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p := baseProject()
			c.mutate(p)
			if _, _, err := BuildExportArgs(p); err == nil {
				t.Error("expected error")
			}
		})
	}
	if _, _, err := BuildExportArgs(nil); err == nil {
		t.Error("nil project should error")
	}
}

func TestFormatFloat(t *testing.T) {
	cases := []struct {
		v    float64
		want string
	}{
		{0, "0"},
		{1, "1"},
		{1.5, "1.5"},
		{12.345, "12.345"},
		{0.1, "0.1"},
		{12.000001, "12.000001"},
	}
	for _, c := range cases {
		if got := formatFloat(c.v); got != c.want {
			t.Errorf("formatFloat(%v) = %q, want %q", c.v, got, c.want)
		}
	}
}

// ---- helpers ------------------------------------------------------------

func indexOfStr(arr []string, s string) int {
	for i, v := range arr {
		if v == s {
			return i
		}
	}
	return -1
}

func sliceHasPair(arr []string, a, b string) bool {
	for i := 0; i+1 < len(arr); i++ {
		if arr[i] == a && arr[i+1] == b {
			return true
		}
	}
	return false
}
