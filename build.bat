@echo off
setlocal EnableDelayedExpansion

cd /d "%~dp0"
if not exist dist mkdir dist

set LDFLAGS=-s -w
set CGO_ENABLED=0

REM ---- Web edition: 4 cross-compiled artifacts (CGO=0) -----------------
call :build windows amd64 easy-ffmpeg.exe            || goto :fail
call :build darwin  arm64 easy-ffmpeg-macos-arm64    || goto :fail
call :build darwin  amd64 easy-ffmpeg-macos-amd64    || goto :fail
call :build linux   amd64 easy-ffmpeg-linux          || goto :fail

REM ---- Desktop edition: native build only (CGO=1, requires Wails) ------
REM    Skipped silently if the wails CLI is not on PATH so developers
REM    without the desktop toolchain still get the 4 Web artifacts.
where wails >nul 2>&1
if errorlevel 1 (
    echo ==^> wails CLI not found, skipping desktop build
    goto :done
)

echo ==^> Building desktop/windows -^> dist\easy-ffmpeg-desktop.exe
pushd cmd\desktop
wails build -clean
set WAILS_RC=%ERRORLEVEL%
popd
if not "%WAILS_RC%"=="0" goto :fail
REM Wails CLI's -o flag mishandles relative paths; use its default output
REM at cmd\desktop\build\bin\ then move into the repo-level dist\.
move /Y "cmd\desktop\build\bin\easy-ffmpeg-desktop.exe" "dist\easy-ffmpeg-desktop.exe" >nul || goto :fail

:done
echo.
echo ========================================
echo   Build successful
echo ========================================
dir /B dist
exit /b 0

:build
setlocal
echo ==^> Building %~1/%~2 -^> dist\%~3
set GOOS=%~1
set GOARCH=%~2
go build -ldflags="%LDFLAGS%" -o "dist\%~3" ./cmd
endlocal & exit /b %ERRORLEVEL%

:fail
echo.
echo ========================================
echo   Build FAILED
echo ========================================
exit /b 1
