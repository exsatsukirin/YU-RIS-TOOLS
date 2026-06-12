#!/usr/bin/env bash
# Build yrt for Linux and Windows
set -euo pipefail

cd "$(dirname "$0")/.."

mkdir -p build

echo "=== Building yrt ==="

# Linux x86_64
echo "  Linux x86_64..."
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o build/yrt -ldflags="-s -w" .

# Windows x86_64
echo "  Windows x86_64..."
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -o build/yrt.exe -ldflags="-s -w" .

echo ""
echo "Done:"
ls -lh build/yrt build/yrt.exe
file build/yrt build/yrt.exe
