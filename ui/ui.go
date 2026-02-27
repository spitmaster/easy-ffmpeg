package ui

import (
	"easy-ffmpeg/service"

	"fyne.io/fyne/v2"
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

	statusText := "FFmpeg: Not Found"
	if ffmpegAvailable {
		statusText = "FFmpeg: Available"
	}

	status := widget.NewLabel(statusText)
	status.Alignment = fyne.TextAlignCenter

	tabContainer := container.NewTabContainer(
		container.NewTabItem("Video", widget.NewLabel("Video conversion coming soon...")),
		container.NewTabItem("Audio", widget.NewLabel("Audio processing coming soon...")),
		container.NewTabItem("Settings", widget.NewLabel("Settings coming soon...")),
	)

	return container.NewVBox(
		status,
		widget.NewSeparator(),
		tabContainer,
	)
}
