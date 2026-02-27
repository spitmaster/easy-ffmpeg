package main

import (
	"easy-ffmpeg/ui"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
)

func main() {
	a := app.New()

	w := a.NewWindow("Easy FFmpeg")
	w.SetContent(ui.CreateMainUI())
	w.Resize(fyne.NewSize(900, 600))
	w.ShowAndRun()
}
