//go:build darwin

package embedded

import _ "embed"

//go:embed darwin/darwin.7z
var archiveData []byte

const (
	ffmpegBinaryName  = "ffmpeg"
	ffprobeBinaryName = "ffprobe"
)
