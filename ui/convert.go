package ui

import (
	"bufio"
	"easy-ffmpeg/service"
	"fmt"
	"image/color"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// ratioLayout 自定义布局,实现比例控制
type ratioLayout struct {
	ratios []int
}

func (l *ratioLayout) Layout(objects []fyne.CanvasObject, size fyne.Size) {
	if len(objects) == 0 || len(l.ratios) == 0 {
		return
	}

	// 计算总比例
	totalRatio := 0
	for i := 0; i < len(objects) && i < len(l.ratios); i++ {
		totalRatio += l.ratios[i]
	}

	// 按比例分配宽度
	x := float32(0)
	for i := 0; i < len(objects) && i < len(l.ratios); i++ {
		width := float32(size.Width) * float32(l.ratios[i]) / float32(totalRatio)
		objects[i].Move(fyne.NewPos(x, 0))
		objects[i].Resize(fyne.NewSize(width, size.Height))
		x += width
	}
}

func (l *ratioLayout) MinSize(objects []fyne.CanvasObject) fyne.Size {
	if len(objects) == 0 {
		return fyne.NewSize(100, 100)
	}
	totalWidth := float32(0)
	maxHeight := float32(0)
	for _, obj := range objects {
		min := obj.MinSize()
		totalWidth += min.Width
		if min.Height > maxHeight {
			maxHeight = min.Height
		}
	}
	return fyne.NewSize(totalWidth, maxHeight)
}

// GreenButton 绿色按钮
type GreenButton struct {
	widget.BaseWidget
	btnText    string
	onTapped   func()
	background *canvas.Rectangle
	text       *canvas.Text
}

func NewGreenButton(text string, tapped func()) *GreenButton {
	btn := &GreenButton{}
	btn.ExtendBaseWidget(btn)
	btn.btnText = text
	btn.onTapped = tapped
	btn.background = canvas.NewRectangle(color.RGBA{R: 46, G: 204, B: 113, A: 255})
	btn.background.CornerRadius = theme.InputRadiusSize()
	btn.text = canvas.NewText(text, color.White)
	btn.text.TextSize = theme.TextSize()
	btn.text.TextStyle = fyne.TextStyle{Bold: true}
	return btn
}

func (b *GreenButton) Tapped(*fyne.PointEvent) {
	if b.onTapped != nil {
		b.onTapped()
	}
}

func (b *GreenButton) TappedSecondary(*fyne.PointEvent) {}

func (b *GreenButton) CreateRenderer() fyne.WidgetRenderer {
	return &greenButtonRenderer{button: b}
}

type greenButtonRenderer struct {
	button *GreenButton
}

func (r *greenButtonRenderer) Layout(size fyne.Size) {
	r.button.background.Resize(size)
	r.button.text.Move(fyne.NewPos(size.Width/2-r.button.text.MinSize().Width/2,
		size.Height/2-r.button.text.MinSize().Height/2))
}

func (r *greenButtonRenderer) MinSize() fyne.Size {
	textSize := r.button.text.MinSize()
	return fyne.NewSize(textSize.Width+40, textSize.Height+20)
}

func (r *greenButtonRenderer) Refresh() {
	r.button.background.Refresh()
	r.button.text.Refresh()
}

func (r *greenButtonRenderer) Objects() []fyne.CanvasObject {
	return []fyne.CanvasObject{r.button.background, r.button.text}
}

func (r *greenButtonRenderer) Destroy() {}

// 当前运行的ffmpeg进程
var currentCmd *exec.Cmd
var cmdMutex sync.Mutex

// ReadOnlyEntry 只读但可选中可复制的Entry
type ReadOnlyEntry struct {
	widget.Entry
}

func (e *ReadOnlyEntry) TypedRune(r rune) {
	// 禁止输入字符
}

func (e *ReadOnlyEntry) TypedShortcut(shortcut fyne.Shortcut) {
	// 允许复制等快捷键
	switch shortcut.(type) {
	case *fyne.ShortcutCopy, *fyne.ShortcutSelectAll:
		e.Entry.TypedShortcut(shortcut)
	}
}

// logLayout 自定义布局，让日志区域有固定高度但宽度自适应
type logLayout struct {
}

func (l *logLayout) Layout(objects []fyne.CanvasObject, size fyne.Size) {
	if len(objects) == 0 {
		return
	}
	// 让子对象占据全部宽度，固定高度300
	objects[0].Move(fyne.NewPos(0, 0))
	objects[0].Resize(fyne.NewSize(size.Width, 300))
}

func (l *logLayout) MinSize(objects []fyne.CanvasObject) fyne.Size {
	if len(objects) == 0 {
		return fyne.NewSize(100, 300)
	}
	min := objects[0].MinSize()
	return fyne.NewSize(min.Width, 300)
}

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
	// 输入文件选择 - 按钮在左侧，更小的按钮
	inputEntry := widget.NewEntry()
	inputEntry.SetPlaceHolder("选择输入视频文件...")

	// 输出文件名 - 需要先定义
	outputEntry := widget.NewEntry()
	outputEntry.SetPlaceHolder("输出文件名（不含扩展名）...")

	// 输出目录选择 - 按钮在左侧，更小的按钮
	outputDirEntry := widget.NewEntry()
	outputDirEntry.SetPlaceHolder("选择输出目录...")

	// 加载保存的输出目录
	if savedDir := getSavedOutputDir(); savedDir != "" {
		outputDirEntry.SetText(savedDir)
	}

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

	// 输入文件行: 按钮1份,输入框3份
	inputRow := container.New(&ratioLayout{ratios: []int{1, 3}}, inputBtn, inputEntry)

	outputDirBtn := widget.NewButton("选择目录", func() {
		dialog.ShowFolderOpen(func(uri fyne.ListableURI, err error) {
			if err == nil && uri != nil {
				outputDirEntry.SetText(uri.Path())
				saveOutputDir(uri.Path())
			}
		}, mainWindow)
	})


	// 输出行: 按钮1份,输入框2份,输出文件名1份
	outputRow := container.New(&ratioLayout{ratios: []int{1, 2, 1}}, outputDirBtn, outputDirEntry, outputEntry)

	// 编码器和输出格式选择 - 放到一行
	encoderSelect := widget.NewSelect([]string{
		"h264 (H.264/AVC)",
		"h265 (H.265/HEVC)",
		"vp9 (VP9)",
		"av1 (AV1)",
		"mpeg4 (MPEG-4)",
		"copy (快速拷贝, 不重新编码)",
	}, nil)
	encoderSelect.SetSelectedIndex(0)

	audioEncoderSelect := widget.NewSelect([]string{
		"aac (AAC)",
		"mp3 (MP3)",
		"libopus (Opus)",
		"libvorbis (Vorbis)",
		"copy (拷贝)",
	}, nil)
	audioEncoderSelect.SetSelectedIndex(0)

	formatSelect := widget.NewSelect([]string{
		"mp4",
		"mkv",
		"avi",
		"mov",
		"flv",
		"webm",
		"m3u8",
	}, nil)
	formatSelect.SetSelectedIndex(0)

	encoderFormatAudioRow := container.NewGridWithColumns(3, encoderSelect, audioEncoderSelect, formatSelect)

	// 日志输出 - 使用只读Entry实现控制台效果
	logEntry := &ReadOnlyEntry{}
	logEntry.ExtendBaseWidget(logEntry)
	logEntry.SetPlaceHolder("转码日志将显示在这里...")
	logEntry.MultiLine = true
	logEntry.Wrapping = fyne.TextWrapOff // 不换行，横向滚动
	logEntry.TextStyle = fyne.TextStyle{Monospace: true} // 等宽字体
	logEntry.Refresh()

	logScroll := container.NewScroll(logEntry)
	// 创建一个固定高度的容器用于日志
	logContainer := container.New(&logLayout{}, logScroll)

	// 日志内容更新函数
	updateLog := func(line string) {
		currentText := logEntry.Text
		logEntry.SetText(currentText + line + "\n")
		logEntry.CursorRow = len(strings.Split(logEntry.Text, "\n")) - 1
		logScroll.ScrollToBottom()
	}

	// 开始按钮（先声明变量）
	var startBtn *widget.Button
	var progressDialog *dialog.CustomDialog

	// 开始按钮回调函数
	startBtnCallback := func() {
		inputPath := inputEntry.Text
		outputDir := outputDirEntry.Text
		outputName := outputEntry.Text
		encoder := encoderSelect.Selected
		audioEncoder := audioEncoderSelect.Selected
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
			// 修正编码器名称
			if codec == "h264" {
				codec = "libx264"
			} else if codec == "h265" {
				codec = "libx265"
			}
		}

		// 解析音频编码器
		var audioCodec string
		if strings.HasPrefix(audioEncoder, "copy") {
			audioCodec = "copy"
		} else {
			audioCodec = strings.Split(audioEncoder, " ")[0]
		}

		if codec != "copy" {
			args = append(args, "-c:v", codec, "-c:a", audioCodec)
		} else {
			args = append(args, "-c", "copy")
		}

		args = append(args, outputPath)

		// 清空日志
		logEntry.SetText("")

		// 执行转码
		updateLog(fmt.Sprintf("开始转码: %s", inputPath))
		updateLog(fmt.Sprintf("输出: %s", outputPath))
		updateLog(fmt.Sprintf("编码器: %s", codec))

		cmd := exec.Command("ffmpeg", args...)
		if embeddedCmd, err := service.GetEmbeddedFFmpegCmd(); err == nil {
			cmd = embeddedCmd
			cmd.Args = append([]string{"ffmpeg"}, args...)
		}

		// 保存当前命令引用
		cmdMutex.Lock()
		currentCmd = cmd
		cmdMutex.Unlock()

		// 创建进度对话框（会阻止所有交互）
		progressLabel := widget.NewLabel("正在转码中...")
		progressLabel.TextStyle = fyne.TextStyle{Bold: true}
		progressLabel.Alignment = fyne.TextAlignCenter

		// 取消按钮
		cancelBtn := widget.NewButton("取消转码", func() {
			cmdMutex.Lock()
			if currentCmd != nil && currentCmd.Process != nil {
				currentCmd.Process.Kill()
			}
			cmdMutex.Unlock()

			fyne.Do(func() {
				progressDialog.Hide()
				currentText := logEntry.Text
				logEntry.SetText(currentText + "\n转码已取消\n")
				lines := strings.Split(logEntry.Text, "\n")
				logEntry.CursorRow = len(lines) - 2
				if logEntry.CursorRow < 0 {
					logEntry.CursorRow = 0
				}
				logEntry.CursorColumn = 0
				logEntry.Refresh()
				logScroll.ScrollToBottom()
			})
		})

		progressContent := container.NewVBox(
			progressLabel,
			widget.NewLabel("请稍候..."),
			cancelBtn,
		)

		progressDialog = dialog.NewCustomWithoutButtons("转码中", progressContent, mainWindow)
		progressDialog.Show()

		_, _ = cmd.StdoutPipe()
		stderr, _ := cmd.StderrPipe()

		cmd.Start()

		// 读取日志并在主线程更新UI
		go func() {
			scanner := bufio.NewScanner(stderr)
			for scanner.Scan() {
				line := scanner.Text()
				fyne.Do(func() {
					currentText := logEntry.Text
					logEntry.SetText(currentText + line + "\n")
					// 设置光标到最后一行的开头
					lines := strings.Split(logEntry.Text, "\n")
					logEntry.CursorRow = len(lines) - 2
					if logEntry.CursorRow < 0 {
						logEntry.CursorRow = 0
					}
					logEntry.CursorColumn = 0 // 移到行首
					logEntry.Refresh()
					logScroll.ScrollToBottom()
				})
			}

			// 转码完成
			err := cmd.Wait()

			// 清除命令引用
			cmdMutex.Lock()
			currentCmd = nil
			cmdMutex.Unlock()

			fyne.Do(func() {
				// 隐藏进度对话框
				progressDialog.Hide()

				if err != nil {
					currentText := logEntry.Text
					logEntry.SetText(currentText + fmt.Sprintf("\n转码失败: %v\n", err))
					lines := strings.Split(logEntry.Text, "\n")
					logEntry.CursorRow = len(lines) - 2
					if logEntry.CursorRow < 0 {
						logEntry.CursorRow = 0
					}
					logEntry.CursorColumn = 0
					logEntry.Refresh()
					logScroll.ScrollToBottom()
					dialog.ShowError(fmt.Errorf("转码失败: %v", err), mainWindow)
				} else {
					currentText := logEntry.Text
					logEntry.SetText(currentText + "\n转码完成!\n")
					lines := strings.Split(logEntry.Text, "\n")
					logEntry.CursorRow = len(lines) - 2
					if logEntry.CursorRow < 0 {
						logEntry.CursorRow = 0
					}
					logEntry.CursorColumn = 0
					logEntry.Refresh()
					logScroll.ScrollToBottom()
					dialog.ShowInformation("完成", "转码成功!", mainWindow)
				}
			})
		}()
	}

	// 创建按钮并关联回调 - 设置为绿色并放大
	startBtn = widget.NewButton("开始转码", startBtnCallback)
	startBtn.Importance = widget.HighImportance // 绿色按钮并放大
	startBtn.Refresh()

	// 创建绿色按钮
	greenBtn := NewGreenButton("开始转码", startBtnCallback)

	// 显示将要执行的ffmpeg命令
	commandEntry := &ReadOnlyEntry{}
	commandEntry.ExtendBaseWidget(commandEntry)
	commandEntry.SetPlaceHolder("命令将在这里显示...")
	commandEntry.TextStyle = fyne.TextStyle{Monospace: true}
	commandEntry.Refresh()

	// 更新命令显示的函数
	updateCommand := func() {
		inputPath := inputEntry.Text
		outputDir := outputDirEntry.Text
		outputName := outputEntry.Text
		encoder := encoderSelect.Selected
		audioEncoder := audioEncoderSelect.Selected
		format := formatSelect.Selected

		if inputPath != "" && outputDir != "" && outputName != "" && encoder != "" && format != "" {
			outputPath := filepath.Join(outputDir, outputName+"."+format)
			args := []string{"-i", inputPath}

			var codec string
			if strings.HasPrefix(encoder, "copy") {
				codec = "copy"
			} else {
				codec = strings.Split(encoder, " ")[0]
				if codec == "h264" {
					codec = "libx264"
				} else if codec == "h265" {
					codec = "libx265"
				}
			}

			// 解析音频编码器
			var audioCodec string
			if strings.HasPrefix(audioEncoder, "copy") {
				audioCodec = "copy"
			} else {
				audioCodec = strings.Split(audioEncoder, " ")[0]
			}

			if codec != "copy" {
				args = append(args, "-c:v", codec)
				args = append(args, "-c:a", audioCodec)
			} else {
				args = append(args, "-c", "copy")
			}
			args = append(args, outputPath)

			cmdStr := fmt.Sprintf("ffmpeg %s", strings.Join(args, " "))
			commandEntry.SetText(cmdStr)
		} else {
			commandEntry.SetText("")
		}
	}

	// 监听输入变化以更新命令显示
	inputEntry.OnChanged = func(string) { updateCommand() }
	outputDirEntry.OnChanged = func(string) { updateCommand() }
	outputEntry.OnChanged = func(string) { updateCommand() }
	encoderSelect.OnChanged = func(string) { updateCommand() }
	audioEncoderSelect.OnChanged = func(string) { updateCommand() }
	formatSelect.OnChanged = func(string) { updateCommand() }

	// 开始转码按钮和命令显示在一行
	startCommandRow := container.NewBorder(nil, nil, greenBtn, nil, commandEntry)

	// 布局
	optionsForm := container.NewVBox(
		widget.NewLabel("输入文件:"),
		inputRow,
		widget.NewLabel("输出目录和文件名:"),
		outputRow,
		widget.NewLabel("编码器和输出格式:"),
		encoderFormatAudioRow,
	)

	logLabel := widget.NewLabel("转码日志:")

	content := container.NewVBox(
		optionsForm,
		widget.NewSeparator(),
		widget.NewSeparator(),
		startCommandRow,
		widget.NewSeparator(),
		logLabel,
		logContainer,
	)

	// 添加外边距
	return container.NewPadded(content)
}
