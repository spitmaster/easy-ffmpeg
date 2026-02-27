package main

import (
	"archive/zip"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

func main() {
	baseDir := filepath.Join("..", "internal", "embedded")
	windowsDir := filepath.Join(baseDir, "windows")

	os.MkdirAll(windowsDir, 0755)

	fmt.Println("开始下载 Windows FFmpeg 二进制文件...")

	url := "https://www.gyan.dev/ffmpeg/builds/ffmpeg-release-essentials.zip"
	tempZip := filepath.Join(os.TempDir(), "ffmpeg-windows.zip")

	fmt.Printf("从 %s 下载...\n", url)

	resp, err := http.Get(url)
	if err != nil {
		fmt.Printf("下载失败: %v\n", err)
		os.Exit(1)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		fmt.Printf("下载失败: HTTP %d\n", resp.StatusCode)
		os.Exit(1)
	}

	out, err := os.Create(tempZip)
	if err != nil {
		fmt.Printf("创建文件失败: %v\n", err)
		os.Exit(1)
	}
	defer out.Close()

	size, err := io.Copy(out, resp.Body)
	if err != nil {
		fmt.Printf("写入文件失败: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("下载完成: %.2f MB\n", float64(size)/1024/1024)

	// 解压
	fmt.Println("解压中...")
	r, err := zip.OpenReader(tempZip)
	if err != nil {
		fmt.Printf("打开 ZIP 失败: %v\n", err)
		os.Exit(1)
	}
	defer r.Close()

	for _, f := range r.File {
		if strings.Contains(f.Name, "/bin/ffmpeg.exe") {
			fmt.Println("找到 ffmpeg.exe，正在复制...")
			copyFile(f, filepath.Join(windowsDir, "ffmpeg.exe"))
		} else if strings.Contains(f.Name, "/bin/ffprobe.exe") {
			fmt.Println("找到 ffprobe.exe，正在复制...")
			copyFile(f, filepath.Join(windowsDir, "ffprobe.exe"))
		}
	}

	os.Remove(tempZip)
	fmt.Println("完成!")
}

func copyFile(f *zip.File, destPath string) error {
	rc, err := f.Open()
	if err != nil {
		return err
	}
	defer rc.Close()

	out, err := os.Create(destPath)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, rc)
	if err != nil {
		return err
	}

	fmt.Printf("  ✓ %s\n", destPath)
	return nil
}
