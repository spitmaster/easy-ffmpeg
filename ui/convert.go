package ui

import (
	"bufio"
	"easy-ffmpeg/service"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"
)

// 当前运行的ffmpeg进程
var currentCmd *exec.Cmd
var cmdMutex sync.Mutex

// 获取保存的输出目录
func getSavedOutputDir() string {
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
func saveOutputDir(dir string) error {
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
func getSavedInputDir() string {
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
func saveInputDir(dir string) error {
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

// CreateConvertTab 创建视频转换标签页
func CreateConvertTab() fyne.CanvasObject {
	// 输入文件选择
	inputEntry := widget.NewEntry()
	inputEntry.SetPlaceHolder("选择输入视频文件...")

	// 输出文件名
	outputEntry := widget.NewEntry()
	outputEntry.SetPlaceHolder("输出文件名（不含扩展名）...")

	inputBtn := widget.NewButton("选择文件", func() {
		fd := dialog.NewFileOpen(func(reader fyne.URIReadCloser, err error) {
			if err == nil && reader != nil {
				inputEntry.SetText(reader.URI().Path())
				reader.Close()

				// 保存输入文件目录
				inputPath := reader.URI().Path()
				inputDir := filepath.Dir(inputPath)
				saveInputDir(inputDir)

				// 自动设置输出文件名
				ext := filepath.Ext(inputPath)
				outputEntry.SetText(strings.TrimSuffix(filepath.Base(inputPath), ext) + "_converted")
			}
		}, mainWindow)
		fd.Show()
	})

	inputRow := container.NewGridWithColumns(2, inputEntry, inputBtn)

	// 输出目录选择
	outputDirEntry := widget.NewEntry()
	outputDirEntry.SetPlaceHolder("选择输出目录...")

	// 加载保存的输出目录
	if savedDir := getSavedOutputDir(); savedDir != "" {
		outputDirEntry.SetText(savedDir)
	}

	outputDirBtn := widget.NewButton("选择输出目录", func() {
		dialog.ShowFolderOpen(func(uri fyne.ListableURI, err error) {
			if err == nil && uri != nil {
				outputDirEntry.SetText(uri.Path())
				saveOutputDir(uri.Path())
			}
		}, mainWindow)
	})

	outputDirRow := container.NewGridWithColumns(2, outputDirEntry, outputDirBtn)

	// 编码器选择
	encoderSelect := widget.NewSelect([]string{
		"h264 (H.264/AVC)",
		"h265 (H.265/HEVC)",
		"vp9 (VP9)",
		"av1 (AV1)",
		"mpeg4 (MPEG-4)",
		"copy (快速拷贝, 不重新编码)",
	}, nil)
	encoderSelect.SetSelectedIndex(0)

	// 输出格式选择
	formatSelect := widget.NewSelect([]string{
		"mp4",
		"mkv",
		"avi",
		"mov",
		"flv",
		"webm",
	}, nil)
	formatSelect.SetSelectedIndex(0)



	// 日志输出
	logEntry := widget.NewMultiLineEntry()
	logEntry.SetPlaceHolder("转码日志将显示在这里...")
	logEntry.Disable()

	// 开始按钮（先定义，后续填充回调）
	startBtn := widget.NewButton("开始转码", nil)

	// 开始按钮回调
	startBtn.OnTapped = func() {
		inputPath := inputEntry.Text
		outputDir := outputDirEntry.Text
		outputName := outputEntry.Text
		encoder := encoderSelect.Selected
		format := formatSelect.Selected

		if inputPath == "" {
			dialog.ShowError(fmt.Errorf("请选择输入文件"), mainWindow)
			return
		}

		if outputDir == "" {
			dialog.ShowError(fmt.Errorf("请选择输出目录"), mainWindow)
			return
		}

		if outputName == "" {
			dialog.ShowError(fmt.Errorf("请输入输出文件名"), mainWindow)
			return
		}

		if encoder == "" {
			dialog.ShowError(fmt.Errorf("请选择编码器"), mainWindow)
			return
		}

		if format == "" {
			dialog.ShowError(fmt.Errorf("请选择输出格式"), mainWindow)
			return
		}

		outputPath := filepath.Join(outputDir, outputName+"."+format)

		// 构建FFmpeg命令
		args := []string{"-i", inputPath}

		// 解析编码器
		var codec string
		if strings.HasPrefix(encoder, "copy") {
			codec = "copy"
		} else {
			codec = strings.Split(encoder, " ")[0]
		}

		if codec != "copy" {
			args = append(args, "-c:v", codec, "-c:a", "aac")
		} else {
			args = append(args, "-c", "copy")
		}

		args = append(args, outputPath)

		// 清空日志
		logEntry.SetText("")

		// 执行转码
		logEntry.SetText(fmt.Sprintf("开始转码: %s\n", inputPath))
		logEntry.SetText(logEntry.Text + fmt.Sprintf("输出: %s\n", outputPath))
		logEntry.SetText(logEntry.Text + fmt.Sprintf("编码器: %s\n", codec))

		cmd := exec.Command("ffmpeg", args...)
		if embeddedCmd, err := service.GetEmbeddedFFmpegCmd(); err == nil {
			cmd = embeddedCmd
			cmd.Args = append([]string{"ffmpeg"}, args...)
		}

		// 保存当前命令引用
		cmdMutex.Lock()
		currentCmd = cmd
		cmdMutex.Unlock()

		// 禁用按钮避免重复点击
		startBtn.Disable()

		_, _ = cmd.StdoutPipe()
		stderr, _ := cmd.StderrPipe()

		cmd.Start()

		// 读取日志并在主线程更新UI
		go func() {
			scanner := bufio.NewScanner(stderr)
			for scanner.Scan() {
				line := scanner.Text()
				fyne.Do(func() {
					logEntry.SetText(logEntry.Text + line + "\n")
				})
			}

			// 转码完成
			err := cmd.Wait()

			// 清除命令引用
			cmdMutex.Lock()
			currentCmd = nil
			cmdMutex.Unlock()

			fyne.Do(func() {
				startBtn.Enable()
				if err != nil {
					logEntry.SetText(logEntry.Text + fmt.Sprintf("\n转码失败: %v\n", err))
					dialog.ShowError(fmt.Errorf("转码失败: %v", err), mainWindow)
				} else {
					logEntry.SetText(logEntry.Text + "\n转码完成!\n")
					dialog.ShowInformation("完成", "转码成功!", mainWindow)
				}
			})
		}()
	})

	// 布局
	optionsForm := container.NewVBox(
		widget.NewLabel("输入文件:"),
		inputRow,
		widget.NewLabel("输出目录:"),
		outputDirRow,
		widget.NewLabel("输出文件名:"),
		outputEntry,
		widget.NewLabel("编码器:"),
		encoderSelect,
		widget.NewLabel("输出格式:"),
		formatSelect,
	)

	actionRow := container.NewHBox(startBtn)

	logLabel := widget.NewLabel("转码日志:")
	// 使用固定高度的滚动容器
	logScroll := container.NewScroll(logEntry)
	logScroll.SetMinSize(fyne.NewSize(700, 300))
	logContainer := logScroll

	content := container.NewVBox(
		optionsForm,
		widget.NewSeparator(),
		actionRow,
		widget.NewSeparator(),
		logLabel,
		logContainer,
	)

	// 添加外边距
	return container.NewPadded(content)
}
