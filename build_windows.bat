@echo off
REM Windows 平台编译脚本
REM 用于编译 easy-ffmpeg 的 Windows 版本

setlocal EnableDelayedExpansion

echo ========================================
echo   Easy-FFmpeg Windows 编译脚本
echo ========================================
echo.

set PROJECT_DIR=%~dp0
set OUTPUT_DIR=%PROJECT_DIR%dist
set OUTPUT_FILE=%OUTPUT_DIR%\easy-ffmpeg.exe

REM 检查是否已有 FFmpeg 二进制文件
if not exist "%PROJECT_DIR%internal\embedded\windows\ffmpeg.exe" (
    echo [警告] 未找到 Windows FFmpeg 二进制文件
    echo 请运行 download_ffmpeg.ps1 下载或手动放置到 internal/embedded/windows/
    echo.
)

REM 创建输出目录
if not exist "%OUTPUT_DIR%" mkdir "%OUTPUT_DIR%"

echo [1/3] 开始编译...
echo.
set GOOS=windows
set GOARCH=amd64
go build -ldflags="-s -w -H=windowsgui" -o "%OUTPUT_FILE%" ./cmd

if %ERRORLEVEL% NEQ 0 (
    echo.
    echo [错误] 编译失败！
    pause
    exit /b 1
)

echo [2/3] 编译完成
echo.

REM 计算文件大小
for %%F in ("%OUTPUT_FILE%") do set SIZE=%%~zF
set /a SIZE_MB=!SIZE! / 1048576

echo [3/3] 输出信息：
echo   输出文件: %OUTPUT_FILE%
echo   文件大小: !SIZE_MB! MB
echo   编译时间: %date% %time%
echo.

echo ========================================
echo   编译成功！
echo ========================================
echo.
echo 可执行文件位置: %OUTPUT_FILE%
echo.
pause
