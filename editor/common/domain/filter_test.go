package domain

import (
	"strings"
	"testing"
)

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
		if got := FormatFloat(c.v); got != c.want {
			t.Errorf("FormatFloat(%v) = %q, want %q", c.v, got, c.want)
		}
	}
}

func TestBuildVideoTrackFilter_Basic(t *testing.T) {
	clips := []Clip{
		{ID: "v1", SourceStart: 0, SourceEnd: 10, ProgramStart: 0},
		{ID: "v2", SourceStart: 20, SourceEnd: 25, ProgramStart: 10},
	}
	parts := BuildVideoTrackFilter(clips, "[0:v]", "[v]", 15, 1920, 1080, 30)
	chain := strings.Join(parts, ";")
	wants := []string{
		"[0:v]trim=start=0:end=10",
		"[0:v]trim=start=20:end=25",
		"concat=n=2:v=1:a=0[v]",
	}
	for _, w := range wants {
		if !strings.Contains(chain, w) {
			t.Errorf("chain missing %q\nchain: %s", w, chain)
		}
	}
}

func TestBuildVideoTrackFilter_GapInsertsBlackPad(t *testing.T) {
	clips := []Clip{
		{ID: "v1", SourceStart: 0, SourceEnd: 5, ProgramStart: 0},
		{ID: "v2", SourceStart: 10, SourceEnd: 15, ProgramStart: 10}, // 5s gap
	}
	parts := BuildVideoTrackFilter(clips, "[0:v]", "[v]", 15, 1920, 1080, 30)
	chain := strings.Join(parts, ";")
	if !strings.Contains(chain, "color=c=black:s=1920x1080:r=30:d=5") {
		t.Errorf("missing 5s black pad: %s", chain)
	}
	if !strings.Contains(chain, "concat=n=3:v=1:a=0[v]") {
		t.Errorf("expected n=3 concat (clip+gap+clip): %s", chain)
	}
}

func TestBuildVideoTrackFilter_TrailingPadToTotalDur(t *testing.T) {
	clips := []Clip{{ID: "v1", SourceStart: 0, SourceEnd: 10, ProgramStart: 0}}
	parts := BuildVideoTrackFilter(clips, "[0:v]", "[v]", 25, 1920, 1080, 30)
	chain := strings.Join(parts, ";")
	if !strings.Contains(chain, "color=c=black:s=1920x1080:r=30:d=15") {
		t.Errorf("missing 15s trailing pad: %s", chain)
	}
	if !strings.Contains(chain, "concat=n=2:v=1:a=0[v]") {
		t.Errorf("expected n=2 concat: %s", chain)
	}
}

func TestBuildVideoTrackFilter_CustomLabels(t *testing.T) {
	// Multitrack uses input label "[1:v]" and unique output label per
	// track e.g. "[v_t0]". Confirm the filter graph honors them
	// verbatim — this is the contract that lets multitrack assemble
	// multiple track filters without label collisions.
	clips := []Clip{{ID: "v1", SourceStart: 0, SourceEnd: 10, ProgramStart: 0}}
	parts := BuildVideoTrackFilter(clips, "[2:v]", "[v_t1]", 10, 1920, 1080, 30)
	chain := strings.Join(parts, ";")
	if !strings.Contains(chain, "[2:v]trim=") {
		t.Errorf("chain should reference [2:v] input: %s", chain)
	}
	if !strings.Contains(chain, "concat=n=1:v=1:a=0[v_t1]") {
		t.Errorf("chain should output to [v_t1]: %s", chain)
	}
}

func TestBuildAudioTrackFilter_UnityVolumeNoVolumeFilter(t *testing.T) {
	clips := []Clip{{ID: "a1", SourceStart: 0, SourceEnd: 10, ProgramStart: 0}}
	parts := BuildAudioTrackFilter(clips, "[0:a]", "[a]", "[a_pre]", 1.0, 10)
	chain := strings.Join(parts, ";")
	if strings.Contains(chain, "volume=") {
		t.Errorf("unity volume should not emit volume filter: %s", chain)
	}
	if !strings.Contains(chain, "concat=n=1:v=0:a=1[a]") {
		t.Errorf("audio chain should end at [a]: %s", chain)
	}
}

func TestBuildAudioTrackFilter_NonUnityRoutesViaPre(t *testing.T) {
	clips := []Clip{{ID: "a1", SourceStart: 0, SourceEnd: 10, ProgramStart: 0}}
	parts := BuildAudioTrackFilter(clips, "[0:a]", "[a]", "[a_pre]", 0.5, 10)
	chain := strings.Join(parts, ";")
	if !strings.Contains(chain, "concat=n=1:v=0:a=1[a_pre]") {
		t.Errorf("concat should output to [a_pre]: %s", chain)
	}
	if !strings.Contains(chain, "[a_pre]volume=0.5[a]") {
		t.Errorf("volume filter should map [a_pre] to [a]: %s", chain)
	}
}

func TestBuildAudioTrackFilter_GapInsertsAnullsrc(t *testing.T) {
	clips := []Clip{
		{ID: "a1", SourceStart: 0, SourceEnd: 5, ProgramStart: 0},
		{ID: "a2", SourceStart: 10, SourceEnd: 15, ProgramStart: 10}, // 5s gap
	}
	parts := BuildAudioTrackFilter(clips, "[0:a]", "[a]", "[a_pre]", 1.0, 15)
	chain := strings.Join(parts, ";")
	if !strings.Contains(chain, "anullsrc=r=48000:cl=stereo:d=5") {
		t.Errorf("missing 5s silence: %s", chain)
	}
	if !strings.Contains(chain, "concat=n=3:v=0:a=1[a]") {
		t.Errorf("expected n=3 concat: %s", chain)
	}
}

func TestBuildAudioTrackFilter_AudioFormatExprApplied(t *testing.T) {
	// Both real clips and gap segments must carry the aformat snippet so
	// concat sees homogeneous PCM specs. A regression here would manifest
	// as an ffmpeg "input streams not matching" error at export time.
	clips := []Clip{{ID: "a1", SourceStart: 0, SourceEnd: 5, ProgramStart: 0}}
	parts := BuildAudioTrackFilter(clips, "[0:a]", "[a]", "[a_pre]", 1.0, 10)
	chain := strings.Join(parts, ";")
	if !strings.Contains(chain, AudioFormatExpr) {
		t.Errorf("missing AudioFormatExpr in chain: %s", chain)
	}
}
