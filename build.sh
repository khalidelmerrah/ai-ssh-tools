#!/usr/bin/env bash
# build.sh — Linux/macOS build script for ai-ssh-tools
set -euo pipefail

MODULE="ai-ssh-tools"
LDFLAGS="-s -w"
TARGET_OS="${GOOS:-$(go env GOOS)}"
TARGET_ARCH="${GOARCH:-$(go env GOARCH)}"
USE_UPX="${USE_UPX:-0}"
BUILD_ALL="${BUILD_ALL:-0}"

DIST_DIR="dist"

build() {
  local os="$1" arch="$2"
  local suffix=""
  [[ "$os" == "windows" ]] && suffix=".exe"
  local out="${MODULE}${suffix}"

  if [[ "$BUILD_ALL" == "1" ]]; then
    # Release mode: always fully qualified names, collected in dist/ for upload.
    out="${DIST_DIR}/${MODULE}-${os}-${arch}${suffix}"
  elif [[ "$os" != "$(go env GOOS)" || "$arch" != "$(go env GOARCH)" ]]; then
    out="${MODULE}-${os}-${arch}${suffix}"
  fi

  echo "► Building $out ($os/$arch)..."
  GOOS="$os" GOARCH="$arch" go build -ldflags="$LDFLAGS" -trimpath -o "$out" ./cmd/ai-ssh-tools
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
  rm -rf "$DIST_DIR"
  mkdir -p "$DIST_DIR"

  build linux   amd64
  build linux   arm64
  build darwin  amd64
  build darwin  arm64
  build windows amd64

  echo ""
  echo "► Generating SHA256SUMS..."
  (
    cd "$DIST_DIR"
    if command -v sha256sum &>/dev/null; then
      sha256sum "${MODULE}"-* > SHA256SUMS
    else
      shasum -a 256 "${MODULE}"-* > SHA256SUMS
    fi
  )
  echo "  ✓ $DIST_DIR/SHA256SUMS"
  cat "$DIST_DIR/SHA256SUMS"
else
  build "$TARGET_OS" "$TARGET_ARCH"
fi

echo ""
echo "Build complete."
