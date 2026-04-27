#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")"
mkdir -p dist

LDFLAGS="-s -w"

# ---- Web edition: 4 cross-compiled artifacts (CGO=0) ----------------------
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

# ---- Desktop edition: native only (CGO=1, requires Wails CLI) ------------
# Skipped silently when the toolchain is missing so a developer without
# Wails still gets the 4 Web artifacts.
build_desktop() {
    if ! command -v wails >/dev/null 2>&1; then
        echo "==> wails CLI not found, skipping desktop build"
        return 0
    fi

    local host
    host="$(uname -s)"
    # Wails CLI's -o flag mishandles relative paths; use its default
    # output under cmd/desktop/build/bin/ and move into repo-level dist/.
    case "$host" in
        Darwin)
            echo "==> Building desktop/darwin/arm64 -> dist/easy-ffmpeg-desktop-macos-arm64.app"
            (cd cmd/desktop && wails build -clean -platform darwin/arm64)
            rm -rf "dist/easy-ffmpeg-desktop-macos-arm64.app"
            mv "cmd/desktop/build/bin/easy-ffmpeg-desktop.app" "dist/easy-ffmpeg-desktop-macos-arm64.app"
            echo "==> Building desktop/darwin/amd64 -> dist/easy-ffmpeg-desktop-macos-amd64.app"
            (cd cmd/desktop && wails build -clean -platform darwin/amd64)
            rm -rf "dist/easy-ffmpeg-desktop-macos-amd64.app"
            mv "cmd/desktop/build/bin/easy-ffmpeg-desktop.app" "dist/easy-ffmpeg-desktop-macos-amd64.app"
            ;;
        Linux)
            echo "==> Building desktop/linux -> dist/easy-ffmpeg-desktop-linux"
            (cd cmd/desktop && wails build -clean)
            mv -f "cmd/desktop/build/bin/easy-ffmpeg-desktop" "dist/easy-ffmpeg-desktop-linux"
            ;;
        MINGW*|MSYS*|CYGWIN*)
            echo "==> Building desktop/windows -> dist/easy-ffmpeg-desktop.exe"
            (cd cmd/desktop && wails build -clean)
            mv -f "cmd/desktop/build/bin/easy-ffmpeg-desktop.exe" "dist/easy-ffmpeg-desktop.exe"
            ;;
        *)
            echo "==> Unknown host '$host', skipping desktop build"
            ;;
    esac
}

build_desktop

echo
echo "========================================"
echo "  Build successful"
echo "========================================"
ls -lh dist/
