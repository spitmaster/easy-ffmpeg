package domain

import (
	"path/filepath"
	"strings"
	"testing"

	common "easy-ffmpeg/editor/common/domain"
)

// baseProject builds a minimal valid multitrack project: two video sources
// (1080p + 720p) and one audio-only source, one video track with a single
// clip, one audio track with the matching slice. Tests mutate copies of
// this fixture to express each scenario.
func baseProject() *Project {
	return &Project{
		ID:   "abcdef01",
		Kind: KindMultitrack,
		Sources: []Source{
			{
				ID: "s1", Path: "/tmp/v1.mp4", Kind: SourceVideo,
				Duration: 60, Width: 1920, Height: 1080, FrameRate: 30, HasAudio: true,
			},
			{
				ID: "s2", Path: "/tmp/v2.mp4", Kind: SourceVideo,
				Duration: 60, Width: 1280, Height: 720, FrameRate: 25, HasAudio: true,
			},
			{
				ID: "s3", Path: "/tmp/bgm.mp3", Kind: SourceAudio,
				Duration: 60, HasAudio: true,
			},
		},
		AudioVolume: 1.0,
		VideoTracks: []VideoTrack{
			{
				ID: "vt1",
				Clips: []Clip{
					mtClip("v1", "s1", 0, 10, 0),
				},
			},
		},
		AudioTracks: []AudioTrack{
			{
				ID: "at1", Volume: 1.0,
				Clips: []Clip{
					mtClip("a1", "s1", 0, 10, 0),
				},
			},
		},
		Export: common.ExportSettings{
			Format:     "mp4",
			VideoCodec: "h264",
			AudioCodec: "aac",
			OutputDir:  "/tmp/out",
			OutputName: "result",
		},
	}
}

func mtClip(id, sourceID string, sStart, sEnd, programStart float64) Clip {
	return Clip{
		Clip: common.Clip{
			ID: id, SourceStart: sStart, SourceEnd: sEnd, ProgramStart: programStart,
		},
		SourceID: sourceID,
	}
}

func filterOf(t *testing.T, args []string) string {
	t.Helper()
	for i, a := range args {
		if a == "-filter_complex" && i+1 < len(args) {
			return args[i+1]
		}
	}
	t.Fatalf("filter_complex not found in args: %v", args)
	return ""
}

func sliceHasPair(arr []string, a, b string) bool {
	for i := 0; i+1 < len(arr); i++ {
		if arr[i] == a && arr[i+1] == b {
			return true
		}
	}
	return false
}

func indexOfStr(arr []string, s string) int {
	for i, v := range arr {
		if v == s {
			return i
		}
	}
	return -1
}

// TestBuildExportArgs_SingleVideoSingleAudio: degenerate "1 video track +
// 1 audio track, single clip each" mirrors a single-video editor project.
// Assert the trivial path skips overlay (single track → outLabel = [V]
// directly) and that maps + codecs are wired.
func TestBuildExportArgs_SingleVideoSingleAudio(t *testing.T) {
	p := baseProject()
	args, outPath, err := BuildExportArgs(p)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if outPath != filepath.Join("/tmp/out", "result.mp4") {
		t.Errorf("outPath = %q", outPath)
	}
	filter := filterOf(t, args)
	want := []string{
		"[0:v]trim=start=0:end=10",
		"[0:a]atrim=start=0:end=10",
		// single video track → outLabel is [V] directly, no overlay step
		"concat=n=1:v=1:a=0[V]",
		// single audio track + unity global volume → outLabel is [A] directly
		"concat=n=1:v=0:a=1[A]",
	}
	for _, w := range want {
		if !strings.Contains(filter, w) {
			t.Errorf("filter missing %q\nfilter: %s", w, filter)
		}
	}
	// No overlay should appear (single video track).
	if strings.Contains(filter, "overlay=") {
		t.Errorf("single video track should not emit overlay, got: %s", filter)
	}
	// No amix should appear (single audio track).
	if strings.Contains(filter, "amix=") {
		t.Errorf("single audio track should not emit amix, got: %s", filter)
	}
	// Volume filter should be omitted at unity.
	if strings.Contains(filter, "volume=") {
		t.Errorf("unity volumes should omit volume filter, got: %s", filter)
	}
	if !sliceHasPair(args, "-map", "[V]") {
		t.Error("missing -map [V]")
	}
	if !sliceHasPair(args, "-map", "[A]") {
		t.Error("missing -map [A]")
	}
	if !sliceHasPair(args, "-c:v", "libx264") {
		t.Errorf("want libx264, got args=%v", args)
	}
	if !sliceHasPair(args, "-c:a", "aac") {
		t.Errorf("want aac, got args=%v", args)
	}
	// Only s1 is referenced — s2 / s3 should not appear as -i. Check by
	// counting -i occurrences.
	iCount := 0
	for _, a := range args {
		if a == "-i" {
			iCount++
		}
	}
	if iCount != 1 {
		t.Errorf("expected 1 input (only s1 is used), got %d in %v", iCount, args)
	}
}

