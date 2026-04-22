package server

import (
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func argsString(args []string) string { return strings.Join(args, " ") }

func assertArgsContain(t *testing.T, args []string, wants ...string) {
	t.Helper()
	joined := argsString(args)
	for _, w := range wants {
		if !strings.Contains(joined, w) {
			t.Errorf("args missing %q\n  got: %s", w, joined)
		}
	}
}

func assertArgsAbsent(t *testing.T, args []string, absent ...string) {
	t.Helper()
	joined := argsString(args)
	for _, a := range absent {
		if strings.Contains(joined, a) {
			t.Errorf("args should not contain %q\n  got: %s", a, joined)
		}
	}
}

// ---------------- buildConvertAudioArgs ----------------

func TestBuildConvertAudioArgs(t *testing.T) {
	tests := []struct {
		name            string
		req             AudioRequest
		wantErr         bool
		wantArgsContain []string
		wantArgsAbsent  []string
		wantOutputBase  string
	}{
		{
			name: "mp3 libmp3lame + 192 kbps + sr + channels",
			req: AudioRequest{
				Mode: "convert", InputPath: "/in.wav",
				OutputDir: "/out", OutputName: "song", Format: "mp3",
				Codec: "libmp3lame", Bitrate: "192", SampleRate: 44100, Channels: 2,
			},
			wantArgsContain: []string{"-vn", "-c:a libmp3lame", "-b:a 192k", "-ar 44100", "-ac 2"},
			wantOutputBase:  "song.mp3",
		},
		{
			name: "flac lossless ignores bitrate",
			req: AudioRequest{
				Mode: "convert", InputPath: "/in.wav",
				OutputDir: "/out", OutputName: "song", Format: "flac",
				Codec: "flac", Bitrate: "320",
			},
			wantArgsContain: []string{"-c:a flac"},
			wantArgsAbsent:  []string{"-b:a"},
			wantOutputBase:  "song.flac",
		},
		{
			name: "wav pcm codec ignores bitrate",
			req: AudioRequest{
				Mode: "convert", InputPath: "/in.mp3",
				OutputDir: "/out", OutputName: "x", Format: "wav",
				Codec: "pcm_s16le", Bitrate: "320",
			},
			wantArgsContain: []string{"-c:a pcm_s16le"},
			wantArgsAbsent:  []string{"-b:a"},
		},
		{
			name: "copy codec skips all encoder params",
			req: AudioRequest{
				Mode: "convert", InputPath: "/in.mp3",
				OutputDir: "/out", OutputName: "x", Format: "mp3",
				Codec: "copy", Bitrate: "192", SampleRate: 44100, Channels: 2,
			},
			wantArgsContain: []string{"-c:a copy"},
			wantArgsAbsent:  []string{"-b:a", "-ar", "-ac"},
		},
		{
			name: "default codec when empty",
			req: AudioRequest{
				Mode: "convert", InputPath: "/in.wav",
				OutputDir: "/out", OutputName: "x", Format: "m4a",
			},
			wantArgsContain: []string{"-c:a aac"},
		},
		{
			name: "bitrate=copy suppresses -b:a",
			req: AudioRequest{
				Mode: "convert", InputPath: "/in.wav",
				OutputDir: "/out", OutputName: "x", Format: "mp3",
				Codec: "libmp3lame", Bitrate: "copy",
			},
			wantArgsContain: []string{"-c:a libmp3lame"},
			wantArgsAbsent:  []string{"-b:a"},
		},
		{
			name: "invalid format",
			req: AudioRequest{
				Mode: "convert", InputPath: "/in.wav",
				OutputDir: "/out", OutputName: "x", Format: "xyz",
			},
			wantErr: true,
		},
		{
			name: "codec not allowed for format",
			req: AudioRequest{
				Mode: "convert", InputPath: "/in.wav",
				OutputDir: "/out", OutputName: "x", Format: "mp3", Codec: "aac",
			},
			wantErr: true,
		},
		{
			name:    "missing input",
			req:     AudioRequest{Mode: "convert", OutputDir: "/out", OutputName: "x", Format: "mp3"},
			wantErr: true,
		},
		{
			name:    "missing output dir",
			req:     AudioRequest{Mode: "convert", InputPath: "/in.wav", OutputName: "x", Format: "mp3"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := buildConvertAudioArgs(tt.req)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			assertArgsContain(t, got.Args, tt.wantArgsContain...)
			assertArgsAbsent(t, got.Args, tt.wantArgsAbsent...)
			if tt.wantOutputBase != "" && filepath.Base(got.OutputPath) != tt.wantOutputBase {
				t.Errorf("OutputPath base = %q, want %q", filepath.Base(got.OutputPath), tt.wantOutputBase)
			}
		})
	}
}

// ---------------- buildExtractAudioArgs ----------------

func TestBuildExtractAudioArgs(t *testing.T) {
	tests := []struct {
		name            string
		req             AudioRequest
		wantErr         bool
		wantArgsContain []string
		wantArgsAbsent  []string
	}{
		{
			name: "copy method with stream 0",
			req: AudioRequest{
				Mode: "extract", InputPath: "/in.mkv",
				OutputDir: "/out", OutputName: "track", Format: "m4a",
				ExtractMethod: "copy", AudioStreamIndex: 0,
			},
			wantArgsContain: []string{"-vn", "-map 0:a:0", "-c:a copy"},
		},
		{
			name: "copy method stream 1",
			req: AudioRequest{
				Mode: "extract", InputPath: "/in.mkv",
				OutputDir: "/out", OutputName: "track", Format: "opus",
				ExtractMethod: "copy", AudioStreamIndex: 1,
			},
			wantArgsContain: []string{"-map 0:a:1", "-c:a copy"},
		},
		{
			name: "transcode method with full params",
			req: AudioRequest{
				Mode: "extract", InputPath: "/in.mp4",
				OutputDir: "/out", OutputName: "track", Format: "mp3",
				ExtractMethod: "transcode",
				Codec:         "libmp3lame", Bitrate: "192",
				SampleRate: 44100, Channels: 2,
			},
			wantArgsContain: []string{"-map 0:a:0", "-c:a libmp3lame", "-b:a 192k", "-ar 44100", "-ac 2"},
		},
		{
			name: "default method is copy",
			req: AudioRequest{
				Mode: "extract", InputPath: "/in.mkv",
				OutputDir: "/out", OutputName: "track", Format: "mka",
			},
			wantArgsContain: []string{"-c:a copy"},
		},
		{
			name: "transcode cannot use codec copy",
			req: AudioRequest{
				Mode: "extract", InputPath: "/in.mkv",
				OutputDir: "/out", OutputName: "track", Format: "mp3",
				ExtractMethod: "transcode", Codec: "copy",
			},
			wantErr: true,
		},
		{
			name: "unknown extractMethod",
			req: AudioRequest{
				Mode: "extract", InputPath: "/in.mkv",
				OutputDir: "/out", OutputName: "track", Format: "mp3",
				ExtractMethod: "weird",
			},
			wantErr: true,
		},
		{
			name: "negative stream index",
			req: AudioRequest{
				Mode: "extract", InputPath: "/in.mkv",
				OutputDir: "/out", OutputName: "x", Format: "mp3",
				AudioStreamIndex: -1,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := buildExtractAudioArgs(tt.req)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			assertArgsContain(t, got.Args, tt.wantArgsContain...)
			assertArgsAbsent(t, got.Args, tt.wantArgsAbsent...)
		})
	}
}

// ---------------- merge helpers ----------------

func TestFormatConcatList(t *testing.T) {
	tests := []struct {
		name  string
		paths []string
		want  string
	}{
		{
			name:  "plain paths",
			paths: []string{"/a/b.mp3", "/c/d.mp3"},
			want:  "file '/a/b.mp3'\nfile '/c/d.mp3'\n",
		},
		{
			name:  "path with apostrophe is escaped",
			paths: []string{"/music/it's a song.mp3"},
			want:  "file '/music/it'\\''s a song.mp3'\n",
		},
		{
			name:  "empty",
			paths: []string{},
			want:  "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatConcatList(tt.paths)
			if got != tt.want {
				t.Errorf("got %q\nwant %q", got, tt.want)
			}
		})
	}
}

