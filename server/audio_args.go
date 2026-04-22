package server

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// AudioRequest is the union of fields for all audio-processing modes.
// Each mode uses only a subset; the mode-specific builder enforces which fields are required.
type AudioRequest struct {
	Mode string `json:"mode"` // "convert" | "extract" | "merge"

	InputPath  string `json:"inputPath,omitempty"`
	OutputDir  string `json:"outputDir"`
	OutputName string `json:"outputName"`
	Format     string `json:"format"`
	Codec      string `json:"codec,omitempty"`
	Bitrate    string `json:"bitrate,omitempty"`    // "192" (treated as kbps) or "copy" or ""
	SampleRate int    `json:"sampleRate,omitempty"` // 0 = keep original
	Channels   int    `json:"channels,omitempty"`   // 0 = keep original
	Overwrite  bool   `json:"overwrite,omitempty"`

	// extract-only (slice 4)
	AudioStreamIndex int    `json:"audioStreamIndex,omitempty"`
	ExtractMethod    string `json:"extractMethod,omitempty"` // "copy" | "transcode"

	// merge-only (slice 5)
	InputPaths    []string `json:"inputPaths,omitempty"`
	MergeStrategy string   `json:"mergeStrategy,omitempty"` // "auto" | "copy" | "reencode"
}

// AudioBuildResult is what a mode-specific builder returns to the handler.
type AudioBuildResult struct {
	Args       []string
	OutputPath string
	// Cleanup is invoked after the job ends (success, error, or cancel).
	// Only some modes need it — e.g. merge's concat-demuxer list file.
	Cleanup func()
}

// BuildAudioArgs dispatches to the mode-specific builder.
// Open/closed: adding a new mode means adding one case + one builder, no changes elsewhere.
func BuildAudioArgs(req AudioRequest) (*AudioBuildResult, error) {
	switch req.Mode {
	case "convert":
		return buildConvertAudioArgs(req)
	case "extract":
		return buildExtractAudioArgs(req)
	case "merge":
		return buildMergeAudioArgs(req)
	default:
		return nil, fmt.Errorf("unsupported audio mode: %q", req.Mode)
	}
}

// ---------------- convert mode ----------------

// audioFormatSpec describes which codecs a container supports and whether the
// container is inherently lossless (in which case bitrate controls are ignored).
type audioFormatSpec struct {
	Codecs   []string // first = default
	Lossless bool
}

var audioFormatTable = map[string]audioFormatSpec{
	"mp3":  {Codecs: []string{"libmp3lame", "copy"}},
	"m4a":  {Codecs: []string{"aac", "copy"}},
	"flac": {Codecs: []string{"flac", "copy"}, Lossless: true},
	"wav":  {Codecs: []string{"pcm_s16le", "pcm_s24le", "copy"}, Lossless: true},
	"ogg":  {Codecs: []string{"libvorbis", "libopus", "copy"}},
	"opus": {Codecs: []string{"libopus", "copy"}},
}

func buildConvertAudioArgs(req AudioRequest) (*AudioBuildResult, error) {
	if req.InputPath == "" {
		return nil, fmt.Errorf("missing inputPath")
	}
	if req.OutputDir == "" || req.OutputName == "" || req.Format == "" {
		return nil, fmt.Errorf("missing output dir / name / format")
	}

	spec, ok := audioFormatTable[req.Format]
	if !ok {
		return nil, fmt.Errorf("unsupported audio format: %q", req.Format)
	}
	codec := req.Codec
	if codec == "" {
		codec = spec.Codecs[0]
	}
	if !slices.Contains(spec.Codecs, codec) {
		return nil, fmt.Errorf("codec %q is not valid for format %q", codec, req.Format)
	}

	outputPath := filepath.Join(req.OutputDir, req.OutputName+"."+req.Format)
	args := []string{"-y", "-i", req.InputPath, "-vn"}

	if codec == "copy" {
		args = append(args, "-c:a", "copy")
	} else {
		args = append(args, "-c:a", codec)
		if bitrateApplies(spec, codec, req.Bitrate) {
			args = append(args, "-b:a", req.Bitrate+"k")
		}
		if req.SampleRate > 0 {
			args = append(args, "-ar", fmt.Sprintf("%d", req.SampleRate))
		}
		if req.Channels > 0 {
			args = append(args, "-ac", fmt.Sprintf("%d", req.Channels))
		}
	}
	args = append(args, outputPath)
	return &AudioBuildResult{Args: args, OutputPath: outputPath}, nil
}

