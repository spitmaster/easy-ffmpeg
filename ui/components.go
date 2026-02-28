package ui

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

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
