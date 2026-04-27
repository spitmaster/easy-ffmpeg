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
			Path:      "/tmp/in.mp4",
			Duration:  60,
			Width:     1920,
			Height:    1080,
			FrameRate: 30,
			HasAudio:  true,
		},
		VideoClips: []Clip{
			{ID: "v1", SourceStart: 0, SourceEnd: 10, ProgramStart: 0},
			{ID: "v2", SourceStart: 20, SourceEnd: 25, ProgramStart: 10},
		},
		AudioClips: []Clip{
			{ID: "a1", SourceStart: 0, SourceEnd: 10, ProgramStart: 0},
			{ID: "a2", SourceStart: 20, SourceEnd: 25, ProgramStart: 10},
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
		"aformat=sample_fmts=fltp:sample_rates=48000:channel_layouts=stereo",
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
	// Video track has 2 clips ending at 15s, audio track has 3 ending at
	// 25s. The shorter track (video here) is auto-padded to programDur,
	// so its concat count gains one trailing gap segment (=> n=3).
	p := baseProject()
	p.AudioClips = []Clip{
		{ID: "a1", SourceStart: 0, SourceEnd: 10, ProgramStart: 0},
		{ID: "a2", SourceStart: 15, SourceEnd: 20, ProgramStart: 10},
		{ID: "a3", SourceStart: 40, SourceEnd: 50, ProgramStart: 15},
	}
	args, _, err := BuildExportArgs(p)
	if err != nil {
		t.Fatal(err)
	}
	filter := args[indexOfStr(args, "-filter_complex")+1]
	// 2 video clips + 10s trailing black pad to match audio's 25s
	if !strings.Contains(filter, "concat=n=3:v=1:a=0[v]") {
		t.Errorf("video concat should be n=3 (2 clips + trailing pad), got: %s", filter)
	}
	// 3 audio clips, no padding needed (already the longest track)
	if !strings.Contains(filter, "concat=n=3:v=0:a=1[a]") {
		t.Errorf("audio concat should be n=3, got: %s", filter)
	}
	// The trailing pad should be 10s (25 - 15) of black at video's
	// resolution and frame rate.
	if !strings.Contains(filter, "color=c=black:s=1920x1080:r=30:d=10") {
		t.Errorf("missing trailing black pad on video, got: %s", filter)
	}
}

// Twin scenario: video is the longer track, audio is shorter — audio
// gets padded with anullsrc at the end so the final mp4's audio stream
// matches the video stream length. Without this, browsers like Chrome
// truncate playback at the shorter stream's end (preview cut-off bug).
func TestBuildExportArgs_AudioShorterThanVideo(t *testing.T) {
	p := baseProject()
	p.VideoClips = []Clip{
		{ID: "v1", SourceStart: 0, SourceEnd: 30, ProgramStart: 0}, // 30s video
	}
	p.AudioClips = []Clip{
		{ID: "a1", SourceStart: 0, SourceEnd: 10, ProgramStart: 0}, // 10s audio
	}
	args, _, err := BuildExportArgs(p)
	if err != nil {
		t.Fatal(err)
	}
	filter := args[indexOfStr(args, "-filter_complex")+1]
	// Video: 1 clip, no pad → n=1
	if !strings.Contains(filter, "concat=n=1:v=1:a=0[v]") {
		t.Errorf("video concat should be n=1 (no pad), got: %s", filter)
	}
	// Audio: 1 clip + 20s trailing silence → n=2
	if !strings.Contains(filter, "concat=n=2:v=0:a=1[a]") {
		t.Errorf("audio concat should be n=2 (1 clip + trailing silence), got: %s", filter)
	}
	if !strings.Contains(filter, "anullsrc=r=48000:cl=stereo:d=20") {
		t.Errorf("missing trailing silence on audio, got: %s", filter)
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

func TestBuildExportArgs_RejectsVideoLeadingGap(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*Project)
	}{
		{
			name: "video track first clip starts late",
			mutate: func(p *Project) {
				p.VideoClips = []Clip{
					{ID: "v1", SourceStart: 0, SourceEnd: 10, ProgramStart: 2.5},
				}
			},
		},
		{
			name: "out-of-order video clips: earliest still leading-gap",
			mutate: func(p *Project) {
				// Earliest by ProgramStart is the second one (0.7s) — should still trip.
				p.VideoClips = []Clip{
					{ID: "v1", SourceStart: 5, SourceEnd: 10, ProgramStart: 5},
					{ID: "v2", SourceStart: 0, SourceEnd: 1, ProgramStart: 0.7},
				}
			},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p := baseProject()
			c.mutate(p)
			_, _, err := BuildExportArgs(p)
			if err == nil {
				t.Fatal("expected video leading-gap error")
			}
			if !strings.Contains(err.Error(), "视频轨道开头") {
				t.Errorf("error message = %q, want to contain %q", err.Error(), "视频轨道开头")
			}
		})
	}
}

// Audio gets a free pass on leading gaps — pre-roll silence is a valid
// edit. Only the video track is gated.
func TestBuildExportArgs_AcceptsAudioLeadingGap(t *testing.T) {
	p := baseProject()
	p.VideoClips = []Clip{
		{ID: "v1", SourceStart: 0, SourceEnd: 10, ProgramStart: 0},
	}
	p.AudioClips = []Clip{
		{ID: "a1", SourceStart: 0, SourceEnd: 8, ProgramStart: 1.5},
	}
	args, _, err := BuildExportArgs(p)
	if err != nil {
		t.Fatalf("audio leading gap should be allowed, got: %v", err)
	}
	filter := args[indexOfStr(args, "-filter_complex")+1]
	// Audio filter graph should include a 1.5s silent prefix.
	if !strings.Contains(filter, "anullsrc=r=48000:cl=stereo:d=1.5") {
		t.Errorf("missing silent leading segment in audio chain: %s", filter)
	}
}

