#!/usr/bin/env bash
# Cross-compiles the portable Windows app: dist/leftoff-<version>-windows-<arch>.exe.
# Double-clicking it starts the local server and opens the default browser;
# it has no console window (-H windowsgui), so logs go to %APPDATA%\leftoff\app.log.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
GOARCH="${GOARCH:-amd64}"
VERSION="${VERSION:-$(git -C "$ROOT" describe --tags --always --dirty 2>/dev/null || echo dev)}"
TARGET="$ROOT/dist/leftoff-$VERSION-windows-$GOARCH.exe"

echo "==> building UI"
(cd "$ROOT/web" && npm ci --no-audit --no-fund && npm run build)

echo "==> building $TARGET"
mkdir -p "$ROOT/dist"
(cd "$ROOT" && CGO_ENABLED=0 GOOS=windows GOARCH="$GOARCH" go build -trimpath -tags rod \
  -ldflags "-s -w -H windowsgui -X main.version=$VERSION -X main.defaultCommand=app" \
  -o "$TARGET" ./cmd/server)
echo "==> $TARGET"