// TestBuildExportArgs_TwoVideoTracksEqualLength: two video tracks each
// with one clip, both 10s — assert the chain emits [V0] [V1] and a single
// overlay into [V].
func TestBuildExportArgs_TwoVideoTracksEqualLength(t *testing.T) {
	p := baseProject()
	p.VideoTracks = []VideoTrack{
		{ID: "vt1", Clips: []Clip{mtClip("v1", "s1", 0, 10, 0)}},
		{ID: "vt2", Clips: []Clip{mtClip("v2", "s1", 20, 30, 0)}},
	}
	p.AudioTracks = nil
	args, _, err := BuildExportArgs(p)
	if err != nil {
		t.Fatal(err)
	}
	filter := filterOf(t, args)
	want := []string{
		"concat=n=1:v=1:a=0[V0]",
		"concat=n=1:v=1:a=0[V1]",
		"[V0][V1]overlay=0:0[V]",
	}
	for _, w := range want {
		if !strings.Contains(filter, w) {
			t.Errorf("filter missing %q\nfilter: %s", w, filter)
		}
	}
}

// TestBuildExportArgs_TwoVideoTracksUnequalShortTrackPads: shorter track
// should get a trailing color pad sized to the difference.
func TestBuildExportArgs_TwoVideoTracksUnequalShortTrackPads(t *testing.T) {
	p := baseProject()
	p.VideoTracks = []VideoTrack{
		{ID: "vt1", Clips: []Clip{mtClip("v1", "s1", 0, 30, 0)}}, // 30s
		{ID: "vt2", Clips: []Clip{mtClip("v2", "s1", 0, 10, 0)}}, // 10s — needs 20s pad
	}
	p.AudioTracks = nil
	args, _, err := BuildExportArgs(p)
	if err != nil {
		t.Fatal(err)
	}
	filter := filterOf(t, args)
	// Short track now has 2 segments (1 clip + 20s pad).
	if !strings.Contains(filter, "concat=n=2:v=1:a=0[V1]") {
		t.Errorf("short track should pad to programDur (n=2), got: %s", filter)
	}
	// Pad emitted as 20s of canvas-sized black at canvasFr (max FR = 30).
	if !strings.Contains(filter, "color=c=black:s=1920x1080:r=30:d=20") {
		t.Errorf("missing 20s trailing black pad on short video track: %s", filter)
	}
}

// TestBuildExportArgs_RejectsVideoLeadingGap: video tracks may not start
// late. Audio gets a free pass (anullsrc fills the leading silence).
func TestBuildExportArgs_RejectsVideoLeadingGap(t *testing.T) {
	p := baseProject()
	p.VideoTracks[0].Clips = []Clip{mtClip("v1", "s1", 0, 10, 2.5)}
	_, _, err := BuildExportArgs(p)
	if err == nil {
		t.Fatal("expected leading-gap error")
	}
	if !strings.Contains(err.Error(), "videoTracks[0]") {
		t.Errorf("error should name the offending track: %v", err)
	}
}

