#!/usr/bin/env bash
# Builds the desktop app (Phase 6, Wails) into dist/:
#   leftoff-desktop-<version>-linux-amd64        (needs webkit2gtk-4.1 and gtk3 to run)
#   leftoff-desktop-<version>-windows-amd64.exe  (needs WebView2, part of Windows 10/11)
#   leftoff-desktop-<version>-windows-setup.exe  (NSIS installer, when makensis exists)
# Linux needs webkit2gtk4.1-devel and gtk3-devel; Windows is cross-compiled
# without CGO. Pass "linux" or "windows" to build only one.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
VERSION="${VERSION:-$(git -C "$ROOT" describe --tags --always --dirty 2>/dev/null || echo dev)}"
ONLY="${1:-all}"
DIST="$ROOT/dist"
LDFLAGS="-s -w -X main.version=$VERSION"
mkdir -p "$DIST"

echo "==> building UI"
(cd "$ROOT/web" && npm ci --no-audit --no-fund && npm run build)

if [[ "$ONLY" == all || "$ONLY" == linux ]]; then
  out="$DIST/leftoff-desktop-$VERSION-linux-amd64"
  echo "==> $out"
  (cd "$ROOT" && CGO_ENABLED=1 go build -trimpath -tags desktop,production,webkit2_41,rod \
    -ldflags "$LDFLAGS" -o "$out" ./cmd/desktop)
fi

if [[ "$ONLY" == all || "$ONLY" == windows ]]; then
  exe="$DIST/leftoff-desktop-$VERSION-windows-amd64.exe"
  echo "==> $exe"
  (cd "$ROOT" && CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -trimpath -tags desktop,production,rod \
    -ldflags "$LDFLAGS -H windowsgui" -o "$exe" ./cmd/desktop)
  if command -v makensis >/dev/null; then
    work="$(mktemp -d)"
    trap 'rm -rf "$work"' EXIT
    magick "$ROOT/web/public/icons/icon-512.png" -define icon:auto-resize=256,128,64,48,32,16 "$work/icon.ico"
    cp "$exe" "$work/leftoff.exe"
    setup="$DIST/leftoff-desktop-$VERSION-windows-setup.exe"
    echo "==> $setup"
    makensis -V2 -DVERSION="$VERSION" -DSRC="$work" -DOUT="$setup" "$ROOT/deploy/desktop/installer.nsi"
  else
    echo "makensis not found: skipping the Windows installer (dnf install mingw32-nsis)"
  fi
fi
