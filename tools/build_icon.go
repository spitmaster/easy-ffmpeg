//go:build ignore

// Renders the app icon to a multi-resolution .ico file.
// Run:  go run tools/build_icon.go
// Output: cmd/icon.ico
package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"path/filepath"
)

// All sizes we render (union of Windows ICO + macOS ICNS needs).
var sizes = []int{16, 32, 48, 64, 128, 256, 512}

// Windows ICO takes this subset.
var icoSizes = []int{16, 32, 48, 64, 128, 256}

// macOS ICNS chunk types for specific pixel sizes (modern PNG payload).
var icnsChunks = []struct {
	size int
	typ  string
}{
	{16, "icp4"},
	{32, "icp5"},
	{64, "ic12"}, // 32x32@2x
	{128, "ic07"},
	{256, "ic08"},
	{512, "ic09"},
}

// Palette — matches the progress bar gradient (emerald → blue).
var (
	colorA = color.RGBA{R: 0x10, G: 0xb9, B: 0x81, A: 0xff}
	colorB = color.RGBA{R: 0x3b, G: 0x82, B: 0xf6, A: 0xff}
	white  = color.RGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff}
)

func main() {
	pngsBySize := make(map[int][]byte, len(sizes))
	for _, s := range sizes {
		img := renderIcon(s)
		var buf bytes.Buffer
		if err := png.Encode(&buf, img); err != nil {
			panic(err)
		}
		pngsBySize[s] = buf.Bytes()
	}

	// Write Windows .ico
	icoPngs := make([][]byte, len(icoSizes))
	for i, s := range icoSizes {
		icoPngs[i] = pngsBySize[s]
	}
	if err := os.WriteFile("cmd/icon.ico", packICO(icoPngs, icoSizes), 0644); err != nil {
		panic(err)
	}
	fmt.Printf("wrote cmd/icon.ico (%d sizes)\n", len(icoSizes))

	// Write macOS .icns
	if err := os.MkdirAll("assets", 0755); err != nil {
		panic(err)
	}
	if err := os.WriteFile("assets/icon.icns", packICNS(pngsBySize), 0644); err != nil {
		panic(err)
	}
	fmt.Printf("wrote assets/icon.icns (%d sizes)\n", len(icnsChunks))

	// Wails desktop icons. Same source as cmd/icon.ico — single source of
	// truth across Web build (cmd/rsrc_windows.syso) and desktop build.
	desktopIco := filepath.Join("cmd", "desktop", "build", "windows", "icon.ico")
	if err := os.MkdirAll(filepath.Dir(desktopIco), 0755); err != nil {
		panic(err)
	}
	if err := os.WriteFile(desktopIco, packICO(icoPngs, icoSizes), 0644); err != nil {
		panic(err)
	}
	fmt.Printf("wrote %s\n", desktopIco)

	// Wails appicon.png — used for taskbar/dock and macOS/Linux. 512 is
	// the largest size we render; Wails will scale up if needed.
	desktopAppicon := filepath.Join("cmd", "desktop", "build", "appicon.png")
	if err := os.WriteFile(desktopAppicon, pngsBySize[512], 0644); err != nil {
		panic(err)
	}
	fmt.Printf("wrote %s\n", desktopAppicon)
}

// Render the icon at the given size. 4× super-sampling + box filter downscale
// gives smooth corners and edges without needing antialiasing libraries.
func renderIcon(size int) image.Image {
	ss := size * 4
	hi := image.NewRGBA(image.Rect(0, 0, ss, ss))

	sz := float64(ss)
	margin := sz * 0.04
	radius := sz * 0.20

	// Background: gradient fill clipped to rounded rect.
	for y := 0; y < ss; y++ {
		for x := 0; x < ss; x++ {
			fx, fy := float64(x), float64(y)
			if !insideRounded(fx, fy, sz, radius, margin) {
				continue
			}
			t := (fx + fy) / (2 * sz)
			hi.SetRGBA(x, y, lerpColor(colorA, colorB, t))
		}
	}

	// Play triangle, slightly right-shifted for optical centering.
	// Vertices (% of size): (36, 28) (72, 50) (36, 72)
	x1, y1 := sz*0.36, sz*0.28
	x2, y2 := sz*0.72, sz*0.50
	x3, y3 := sz*0.36, sz*0.72
	for y := 0; y < ss; y++ {
		for x := 0; x < ss; x++ {
			if pointInTriangle(float64(x), float64(y), x1, y1, x2, y2, x3, y3) {
				hi.SetRGBA(x, y, white)
			}
		}
	}

	return downscale(hi, size)
}