func TestBuildMergeCopyArgs(t *testing.T) {
	got := buildMergeCopyArgs("/tmp/list.txt", "/out/merged.mp3")
	want := []string{"-y", "-f", "concat", "-safe", "0", "-i", "/tmp/list.txt", "-c", "copy", "/out/merged.mp3"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestBuildMergeReencodeArgs(t *testing.T) {
	req := AudioRequest{
		InputPaths: []string{"/a.mp3", "/b.mp3", "/c.mp3"},
		Format:     "mp3", Codec: "libmp3lame", Bitrate: "192",
		SampleRate: 44100, Channels: 2,
	}
	got, err := buildMergeReencodeArgs(req, "/out/merged.mp3")
	if err != nil {
		t.Fatal(err)
	}
	assertArgsContain(t, got,
		"-i /a.mp3", "-i /b.mp3", "-i /c.mp3",
		"[0:a][1:a][2:a]concat=n=3:v=0:a=1[out]",
		"-map [out]", "-c:a libmp3lame", "-b:a 192k",
		"-ar 44100", "-ac 2",
		"/out/merged.mp3",
	)
}

func TestBuildMergeReencodeArgs_Errors(t *testing.T) {
	tests := []struct {
		name string
		req  AudioRequest
	}{
		{"invalid format", AudioRequest{InputPaths: []string{"/a", "/b"}, Format: "xyz"}},
		{"codec copy", AudioRequest{InputPaths: []string{"/a", "/b"}, Format: "mp3", Codec: "copy"}},
		{"invalid codec for format", AudioRequest{InputPaths: []string{"/a", "/b"}, Format: "mp3", Codec: "aac"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := buildMergeReencodeArgs(tt.req, "/out/x.mp3"); err == nil {
				t.Error("expected error, got nil")
			}
		})
	}
}

func TestBuildMergeAudioArgs_Dispatch(t *testing.T) {
	base := AudioRequest{
		Mode:       "merge",
		InputPaths: []string{"/a.mp3", "/b.mp3"},
		OutputDir:  "/out", OutputName: "m", Format: "mp3",
		Codec: "libmp3lame", Bitrate: "192",
	}

	t.Run("auto must be pre-resolved", func(t *testing.T) {
		req := base
		req.MergeStrategy = "auto"
		if _, err := buildMergeAudioArgs(req); err == nil {
			t.Error(`expected error for unresolved "auto"`)
		}
	})

	t.Run("unknown strategy", func(t *testing.T) {
		req := base
		req.MergeStrategy = "garbage"
		if _, err := buildMergeAudioArgs(req); err == nil {
			t.Error("expected error")
		}
	})

	t.Run("too few inputs", func(t *testing.T) {
		req := base
		req.InputPaths = []string{"/only.mp3"}
		if _, err := buildMergeAudioArgs(req); err == nil {
			t.Error("expected error for single input")
		}
	})

	t.Run("reencode branch", func(t *testing.T) {
		req := base
		req.MergeStrategy = "reencode"
		got, err := buildMergeAudioArgs(req)
		if err != nil {
			t.Fatal(err)
		}
		assertArgsContain(t, got.Args, "-filter_complex", "-c:a libmp3lame")
		if got.Cleanup != nil {
			t.Error("reencode should not schedule cleanup")
		}
	})

	t.Run("copy branch writes list file", func(t *testing.T) {
		req := base
		req.MergeStrategy = "copy"
		got, err := buildMergeAudioArgs(req)
		if err != nil {
			t.Fatal(err)
		}
		if got.Cleanup == nil {
			t.Error("copy branch must provide a cleanup closure for the list file")
		}
		assertArgsContain(t, got.Args, "-f", "concat", "-c", "copy")
		got.Cleanup() // make sure it doesn't panic
	})
}

// ---------------- bitrateApplies ----------------

func TestBitrateApplies(t *testing.T) {
	mp3 := audioFormatTable["mp3"]
	flac := audioFormatTable["flac"]
	wav := audioFormatTable["wav"]

	tests := []struct {
		name    string
		spec    audioFormatSpec
		codec   string
		bitrate string
		want    bool
	}{
		{"mp3 + libmp3lame + 192", mp3, "libmp3lame", "192", true},
		{"mp3 + copy bitrate", mp3, "libmp3lame", "copy", false},
		{"mp3 + empty bitrate", mp3, "libmp3lame", "", false},
		{"flac (lossless container)", flac, "flac", "320", false},
		{"wav + pcm codec", wav, "pcm_s16le", "320", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := bitrateApplies(tt.spec, tt.codec, tt.bitrate); got != tt.want {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

// ---------------- top-level dispatcher ----------------

func TestBuildAudioArgs_UnknownMode(t *testing.T) {
	if _, err := BuildAudioArgs(AudioRequest{Mode: "nope"}); err == nil {
		t.Error("expected error for unknown mode")
	}
}
