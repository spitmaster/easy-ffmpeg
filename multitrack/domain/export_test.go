package domain

import (
	"path/filepath"
	"strings"
	"testing"

	common "easy-ffmpeg/editor/common/domain"
)

// baseProject builds a minimal valid v0.5.1 multitrack project: 1920×1080@30
// canvas, two video sources (1080p + 720p) and one audio-only source, one
// video track with a single clip, one audio track with the matching slice.
// Tests mutate copies of this fixture to express each scenario.
//
// Default Transform on every video clip is full canvas (0, 0, 1920, 1080),
// reproducing v0.5.0's "stretch to canvas" behavior for tests that don't
// care about composition specifics.
func baseProject() *Project {
	return &Project{
		ID:            "abcdef01",
		Kind:          KindMultitrack,
		SchemaVersion: SchemaVersion,
		Canvas:        Canvas{Width: 1920, Height: 1080, FrameRate: 30},
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

// mtClip is the default-Transform helper. Most tests don't care about
// composition geometry — they want a clip and a 1920×1080 canvas-filling
// transform. Tests that need a specific layout use mtClipT.
func mtClip(id, sourceID string, sStart, sEnd, programStart float64) Clip {
	return Clip{
		Clip: common.Clip{
			ID: id, SourceStart: sStart, SourceEnd: sEnd, ProgramStart: programStart,
		},
		SourceID:  sourceID,
		Transform: Transform{X: 0, Y: 0, W: 1920, H: 1080},
	}
}

// mtClipT places a clip at an explicit (x, y, w, h). Used by composition
// tests (PIP, out-of-bounds, partial overflow).
func mtClipT(id, sourceID string, sStart, sEnd, programStart float64, x, y, w, h int) Clip {
	c := mtClip(id, sourceID, sStart, sEnd, programStart)
	c.Transform = Transform{X: x, Y: y, W: w, H: h}
	return c
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

// TestBuildExportArgs_SingleVideoSingleAudio: degenerate "1 video clip + 1
// audio clip" mirrors a single-video editor project. v0.5.1 always goes
// through the base + 1 overlay path (no fast path for N=1; simpler matrix).
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
		"color=c=black:s=1920x1080:r=30:d=10,format=yuv420p[base]",
		"[0:v]trim=start=0:end=10,setpts=PTS-STARTPTS+0/TB,scale=1920:1080,setsar=1,fps=30,format=yuva420p[seg_0]",
		"[base][seg_0]overlay=x=0:y=0:enable='between(t,0,10)':eof_action=pass[V]",
		"[0:a]atrim=start=0:end=10",
		"concat=n=1:v=0:a=1[A]",
	}
	for _, w := range want {
		if !strings.Contains(filter, w) {
			t.Errorf("filter missing %q\nfilter: %s", w, filter)
		}
	}
	if strings.Contains(filter, "amix=") {
		t.Errorf("single audio track should not emit amix, got: %s", filter)
	}
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

// TestBuildExportArgs_SingleVideoTrackTwoClips: two non-overlapping clips
// with different transforms on the same video track → two segments + two
// overlays. Same-track clips sort by programStart for stable filter
// strings (z-order convention only differentiates between tracks).
func TestBuildExportArgs_SingleVideoTrackTwoClips(t *testing.T) {
	p := baseProject()
	p.VideoTracks[0].Clips = []Clip{
		mtClipT("v1", "s1", 0, 5, 0, 0, 0, 960, 540),    // top-left half
		mtClipT("v2", "s1", 5, 10, 5, 960, 540, 960, 540), // bottom-right half
	}
	p.AudioTracks = nil
	args, _, err := BuildExportArgs(p)
	if err != nil {
		t.Fatal(err)
	}
	filter := filterOf(t, args)
	want := []string{
		"[0:v]trim=start=0:end=5,setpts=PTS-STARTPTS+0/TB,scale=960:540,setsar=1,fps=30,format=yuva420p[seg_0]",
		"[0:v]trim=start=5:end=10,setpts=PTS-STARTPTS+5/TB,scale=960:540,setsar=1,fps=30,format=yuva420p[seg_1]",
		"[base][seg_0]overlay=x=0:y=0:enable='between(t,0,5)':eof_action=pass[v_0]",
		"[v_0][seg_1]overlay=x=960:y=540:enable='between(t,5,10)':eof_action=pass[V]",
	}
	for _, w := range want {
		if !strings.Contains(filter, w) {
			t.Errorf("filter missing %q\nfilter: %s", w, filter)
		}
	}
}

// TestBuildExportArgs_PipTwoTracks: classic Picture-in-Picture — the first
// (top) video track (lower index = top of z, displayed at the top of the
// timeline column) holds a small window in the bottom-right corner; the
// second (bottom) track holds a full-screen clip. The full-screen segment
// is composited first (over base), the PIP last (on top of z).
func TestBuildExportArgs_PipTwoTracks(t *testing.T) {
	p := baseProject()
	p.VideoTracks = []VideoTrack{
		{ID: "vt1", Clips: []Clip{mtClipT("v1", "s1", 0, 10, 0, 1440, 720, 480, 360)}}, // top of z: small PIP
		{ID: "vt2", Clips: []Clip{mtClipT("v2", "s2", 0, 10, 0, 0, 0, 1920, 1080)}},     // bottom of z: full-screen
	}
	p.AudioTracks = nil
	args, _, err := BuildExportArgs(p)
	if err != nil {
		t.Fatal(err)
	}
	filter := filterOf(t, args)
	// Two segments — bottom-of-z (vt2 full-screen) emits first as seg_0,
	// top-of-z (vt1 PIP) last as seg_1.
	wantOrder := []string{
		"scale=1920:1080,setsar=1,fps=30,format=yuva420p[seg_0]", // bottom of z
		"scale=480:360,setsar=1,fps=30,format=yuva420p[seg_1]",   // PIP (top of z)
		"[base][seg_0]overlay=x=0:y=0:",
		"[v_0][seg_1]overlay=x=1440:y=720:",
		":eof_action=pass[V]",
	}
	for _, w := range wantOrder {
		if !strings.Contains(filter, w) {
			t.Errorf("filter missing %q\nfilter: %s", w, filter)
		}
	}
	// Both s1 and s2 should be inputs (in Sources order).
	if !sliceHasPair(args, "-i", "/tmp/v1.mp4") || !sliceHasPair(args, "-i", "/tmp/v2.mp4") {
		t.Errorf("expected s1 and s2 as inputs, got args=%v", args)
	}
}

// TestBuildExportArgs_TwoTracksUpperFullCovers: v0.5.0 visual equivalence
// when both clips are full-canvas and the top-of-z track is opaque (no
// alpha transparency in the source). The bottom-of-z should be invisible
// — verified by the overlay chain order; we don't assert pixel output
// here, just the chain shape. Convention: vt1 (lower index) = top of z,
// vt2 (higher index) = bottom of z.
func TestBuildExportArgs_TwoTracksUpperFullCovers(t *testing.T) {
	p := baseProject()
	p.VideoTracks = []VideoTrack{
		{ID: "vt1", Clips: []Clip{mtClip("v1", "s1", 0, 10, 0)}}, // top of z
		{ID: "vt2", Clips: []Clip{mtClip("v2", "s2", 0, 10, 0)}}, // bottom of z
	}
	p.AudioTracks = nil
	args, _, err := BuildExportArgs(p)
	if err != nil {
		t.Fatal(err)
	}
	filter := filterOf(t, args)
	// Both segments scale to full 1920×1080.
	if !strings.Contains(filter, "scale=1920:1080,setsar=1,fps=30,format=yuva420p[seg_0]") {
		t.Errorf("missing seg_0 full-canvas scale: %s", filter)
	}
	if !strings.Contains(filter, "scale=1920:1080,setsar=1,fps=30,format=yuva420p[seg_1]") {
		t.Errorf("missing seg_1 full-canvas scale: %s", filter)
	}
	// Bottom of z (vt2, source s2) emits first as seg_0; top of z (vt1,
	// source s1) emits last as seg_1.
	if !strings.Contains(filter, "[1:v]trim=start=0:end=10,setpts=PTS-STARTPTS+0/TB,scale=1920:1080,setsar=1,fps=30,format=yuva420p[seg_0]") {
		t.Errorf("expected vt2 (bottom of z, input 1) as seg_0: %s", filter)
	}
	if !strings.Contains(filter, "[0:v]trim=start=0:end=10,setpts=PTS-STARTPTS+0/TB,scale=1920:1080,setsar=1,fps=30,format=yuva420p[seg_1]") {
		t.Errorf("expected vt1 (top of z, input 0) as seg_1: %s", filter)
	}
	if !strings.Contains(filter, "[base][seg_0]overlay=x=0:y=0:") {
		t.Errorf("expected bottom-of-z segment over base: %s", filter)
	}
	if !strings.Contains(filter, "[v_0][seg_1]overlay=x=0:y=0:") {
		t.Errorf("expected top-of-z segment over v_0: %s", filter)
	}
}

// TestBuildExportArgs_ThreeTrackZOrder: three tracks → three segments,
// chain order = (base→seg_0)→seg_1→seg_2. Convention: lower track index =
// top of z, so seg_0 = vt3 (highest index, bottom), seg_2 = vt1 (lowest
// index, top). Each clip's trim window is unique so we can verify which
// track ended up where in the chain by source-time signature.
func TestBuildExportArgs_ThreeTrackZOrder(t *testing.T) {
	p := baseProject()
	p.VideoTracks = []VideoTrack{
		{ID: "vt1", Clips: []Clip{mtClip("v1", "s1", 0, 10, 0)}},   // top of z   → seg_2
		{ID: "vt2", Clips: []Clip{mtClip("v2", "s1", 20, 30, 0)}},  // middle     → seg_1
		{ID: "vt3", Clips: []Clip{mtClip("v3", "s1", 40, 50, 0)}},  // bottom of z→ seg_0
	}
	p.AudioTracks = nil
	args, _, err := BuildExportArgs(p)
	if err != nil {
		t.Fatal(err)
	}
	filter := filterOf(t, args)
	want := []string{
		"[base][seg_0]overlay=",
		"[v_0][seg_1]overlay=",
		"[v_1][seg_2]overlay=",
		// vt3 (bottom of z) emits first as seg_0 → trim 40..50.
		"trim=start=40:end=50,setpts=PTS-STARTPTS+0/TB,scale=1920:1080,setsar=1,fps=30,format=yuva420p[seg_0]",
		// vt2 in the middle → trim 20..30.
		"trim=start=20:end=30,setpts=PTS-STARTPTS+0/TB,scale=1920:1080,setsar=1,fps=30,format=yuva420p[seg_1]",
		// vt1 (top of z) emits last as seg_2 → trim 0..10.
		"trim=start=0:end=10,setpts=PTS-STARTPTS+0/TB,scale=1920:1080,setsar=1,fps=30,format=yuva420p[seg_2]",
	}
	for _, w := range want {
		if !strings.Contains(filter, w) {
			t.Errorf("missing rung %q\nfilter: %s", w, filter)
		}
	}
	// Final overlay must terminate at [V].
	if !strings.Contains(filter, ":eof_action=pass[V]") {
		t.Errorf("missing final [V] terminator: %s", filter)
	}
}

// TestBuildExportArgs_TransformOutOfBoundsAllowed: a clip whose transform
// places it entirely outside the canvas is allowed. The filter graph still
// emits the segment and overlay; ffmpeg's overlay drops the unseen pixels.
func TestBuildExportArgs_TransformOutOfBoundsAllowed(t *testing.T) {
	p := baseProject()
	p.VideoTracks[0].Clips = []Clip{
		// Way out: x=3000 on a 1920-wide canvas.
		mtClipT("v1", "s1", 0, 10, 0, 3000, 0, 200, 200),
	}
	p.AudioTracks = nil
	args, _, err := BuildExportArgs(p)
	if err != nil {
		t.Fatalf("OOB transform should be allowed at export, got %v", err)
	}
	filter := filterOf(t, args)
	if !strings.Contains(filter, "scale=200:200,setsar=1,fps=30,format=yuva420p[seg_0]") {
		t.Errorf("OOB segment still expected: %s", filter)
	}
	if !strings.Contains(filter, "[base][seg_0]overlay=x=3000:y=0:") {
		t.Errorf("OOB overlay should still be emitted: %s", filter)
	}
}

// TestBuildExportArgs_TransformPartialOutOfBoundsNoCrop: a transform that
// straddles the canvas edge must NOT be clipped at the filter graph level
// — overlay handles the visible region; we don't pre-crop.
func TestBuildExportArgs_TransformPartialOutOfBoundsNoCrop(t *testing.T) {
	p := baseProject()
	p.VideoTracks[0].Clips = []Clip{
		// Half off the right edge: x=1800, w=300 → 120 visible.
		mtClipT("v1", "s1", 0, 10, 0, 1800, 0, 300, 200),
	}
	p.AudioTracks = nil
	args, _, err := BuildExportArgs(p)
	if err != nil {
		t.Fatal(err)
	}
	filter := filterOf(t, args)
	if !strings.Contains(filter, "scale=300:300,setsar=1,fps=30,format=yuva420p[seg_0]") {
		// Note: scale uses transform W (300), H (200). Update assertion.
	}
	if !strings.Contains(filter, "scale=300:200,setsar=1,fps=30,format=yuva420p[seg_0]") {
		t.Errorf("missing partial-OOB segment with full transform W×H: %s", filter)
	}
	if strings.Contains(filter, "crop=") {
		t.Errorf("partial OOB should not introduce crop filter: %s", filter)
	}
}

// TestBuildExportArgs_CanvasFrameRateApplied: a non-default canvas FR
// (60fps) must surface in both the base and each segment's fps= clause.
func TestBuildExportArgs_CanvasFrameRateApplied(t *testing.T) {
	p := baseProject()
	p.Canvas.FrameRate = 60
	p.AudioTracks = nil
	args, _, err := BuildExportArgs(p)
	if err != nil {
		t.Fatal(err)
	}
	filter := filterOf(t, args)
	if !strings.Contains(filter, "color=c=black:s=1920x1080:r=60:d=10,format=yuv420p[base]") {
		t.Errorf("base must use canvas FR 60: %s", filter)
	}
	if !strings.Contains(filter, "fps=60,format=yuva420p[seg_0]") {
		t.Errorf("segment must force canvas FR 60: %s", filter)
	}
}

// TestBuildExportArgs_PtsShiftToProgramStart: a clip with programStart=2.5
// must shift its setpts by exactly 2.5/TB, not start at 0. This is the
// single most important correctness check — without it segments stack at
// the timeline's beginning regardless of programStart.
func TestBuildExportArgs_PtsShiftToProgramStart(t *testing.T) {
	p := baseProject()
	p.VideoTracks[0].Clips = []Clip{
		mtClipT("v1", "s1", 0, 5, 0, 0, 0, 1920, 1080),
		mtClipT("v2", "s1", 0, 5, 7.5, 0, 0, 1920, 1080), // gap before, programStart=7.5
	}
	p.AudioTracks = nil
	args, _, err := BuildExportArgs(p)
	if err != nil {
		t.Fatal(err)
	}
	filter := filterOf(t, args)
	if !strings.Contains(filter, "setpts=PTS-STARTPTS+0/TB,") {
		t.Errorf("first segment should shift by 0/TB (= no shift): %s", filter)
	}
	if !strings.Contains(filter, "setpts=PTS-STARTPTS+7.5/TB,") {
		t.Errorf("second segment should shift by 7.5/TB: %s", filter)
	}
	// And the matching overlay enable window.
	if !strings.Contains(filter, "enable='between(t,7.5,12.5)'") {
		t.Errorf("missing 7.5..12.5 enable window for second segment: %s", filter)
	}
}

// TestBuildExportArgs_BaseDurationMatchesProgramDur: the base canvas must
// span exactly the longest track's duration so the composite plays for
// the whole program (no premature cut, no trailing dead frames).
func TestBuildExportArgs_BaseDurationMatchesProgramDur(t *testing.T) {
	p := baseProject()
	p.VideoTracks = []VideoTrack{
		{ID: "vt1", Clips: []Clip{mtClip("v1", "s1", 0, 10, 0)}},
	}
	// Audio extends past video — programDur should reflect the audio length (15s).
	p.AudioTracks = []AudioTrack{
		{ID: "at1", Volume: 1.0, Clips: []Clip{mtClip("a1", "s1", 0, 15, 0)}},
	}
	args, _, err := BuildExportArgs(p)
	if err != nil {
		t.Fatal(err)
	}
	filter := filterOf(t, args)
	if !strings.Contains(filter, "color=c=black:s=1920x1080:r=30:d=15,format=yuv420p[base]") {
		t.Errorf("base must extend to programDur=15s: %s", filter)
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
// an anullsrc prefix when the first audio clip starts after 0. Video path
// is independent (default fixture clip at programStart=0).
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

// TestBuildExportArgs_SingleAudioGlobalVolume: unity vs non-unity branch
// for the single-audio-track + global-volume case (audio path unchanged).
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
	p.AudioVolume = 1.0
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
		"concat=n=1:v=0:a=1[A0]",
		"concat=n=1:v=0:a=1[A1_pre]",
		"[A1_pre]volume=0.6[A1]",
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

// TestBuildExportArgs_CrossSourceClipSequence: a single track with clips
// from two different sources still produces both [0:v]trim and [1:v]trim,
// each scaled to its own transform (not a shared scale+pad chain).
func TestBuildExportArgs_CrossSourceClipSequence(t *testing.T) {
	p := baseProject()
	p.VideoTracks[0].Clips = []Clip{
		mtClipT("v1", "s1", 0, 5, 0, 0, 0, 1920, 1080),
		mtClipT("v2", "s2", 0, 5, 5, 0, 0, 1280, 720), // smaller window
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
	// Each clip scales to its own transform, not to a shared canvas.
	if !strings.Contains(filter, "scale=1920:1080,setsar=1,fps=30,format=yuva420p[seg_0]") {
		t.Errorf("seg_0 should scale to its transform: %s", filter)
	}
	if !strings.Contains(filter, "scale=1280:720,setsar=1,fps=30,format=yuva420p[seg_1]") {
		t.Errorf("seg_1 should scale to its transform: %s", filter)
	}
	if !sliceHasPair(args, "-i", "/tmp/v1.mp4") || !sliceHasPair(args, "-i", "/tmp/v2.mp4") {
		t.Errorf("expected both s1 and s2 as inputs, got args=%v", args)
	}
}

// TestBuildExportArgs_PerClipScaleNoSharedPad: heterogeneous resolutions
// no longer require a shared scale+pad chain. v0.5.0 emitted
// `scale=...,pad=...` per segment to homogenise concat inputs; v0.5.1
// instead lets each clip carry its own transform W×H.
func TestBuildExportArgs_PerClipScaleNoSharedPad(t *testing.T) {
	p := baseProject()
	p.VideoTracks[0].Clips = []Clip{
		mtClipT("v1", "s1", 0, 5, 0, 0, 0, 1920, 1080),
		mtClipT("v2", "s2", 0, 5, 5, 0, 0, 1280, 720),
	}
	p.AudioTracks = nil
	args, _, err := BuildExportArgs(p)
	if err != nil {
		t.Fatal(err)
	}
	filter := filterOf(t, args)
	// v0.5.0's pad chain must be gone — segments don't pad to canvas anymore.
	if strings.Contains(filter, "force_original_aspect_ratio=decrease") {
		t.Errorf("v0.5.1 should not emit force_original_aspect_ratio: %s", filter)
	}
	if strings.Contains(filter, "pad=1920:1080") {
		t.Errorf("v0.5.1 should not emit shared pad: %s", filter)
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
	if strings.Contains(filter, "[base]") || strings.Contains(filter, "color=c=black") {
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
	if _, _, err := BuildExportArgs(nil); err == nil {
		t.Error("nil project should error")
	}
}

// TestBuildExportArgs_UnreferencedSourcesNotInputs: sources never used by
// any clip must not appear as -i.
func TestBuildExportArgs_UnreferencedSourcesNotInputs(t *testing.T) {
	p := baseProject()
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
}

// TestBuildExportArgs_GapNoBlackEmitted: in v0.5.1, mid-track video gaps
// no longer emit a `color=c=black` segment per gap — the base canvas IS
// the visible "between clips" frame, and a gap is just the absence of an
// overlay during that window. Audio path still emits anullsrc per gap.
func TestBuildExportArgs_GapNoBlackEmitted(t *testing.T) {
	p := baseProject()
	p.VideoTracks[0].Clips = []Clip{
		mtClip("v1", "s1", 0, 10, 0),
		mtClip("v2", "s1", 20, 25, 15), // 5s gap on the video track
	}
	p.AudioTracks[0].Clips = []Clip{
		mtClip("a1", "s1", 0, 10, 0),
		mtClip("a2", "s1", 20, 25, 15), // 5s gap on the audio track
	}
	args, _, err := BuildExportArgs(p)
	if err != nil {
		t.Fatal(err)
	}
	filter := filterOf(t, args)

	// The base canvas spans the full programDur (20s); video gap is just
	// the time window with no overlay enabled.
	if !strings.Contains(filter, "color=c=black:s=1920x1080:r=30:d=20,format=yuv420p[base]") {
		t.Errorf("base must span full programDur: %s", filter)
	}
	// Video gaps no longer produce per-segment color=c=black inserts.
	if strings.Contains(filter, "color=c=black:s=1920x1080:r=30:d=5") {
		t.Errorf("v0.5.1 should not emit per-gap black segments: %s", filter)
	}
	// Both segments are present with their own enable windows.
	wantSegments := []string{
		"setpts=PTS-STARTPTS+0/TB,",
		"setpts=PTS-STARTPTS+15/TB,",
	}
	for _, w := range wantSegments {
		if !strings.Contains(filter, w) {
			t.Errorf("missing segment shift %q: %s", w, filter)
		}
	}
	// Audio still has its anullsrc gap segment.
	if !strings.Contains(filter, "anullsrc=r=48000:cl=stereo:d=5") {
		t.Errorf("audio gap must still emit anullsrc: %s", filter)
	}
}

// TestBuildExportArgs_V050BackwardCompat: a project loaded as v0.5.0
// (schemaVersion=1, zero Canvas, all clips with zero Transform) must
// migrate cleanly and produce a working filter graph. The Migrate-injected
// canvas comes from max(referenced video sources), and Transform fills
// to full canvas — i.e. all segments are full-screen, replicating the
// old behavior.
func TestBuildExportArgs_V050BackwardCompat(t *testing.T) {
	// Legacy-shaped project: no Canvas, no per-clip Transform.
	p := &Project{
		ID:            "legacy",
		Kind:          KindMultitrack,
		SchemaVersion: 1,
		Sources: []Source{
			{ID: "s1", Path: "/tmp/v1.mp4", Kind: SourceVideo, Duration: 60, Width: 1920, Height: 1080, FrameRate: 30, HasAudio: true},
			{ID: "s2", Path: "/tmp/v2.mp4", Kind: SourceVideo, Duration: 60, Width: 1280, Height: 720, FrameRate: 25, HasAudio: true},
		},
		AudioVolume: 1.0,
		VideoTracks: []VideoTrack{
			{
				ID: "vt1",
				Clips: []Clip{
					{Clip: common.Clip{ID: "v1", SourceStart: 0, SourceEnd: 5, ProgramStart: 0}, SourceID: "s1"},
					{Clip: common.Clip{ID: "v2", SourceStart: 0, SourceEnd: 5, ProgramStart: 5}, SourceID: "s2"},
				},
			},
		},
		Export: common.ExportSettings{
			Format: "mp4", VideoCodec: "h264", AudioCodec: "aac",
			OutputDir: "/tmp", OutputName: "legacy",
		},
	}
	p.Migrate()

	// Migrated canvas = max(1920, 1280) × max(1080, 720) @ max(30, 25) = 1920×1080@30.
	if p.Canvas.Width != 1920 || p.Canvas.Height != 1080 || p.Canvas.FrameRate != 30 {
		t.Fatalf("Migrate canvas = %+v, want 1920x1080@30", p.Canvas)
	}
	// Every clip's transform = full canvas.
	for _, c := range p.VideoTracks[0].Clips {
		if c.Transform != (Transform{X: 0, Y: 0, W: 1920, H: 1080}) {
			t.Errorf("Migrate clip %q transform = %+v, want full canvas", c.ID, c.Transform)
		}
	}
	// SchemaVersion bumped.
	if p.SchemaVersion != SchemaVersion {
		t.Fatalf("SchemaVersion = %d, want %d", p.SchemaVersion, SchemaVersion)
	}
	// And the export builds without error.
	args, _, err := BuildExportArgs(p)
	if err != nil {
		t.Fatalf("legacy project should export: %v", err)
	}
	filter := filterOf(t, args)
	wantPieces := []string{
		"color=c=black:s=1920x1080:r=30:d=10,format=yuv420p[base]",
		"scale=1920:1080,setsar=1,fps=30,format=yuva420p[seg_0]",
		"scale=1920:1080,setsar=1,fps=30,format=yuva420p[seg_1]",
	}
	for _, w := range wantPieces {
		if !strings.Contains(filter, w) {
			t.Errorf("missing legacy-migrated piece %q: %s", w, filter)
		}
	}
}
