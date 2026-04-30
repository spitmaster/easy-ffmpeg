package service

import (
	"easy-ffmpeg/internal/procutil"
	"encoding/json"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

// MediaFormat describes container-level information common to audio and video files.
type MediaFormat struct {
	Duration float64 `json:"duration"`
	BitRate  int     `json:"bitrate"`
	Size     int64   `json:"size"`
}

// AudioStream describes a single audio track in a media file.
// Index is the 0-based position among audio streams, suitable for `-map 0:a:<Index>`.
type AudioStream struct {
	Index      int    `json:"index"`
	CodecName  string `json:"codecName"`
	Channels   int    `json:"channels"`
	SampleRate int    `json:"sampleRate"`
	BitRate    int    `json:"bitRate"`
	Language   string `json:"lang,omitempty"`
	Title      string `json:"title,omitempty"`
}

// VideoStream describes the primary video stream of a media file.
type VideoStream struct {
	CodecName string  `json:"codecName"`
	Width     int     `json:"width"`
	Height    int     `json:"height"`
	FrameRate float64 `json:"frameRate"`
}

// ProbeResult is what ProbeAudio returns (audio-only probe).
type ProbeResult struct {
	Format  MediaFormat   `json:"format"`
	Streams []AudioStream `json:"streams"`
}

// VideoProbeResult is what ProbeVideo returns.
// Video may be zero-valued if the file contains no video; Audio nil likewise.
type VideoProbeResult struct {
	Format MediaFormat  `json:"format"`
	Video  VideoStream  `json:"video"`
	Audio  *AudioStream `json:"audio,omitempty"`
}

// ProbeAudio runs ffprobe and returns audio stream + container information.
// Only audio streams are returned (thanks to `-select_streams a`).
func ProbeAudio(path string) (*ProbeResult, error) {
	out, err := runFFprobe(path, "-select_streams", "a")
	if err != nil {
		return nil, err
	}

	var raw rawProbe
	if err := json.Unmarshal(out, &raw); err != nil {
		return nil, fmt.Errorf("parse ffprobe output: %w", err)
	}

	streams := make([]AudioStream, 0, len(raw.Streams))
	for i, s := range raw.Streams {
		streams = append(streams, audioStreamFromRaw(i, s))
	}
	return &ProbeResult{Format: mediaFormatFromRaw(raw.Format), Streams: streams}, nil
}

// ProbeVideo runs ffprobe and returns primary video / audio streams + container info.
func ProbeVideo(path string) (*VideoProbeResult, error) {
	out, err := runFFprobe(path)
	if err != nil {
		return nil, err
	}

	var raw rawProbe
	if err := json.Unmarshal(out, &raw); err != nil {
		return nil, fmt.Errorf("parse ffprobe output: %w", err)
	}

	res := &VideoProbeResult{Format: mediaFormatFromRaw(raw.Format)}
	audioIndex := 0
	for _, s := range raw.Streams {
		switch s.CodecType {
		case "video":
			if res.Video.CodecName == "" {
				res.Video = VideoStream{
					CodecName: s.CodecName,
					Width:     s.Width,
					Height:    s.Height,
					FrameRate: parseRational(s.AvgFrameRate, s.RFrameRate),
				}
			}
		case "audio":
			if res.Audio == nil {
				as := audioStreamFromRaw(audioIndex, s)
				res.Audio = &as
			}
			audioIndex++
		}
	}
	return res, nil
}

// ---------------- internal helpers ----------------

type rawStream struct {
	CodecName    string            `json:"codec_name"`
	CodecType    string            `json:"codec_type"`
	Channels     int               `json:"channels"`
	SampleRate   string            `json:"sample_rate"`
	BitRate      string            `json:"bit_rate"`
	Width        int               `json:"width"`
	Height       int               `json:"height"`
	RFrameRate   string            `json:"r_frame_rate"`
	AvgFrameRate string            `json:"avg_frame_rate"`
	Tags         map[string]string `json:"tags"`
}

type rawFormat struct {
	Duration string `json:"duration"`
	BitRate  string `json:"bit_rate"`
	Size     string `json:"size"`
}

type rawProbe struct {
	Streams []rawStream `json:"streams"`
	Format  rawFormat   `json:"format"`
}

func runFFprobe(path string, extra ...string) ([]byte, error) {
	args := append([]string{
		"-v", "quiet",
		"-print_format", "json",
		"-show_format",
		"-show_streams",
	}, extra...)
	args = append(args, path)

	cmd := exec.Command(GetFFprobePath(), args...)
	procutil.HideWindow(cmd)
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("ffprobe failed: %w", err)
	}
	return out, nil
}

func mediaFormatFromRaw(f rawFormat) MediaFormat {
	return MediaFormat{
		Duration: atof(f.Duration),
		BitRate:  atoi(f.BitRate),
		Size:     atoi64(f.Size),
	}
}

func audioStreamFromRaw(index int, s rawStream) AudioStream {
	return AudioStream{
		Index:      index,
		CodecName:  s.CodecName,
		Channels:   s.Channels,
		SampleRate: atoi(s.SampleRate),
		BitRate:    atoi(s.BitRate),
		Language:   s.Tags["language"],
		Title:      s.Tags["title"],
	}
}

// parseRational turns ffprobe's "num/den" rational strings into a float.
// Falls back through a list of candidates (first non-zero wins).
func parseRational(candidates ...string) float64 {
	for _, s := range candidates {
		parts := strings.Split(s, "/")
		if len(parts) == 2 {
			num := atof(parts[0])
			den := atof(parts[1])
			if den > 0 && num > 0 {
				return num / den
			}
			continue
		}
		if v := atof(s); v > 0 {
			return v
		}
	}
	return 0
}

func atoi(s string) int {
	v, _ := strconv.Atoi(s)
	return v
}

func atoi64(s string) int64 {
	v, _ := strconv.ParseInt(s, 10, 64)
	return v
}

func atof(s string) float64 {
	v, _ := strconv.ParseFloat(s, 64)
	return v
}
