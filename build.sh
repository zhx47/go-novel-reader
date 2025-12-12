#!/bin/bash

# 构建脚本 - 打包 macOS ARM 和 Windows x86 版本

set -e

APP_NAME="novel-reader"
OUTPUT_DIR="dist"

# 清理并创建输出目录
rm -rf "$OUTPUT_DIR"
mkdir -p "$OUTPUT_DIR"

echo "开始构建..."

# macOS ARM64 (Apple Silicon)
echo "构建 macOS ARM64..."
GOOS=darwin GOARCH=arm64 go build -ldflags="-s -w" -o "$OUTPUT_DIR/${APP_NAME}-darwin-arm64" .
echo "✓ macOS ARM64 完成"

# Windows x86 (32位)
echo "构建 Windows x86..."
GOOS=windows GOARCH=386 go build -ldflags="-s -w" -o "$OUTPUT_DIR/${APP_NAME}-windows-386.exe" .
echo "✓ Windows x86 完成"

# Windows x64 (64位)
echo "构建 Windows x64..."
GOOS=windows GOARCH=amd64 go build -ldflags="-s -w" -o "$OUTPUT_DIR/${APP_NAME}-windows-amd64.exe" .
echo "✓ Windows x64 完成"

echo ""
echo "构建完成! 输出目录: $OUTPUT_DIR"
ls -lh "$OUTPUT_DIR"
