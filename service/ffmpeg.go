package service

import (
	"easy-ffmpeg/internal/embedded"
	"fmt"
	"os/exec"
	"strings"
)

// CheckFFmpeg 检查FFmpeg是否可用
// 优先使用嵌入的FFmpeg，如果不可用则尝试系统FFmpeg
func CheckFFmpeg() bool {
	if embedded.CheckEmbeddedFFmpeg() {
		return true
	}

	// 降级到系统FFmpeg
	cmd := exec.Command("ffmpeg", "-version")
	err := cmd.Run()
	return err == nil
}

// RunFFmpeg 执行FFmpeg命令
// 优先使用嵌入的FFmpeg，如果不可用则尝试系统FFmpeg
func RunFFmpeg(args []string) (string, error) {
	// 尝试使用嵌入的FFmpeg
	if cmd, err := embedded.Command(args...); err == nil {
		output, err := cmd.CombinedOutput()
		return string(output), err
	}

	// 降级到系统FFmpeg
	cmd := exec.Command("ffmpeg", args...)
	output, err := cmd.CombinedOutput()
	return string(output), err
}

// GetFFmpegVersion 获取FFmpeg版本信息
// 优先使用嵌入的FFmpeg，如果不可用则尝试系统FFmpeg
func GetFFmpegVersion() string {
	// 尝试使用嵌入的FFmpeg
	version := embedded.GetEmbeddedFFmpegVersion()
	if version != "" {
		return version
	}

	// 降级到系统FFmpeg
	cmd := exec.Command("ffmpeg", "-version")
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

// RunFFprobe 执行FFprobe命令
func RunFFprobe(args []string) (string, error) {
	// 尝试使用嵌入的FFprobe
	if cmd, err := embedded.FFprobeCommand(args...); err == nil {
		output, err := cmd.CombinedOutput()
		return string(output), err
	}

	// 降级到系统FFprobe
	cmd := exec.Command("ffprobe", args...)
	output, err := cmd.CombinedOutput()
	return string(output), err
}

// GetFFmpegBinaryInfo 获取FFmpeg二进制信息
func GetFFmpegBinaryInfo() (string, error) {
	// 尝试使用嵌入的FFmpeg
	binaryPath, err := embedded.GetFFmpegBinary()
	if err == nil {
		return fmt.Sprintf("使用嵌入的FFmpeg: %s", binaryPath), nil
	}

	// 降级到系统FFmpeg
	return "使用系统FFmpeg", nil
}

// GetEmbeddedFFmpegCmd 获取嵌入的FFmpeg命令对象
func GetEmbeddedFFmpegCmd() (*exec.Cmd, error) {
	return embedded.Command()
}
