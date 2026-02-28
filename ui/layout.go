package ui

import (
	"fyne.io/fyne/v2"
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
