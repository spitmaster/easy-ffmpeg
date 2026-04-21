@echo off
setlocal EnableDelayedExpansion

cd /d "%~dp0"
if not exist dist mkdir dist

set LDFLAGS=-s -w
set CGO_ENABLED=0

call :build windows amd64 easy-ffmpeg.exe            || goto :fail
call :build darwin  arm64 easy-ffmpeg-macos-arm64    || goto :fail
call :build darwin  amd64 easy-ffmpeg-macos-amd64    || goto :fail
call :build linux   amd64 easy-ffmpeg-linux          || goto :fail

echo ==^> Wrapping macOS binaries into .app bundles
go run tools\build_macapp.go -bin dist\easy-ffmpeg-macos-arm64 -out "dist\Easy FFmpeg (arm64).app" || goto :fail
go run tools\build_macapp.go -bin dist\easy-ffmpeg-macos-amd64 -out "dist\Easy FFmpeg (amd64).app" || goto :fail

echo.
echo ========================================
echo   Build successful
echo ========================================
dir /B dist
exit /b 0

:build
echo ==^> Building %~1/%~2 -^> dist\%~3
set GOOS=%~1
set GOARCH=%~2
go build -ldflags="%LDFLAGS%" -o "dist\%~3" ./cmd
exit /b %ERRORLEVEL%

:fail
echo.
echo ========================================
echo   Build FAILED
echo ========================================
exit /b 1
