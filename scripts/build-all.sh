#!/usr/bin/env bash
# Cross-compile the QORM runtime for every supported platform from one machine.
# Pure Go (CGO disabled) means no cross toolchain is required.
set -euo pipefail

cd "$(dirname "$0")/.."
OUT="${1:-dist}"
mkdir -p "$OUT"

VERSION="${QORM_VERSION:-$(git describe --tags --always --dirty 2>/dev/null || echo 0.1.0)}"

targets=(
  "darwin/amd64"
  "darwin/arm64"
  "linux/amd64"
  "linux/arm64"
  "windows/amd64"
  "windows/arm64"
)

echo "building qorm $VERSION for ${#targets[@]} targets -> $OUT/"
for t in "${targets[@]}"; do
  os="${t%/*}"; arch="${t#*/}"
  ext=""; [ "$os" = "windows" ] && ext=".exe"
  name="$OUT/qorm-$os-$arch$ext"
  CGO_ENABLED=0 GOOS="$os" GOARCH="$arch" \
    go build -trimpath -ldflags "-s -w -X main.version=$VERSION" -o "$name" ./cmd/qorm
  printf "  %-22s %s\n" "$t" "$(du -h "$name" | cut -f1)"
done
echo "done."
