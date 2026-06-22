#!/bin/bash

# 构建脚本 - 打包多平台版本

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

# macOS x64 (Intel)
echo "构建 macOS x64..."
GOOS=darwin GOARCH=amd64 go build -ldflags="-s -w" -o "$OUTPUT_DIR/${APP_NAME}-darwin-amd64" .
echo "✓ macOS x64 完成"

# Windows x86 (32位)
echo "构建 Windows x86..."
GOOS=windows GOARCH=386 go build -ldflags="-s -w" -o "$OUTPUT_DIR/${APP_NAME}-windows-386.exe" .
echo "✓ Windows x86 完成"

# Windows x64 (64位)
echo "构建 Windows x64..."
GOOS=windows GOARCH=amd64 go build -ldflags="-s -w" -o "$OUTPUT_DIR/${APP_NAME}-windows-amd64.exe" .
echo "✓ Windows x64 完成"

# Linux x86 (32位)
echo "构建 Linux x86..."
GOOS=linux GOARCH=386 go build -ldflags="-s -w" -o "$OUTPUT_DIR/${APP_NAME}-linux-386" .
echo "✓ Linux x86 完成"

# Linux x64 (64位)
echo "构建 Linux x64..."
GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o "$OUTPUT_DIR/${APP_NAME}-linux-amd64" .
echo "✓ Linux x64 完成"

# Linux ARM64 (64位 ARM，如树莓派4、云服务器等)
echo "构建 Linux ARM64..."
GOOS=linux GOARCH=arm64 go build -ldflags="-s -w" -o "$OUTPUT_DIR/${APP_NAME}-linux-arm64" .
echo "✓ Linux ARM64 完成"

# Linux ARM (32位 ARM，如旧版树莓派)
echo "构建 Linux ARM..."
GOOS=linux GOARCH=arm GOARM=7 go build -ldflags="-s -w" -o "$OUTPUT_DIR/${APP_NAME}-linux-arm" .
echo "✓ Linux ARM 完成"

echo ""
echo "构建完成! 输出目录: $OUTPUT_DIR"
ls -lh "$OUTPUT_DIR"
