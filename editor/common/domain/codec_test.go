package domain

import "testing"

func TestNormalizeVideoCodec(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"", "libx264"},
		{"h264", "libx264"},
		{"H264", "libx264"},
		{"  h264 ", "libx264"},
		{"h265", "libx265"},
		{"libx264rgb", "libx264rgb"}, // passthrough
		{"h264_nvenc", "h264_nvenc"}, // passthrough
	}
	for _, c := range cases {
		if got := NormalizeVideoCodec(c.in); got != c.want {
			t.Errorf("NormalizeVideoCodec(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestNormalizeAudioCodec(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"", "aac"},
		{"aac", "aac"},
		{"libopus", "libopus"},
	}
	for _, c := range cases {
		if got := NormalizeAudioCodec(c.in); got != c.want {
			t.Errorf("NormalizeAudioCodec(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
