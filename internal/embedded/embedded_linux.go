//go:build linux

package embedded

import _ "embed"

//go:embed linux/linux.7z
var archiveData []byte

const (
	ffmpegBinaryName  = "ffmpeg"
	ffprobeBinaryName = "ffprobe"
)