// insideRounded returns true if (x,y) is within a rounded rect of given size,
// corner radius r, and margin from each edge.
func insideRounded(x, y, size, r, margin float64) bool {
	left, top := margin, margin
	right, bot := size-margin, size-margin
	if x < left || x >= right || y < top || y >= bot {
		return false
	}
	switch {
	case x < left+r && y < top+r:
		return dist(x, y, left+r, top+r) <= r
	case x >= right-r && y < top+r:
		return dist(x, y, right-r, top+r) <= r
	case x < left+r && y >= bot-r:
		return dist(x, y, left+r, bot-r) <= r
	case x >= right-r && y >= bot-r:
		return dist(x, y, right-r, bot-r) <= r
	}
	return true
}

func dist(x1, y1, x2, y2 float64) float64 {
	dx, dy := x1-x2, y1-y2
	return math.Sqrt(dx*dx + dy*dy)
}

func pointInTriangle(px, py, ax, ay, bx, by, cx, cy float64) bool {
	s := func(x1, y1, x2, y2 float64) float64 {
		return (px-x2)*(y1-y2) - (x1-x2)*(py-y2)
	}
	d1, d2, d3 := s(ax, ay, bx, by), s(bx, by, cx, cy), s(cx, cy, ax, ay)
	hasNeg := d1 < 0 || d2 < 0 || d3 < 0
	hasPos := d1 > 0 || d2 > 0 || d3 > 0
	return !(hasNeg && hasPos)
}

func lerpColor(a, b color.RGBA, t float64) color.RGBA {
	if t < 0 {
		t = 0
	} else if t > 1 {
		t = 1
	}
	lerp := func(x, y uint8) uint8 {
		return uint8(float64(x) + (float64(y)-float64(x))*t + 0.5)
	}
	return color.RGBA{lerp(a.R, b.R), lerp(a.G, b.G), lerp(a.B, b.B), 0xff}
}

// Box filter downscale by integer factor. Gives clean antialiasing for our
// simple geometric shapes; no external image library needed.
func downscale(src *image.RGBA, target int) *image.RGBA {
	srcW := src.Bounds().Dx()
	factor := srcW / target
	out := image.NewRGBA(image.Rect(0, 0, target, target))
	n := factor * factor
	for y := 0; y < target; y++ {
		for x := 0; x < target; x++ {
			var r, g, b, a int
			for dy := 0; dy < factor; dy++ {
				for dx := 0; dx < factor; dx++ {
					c := src.RGBAAt(x*factor+dx, y*factor+dy)
					r += int(c.R)
					g += int(c.G)
					b += int(c.B)
					a += int(c.A)
				}
			}
			out.SetRGBA(x, y, color.RGBA{
				uint8(r / n), uint8(g / n), uint8(b / n), uint8(a / n),
			})
		}
	}
	return out
}

// packICO writes a multi-resolution ICO file with embedded PNG payloads
// (Windows Vista+ supports PNG-in-ICO).
func packICO(pngs [][]byte, sizes []int) []byte {
	buf := new(bytes.Buffer)
	// ICONDIR (6 bytes): reserved, type=1 (ICO), count
	binary.Write(buf, binary.LittleEndian, uint16(0))
	binary.Write(buf, binary.LittleEndian, uint16(1))
	binary.Write(buf, binary.LittleEndian, uint16(len(pngs)))

	offset := 6 + 16*len(pngs)
	for i, sz := range sizes {
		// ICONDIRENTRY (16 bytes)
		w, h := byte(0), byte(0) // 0 means 256
		if sz < 256 {
			w, h = byte(sz), byte(sz)
		}
		buf.WriteByte(w)
		buf.WriteByte(h)
		buf.WriteByte(0) // palette colors
		buf.WriteByte(0) // reserved
		binary.Write(buf, binary.LittleEndian, uint16(1))             // color planes
		binary.Write(buf, binary.LittleEndian, uint16(32))            // bits per pixel
		binary.Write(buf, binary.LittleEndian, uint32(len(pngs[i]))) // size of image data
		binary.Write(buf, binary.LittleEndian, uint32(offset))        // offset into file
		offset += len(pngs[i])
	}
	for _, p := range pngs {
		buf.Write(p)
	}
	return buf.Bytes()
}

func totalLen(bs [][]byte) int {
	n := 0
	for _, b := range bs {
		n += len(b)
	}
	return n
}

// packICNS writes a macOS icon file with PNG payloads.
// Format: "icns" magic (4) + total size big-endian (4), then
// repeated chunks of (type 4 + chunk-total-size big-endian 4 + data).
func packICNS(pngsBySize map[int][]byte) []byte {
	buf := new(bytes.Buffer)
	buf.WriteString("icns")
	binary.Write(buf, binary.BigEndian, uint32(0)) // total size placeholder

	for _, c := range icnsChunks {
		data, ok := pngsBySize[c.size]
		if !ok {
			continue
		}
		buf.WriteString(c.typ)
		binary.Write(buf, binary.BigEndian, uint32(8+len(data))) // 8 = type + size header
		buf.Write(data)
	}

	result := buf.Bytes()
	binary.BigEndian.PutUint32(result[4:8], uint32(len(result)))
	return result
}
