#!/bin/bash
# Local build script for aria2-tui
# Created for testing version injection and local development

set -e

# Get version from git tag or use default
VERSION=$(git describe --tags --always --dirty 2>/dev/null || echo "dev-$(date +%Y%m%d)")
MAIN_PACKAGE="./cmd/aria2-tui"
OUTPUT_NAME="aria2-tui"

echo "Building aria2-tui version: $VERSION"

# Build with version injection
go build -ldflags="-s -w -X github.com/KevinYouu/aria2-tui/internal/version.Version=$VERSION" \
    -o "$OUTPUT_NAME" "$MAIN_PACKAGE"

echo "Build completed: $OUTPUT_NAME"
echo "Run './$OUTPUT_NAME version' to verify version injection"
