#!/usr/bin/env bash
# Build yrt for Linux and Windows
set -euo pipefail

ROOT="$(dirname "$(readlink -f "$0")")"

(
  cd "$ROOT"
  mkdir -p build

  echo "=== Building yrt ==="

  echo "  Linux x86_64..."
  GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o build/yrt -ldflags="-s -w" .

  echo "  Windows x86_64..."
  GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -o build/yrt.exe -ldflags="-s -w" .

  echo ""
  echo "Done:"
  ls -lh build/yrt build/yrt.exe
  file build/yrt build/yrt.exe
)
