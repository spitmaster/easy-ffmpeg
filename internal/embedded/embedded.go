package embedded

import (
	"embed"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

//go:embed windows/*
//go:embed darwin/*
//go:embed linux/*
var ffmpegBinaries embed.FS

// GetFFmpegBinary 获取适合当前平台的FFmpeg二进制文件路径
// 返回解压后的临时文件路径
func GetFFmpegBinary() (string, error) {
	var binaryName string
	var embeddedPath string

	switch runtime.GOOS {
	case "windows":
		binaryName = "ffmpeg.exe"
		embeddedPath = "windows/" + binaryName
	case "darwin":
		binaryName = "ffmpeg"
		embeddedPath = "darwin/" + binaryName
	case "linux":
		binaryName = "ffmpeg"
		embeddedPath = "linux/" + binaryName
	default:
		binaryName = "ffmpeg"
		embeddedPath = "linux/" + binaryName
	}

	// 读取嵌入的二进制文件
	data, err := ffmpegBinaries.ReadFile(embeddedPath)
	if err != nil {
		return "", err
	}

	// 创建临时文件
	tempDir := os.TempDir()
	tempPath := filepath.Join(tempDir, binaryName)

	// 检查是否已存在且大小相同（避免重复写入大文件）
	if fileExists(tempPath) {
		existingInfo, _ := os.Stat(tempPath)
		if existingInfo.Size() == int64(len(data)) {
			return tempPath, nil
		}
	}

	// 写入临时文件
	err = os.WriteFile(tempPath, data, 0755)
	if err != nil {
		return "", err
	}

	return tempPath, nil
}

// GetFFprobeBinary 获取适合当前平台的FFprobe二进制文件路径
func GetFFprobeBinary() (string, error) {
	var binaryName string
	var embeddedPath string

	switch runtime.GOOS {
	case "windows":
		binaryName = "ffprobe.exe"
		embeddedPath = "windows/" + binaryName
	case "darwin":
		binaryName = "ffprobe"
		embeddedPath = "darwin/" + binaryName
	case "linux":
		binaryName = "ffprobe"
		embeddedPath = "linux/" + binaryName
	default:
		binaryName = "ffprobe"
		embeddedPath = "linux/" + binaryName
	}

	data, err := ffmpegBinaries.ReadFile(embeddedPath)
	if err != nil {
		return "", err
	}

	tempDir := os.TempDir()
	tempPath := filepath.Join(tempDir, binaryName)

	if fileExists(tempPath) {
		existingInfo, _ := os.Stat(tempPath)
		if existingInfo.Size() == int64(len(data)) {
			return tempPath, nil
		}
	}

	err = os.WriteFile(tempPath, data, 0755)
	if err != nil {
		return "", err
	}

	return tempPath, nil
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// Command 创建使用嵌入FFmpeg的命令
func Command(args ...string) (*exec.Cmd, error) {
	binaryPath, err := GetFFmpegBinary()
	if err != nil {
		return nil, err
	}
	return exec.Command(binaryPath, args...), nil
}

// FFprobeCommand 创建使用嵌入FFprobe的命令
func FFprobeCommand(args ...string) (*exec.Cmd, error) {
	binaryPath, err := GetFFprobeBinary()
	if err != nil {
		return nil, err
	}
	return exec.Command(binaryPath, args...), nil
}

// CheckEmbeddedFFmpeg 检查嵌入的FFmpeg是否可用
func CheckEmbeddedFFmpeg() bool {
	binaryPath, err := GetFFmpegBinary()
	if err != nil {
		return false
	}
	cmd := exec.Command(binaryPath, "-version")
	err = cmd.Run()
	return err == nil
}

// GetEmbeddedFFmpegVersion 获取嵌入的FFmpeg版本
func GetEmbeddedFFmpegVersion() string {
	binaryPath, err := GetFFmpegBinary()
	if err != nil {
		return ""
	}
	cmd := exec.Command(binaryPath, "-version")
	output, err := cmd.Output()
	if err != nil {
		return ""
	}
	lines := strings.Split(string(output), "\n")
	if len(lines) > 0 {
		return lines[0]
	}
	return ""
}
