package server

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// TrimRequest is the payload for POST /api/trim/start. It carries three
// independent operation blocks (trim / crop / scale); any combination of the
// three is allowed — they are applied in a single ffmpeg invocation.
type TrimRequest struct {
	InputPath    string `json:"inputPath"`
	OutputDir    string `json:"outputDir"`
	OutputName   string `json:"outputName"`
	Format       string `json:"format"`
	VideoEncoder string `json:"videoEncoder"`
	AudioEncoder string `json:"audioEncoder"`
	Overwrite    bool   `json:"overwrite,omitempty"`

	Trim  TrimOperation  `json:"trim"`
	Crop  CropOperation  `json:"crop"`
	Scale ScaleOperation `json:"scale"`
}

type TrimOperation struct {
	Enabled bool   `json:"enabled"`
	Start   string `json:"start"` // "HH:MM:SS" or "HH:MM:SS.mmm"
	End     string `json:"end"`
}

type CropOperation struct {
	Enabled bool `json:"enabled"`
	X       int  `json:"x"`
	Y       int  `json:"y"`
	W       int  `json:"w"`
	H       int  `json:"h"`
}

type ScaleOperation struct {
	Enabled   bool `json:"enabled"`
	W         int  `json:"w"`
	H         int  `json:"h"`
	KeepRatio bool `json:"keepRatio"`
}

// TrimBuildResult mirrors AudioBuildResult's shape.
type TrimBuildResult struct {
	Args       []string
	OutputPath string
}

// BuildTrimArgs assembles the ffmpeg command for any combination of trim / crop / scale.
// Pure function — no I/O — so it's directly unit-testable.
//
// Command layout:
//	ffmpeg -y -i <input> [-ss <start> -to <end>] [-vf "crop=...,scale=..."] -c:v <v> -c:a <a> <output>
func BuildTrimArgs(req TrimRequest) (*TrimBuildResult, error) {
	if req.InputPath == "" {
		return nil, fmt.Errorf("missing inputPath")
	}
	if req.OutputDir == "" || req.OutputName == "" || req.Format == "" {
		return nil, fmt.Errorf("missing output dir / name / format")
	}
	if !req.Trim.Enabled && !req.Crop.Enabled && !req.Scale.Enabled {
		return nil, fmt.Errorf("请至少启用一项操作（时间裁剪 / 空间裁剪 / 分辨率缩放）")
	}

	videoCodec := normalizeVideoCodec(req.VideoEncoder)
	if videoCodec == "copy" {
		return nil, fmt.Errorf("裁剪 / 缩放需要重编码，请选择具体的视频编码器")
	}
	audioCodec := req.AudioEncoder
	if audioCodec == "" {
		audioCodec = "aac"
	}

	outputPath := filepath.Join(req.OutputDir, req.OutputName+"."+req.Format)
	args := []string{"-y", "-i", req.InputPath}

	if req.Trim.Enabled {
		if err := validateTrim(req.Trim); err != nil {
			return nil, err
		}
		args = append(args, "-ss", req.Trim.Start, "-to", req.Trim.End)
	}

	var filters []string
	if req.Crop.Enabled {
		if err := validateCrop(req.Crop); err != nil {
			return nil, err
		}
		filters = append(filters, fmt.Sprintf("crop=%d:%d:%d:%d",
			req.Crop.W, req.Crop.H, req.Crop.X, req.Crop.Y))
	}
	if req.Scale.Enabled {
		w, h, err := resolveScale(req.Scale)
		if err != nil {
			return nil, err
		}
		filters = append(filters, fmt.Sprintf("scale=%d:%d", w, h))
	}
	if len(filters) > 0 {
		args = append(args, "-vf", strings.Join(filters, ","))
	}

	args = append(args, "-c:v", videoCodec, "-c:a", audioCodec, outputPath)
	return &TrimBuildResult{Args: args, OutputPath: outputPath}, nil
}

// ---------------- validation helpers ----------------

var trimTimeRE = regexp.MustCompile(`^(\d{1,2}):(\d{1,2}):(\d{1,2})(?:\.(\d{1,3}))?$`)

func validateTrim(t TrimOperation) error {
	if !trimTimeRE.MatchString(t.Start) {
		return fmt.Errorf(`时间格式不合法: %q（应为 HH:MM:SS[.mmm]）`, t.Start)
	}
	if !trimTimeRE.MatchString(t.End) {
		return fmt.Errorf(`时间格式不合法: %q（应为 HH:MM:SS[.mmm]）`, t.End)
	}
	a := parseTimeSeconds(t.Start)
	b := parseTimeSeconds(t.End)
	if a >= b {
		return fmt.Errorf("时间裁剪：起始 %q 必须早于结束 %q", t.Start, t.End)
	}
	return nil
}

func parseTimeSeconds(s string) float64 {
	m := trimTimeRE.FindStringSubmatch(s)
	if m == nil {
		return 0
	}
	h, _ := strconv.Atoi(m[1])
	min, _ := strconv.Atoi(m[2])
	sec, _ := strconv.Atoi(m[3])
	var ms int
	if m[4] != "" {
		pad := m[4]
		for len(pad) < 3 {
			pad += "0"
		}
		ms, _ = strconv.Atoi(pad)
	}
	if min >= 60 || sec >= 60 {
		return 0
	}
	return float64(h*3600+min*60+sec) + float64(ms)/1000
}

func validateCrop(c CropOperation) error {
	if c.W <= 0 || c.H <= 0 {
		return fmt.Errorf("空间裁剪：宽与高必须大于 0")
	}
	if c.X < 0 || c.Y < 0 {
		return fmt.Errorf("空间裁剪：X / Y 不能为负")
	}
	return nil
}

// resolveScale returns the (W, H) pair to pass to ffmpeg's scale filter.
// When KeepRatio is true, a non-positive dimension is rewritten as -2
// (ffmpeg's "auto keep ratio, round to even") — exactly one such dimension allowed.
// When KeepRatio is false, both dimensions must be positive.
func resolveScale(s ScaleOperation) (int, int, error) {
	w, h := s.W, s.H
	if s.KeepRatio {
		if w <= 0 && h <= 0 {
			return 0, 0, fmt.Errorf("分辨率缩放：保持比例时至少填一维")
		}
		if w <= 0 {
			w = -2
		}
		if h <= 0 {
			h = -2
		}
		return w, h, nil
	}
	if w <= 0 || h <= 0 {
		return 0, 0, fmt.Errorf("分辨率缩放：宽与高必须大于 0")
	}
	return w, h, nil
}
