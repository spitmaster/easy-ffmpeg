package ui

import (
	"easy-ffmpeg/service"
	"os"
	"path/filepath"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

func GetTheme() fyne.Theme {
	return theme.DefaultTheme()
}

func CreateMainUI() fyne.CanvasObject {
	// Check FFmpeg availability
	ffmpegAvailable := service.CheckFFmpeg()

	var statusIcon fyne.CanvasObject
	var statusText string

	// Get SVG path - use current executable directory
	uiDir, _ := os.Getwd()
	svgPath := filepath.Join(uiDir, "ui")

	if ffmpegAvailable {
		svgFile := filepath.Join(svgPath, "status-green.svg")
		img := canvas.NewImageFromFile(svgFile)
		img.SetMinSize(fyne.NewSize(20, 20))
		img.FillMode = canvas.ImageFillContain
		statusIcon = img
		statusText = "FFmpeg 可用"
	} else {
		svgFile := filepath.Join(svgPath, "status-red.svg")
		img := canvas.NewImageFromFile(svgFile)
		img.SetMinSize(fyne.NewSize(20, 20))
		img.FillMode = canvas.ImageFillContain
		statusIcon = img
		statusText = "FFmpeg 未安装"
	}

	tabContainer := container.NewAppTabs(
		container.NewTabItem("视频转换", widget.NewLabel("视频转换功能 - coming soon...")),
		container.NewTabItem("视频裁剪", widget.NewLabel("视频裁剪功能 - coming soon...")),
		container.NewTabItem("音频处理", widget.NewLabel("音频处理功能 - coming soon...")),
		container.NewTabItem("媒体信息", widget.NewLabel("媒体信息功能 - coming soon...")),
		container.NewTabItem("设置", widget.NewLabel("设置功能 - coming soon...")),
	)

	statusBar := container.NewHBox(
		container.NewGridWithColumns(1, widget.NewLabel("")),
		statusIcon,
		widget.NewLabel(statusText),
	)

	return container.NewBorder(
		nil, statusBar, nil, nil,
		tabContainer,
	)
}