func TestBuildExportArgs_AudioVolume(t *testing.T) {
	t.Run("unity volume emits no volume filter", func(t *testing.T) {
		p := baseProject()
		p.AudioVolume = 1.0
		args, _, err := BuildExportArgs(p)
		if err != nil {
			t.Fatal(err)
		}
		filter := args[indexOfStr(args, "-filter_complex")+1]
		if strings.Contains(filter, "volume=") {
			t.Errorf("unity volume should not emit volume filter, got: %s", filter)
		}
		if !strings.Contains(filter, "concat=n=2:v=0:a=1[a]") {
			t.Errorf("audio chain should end at [a], got: %s", filter)
		}
	})
	t.Run("non-unity volume routes via [a_pre] then volume filter", func(t *testing.T) {
		p := baseProject()
		p.AudioVolume = 0.5
		args, _, err := BuildExportArgs(p)
		if err != nil {
			t.Fatal(err)
		}
		filter := args[indexOfStr(args, "-filter_complex")+1]
		if !strings.Contains(filter, "concat=n=2:v=0:a=1[a_pre]") {
			t.Errorf("concat should output to [a_pre], got: %s", filter)
		}
		if !strings.Contains(filter, "[a_pre]volume=0.5[a]") {
			t.Errorf("volume filter should map [a_pre] → [a], got: %s", filter)
		}
	})
	t.Run("zero volume falls back to unity (treated as missing)", func(t *testing.T) {
		// AudioVolume == 0 means "field absent in JSON"; export should
		// behave as unity gain so older projects without the field don't
		// silently mute themselves.
		p := baseProject()
		p.AudioVolume = 0
		args, _, err := BuildExportArgs(p)
		if err != nil {
			t.Fatal(err)
		}
		filter := args[indexOfStr(args, "-filter_complex")+1]
		if strings.Contains(filter, "volume=") {
			t.Errorf("zero AudioVolume should be treated as unity, got: %s", filter)
		}
	})
}

func TestBuildExportArgs_AcceptsMidGap(t *testing.T) {
	// Mid-track gaps are still allowed — only the video leading position is gated.
	p := baseProject()
	p.VideoClips = []Clip{
		{ID: "v1", SourceStart: 0, SourceEnd: 5, ProgramStart: 0},
		{ID: "v2", SourceStart: 10, SourceEnd: 15, ProgramStart: 8},
	}
	p.AudioClips = []Clip{
		{ID: "a1", SourceStart: 0, SourceEnd: 5, ProgramStart: 0},
		{ID: "a2", SourceStart: 10, SourceEnd: 15, ProgramStart: 8},
	}
	if _, _, err := BuildExportArgs(p); err != nil {
		t.Fatalf("mid-gap should be allowed, got: %v", err)
	}
}

func TestBuildExportArgs_GapInsertsBlackAndSilence(t *testing.T) {
	// Two clips with a 5-second gap between them on both tracks. The filter
	// graph should include a black-frame segment and a silent-audio segment
	// sized to the gap, and concat should see n=3 per track.
	p := baseProject()
	p.VideoClips = []Clip{
		{ID: "v1", SourceStart: 0, SourceEnd: 10, ProgramStart: 0},
		{ID: "v2", SourceStart: 20, SourceEnd: 25, ProgramStart: 15}, // 5s gap
	}
	p.AudioClips = []Clip{
		{ID: "a1", SourceStart: 0, SourceEnd: 10, ProgramStart: 0},
		{ID: "a2", SourceStart: 20, SourceEnd: 25, ProgramStart: 15},
	}
	args, _, err := BuildExportArgs(p)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	filter := args[indexOfStr(args, "-filter_complex")+1]
	wantSnippets := []string{
		"color=c=black:s=1920x1080:r=30:d=5",
		"anullsrc=r=48000:cl=stereo:d=5",
		"concat=n=3:v=1:a=0[v]",
		"concat=n=3:v=0:a=1[a]",
	}
	for _, w := range wantSnippets {
		if !strings.Contains(filter, w) {
			t.Errorf("filter missing %q\nfilter: %s", w, filter)
		}
	}
}

// Leading gaps used to silently produce a black/silent prefix. Product
// decision: editing keeps the gap permitted, but export now refuses with
// a clear message — the dedicated tests for that live in
// TestBuildExportArgs_RejectsLeadingGap.

func TestBuildExportArgs_UnorderedClips(t *testing.T) {
	// Clips provided out of ProgramStart order should still render correctly
	// because the planner sorts them internally.
	p := baseProject()
	p.VideoClips = []Clip{
		{ID: "v2", SourceStart: 20, SourceEnd: 25, ProgramStart: 10},
		{ID: "v1", SourceStart: 0, SourceEnd: 10, ProgramStart: 0},
	}
	p.AudioClips = nil
	p.Source.HasAudio = false
	args, _, err := BuildExportArgs(p)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	filter := args[indexOfStr(args, "-filter_complex")+1]
	// v1's slice should appear before v2's in the concat chain regardless
	// of input order. Easiest way to verify: find their indices in the
	// filter string.
	pos1 := strings.Index(filter, "[0:v]trim=start=0:end=10")
	pos2 := strings.Index(filter, "[0:v]trim=start=20:end=25")
	if pos1 < 0 || pos2 < 0 || pos1 >= pos2 {
		t.Errorf("clips not sorted by programStart\nfilter: %s", filter)
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
