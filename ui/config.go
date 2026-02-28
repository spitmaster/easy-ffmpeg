package ui

import (
	"os"
	"path/filepath"
	"strings"
)

// 获取保存的输出目录
func GetSavedOutputDir() string {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	configPath := filepath.Join(configDir, "easy-ffmpeg", "output_dir.txt")
	if data, err := os.ReadFile(configPath); err == nil {
		return strings.TrimSpace(string(data))
	}
	return ""
}

// 保存输出目录
func SaveOutputDir(dir string) error {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return err
	}
	configPath := filepath.Join(configDir, "easy-ffmpeg", "output_dir.txt")
	if err := os.MkdirAll(filepath.Dir(configPath), 0755); err != nil {
		return err
	}
	return os.WriteFile(configPath, []byte(dir), 0644)
}

// 获取保存的输入文件目录
func GetSavedInputDir() string {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	configPath := filepath.Join(configDir, "easy-ffmpeg", "input_dir.txt")
	if data, err := os.ReadFile(configPath); err == nil {
		return strings.TrimSpace(string(data))
	}
	return ""
}

// 保存输入文件目录
func SaveInputDir(dir string) error {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return err
	}
	configPath := filepath.Join(configDir, "easy-ffmpeg", "input_dir.txt")
	if err := os.MkdirAll(filepath.Dir(configPath), 0755); err != nil {
		return err
	}
	return os.WriteFile(configPath, []byte(dir), 0644)
}