// ---------------- extract mode ----------------

// buildExtractAudioArgs pulls an audio track out of a video file.
// method = "copy"       → stream-copy (infer container from ffprobe on the front-end)
// method = "transcode"  → full re-encode via the same format/codec/bitrate rules as convert
func buildExtractAudioArgs(req AudioRequest) (*AudioBuildResult, error) {
	if req.InputPath == "" {
		return nil, fmt.Errorf("missing inputPath")
	}
	if req.OutputDir == "" || req.OutputName == "" || req.Format == "" {
		return nil, fmt.Errorf("missing output dir / name / format")
	}
	if req.AudioStreamIndex < 0 {
		return nil, fmt.Errorf("audioStreamIndex must be >= 0")
	}

	method := req.ExtractMethod
	if method == "" {
		method = "copy"
	}

	outputPath := filepath.Join(req.OutputDir, req.OutputName+"."+req.Format)
	args := []string{
		"-y", "-i", req.InputPath,
		"-vn",
		"-map", fmt.Sprintf("0:a:%d", req.AudioStreamIndex),
	}

	switch method {
	case "copy":
		args = append(args, "-c:a", "copy")

	case "transcode":
		spec, ok := audioFormatTable[req.Format]
		if !ok {
			return nil, fmt.Errorf("unsupported audio format: %q", req.Format)
		}
		codec := req.Codec
		if codec == "" {
			codec = spec.Codecs[0]
		}
		if !slices.Contains(spec.Codecs, codec) {
			return nil, fmt.Errorf("codec %q is not valid for format %q", codec, req.Format)
		}
		if codec == "copy" {
			return nil, fmt.Errorf(`extractMethod "transcode" cannot use codec="copy"; use extractMethod "copy" instead`)
		}
		args = append(args, "-c:a", codec)
		if bitrateApplies(spec, codec, req.Bitrate) {
			args = append(args, "-b:a", req.Bitrate+"k")
		}
		if req.SampleRate > 0 {
			args = append(args, "-ar", fmt.Sprintf("%d", req.SampleRate))
		}
		if req.Channels > 0 {
			args = append(args, "-ac", fmt.Sprintf("%d", req.Channels))
		}

	default:
		return nil, fmt.Errorf("unknown extractMethod: %q", method)
	}

	args = append(args, outputPath)
	return &AudioBuildResult{Args: args, OutputPath: outputPath}, nil
}

// bitrateApplies reports whether `-b:a` should be added.
// Lossless containers, PCM codecs, and an empty / "copy" bitrate all suppress it.
func bitrateApplies(spec audioFormatSpec, codec, bitrate string) bool {
	if spec.Lossless {
		return false
	}
	if strings.HasPrefix(codec, "pcm_") {
		return false
	}
	if bitrate == "" || bitrate == "copy" {
		return false
	}
	return true
}

// ---------------- merge mode ----------------

