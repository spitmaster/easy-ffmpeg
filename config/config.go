package config

import (
	"os"
	"path/filepath"
	"strings"
)

func configPath(name string) (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "easy-ffmpeg", name), nil
}

func readText(name string) string {
	path, err := configPath(name)
	if err != nil {
		return ""
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

func writeText(name, value string) error {
	path, err := configPath(name)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(value), 0644)
}

func GetInputDir() string       { return readText("input_dir.txt") }
func SaveInputDir(d string) error { return writeText("input_dir.txt", d) }

func GetOutputDir() string        { return readText("output_dir.txt") }
func SaveOutputDir(d string) error { return writeText("output_dir.txt", d) }
