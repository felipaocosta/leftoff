#!/usr/bin/env bash
# Builds dist/LeftOff-<version>-<arch>.AppImage.
# Requirements: Go, Node/npm. appimagetool is downloaded on first use into
# ~/.cache/leftoff (or set APPIMAGETOOL=/path/to/appimagetool).
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
ARCH="${ARCH:-x86_64}"
case "$ARCH" in
  x86_64) GOARCH=amd64 ;;
  aarch64) GOARCH=arm64 ;;
  *) echo "unsupported ARCH=$ARCH (x86_64 or aarch64)" >&2; exit 1 ;;
esac
VERSION="${VERSION:-$(git -C "$ROOT" describe --tags --always --dirty 2>/dev/null || echo dev)}"
OUT="$ROOT/dist"
WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT
APPDIR="$WORK/AppDir"

echo "==> building UI"
(cd "$ROOT/web" && npm ci --no-audit --no-fund && npm run build)

echo "==> building binary ($GOARCH)"
mkdir -p "$APPDIR/usr/bin"
(cd "$ROOT" && CGO_ENABLED=0 GOOS=linux GOARCH="$GOARCH" go build -trimpath -tags rod \
  -ldflags "-s -w -X main.version=$VERSION -X main.defaultCommand=app" \
  -o "$APPDIR/usr/bin/leftoff" ./cmd/server)

echo "==> assembling AppDir"
cp "$ROOT/deploy/appimage/AppRun" "$APPDIR/AppRun"
cp "$ROOT/deploy/appimage/leftoff.desktop" "$APPDIR/"
cp "$ROOT/deploy/icons/leftoff.svg" "$APPDIR/leftoff.svg"
ln -s leftoff.svg "$APPDIR/.DirIcon"

TOOL="${APPIMAGETOOL:-}"
if [ -z "$TOOL" ]; then
  CACHE="${XDG_CACHE_HOME:-$HOME/.cache}/leftoff"
  TOOL="$CACHE/appimagetool-$(uname -m).AppImage"
  if [ ! -x "$TOOL" ]; then
    echo "==> downloading appimagetool"
    mkdir -p "$CACHE"
    curl -fsSL -o "$TOOL.tmp" \
      "https://github.com/AppImage/appimagetool/releases/download/continuous/appimagetool-$(uname -m).AppImage"
    chmod +x "$TOOL.tmp" && mv "$TOOL.tmp" "$TOOL"
  fi
fi

echo "==> packing"
mkdir -p "$OUT"
TARGET="$OUT/LeftOff-$VERSION-$ARCH.AppImage"
# --appimage-extract-and-run lets appimagetool run without FUSE.
ARCH="$ARCH" "$TOOL" --appimage-extract-and-run --no-appstream "$APPDIR" "$TARGET"
echo "==> $TARGET"
