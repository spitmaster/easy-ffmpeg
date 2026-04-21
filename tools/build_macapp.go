//go:build ignore

// Wrap a macOS Go binary into a proper .app bundle.
// Run:  go run tools/build_macapp.go -bin <binary> -out <app-path>
// Example:
//   go run tools/build_macapp.go -bin dist/easy-ffmpeg-macos-arm64 \
//       -out "dist/Easy FFmpeg (arm64).app"
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

const infoPlist = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>CFBundleName</key><string>Easy FFmpeg</string>
    <key>CFBundleDisplayName</key><string>Easy FFmpeg</string>
    <key>CFBundleExecutable</key><string>easy-ffmpeg</string>
    <key>CFBundleIdentifier</key><string>com.easyffmpeg.app</string>
    <key>CFBundleIconFile</key><string>icon.icns</string>
    <key>CFBundlePackageType</key><string>APPL</string>
    <key>CFBundleVersion</key><string>1.0</string>
    <key>CFBundleShortVersionString</key><string>1.0</string>
    <key>LSMinimumSystemVersion</key><string>10.13</string>
    <key>NSHighResolutionCapable</key><true/>
</dict>
</plist>
`

func main() {
	binPath := flag.String("bin", "", "path to compiled macOS binary")
	icnsPath := flag.String("icns", "assets/icon.icns", "path to icns icon")
	outPath := flag.String("out", "", "output .app bundle path")
	flag.Parse()

	if *binPath == "" || *outPath == "" {
		fmt.Fprintln(os.Stderr, "usage: build_macapp.go -bin <file> -out <app-path>")
		os.Exit(1)
	}

	if err := os.RemoveAll(*outPath); err != nil {
		panic(err)
	}
	macosDir := filepath.Join(*outPath, "Contents", "MacOS")
	resDir := filepath.Join(*outPath, "Contents", "Resources")
	if err := os.MkdirAll(macosDir, 0755); err != nil {
		panic(err)
	}
	if err := os.MkdirAll(resDir, 0755); err != nil {
		panic(err)
	}

	if err := copyFile(*binPath, filepath.Join(macosDir, "easy-ffmpeg"), 0755); err != nil {
		panic(fmt.Errorf("copy binary: %w", err))
	}

	if _, err := os.Stat(*icnsPath); err == nil {
		if err := copyFile(*icnsPath, filepath.Join(resDir, "icon.icns"), 0644); err != nil {
			panic(fmt.Errorf("copy icns: %w", err))
		}
	} else {
		fmt.Fprintf(os.Stderr, "warning: %s not found, app will have no icon\n", *icnsPath)
	}

	plistPath := filepath.Join(*outPath, "Contents", "Info.plist")
	if err := os.WriteFile(plistPath, []byte(infoPlist), 0644); err != nil {
		panic(err)
	}

	fmt.Printf("wrote %s\n", *outPath)
}

func copyFile(src, dst string, mode os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return os.Chmod(dst, mode)
}
