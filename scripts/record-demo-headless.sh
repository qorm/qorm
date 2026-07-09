#!/usr/bin/env bash
# Record the QORM human-AI demo GIF WITHOUT the native WebKit path.
#
# Why this exists: scripts/record-demo.sh captures via `qorm shot` (WKWebView
# takeSnapshot), which returns blank/white frames in headless or sandboxed macOS
# contexts (and hard-crashes with 0xbad4007 where there's no WindowServer). This
# variant renders the pages with headless Chromium in Docker instead — reliable
# anywhere Docker runs — then composites macOS-style window chrome with
# ImageMagick so each pane still reads as a real app window.
#
# It drives the same shared session as the native recorder: a "human" taps +/-
# over the token-authenticated /event channel, then the AI edits over MCP.
#
# Requirements: docker, ImageMagick (`magick`), python3, and a headless-Chromium
# image (default zenika/alpine-chrome — `docker pull` it first). On macOS the
# frames dir must live under $HOME so Docker Desktop can share it.
#
#   ./scripts/record-demo-headless.sh                 # examples/counter -> assets/qorm-demo.gif
#   APP=examples/todo OUT=/tmp/x.gif ./scripts/record-demo-headless.sh
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"; cd "$ROOT"

APP="${APP:-examples/counter}"
OUT="${OUT:-$ROOT/assets/qorm-demo.gif}"
IMG="${CHROME_IMAGE:-zenika/alpine-chrome:latest}"
PORT="${PORT:-8878}"
HOSTURL="http://127.0.0.1:$PORT"
DOCKURL="http://host.docker.internal:$PORT"       # Docker Desktop -> host loopback
FR="$HOME/.qorm-demo-frames"                       # under $HOME: Docker-shareable

for bin in docker magick python3; do
  command -v "$bin" >/dev/null || { echo "error: '$bin' is required" >&2; exit 1; }
done
docker image inspect "$IMG" >/dev/null 2>&1 || { echo "error: pull the chromium image first: docker pull $IMG" >&2; exit 1; }

BIN="$(mktemp -d)/qorm"
echo "==> building qorm"
go build -o "$BIN" ./cmd/qorm
rm -rf "$FR"; mkdir -p "$FR"

echo "==> starting $APP on :$PORT (0.0.0.0 so Docker can reach it)"
"$BIN" run "$APP" --lan --port "$PORT" --no-open >/tmp/qorm-rec.log 2>&1 & SRV=$!
trap 'kill $SRV 2>/dev/null || true' EXIT
until curl -s -m2 "$HOSTURL/" >/dev/null 2>&1; do sleep 0.3; done; sleep 1

mcp(){ curl -s -X POST "$HOSTURL/mcp" -H 'Content-Type: application/json' \
  -d "{\"jsonrpc\":\"2.0\",\"id\":1,\"method\":\"tools/call\",\"params\":{\"name\":\"$1\",\"arguments\":$2}}"; }
editprop(){ local ops="[{\"op\":\"setProp\",\"target\":\"$1\",\"key\":\"$2\",\"value\":\"$3\"}]"
  local tok; tok=$(mcp qorm_preview_patch "{\"ops\":$ops}"|python3 -c 'import sys,json;print(json.loads(json.load(sys.stdin)["result"]["content"][0]["text"])["previewToken"])')
  mcp qorm_apply_patch "{\"ops\":$ops,\"previewToken\":\"$tok\"}">/dev/null; }

# headless Chromium screenshot of URL -> file. headless=new + compositor-stages
# captures on the load event and forces a paint, so it exits in ~1s. Do NOT add
# --virtual-time-budget: it never settles on the app's persistent SSE connection.
cap(){ docker run --rm -v "$FR":/frames "$IMG" --no-sandbox --headless=new --disable-gpu \
  --hide-scrollbars --force-device-scale-factor=2 --run-all-compositor-stages-before-draw \
  --window-size="$2,$3" --screenshot="/frames/$4" "$1" >/dev/null 2>&1; }

FONT=/System/Library/Fonts/Helvetica.ttc
[ -f "$FONT" ] || FONT=""   # ImageMagick default font elsewhere
titlebar(){ local w2=$(( $1 * 2 ))
  magick -size ${w2}x56 gradient:'#3a3d44-#2d2f36' \
    -fill '#ff5f57' -draw "circle 34,28 34,17" -fill '#febc2e' -draw "circle 70,28 70,17" \
    -fill '#28c840' -draw "circle 106,28 106,17" \
    -fill '#d0d2d8' -gravity center -pointsize 22 ${FONT:+-font "$FONT"} -annotate 0 "$2" "$FR/$3"; }
chrome(){ titlebar "$3" "$2" _bar.png
  magick "$FR/_bar.png" \( "$1" -background '#0b0c0f' -gravity North -extent x$(( $4 - 56 )) \) \
    -append -background '#0b0c0f' -gravity North -extent x$4 "$FR/$5"; }

FN=0
shot(){ sleep 0.7; local n; n=$(printf %03d $FN)
  cap "$DOCKURL/"          360 480 "app_$n.png"
  cap "$DOCKURL/logwindow" 470 520 "log_$n.png"
  magick "$FR/app_$n.png" -gravity North -crop 720x740+0+0 +repage "$FR/appc_$n.png"
  magick "$FR/log_$n.png" -crop 940x640+0+92 +repage "$FR/logc_$n.png"   # drop dup header
  chrome "$FR/appc_$n.png" "Counter"      360 800 "af_$n.png"
  chrome "$FR/logc_$n.png" "QORM DevTool" 470 800 "lf_$n.png"
  magick "$FR/af_$n.png" "$FR/lf_$n.png" +append -bordercolor '#0b0c0f' -border 18x18 "$FR/f$n.png"
  FN=$((FN+1)); cp "$FR/f$n.png" "$FR/f$(printf %03d $FN).png"; FN=$((FN+1)); }

echo "==> capturing frames (human taps, then AI edits over MCP)"
shot                                                                # initial
TOK=$(curl -s "$HOSTURL/" | grep -o "var __tok='[^']*'" | cut -d"'" -f2)
click(){ curl -s -X POST "$HOSTURL/event" -H "X-Qorm-Token: $TOK" -H 'Content-Type: application/json' -d "{\"h\":$1,\"inputs\":{}}" >/dev/null; }
click 1; sleep 0.2; click 1; sleep 0.2; click 0; sleep 0.2; click 1  # the "human"
shot; shot
mcp qorm_set_state '{"path":"count","value":7}'>/dev/null;                 shot   # the AI
mcp qorm_set_state '{"path":"status","value":"AI is editing…"}'>/dev/null; shot
mcp qorm_set_state '{"path":"count","value":42}'>/dev/null;                shot
editprop title text "COUNTER · live with AI";                             shot
shot

echo "==> assembling $OUT"
magick -delay 65 -loop 0 "$FR"/f*.png -resize 620x "$OUT"
magick "$OUT" -layers optimize "$OUT"
magick identify -format 'wrote %f: %wx%h, %n frames, %b\n' "$OUT" | head -1
