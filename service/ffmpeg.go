package service

import (
	"easy-ffmpeg/internal/embedded"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
)

// GetFFmpegPath returns the ffmpeg binary path.
// Prefers embedded, falls back to "ffmpeg" on system PATH.
func GetFFmpegPath() string {
	if path, err := embedded.GetFFmpegBinary(); err == nil {
		return path
	}
	return "ffmpeg"
}

// Prepare extracts the embedded ffmpeg archive if needed.
// Call once at startup so the first API request doesn't block on decompression.
func Prepare() error {
	_, err := embedded.GetFFmpegBinary()
	return err
}

// GetFFmpegDir returns the directory containing the ffmpeg binary.
// Prefers embedded (cache dir), falls back to the directory of ffmpeg on PATH.
func GetFFmpegDir() (string, error) {
	if path, err := embedded.GetFFmpegBinary(); err == nil {
		return filepath.Dir(path), nil
	}
	if path, err := exec.LookPath("ffmpeg"); err == nil {
		return filepath.Dir(path), nil
	}
	return "", fmt.Errorf("ffmpeg not found")
}

// GetFFprobePath returns the ffprobe binary path.
func GetFFprobePath() string {
	if path, err := embedded.GetFFprobeBinary(); err == nil {
		return path
	}
	return "ffprobe"
}

// CheckFFmpeg returns whether any ffmpeg (embedded or system) is runnable.
func CheckFFmpeg() bool {
	if embedded.CheckEmbeddedFFmpeg() {
		return true
	}
	return exec.Command("ffmpeg", "-version").Run() == nil
}

// GetFFmpegVersion returns the first line of ffmpeg -version output, or empty.
func GetFFmpegVersion() string {
	path := GetFFmpegPath()
	out, err := exec.Command(path, "-version").Output()
	if err != nil {
		return ""
	}
	lines := strings.SplitN(string(out), "\n", 2)
	if len(lines) > 0 {
		return strings.TrimSpace(lines[0])
	}
	return ""
}

// IsEmbedded reports whether the embedded binary is being used.
func IsEmbedded() bool {
	_, err := embedded.GetFFmpegBinary()
	return err == nil
}