// TestBuildExportArgs_AcceptsAudioLeadingGap: the audio chain should emit
// an anullsrc prefix when the first audio clip starts after 0.
func TestBuildExportArgs_AcceptsAudioLeadingGap(t *testing.T) {
	p := baseProject()
	p.AudioTracks[0].Clips = []Clip{mtClip("a1", "s1", 0, 8, 1.5)}
	args, _, err := BuildExportArgs(p)
	if err != nil {
		t.Fatalf("audio leading gap should be allowed: %v", err)
	}
	filter := filterOf(t, args)
	if !strings.Contains(filter, "anullsrc=r=48000:cl=stereo:d=1.5") {
		t.Errorf("missing leading silence: %s", filter)
	}
}

// TestBuildExportArgs_ThreeVideoTracksZOrder: chain overlay must apply
// in track order — V0 bottom, V1 over V0, V2 over the result.
func TestBuildExportArgs_ThreeVideoTracksZOrder(t *testing.T) {
	p := baseProject()
	p.VideoTracks = []VideoTrack{
		{ID: "vt1", Clips: []Clip{mtClip("v1", "s1", 0, 10, 0)}},
		{ID: "vt2", Clips: []Clip{mtClip("v2", "s1", 20, 30, 0)}},
		{ID: "vt3", Clips: []Clip{mtClip("v3", "s1", 40, 50, 0)}},
	}
	p.AudioTracks = nil
	args, _, err := BuildExportArgs(p)
	if err != nil {
		t.Fatal(err)
	}
	filter := filterOf(t, args)
	// V0 bottom, V1 onto V0 → Vmix1; V2 onto Vmix1 → V (final).
	want := []string{
		"[V0][V1]overlay=0:0[Vmix1]",
		"[Vmix1][V2]overlay=0:0[V]",
	}
	for _, w := range want {
		if !strings.Contains(filter, w) {
			t.Errorf("missing chain rung %q\nfilter: %s", w, filter)
		}
	}
}

// TestBuildExportArgs_SingleAudioGlobalVolume: unity vs non-unity branch
// for the single-audio-track + global-volume case.
func TestBuildExportArgs_SingleAudioGlobalVolume(t *testing.T) {
	t.Run("unity volume → straight to [A]", func(t *testing.T) {
		p := baseProject()
		p.VideoTracks = nil
		p.AudioVolume = 1.0
		args, _, err := BuildExportArgs(p)
		if err != nil {
			t.Fatal(err)
		}
		filter := filterOf(t, args)
		if !strings.Contains(filter, "concat=n=1:v=0:a=1[A]") {
			t.Errorf("unity should concat directly to [A], got: %s", filter)
		}
		if strings.Contains(filter, "volume=") {
			t.Errorf("unity should not emit volume, got: %s", filter)
		}
	})
	t.Run("non-unity → [A_pre] then volume → [A]", func(t *testing.T) {
		p := baseProject()
		p.VideoTracks = nil
		p.AudioVolume = 0.5
		args, _, err := BuildExportArgs(p)
		if err != nil {
			t.Fatal(err)
		}
		filter := filterOf(t, args)
		if !strings.Contains(filter, "concat=n=1:v=0:a=1[A_pre]") {
			t.Errorf("non-unity should concat into [A_pre], got: %s", filter)
		}
		if !strings.Contains(filter, "[A_pre]volume=0.5[A]") {
			t.Errorf("missing global volume step, got: %s", filter)
		}
	})
}

