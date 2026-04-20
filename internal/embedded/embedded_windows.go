//go:build windows

package embedded

import _ "embed"

//go:embed windows/windows.7z
var archiveData []byte

const (
	ffmpegBinaryName  = "ffmpeg.exe"
	ffprobeBinaryName = "ffprobe.exe"
)
