package main

import (
	"easy-ffmpeg/ui"

	"fyne.io/fyne/v2/app"
)

func main() {
	a := app.New()
	a.SetTheme(ui.GetTheme())

	w := a.NewWindow("Easy FFmpeg")
	w.SetContent(ui.CreateMainUI())
	w.Resize(800, 600)
	w.ShowAndRun()
}