// TestBuildExportArgs_TwoAudioTracksAmix: two audio tracks, each with
// independent volume, mix into [A_pre] (or [A] when global is unity).
func TestBuildExportArgs_TwoAudioTracksAmix(t *testing.T) {
	p := baseProject()
	p.VideoTracks = nil
	p.AudioVolume = 1.0 // unity → amix output is [A] directly
	p.AudioTracks = []AudioTrack{
		{ID: "at1", Volume: 1.0, Clips: []Clip{mtClip("a1", "s1", 0, 10, 0)}},
		{ID: "at2", Volume: 0.6, Clips: []Clip{mtClip("a2", "s3", 0, 10, 0)}},
	}
	args, _, err := BuildExportArgs(p)
	if err != nil {
		t.Fatal(err)
	}
	filter := filterOf(t, args)
	want := []string{
		"concat=n=1:v=0:a=1[A0]",       // track 0 unity → straight to [A0]
		"concat=n=1:v=0:a=1[A1_pre]",    // track 1 non-unity → [A1_pre]
		"[A1_pre]volume=0.6[A1]",        // → [A1]
		"[A0][A1]amix=inputs=2:duration=longest:dropout_transition=0[A]",
	}
	for _, w := range want {
		if !strings.Contains(filter, w) {
			t.Errorf("missing %q\nfilter: %s", w, filter)
		}
	}
}

// TestBuildExportArgs_TwoAudioTracksAmix_GlobalVolume: when global volume
// is non-unity the amix output goes to [A_pre] then through volume → [A].
func TestBuildExportArgs_TwoAudioTracksAmix_GlobalVolume(t *testing.T) {
	p := baseProject()
	p.VideoTracks = nil
	p.AudioVolume = 0.8
	p.AudioTracks = []AudioTrack{
		{ID: "at1", Volume: 1.0, Clips: []Clip{mtClip("a1", "s1", 0, 10, 0)}},
		{ID: "at2", Volume: 1.0, Clips: []Clip{mtClip("a2", "s3", 0, 10, 0)}},
	}
	args, _, err := BuildExportArgs(p)
	if err != nil {
		t.Fatal(err)
	}
	filter := filterOf(t, args)
	want := []string{
		"[A0][A1]amix=inputs=2:duration=longest:dropout_transition=0[A_pre]",
		"[A_pre]volume=0.8[A]",
	}
	for _, w := range want {
		if !strings.Contains(filter, w) {
			t.Errorf("missing %q\nfilter: %s", w, filter)
		}
	}
}

// TestBuildExportArgs_CrossSourceClipSequence: a single track containing
// clips from two different sources should produce [0:v]trim and [1:v]trim
// references with the input-order map honored.
func TestBuildExportArgs_CrossSourceClipSequence(t *testing.T) {
	p := baseProject()
	// Use both s1 and s2 on the video track, drop the audio track to
	// keep the assertion focused.
	p.VideoTracks[0].Clips = []Clip{
		mtClip("v1", "s1", 0, 5, 0),
		mtClip("v2", "s2", 0, 5, 5),
	}
	p.AudioTracks = nil
	args, _, err := BuildExportArgs(p)
	if err != nil {
		t.Fatal(err)
	}
	filter := filterOf(t, args)
	if !strings.Contains(filter, "[0:v]trim=start=0:end=5") {
		t.Errorf("missing s1 trim from input 0: %s", filter)
	}
	if !strings.Contains(filter, "[1:v]trim=start=0:end=5") {
		t.Errorf("missing s2 trim from input 1: %s", filter)
	}
	// Both s1 and s2 should be -i, in Sources order.
	if !sliceHasPair(args, "-i", "/tmp/v1.mp4") || !sliceHasPair(args, "-i", "/tmp/v2.mp4") {
		t.Errorf("expected both s1 and s2 as inputs, got args=%v", args)
	}
}

// TestBuildExportArgs_ResolutionMismatchScalePad: every video segment
// must end with the canvas-sized scale+pad+setsar+format chain so concat
// across heterogeneous resolutions is legal.
func TestBuildExportArgs_ResolutionMismatchScalePad(t *testing.T) {
	p := baseProject()
	p.VideoTracks[0].Clips = []Clip{
		mtClip("v1", "s1", 0, 5, 0), // 1080p
		mtClip("v2", "s2", 0, 5, 5), // 720p
	}
	p.AudioTracks = nil
	args, _, err := BuildExportArgs(p)
	if err != nil {
		t.Fatal(err)
	}
	filter := filterOf(t, args)
	// canvas = max(1920, 1280) × max(1080, 720) = 1920×1080
	scale := "scale=1920:1080:force_original_aspect_ratio=decrease,pad=1920:1080:(ow-iw)/2:(oh-ih)/2:black,setsar=1,format=yuv420p"
	if !strings.Contains(filter, scale) {
		t.Errorf("missing scale+pad to canvas dims: %s", filter)
	}
}

