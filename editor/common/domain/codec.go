package domain

import "strings"

// NormalizeVideoCodec maps UI codec names to ffmpeg encoder names. The
// permissive default (passthrough on unknown names) lets advanced users
// type encoder names like "libx264rgb" or "h264_nvenc" directly without
// the editor blocking them at validation time. Empty / "h264" → libx264
// to keep existing project JSONs (which have no codec field) rendering
// the same as the M0 default.
func NormalizeVideoCodec(name string) string {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "", "h264":
		return "libx264"
	case "h265":
		return "libx265"
	default:
		return name
	}
}

// NormalizeAudioCodec mirrors NormalizeVideoCodec for audio. Empty
// defaults to aac, which matches ffmpeg's mp4 default; passthrough on
// unknown names.
func NormalizeAudioCodec(name string) string {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "":
		return "aac"
	default:
		return name
	}
}