// buildMergeAudioArgs handles the "copy" and "reencode" strategies.
// "auto" must be resolved to one of the two by the caller (via ffprobe);
// this keeps the builder side-effect-free except for the temp list file
// that the "copy" branch needs.
func buildMergeAudioArgs(req AudioRequest) (*AudioBuildResult, error) {
	if len(req.InputPaths) < 2 {
		return nil, fmt.Errorf("merge requires at least 2 inputs")
	}
	if req.OutputDir == "" || req.OutputName == "" || req.Format == "" {
		return nil, fmt.Errorf("missing output dir / name / format")
	}

	strategy := req.MergeStrategy
	if strategy == "" {
		strategy = "reencode"
	}

	outputPath := filepath.Join(req.OutputDir, req.OutputName+"."+req.Format)

	switch strategy {
	case "copy":
		listPath, cleanup, err := writeConcatList(req.InputPaths)
		if err != nil {
			return nil, err
		}
		args := buildMergeCopyArgs(listPath, outputPath)
		return &AudioBuildResult{Args: args, OutputPath: outputPath, Cleanup: cleanup}, nil

	case "reencode":
		args, err := buildMergeReencodeArgs(req, outputPath)
		if err != nil {
			return nil, err
		}
		return &AudioBuildResult{Args: args, OutputPath: outputPath}, nil

	case "auto":
		return nil, fmt.Errorf(`"auto" strategy must be resolved to "copy" or "reencode" before dispatch`)

	default:
		return nil, fmt.Errorf("unknown mergeStrategy: %q", strategy)
	}
}

// formatConcatList produces the content of a ffmpeg `-f concat` list file.
// Path single-quotes are escaped so paths with apostrophes don't break the parser.
func formatConcatList(inputPaths []string) string {
	var buf strings.Builder
	for _, p := range inputPaths {
		esc := strings.ReplaceAll(p, "'", `'\''`)
		fmt.Fprintf(&buf, "file '%s'\n", esc)
	}
	return buf.String()
}

// writeConcatList writes the list file to the OS temp dir and returns its path
// plus a cleanup closure. Split from buildMergeCopyArgs so the pure arg-builder
// can be tested in isolation.
func writeConcatList(inputPaths []string) (string, func(), error) {
	f, err := os.CreateTemp("", "easy-ffmpeg-merge-*.txt")
	if err != nil {
		return "", nil, fmt.Errorf("create temp list file: %w", err)
	}
	if _, err := f.WriteString(formatConcatList(inputPaths)); err != nil {
		f.Close()
		os.Remove(f.Name())
		return "", nil, fmt.Errorf("write list file: %w", err)
	}
	if err := f.Close(); err != nil {
		os.Remove(f.Name())
		return "", nil, err
	}
	path := f.Name()
	cleanup := func() { _ = os.Remove(path) }
	return path, cleanup, nil
}

func buildMergeCopyArgs(listPath, outputPath string) []string {
	return []string{
		"-y",
		"-f", "concat",
		"-safe", "0",
		"-i", listPath,
		"-c", "copy",
		outputPath,
	}
}

func buildMergeReencodeArgs(req AudioRequest, outputPath string) ([]string, error) {
	spec, ok := audioFormatTable[req.Format]
	if !ok {
		return nil, fmt.Errorf("unsupported audio format: %q", req.Format)
	}
	codec := req.Codec
	if codec == "" {
		codec = spec.Codecs[0]
	}
	if !slices.Contains(spec.Codecs, codec) {
		return nil, fmt.Errorf("codec %q is not valid for format %q", codec, req.Format)
	}
	if codec == "copy" {
		return nil, fmt.Errorf(`mergeStrategy "reencode" cannot use codec="copy"`)
	}

	args := []string{"-y"}
	for _, p := range req.InputPaths {
		args = append(args, "-i", p)
	}

	var filter strings.Builder
	for i := range req.InputPaths {
		fmt.Fprintf(&filter, "[%d:a]", i)
	}
	fmt.Fprintf(&filter, "concat=n=%d:v=0:a=1[out]", len(req.InputPaths))

	args = append(args,
		"-filter_complex", filter.String(),
		"-map", "[out]",
		"-c:a", codec,
	)
	if bitrateApplies(spec, codec, req.Bitrate) {
		args = append(args, "-b:a", req.Bitrate+"k")
	}
	if req.SampleRate > 0 {
		args = append(args, "-ar", fmt.Sprintf("%d", req.SampleRate))
	}
	if req.Channels > 0 {
		args = append(args, "-ac", fmt.Sprintf("%d", req.Channels))
	}
	args = append(args, outputPath)
	return args, nil
}