// TestBuildExportArgs_EmptyProject: with no clips on any track at all the
// builder should refuse to produce a command. Empty clip lists on tracks
// also count as empty.
func TestBuildExportArgs_EmptyProject(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*Project)
	}{
		{
			name: "no tracks",
			mutate: func(p *Project) {
				p.VideoTracks = nil
				p.AudioTracks = nil
			},
		},
		{
			name: "all tracks have empty clips",
			mutate: func(p *Project) {
				p.VideoTracks = []VideoTrack{{ID: "vt1", Clips: nil}}
				p.AudioTracks = []AudioTrack{{ID: "at1", Volume: 1.0, Clips: nil}}
			},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p := baseProject()
			c.mutate(p)
			if _, _, err := BuildExportArgs(p); err == nil {
				t.Error("expected 'no clips' error")
			}
		})
	}
}

// TestBuildExportArgs_VideoOnly: an audio-less project should not emit
// -map [A] / -c:a, and the filter graph should contain no audio chain.
func TestBuildExportArgs_VideoOnly(t *testing.T) {
	p := baseProject()
	p.AudioTracks = nil
	args, _, err := BuildExportArgs(p)
	if err != nil {
		t.Fatal(err)
	}
	filter := filterOf(t, args)
	if strings.Contains(filter, "atrim") || strings.Contains(filter, "anullsrc") {
		t.Errorf("video-only project should have no audio chain: %s", filter)
	}
	if sliceHasPair(args, "-map", "[A]") {
		t.Error("video-only should not -map [A]")
	}
	if sliceHasPair(args, "-c:a", "aac") {
		t.Error("video-only should not set -c:a")
	}
	if !sliceHasPair(args, "-map", "[V]") {
		t.Error("missing -map [V]")
	}
}

// TestBuildExportArgs_AudioOnly: a video-less project should not emit
// -map [V] / -c:v, and the filter graph should contain no video chain.
func TestBuildExportArgs_AudioOnly(t *testing.T) {
	p := baseProject()
	p.VideoTracks = nil
	args, _, err := BuildExportArgs(p)
	if err != nil {
		t.Fatal(err)
	}
	filter := filterOf(t, args)
	if strings.Contains(filter, "[0:v]trim") || strings.Contains(filter, "color=c=black") {
		t.Errorf("audio-only project should have no video chain: %s", filter)
	}
	if sliceHasPair(args, "-map", "[V]") {
		t.Error("audio-only should not -map [V]")
	}
	if sliceHasPair(args, "-c:v", "libx264") {
		t.Error("audio-only should not set -c:v")
	}
	if !sliceHasPair(args, "-map", "[A]") {
		t.Error("missing -map [A]")
	}
}

// TestBuildExportArgs_OutPath: outputDir + name + "." + format.
func TestBuildExportArgs_OutPath(t *testing.T) {
	p := baseProject()
	p.Export.OutputDir = "/var/exports"
	p.Export.OutputName = "my-cut"
	p.Export.Format = "mkv"
	_, outPath, err := BuildExportArgs(p)
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join("/var/exports", "my-cut.mkv")
	if outPath != want {
		t.Errorf("outPath = %q, want %q", outPath, want)
	}
}

