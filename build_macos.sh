#!/bin/bash
# macOS 平台编译脚本
# 用于编译 easy-ffmpeg 的 macOS 版本

set -e

echo "========================================"
echo "  Easy-FFmpeg macOS 编译脚本"
echo "========================================"
echo ""

PROJECT_DIR="$(cd "$(dirname "$0")" && pwd)"
OUTPUT_DIR="$PROJECT_DIR/dist"
OUTPUT_FILE="$OUTPUT_DIR/easy-ffmpeg"

# 检查是否已有 FFmpeg 二进制文件
if [ ! -f "$PROJECT_DIR/internal/embedded/darwin/ffmpeg" ]; then
    echo "[警告] 未找到 macOS FFmpeg 二进制文件"
    echo "请手动下载并放置到 internal/embedded/darwin/"
    echo ""
fi

# 创建输出目录
mkdir -p "$OUTPUT_DIR"

echo "[1/3] 开始编译..."
echo ""

# 编译 macOS amd64 版本
GOOS=darwin GOARCH=amd64 go build -ldflags="-s -w" -o "$OUTPUT_FILE-amd64" ./cmd
echo "  ✓ 编译 amd64 版本完成"

# 编译 macOS arm64 版本（Apple Silicon）
GOOS=darwin GOARCH=arm64 go build -ldflags="-s -w" -o "$OUTPUT_FILE-arm64" ./cmd
echo "  ✓ 编译 arm64 版本完成"

# 创建通用二进制文件
lipo -create -output "$OUTPUT_FILE" "$OUTPUT_FILE-amd64" "$OUTPUT_FILE-arm64"
echo "  ✓ 创建通用二进制文件完成"

# 删除临时文件
rm -f "$OUTPUT_FILE-amd64" "$OUTPUT_FILE-arm64"

echo ""
echo "[2/3] 编译完成"
echo ""

# 计算文件大小
SIZE=$(du -h "$OUTPUT_FILE" | cut -f1)

echo "[3/3] 输出信息："
echo "  输出文件: $OUTPUT_FILE"
echo "  文件大小: $SIZE"
echo "  编译时间: $(date '+%Y-%m-%d %H:%M:%S')"
echo ""

echo "========================================"
echo "  编译成功！"
echo "========================================"
echo ""
echo "可执行文件位置: $OUTPUT_FILE"
echo ""
