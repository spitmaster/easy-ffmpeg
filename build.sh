#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")"
mkdir -p dist

LDFLAGS="-s -w"
# Disable ANSI color escape sequences in child tool output. Cross-tool
# standard, respected by wails CLI's logger (pterm) and most modern
# CLIs. Keeps captured logs readable when piped or shown in IDEs that
# don't render VT codes.
export NO_COLOR=1

# ---- Frontend: build Vue UI into web/dist/ before Go embeds it ----------
# Must run before any go build, since web/embed.go does //go:embed all:dist.
build_frontend() {
    if ! command -v npm >/dev/null 2>&1; then
        echo "ERROR: npm not found. Install Node.js >= 20 to build the frontend." >&2
        return 1
    fi
    echo "==> Building frontend (web/) -> web/dist/"
    (cd web && npm install --no-audit --no-fund && npm run build)
}

build_frontend

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
            (cd cmd/desktop && wails build -clean -platform darwin/arm64 2>&1 | sed -E 's/\x1b\[[0-9;]*[a-zA-Z]//g')
            rm -rf "dist/easy-ffmpeg-desktop-macos-arm64.app"
            mv "cmd/desktop/build/bin/easy-ffmpeg-desktop.app" "dist/easy-ffmpeg-desktop-macos-arm64.app"
            echo "==> Building desktop/darwin/amd64 -> dist/easy-ffmpeg-desktop-macos-amd64.app"
            (cd cmd/desktop && wails build -clean -platform darwin/amd64 2>&1 | sed -E 's/\x1b\[[0-9;]*[a-zA-Z]//g')
            rm -rf "dist/easy-ffmpeg-desktop-macos-amd64.app"
            mv "cmd/desktop/build/bin/easy-ffmpeg-desktop.app" "dist/easy-ffmpeg-desktop-macos-amd64.app"
            ;;
        Linux)
            echo "==> Building desktop/linux -> dist/easy-ffmpeg-desktop-linux"
            (cd cmd/desktop && wails build -clean 2>&1 | sed -E 's/\x1b\[[0-9;]*[a-zA-Z]//g')
            mv -f "cmd/desktop/build/bin/easy-ffmpeg-desktop" "dist/easy-ffmpeg-desktop-linux"
            ;;
        MINGW*|MSYS*|CYGWIN*)
            echo "==> Building desktop/windows -> dist/easy-ffmpeg-desktop.exe"
            (cd cmd/desktop && wails build -clean 2>&1 | sed -E 's/\x1b\[[0-9;]*[a-zA-Z]//g')
            mv -f "cmd/desktop/build/bin/easy-ffmpeg-desktop.exe" "dist/easy-ffmpeg-desktop.exe"
            ;;
        *)
            echo "==> Unknown host '$host', skipping desktop build"
            ;;
    esac
}

build_desktop

# Nudge the host's file manager / launcher to drop any cached thumbnail
# of the just-rebuilt artifacts so the new icon shows up immediately.
# All steps are silent + non-fatal — the build succeeds even if the
# refresh fails (e.g. running headless or in CI).
refresh_icons() {
    case "$(uname -s)" in
        Darwin)
            # Touch updates mtime, which Finder uses to invalidate its
            # icon thumbnail cache for a given path. lsregister forces
            # Launch Services to re-read the .app bundle's Info.plist
            # (which references the icon file).
            touch -c dist/*.app 2>/dev/null || true
            local lsr="/System/Library/Frameworks/CoreServices.framework/Frameworks/LaunchServices.framework/Support/lsregister"
            if [ -x "$lsr" ]; then
                for app in dist/*.app; do
                    [ -e "$app" ] && "$lsr" -f "$app" >/dev/null 2>&1 || true
                done
            fi
            ;;
        Linux)
            # Linux file managers cache thumbnails per inode; touching
            # the file changes mtime and most file managers re-evaluate
            # on next directory open. There is no global icon cache to
            # bust for ELF executables (icons live in .desktop files,
            # which we do not ship from this build).
            touch -c dist/easy-ffmpeg-linux dist/easy-ffmpeg-desktop-linux 2>/dev/null || true
            ;;
        MINGW*|MSYS*|CYGWIN*)
            # Same shell-refresh as build.bat for users invoking via Git Bash.
            ie4uinit.exe -show >/dev/null 2>&1 || true
            ;;
    esac
}

refresh_icons

echo
echo "========================================"
echo "  Build successful"
echo "========================================"
ls -lh dist/