// TestBuildExportArgs_RejectsMissingExportSettings: empty outputDir /
// outputName / format come from common.ValidateExportSettings — confirm
// they propagate up rather than silently producing broken commands.
func TestBuildExportArgs_RejectsMissingExportSettings(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*Project)
	}{
		{"empty outputDir", func(p *Project) { p.Export.OutputDir = "" }},
		{"empty outputName", func(p *Project) { p.Export.OutputName = "" }},
		{"empty format", func(p *Project) { p.Export.Format = "" }},
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
	// nil project should also error.
	if _, _, err := BuildExportArgs(nil); err == nil {
		t.Error("nil project should error")
	}
}

// TestBuildExportArgs_UnreferencedSourcesNotInputs: sources never used by
// any clip must not appear as -i. Saves CPU/IO at ffmpeg start-up; also
// avoids polluting the input index map for sources that exist only in the
// library.
func TestBuildExportArgs_UnreferencedSourcesNotInputs(t *testing.T) {
	p := baseProject()
	// Only s1 used on video, only s1 on audio (default fixture). s2 / s3
	// are unreferenced. Ensure -i count is 1 and the path matches s1.
	args, _, err := BuildExportArgs(p)
	if err != nil {
		t.Fatal(err)
	}
	for _, sPath := range []string{"/tmp/v2.mp4", "/tmp/bgm.mp3"} {
		if sliceHasPair(args, "-i", sPath) {
			t.Errorf("unreferenced source %q should not be -i'd, got args=%v", sPath, args)
		}
	}
	if !sliceHasPair(args, "-i", "/tmp/v1.mp4") {
		t.Errorf("referenced source should be -i'd, got args=%v", args)
	}
	_ = indexOfStr(args, "-i") // sanity
}

// TestBuildExportArgs_GapEmitsBlackAndSilence: mid-track gaps render as
// canvas-sized black (video) and silence (audio).
func TestBuildExportArgs_GapEmitsBlackAndSilence(t *testing.T) {
	p := baseProject()
	p.VideoTracks[0].Clips = []Clip{
		mtClip("v1", "s1", 0, 10, 0),
		mtClip("v2", "s1", 20, 25, 15), // 5s gap
	}
	p.AudioTracks[0].Clips = []Clip{
		mtClip("a1", "s1", 0, 10, 0),
		mtClip("a2", "s1", 20, 25, 15), // 5s gap
	}
	args, _, err := BuildExportArgs(p)
	if err != nil {
		t.Fatal(err)
	}
	filter := filterOf(t, args)
	want := []string{
		"color=c=black:s=1920x1080:r=30:d=5",
		"anullsrc=r=48000:cl=stereo:d=5",
		"concat=n=3:v=1:a=0[V]", // single video track → outLabel is [V] directly
		"concat=n=1:v=0:a=1[A]", // single audio track unity → outLabel [A]
	}
	// Audio track: 2 clips + 5s gap = n=3 segments
	if !strings.Contains(filter, "concat=n=3:v=0:a=1[A]") {
		t.Errorf("audio track expected n=3 concat, got: %s", filter)
	}
	// Tweak: video should also be n=3.
	for _, w := range want[:3] {
		if !strings.Contains(filter, w) {
			t.Errorf("missing %q\nfilter: %s", w, filter)
		}
	}
}

// TestBuildExportArgs_LabelPrefixIsolation: a project with two video tracks
// must use distinct label prefixes so [v0_v0] and [v1_v0] don't collide
// on the second track's first segment.
func TestBuildExportArgs_LabelPrefixIsolation(t *testing.T) {
	p := baseProject()
	p.VideoTracks = []VideoTrack{
		{ID: "vt1", Clips: []Clip{mtClip("v1", "s1", 0, 5, 0)}},
		{ID: "vt2", Clips: []Clip{mtClip("v2", "s1", 5, 10, 0)}},
	}
	p.AudioTracks = nil
	args, _, err := BuildExportArgs(p)
	if err != nil {
		t.Fatal(err)
	}
	filter := filterOf(t, args)
	if !strings.Contains(filter, "[v0_v0]") {
		t.Errorf("missing v0 track prefix label: %s", filter)
	}
	if !strings.Contains(filter, "[v1_v0]") {
		t.Errorf("missing v1 track prefix label: %s", filter)
	}
}
