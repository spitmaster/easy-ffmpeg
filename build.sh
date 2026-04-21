#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")"
mkdir -p dist

LDFLAGS="-s -w"

build() {
    local os="$1" arch="$2" out="$3"
    echo "==> Building ${os}/${arch} -> dist/${out}"
    CGO_ENABLED=0 GOOS="$os" GOARCH="$arch" \
        go build -ldflags="$LDFLAGS" -o "dist/${out}" ./cmd
}

build windows amd64 easy-ffmpeg.exe
build darwin  arm64 easy-ffmpeg-macos-arm64
build darwin  amd64 easy-ffmpeg-macos-amd64
build linux   amd64 easy-ffmpeg-linux

# Wrap macOS binaries into .app bundles (icon + Info.plist)
echo "==> Wrapping macOS binaries into .app bundles"
go run tools/build_macapp.go -bin dist/easy-ffmpeg-macos-arm64 -out "dist/Easy FFmpeg (arm64).app"
go run tools/build_macapp.go -bin dist/easy-ffmpeg-macos-amd64 -out "dist/Easy FFmpeg (amd64).app"

echo
echo "========================================"
echo "  Build successful"
echo "========================================"
ls -lh dist/
