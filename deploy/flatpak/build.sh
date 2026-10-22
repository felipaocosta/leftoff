#!/usr/bin/env bash
# Builds the Flatpak of the desktop app into dist/leftoff-<version>.flatpak
# and installs it for the current user (pass --no-install to skip).
# Needs flatpak-builder and, from Flathub: org.gnome.Platform//51,
# org.gnome.Sdk//51 and org.freedesktop.Sdk.Extension.golang//26.08.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
HERE="$ROOT/deploy/flatpak"
ID=io.github.felipaocosta.LeftOff
VERSION="${VERSION:-$(git -C "$ROOT" describe --tags --always --dirty 2>/dev/null || echo dev)}"

echo "==> building UI"
(cd "$ROOT/web" && npm ci --no-audit --no-fund && npm run build)
echo "$VERSION" > "$ROOT/VERSION" # read by the manifest
trap 'rm -f "$ROOT/VERSION"' EXIT

echo "==> flatpak-builder"
cd "$HERE"
flatpak-builder --user --force-clean --repo=repo --state-dir=.flatpak-builder build-dir "$ID.yml"
mkdir -p "$ROOT/dist"
flatpak build-bundle repo "$ROOT/dist/leftoff-$VERSION.flatpak" "$ID"
echo "==> $ROOT/dist/leftoff-$VERSION.flatpak"
if [[ "${1:-}" != --no-install ]]; then
  flatpak install --user -y --noninteractive --reinstall "$ROOT/dist/leftoff-$VERSION.flatpak"
  echo "==> installed: flatpak run $ID"
fi
