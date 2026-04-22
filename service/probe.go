package service

import (
	"easy-ffmpeg/internal/procutil"
	"encoding/json"
	"fmt"
	"os/exec"
	"strconv"
)

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

type AudioFormat struct {
	Duration float64 `json:"duration"`
	BitRate  int     `json:"bitrate"`
	Size     int64   `json:"size"`
}

type ProbeResult struct {
	Format  AudioFormat   `json:"format"`
	Streams []AudioStream `json:"streams"`
}

// ProbeAudio runs ffprobe and returns audio stream + container information.
// Only audio streams are returned (thanks to `-select_streams a`).
func ProbeAudio(path string) (*ProbeResult, error) {
	cmd := exec.Command(GetFFprobePath(),
		"-v", "quiet",
		"-print_format", "json",
		"-show_format",
		"-show_streams",
		"-select_streams", "a",
		path,
	)
	procutil.HideWindow(cmd)
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("ffprobe failed: %w", err)
	}

	var raw struct {
		Streams []struct {
			CodecName  string            `json:"codec_name"`
			Channels   int               `json:"channels"`
			SampleRate string            `json:"sample_rate"`
			BitRate    string            `json:"bit_rate"`
			Tags       map[string]string `json:"tags"`
		} `json:"streams"`
		Format struct {
			Duration string `json:"duration"`
			BitRate  string `json:"bit_rate"`
			Size     string `json:"size"`
		} `json:"format"`
	}
	if err := json.Unmarshal(out, &raw); err != nil {
		return nil, fmt.Errorf("parse ffprobe output: %w", err)
	}

	streams := make([]AudioStream, 0, len(raw.Streams))
	for i, s := range raw.Streams {
		streams = append(streams, AudioStream{
			Index:      i,
			CodecName:  s.CodecName,
			Channels:   s.Channels,
			SampleRate: atoi(s.SampleRate),
			BitRate:    atoi(s.BitRate),
			Language:   s.Tags["language"],
			Title:      s.Tags["title"],
		})
	}
	return &ProbeResult{
		Format: AudioFormat{
			Duration: atof(raw.Format.Duration),
			BitRate:  atoi(raw.Format.BitRate),
			Size:     atoi64(raw.Format.Size),
		},
		Streams: streams,
	}, nil
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
