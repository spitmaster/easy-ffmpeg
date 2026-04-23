package server

import (
	"path/filepath"
	"strings"
	"testing"
)

// ---------------- BuildTrimArgs: primary happy paths ----------------

func TestBuildTrimArgs(t *testing.T) {
	base := TrimRequest{
		InputPath:    "/in.mp4",
		OutputDir:    "/out",
		OutputName:   "clip",
		Format:       "mp4",
		VideoEncoder: "h264",
		AudioEncoder: "aac",
	}

	tests := []struct {
		name            string
		req             TrimRequest
		wantArgsContain []string
		wantArgsAbsent  []string
		wantOutputBase  string
	}{
		{
			name: "time trim only",
			req: func() TrimRequest {
				r := base
				r.Trim = TrimOperation{Enabled: true, Start: "00:00:10", End: "00:00:30"}
				return r
			}(),
			wantArgsContain: []string{
				"-i /in.mp4", "-ss 00:00:10", "-to 00:00:30",
				"-c:v libx264", "-c:a aac",
			},
			wantArgsAbsent: []string{"-vf"},
			wantOutputBase: "clip.mp4",
		},
		{
			name: "crop only",
			req: func() TrimRequest {
				r := base
				r.Crop = CropOperation{Enabled: true, X: 10, Y: 20, W: 640, H: 480}
				return r
			}(),
			wantArgsContain: []string{"-vf crop=640:480:10:20", "-c:v libx264"},
			wantArgsAbsent:  []string{"-ss", "scale="},
		},
		{
			name: "scale only (explicit w & h)",
			req: func() TrimRequest {
				r := base
				r.Scale = ScaleOperation{Enabled: true, W: 1280, H: 720}
				return r
			}(),
			wantArgsContain: []string{"-vf scale=1280:720"},
			wantArgsAbsent:  []string{"-ss", "crop="},
		},
		{
			name: "scale keep ratio, width only -> height=-2",
			req: func() TrimRequest {
				r := base
				r.Scale = ScaleOperation{Enabled: true, W: 1280, H: 0, KeepRatio: true}
				return r
			}(),
			wantArgsContain: []string{"-vf scale=1280:-2"},
		},
		{
			name: "scale keep ratio, height only -> width=-2",
			req: func() TrimRequest {
				r := base
				r.Scale = ScaleOperation{Enabled: true, W: 0, H: 720, KeepRatio: true}
				return r
			}(),
			wantArgsContain: []string{"-vf scale=-2:720"},
		},
		{
			name: "trim + crop + scale (filter chain order: crop,scale)",
			req: func() TrimRequest {
				r := base
				r.Trim = TrimOperation{Enabled: true, Start: "00:00:01", End: "00:00:05.500"}
				r.Crop = CropOperation{Enabled: true, X: 0, Y: 0, W: 1920, H: 1080}
				r.Scale = ScaleOperation{Enabled: true, W: 854, H: 480}
				return r
			}(),
			wantArgsContain: []string{
				"-ss 00:00:01", "-to 00:00:05.500",
				"-vf crop=1920:1080:0:0,scale=854:480",
				"-c:v libx264", "-c:a aac",
			},
		},
		{
			name: "videoEncoder passthrough for vp9",
			req: func() TrimRequest {
				r := base
				r.VideoEncoder = "vp9"
				r.Trim = TrimOperation{Enabled: true, Start: "00:00:00", End: "00:00:10"}
				return r
			}(),
			wantArgsContain: []string{"-c:v vp9"},
		},
		{
			name: "audio encoder copy is allowed (only video forbids copy)",
			req: func() TrimRequest {
				r := base
				r.AudioEncoder = "copy"
				r.Trim = TrimOperation{Enabled: true, Start: "00:00:00", End: "00:00:05"}
				return r
			}(),
			wantArgsContain: []string{"-c:a copy"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := BuildTrimArgs(tt.req)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			joined := strings.Join(got.Args, " ")
			for _, w := range tt.wantArgsContain {
				if !strings.Contains(joined, w) {
					t.Errorf("args missing %q\n  got: %s", w, joined)
				}
			}
			for _, a := range tt.wantArgsAbsent {
				if strings.Contains(joined, a) {
					t.Errorf("args should not contain %q\n  got: %s", a, joined)
				}
			}
			if tt.wantOutputBase != "" && filepath.Base(got.OutputPath) != tt.wantOutputBase {
				t.Errorf("OutputPath base = %q, want %q",
					filepath.Base(got.OutputPath), tt.wantOutputBase)
			}
		})
	}
}

// ---------------- BuildTrimArgs: errors ----------------

