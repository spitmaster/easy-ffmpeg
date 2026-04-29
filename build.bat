@echo off
setlocal EnableDelayedExpansion

REM Force UTF-8 console codepage. Wails and most Go tools emit UTF-8;
REM on Chinese Windows the default is GBK (936) which mangles non-ASCII
REM bullet / heart glyphs into garbage when stdout is captured by IDE
REM Run buttons. 65001 is the UTF-8 codepage. Keep this comment pure
REM ASCII so cmd's pre-chcp parse does not choke on it.
chcp 65001 >nul

cd /d "%~dp0"
if not exist dist mkdir dist

set LDFLAGS=-s -w
set CGO_ENABLED=0
REM Disable ANSI color escape sequences in child tool output (wails CLI,
REM go test, etc.). When this script's stdout is captured by VS Code's
REM Run button or piped to a file, the codes show up as literal "[1;33m"
REM garbage. NO_COLOR is the cross-tool standard (https://no-color.org/).
set NO_COLOR=1

REM ---- Frontend: build Vue UI into web\dist\ before Go embeds it ------
REM Must run before any go build, since web\embed.go does //go:embed all:dist.
where npm >nul 2>&1
if errorlevel 1 (
    echo ERROR: npm not found. Install Node.js ^>= 20 to build the frontend.
    goto :fail
)
echo ==^> Building frontend (web\) -^> web\dist\
pushd web
call npm install --no-audit --no-fund
if errorlevel 1 (popd ^& goto :fail)
call npm run build
if errorlevel 1 (popd ^& goto :fail)
popd

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
REM Wails CLI's banner bypasses NO_COLOR and prints raw ANSI escape
REM sequences. Pipe through PowerShell to strip CSI codes; preserve the
REM real exit code via $LASTEXITCODE. Also pin all encodings to UTF-8
REM so non-ASCII bullet / heart glyphs survive the pipe regardless of
REM the user's console codepage.
powershell -NoProfile -Command "$OutputEncoding=[Text.Encoding]::UTF8; [Console]::OutputEncoding=$OutputEncoding; [Console]::InputEncoding=$OutputEncoding; & wails build -clean 2>&1 | ForEach-Object { $_ -replace '\x1b\[[0-9;]*[a-zA-Z]', '' }; exit $LASTEXITCODE"
set WAILS_RC=%ERRORLEVEL%
popd
if not "%WAILS_RC%"=="0" goto :fail
REM Wails CLI's -o flag mishandles relative paths; use its default output
REM at cmd\desktop\build\bin\ then move into the repo-level dist\.
move /Y "cmd\desktop\build\bin\easy-ffmpeg-desktop.exe" "dist\easy-ffmpeg-desktop.exe" >nul || goto :fail

:done
REM Ask Windows shell to re-evaluate icon associations so Explorer picks
REM up the freshly built exe icons instead of showing a stale cached one.
REM Silent + non-fatal: failure here doesn't fail the build.
ie4uinit.exe -show >nul 2>&1

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
