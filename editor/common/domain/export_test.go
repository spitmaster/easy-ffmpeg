package domain

import "testing"

func TestValidateExportSettings(t *testing.T) {
	cases := []struct {
		name    string
		in      ExportSettings
		wantErr bool
	}{
		{"happy", ExportSettings{Format: "mp4", OutputDir: "/tmp", OutputName: "out"}, false},
		{"missing format", ExportSettings{OutputDir: "/tmp", OutputName: "out"}, true},
		{"missing dir", ExportSettings{Format: "mp4", OutputName: "out"}, true},
		{"missing name", ExportSettings{Format: "mp4", OutputDir: "/tmp"}, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := ValidateExportSettings(c.in)
			if (err != nil) != c.wantErr {
				t.Errorf("err=%v, wantErr=%v", err, c.wantErr)
			}
		})
	}
}