func TestBuildTrimArgs_Errors(t *testing.T) {
	base := TrimRequest{
		InputPath: "/in.mp4", OutputDir: "/out", OutputName: "c", Format: "mp4",
		VideoEncoder: "h264", AudioEncoder: "aac",
	}

	tests := []struct {
		name string
		mut  func(r *TrimRequest)
	}{
		{"missing input", func(r *TrimRequest) { r.InputPath = "" }},
		{"missing output dir", func(r *TrimRequest) { r.OutputDir = "" }},
		{"no operation enabled", func(r *TrimRequest) { /* leave all disabled */ }},
		{
			"video encoder = copy is rejected",
			func(r *TrimRequest) {
				r.VideoEncoder = "copy"
				r.Trim = TrimOperation{Enabled: true, Start: "00:00:00", End: "00:00:05"}
			},
		},
		{
			"trim start >= end",
			func(r *TrimRequest) {
				r.Trim = TrimOperation{Enabled: true, Start: "00:01:00", End: "00:00:30"}
			},
		},
		{
			"trim bad format",
			func(r *TrimRequest) {
				r.Trim = TrimOperation{Enabled: true, Start: "1m", End: "2m"}
			},
		},
		{
			"crop zero width",
			func(r *TrimRequest) {
				r.Crop = CropOperation{Enabled: true, W: 0, H: 100}
			},
		},
		{
			"crop negative x",
			func(r *TrimRequest) {
				r.Crop = CropOperation{Enabled: true, X: -1, W: 100, H: 100}
			},
		},
		{
			"scale no keep-ratio, zero height",
			func(r *TrimRequest) {
				r.Scale = ScaleOperation{Enabled: true, W: 1280, H: 0}
			},
		},
		{
			"scale keep-ratio, both zero",
			func(r *TrimRequest) {
				r.Scale = ScaleOperation{Enabled: true, W: 0, H: 0, KeepRatio: true}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := base
			tt.mut(&r)
			if _, err := BuildTrimArgs(r); err == nil {
				t.Error("expected error, got nil")
			}
		})
	}
}

// ---------------- unit-level helpers ----------------

func TestParseTimeSeconds(t *testing.T) {
	tests := []struct {
		in   string
		want float64
	}{
		{"00:00:00", 0},
		{"00:00:10", 10},
		{"00:01:30", 90},
		{"01:23:45", 5025},
		{"00:00:00.500", 0.5},
		{"00:00:01.250", 1.25},
		{"00:00:01.5", 1.5},    // partial ms padded
		{"00:00:01.05", 1.05},  // two-digit ms
		{"99:59:59", 99*3600 + 59*60 + 59},
		{"bogus", 0}, // parseTimeSeconds returns 0 for non-matching strings
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got := parseTimeSeconds(tt.in)
			if got != tt.want {
				t.Errorf("parseTimeSeconds(%q) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

func TestResolveScale(t *testing.T) {
	tests := []struct {
		name     string
		s        ScaleOperation
		wantW    int
		wantH    int
		wantErr  bool
	}{
		{"both positive", ScaleOperation{W: 1280, H: 720}, 1280, 720, false},
		{"keep ratio, both positive", ScaleOperation{W: 1280, H: 720, KeepRatio: true}, 1280, 720, false},
		{"keep ratio, width only", ScaleOperation{W: 1280, H: 0, KeepRatio: true}, 1280, -2, false},
		{"keep ratio, height only", ScaleOperation{W: 0, H: 720, KeepRatio: true}, -2, 720, false},
		{"keep ratio, both zero", ScaleOperation{W: 0, H: 0, KeepRatio: true}, 0, 0, true},
		{"no keep ratio, both zero", ScaleOperation{}, 0, 0, true},
		{"no keep ratio, one zero", ScaleOperation{W: 1280, H: 0}, 0, 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w, h, err := resolveScale(tt.s)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if w != tt.wantW || h != tt.wantH {
				t.Errorf("got (%d, %d), want (%d, %d)", w, h, tt.wantW, tt.wantH)
			}
		})
	}
}

func TestValidateTrim(t *testing.T) {
	ok := []TrimOperation{
		{Enabled: true, Start: "00:00:00", End: "00:00:01"},
		{Enabled: true, Start: "00:00:00.000", End: "00:00:00.500"},
		{Enabled: true, Start: "1:2:3", End: "1:2:4"},
	}
	bad := []TrimOperation{
		{Enabled: true, Start: "", End: "00:00:10"},
		{Enabled: true, Start: "10s", End: "20s"},
		{Enabled: true, Start: "00:00:10", End: "00:00:10"},
		{Enabled: true, Start: "00:60:00", End: "00:60:10"},
	}
	for _, tt := range ok {
		if err := validateTrim(tt); err != nil {
			t.Errorf("expected ok for %+v, got %v", tt, err)
		}
	}
	for _, tt := range bad {
		if err := validateTrim(tt); err == nil {
			t.Errorf("expected error for %+v", tt)
		}
	}
}
