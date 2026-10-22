#!/usr/bin/env bash
# Builds the Android app (Phase 7) into dist/leftoff-<version>.apk:
# the Go core (mobile/) with gomobile bind, then the Kotlin shell with Gradle.
# Needs ANDROID_HOME (SDK with platform 36, build-tools and NDK), a JDK 17+
# (JAVA_HOME) and gomobile (go install golang.org/x/mobile/cmd/gomobile@latest).
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
VERSION="${VERSION:-$(git -C "$ROOT" describe --tags --always --dirty 2>/dev/null || echo dev)}"
: "${ANDROID_HOME:=$HOME/Android/Sdk}"
: "${ANDROID_NDK_HOME:=$(ls -d "$ANDROID_HOME"/ndk/* | sort -V | tail -1)}"
export ANDROID_HOME ANDROID_NDK_HOME
[[ -n "${JAVA_HOME:-}" ]] && export PATH="$JAVA_HOME/bin:$PATH"
export PATH="$PATH:$(go env GOPATH)/bin"

echo "==> building UI"
(cd "$ROOT/web" && npm ci --no-audit --no-fund && npm run build)

echo "==> gomobile bind"
mkdir -p "$ROOT/android/app/libs"
(cd "$ROOT" && gomobile bind -target android/arm64,android/amd64 -androidapi 24 -javapkg io.github.felipaocosta.leftoff \
  -ldflags "-s -w -X github.com/felipaocosta/leftoff/mobile.Version=$VERSION" -o android/app/libs/leftoff.aar ./mobile)

# The release key (see app/build.gradle.kts) stays outside the repository.
SIGNING_ENV="${LEFTOFF_SIGNING_ENV:-$HOME/.local/share/leftoff-release/signing.env}"
if [[ -f "$SIGNING_ENV" ]]; then
  set -a; . "$SIGNING_ENV"; set +a
  echo "==> signing with the release key"
else
  echo "==> no release key ($SIGNING_ENV): signing with the debug key, not for distribution"
fi

echo "==> gradle assembleRelease"
(cd "$ROOT/android" && ./gradlew --no-daemon -q assembleRelease -PappVersion="$VERSION")
mkdir -p "$ROOT/dist"
cp "$ROOT/android/app/build/outputs/apk/release/app-release.apk" "$ROOT/dist/leftoff-$VERSION.apk"
echo "==> $ROOT/dist/leftoff-$VERSION.apk"
