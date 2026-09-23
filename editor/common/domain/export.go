package domain

import "errors"

// ExportSettings carries the user's export preferences. Persisted
// alongside both single-video and multitrack projects so the next
// export starts with the same choices.
type ExportSettings struct {
	Format     string `json:"format"`
	VideoCodec string `json:"videoCodec"`
	AudioCodec string `json:"audioCodec"`
	OutputDir  string `json:"outputDir"`
	OutputName string `json:"outputName"`
}

// ValidateExportSettings checks that the three mandatory fields are
// non-empty. Codec fields are intentionally not validated here —
// NormalizeVideoCodec / NormalizeAudioCodec are permissive on unknown
// names so users can type raw encoder names. Returns nil when valid.
func ValidateExportSettings(e ExportSettings) error {
	if e.OutputDir == "" || e.OutputName == "" || e.Format == "" {
		return errors.New("export: outputDir / outputName / format required")
	}
	return nil
}
