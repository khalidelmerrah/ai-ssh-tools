#!/usr/bin/env bash
# build.sh — Linux/macOS build script for ai-ssh-tools
set -euo pipefail

MODULE="ai-ssh-tools"
LDFLAGS="-s -w"
TARGET_OS="${GOOS:-$(go env GOOS)}"
TARGET_ARCH="${GOARCH:-$(go env GOARCH)}"
USE_UPX="${USE_UPX:-0}"
BUILD_ALL="${BUILD_ALL:-0}"

build() {
  local os="$1" arch="$2"
  local suffix=""
  [[ "$os" == "windows" ]] && suffix=".exe"
  local out="${MODULE}${suffix}"
  [[ "$os" != "$(go env GOOS)" || "$arch" != "$(go env GOARCH)" ]] && \
    out="${MODULE}-${os}-${arch}${suffix}"

  echo "► Building $out ($os/$arch)..."
  GOOS="$os" GOARCH="$arch" go build -ldflags="$LDFLAGS" -trimpath -o "$out" .
  echo "  ✓ $(du -sh "$out" | cut -f1) — $out"

  if [[ "$USE_UPX" == "1" ]] && command -v upx &>/dev/null; then
    echo "  Compressing with UPX..."
    upx --best --lzma "$out" >/dev/null 2>&1
    echo "  ✓ Compressed: $(du -sh "$out" | cut -f1)"
  fi
}

echo ""
echo "╔══════════════════════════════════╗"
echo "║   ai-ssh-tools build script      ║"
echo "╚══════════════════════════════════╝"
echo ""

echo "► Tidying module dependencies..."
go mod tidy

if [[ "$BUILD_ALL" == "1" ]]; then
  build linux   amd64
  build linux   arm64
  build darwin  amd64
  build darwin  arm64
  build windows amd64
else
  build "$TARGET_OS" "$TARGET_ARCH"
fi

echo ""
echo "Build complete."
