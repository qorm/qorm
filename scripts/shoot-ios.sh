#!/bin/bash
# Reshoot an example app's iOS screenshot: package, install in the booted
# simulator, launch, capture, downscale to the 300x650 site format.
# usage: shoot-ios.sh <example-name> [bundle-id]
set -e
cd "$(dirname "$0")/.."
name="$1"
bid="${2:-com.qorm.$1}"
out="/tmp/qorm-ios/$name"
shot="/tmp/qorm-ios/$name.png"

if [ ! -d "$out/build/Build/Products/Debug-iphonesimulator/$name.app" ]; then
  echo "== packaging $name =="
  rm -rf "$out"
  go run ./cmd/qorm package "examples/$name" -p ios -o "$out" >/tmp/qorm-ios/$name.build.log 2>&1 || {
    tail -5 /tmp/qorm-ios/$name.build.log; exit 1; }
fi

app="$out/build/Build/Products/Debug-iphonesimulator/$name.app"
echo "== installing $name ($bid) =="
xcrun simctl install booted "$app"
xcrun simctl launch booted "$bid"
sleep 8
xcrun simctl io booted screenshot "$shot" >/dev/null
sips -Z 650 "$shot" --out "$shot" >/dev/null
echo "== shot: $shot =="
