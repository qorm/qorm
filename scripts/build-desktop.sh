#!/usr/bin/env bash
# Build the native-WebView desktop binary for THIS platform (Wails-style).
#
# Unlike build-all.sh (pure Go, cross-compiles everywhere), the desktop build
# uses cgo + the platform's native WebView (WKWebView / WebView2 / WebKitGTK),
# so it is built per-platform — run this on each target OS (e.g. in a CI matrix).
#
# Linux additionally needs the WebKitGTK dev headers, e.g.:
#   sudo apt-get install libgtk-3-dev libwebkit2gtk-4.1-dev
set -euo pipefail

cd "$(dirname "$0")/.."
OUT="${1:-dist}"
mkdir -p "$OUT"

os="$(go env GOOS)"; arch="$(go env GOARCH)"
ext=""; [ "$os" = "windows" ] && ext=".exe"
name="$OUT/qorm-desktop-$os-$arch$ext"
VERSION="${QORM_VERSION:-$(git describe --tags --always --dirty 2>/dev/null || echo 0.1.0)}"

echo "building native-WebView desktop binary for $os/$arch"
CGO_ENABLED=1 go build -tags desktop -ldflags "-s -w -X main.version=$VERSION" -o "$name" ./cmd/qorm
echo "  -> $name ($(du -h "$name" | cut -f1))"
echo "run it with:  $name run <app-dir|bundle> --app"
