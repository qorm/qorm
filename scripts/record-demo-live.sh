#!/usr/bin/env bash
# Record the QORM human-AI demo GIF on macOS 15+/26, where the in-process capture
# APIs are dead (WKWebView takeSnapshot -> white; CGWindowListCreateImage ->
# removed; ScreenCaptureKit -> SIGBUS for a bare CLI). Uses the fixed
# `qorm shot --live`, which captures the REAL running windows via Apple's
# entitled screencapture tool.
#
# Prereq: launch the app first (in its own terminal) and grant that terminal
# Screen Recording (System Settings > Privacy & Security > Screen Recording):
#     qorm run examples/counter --app
# Then run this from the repo root. Overrides: QORM=/path/to/qorm (a -tags desktop
# build), OUT=/path/to.gif, PORT=10383, APPWIN / LOGWIN window-title substrings.
set -uo pipefail
Q="${QORM:-qorm}"
OUT="${OUT:-assets/qorm-demo.gif}"
PORT="${PORT:-10383}"
APPWIN="${APPWIN:-QORM Premium Counter}"
LOGWIN="${LOGWIN:-Activity log}"
U="http://127.0.0.1:$PORT"
FR="$HOME/.qorm-demo-frames"; rm -rf "$FR"; mkdir -p "$FR"

command -v magick >/dev/null || { echo "need ImageMagick (magick)"; exit 1; }
curl -s -m2 "$U/" >/dev/null || { echo "app not running on :$PORT — start: $Q run examples/counter --app"; exit 1; }

mcp(){ curl -s -X POST "$U/mcp" -H 'Content-Type: application/json' -d "{\"jsonrpc\":\"2.0\",\"id\":1,\"method\":\"tools/call\",\"params\":{\"name\":\"$1\",\"arguments\":$2}}" >/dev/null; }
editprop(){ local ops="[{\"op\":\"setProp\",\"target\":\"$1\",\"key\":\"$2\",\"value\":\"$3\"}]"
  local tok; tok=$(curl -s -X POST "$U/mcp" -H 'Content-Type: application/json' -d "{\"jsonrpc\":\"2.0\",\"id\":1,\"method\":\"tools/call\",\"params\":{\"name\":\"qorm_preview_patch\",\"arguments\":{\"ops\":$ops}}}" | python3 -c 'import sys,json;print(json.loads(json.load(sys.stdin)["result"]["content"][0]["text"])["previewToken"])' 2>/dev/null)
  [ -n "$tok" ] && mcp qorm_apply_patch "{\"ops\":$ops,\"previewToken\":\"$tok\"}"; }

FN=0
frame(){ sleep 0.6; local n; n=$(printf %03d $FN)
  "$Q" shot --live "$APPWIN" -o "$FR/a_$n.png" >/dev/null 2>&1
  "$Q" shot --live "$LOGWIN" -o "$FR/l_$n.png" >/dev/null 2>&1
  magick \( "$FR/a_$n.png" -background '#0b0c0f' -gravity North -extent x672 \) \
         "$FR/l_$n.png" -background '#0b0c0f' +append -bordercolor '#0b0c0f' -border 16x16 "$FR/f$n.png" 2>/dev/null
  FN=$((FN+1)); cp "$FR/f$n.png" "$FR/f$(printf %03d $FN).png" 2>/dev/null; FN=$((FN+1)); }

# clean progression; the human's real +/- clicks stay in the log history
mcp qorm_set_state '{"path":"count","value":0}'; editprop title text "COUNTER"; mcp qorm_set_state '{"path":"status","value":"Ready"}'
echo "frame: start";  frame
echo "AI: count 7";   mcp qorm_set_state '{"path":"count","value":7}';                        frame
echo "AI: status";    mcp qorm_set_state '{"path":"status","value":"AI is editing with you…"}'; frame
echo "AI: count 42";  mcp qorm_set_state '{"path":"count","value":42}';                       frame
echo "AI: title";     editprop title text "COUNTER · live with AI";                           frame
frame

magick -delay 70 -loop 0 "$FR"/f*.png -resize 700x "$OUT" 2>/dev/null
magick "$OUT" -layers optimize "$OUT" 2>/dev/null
magick identify -format 'wrote %f: %wx%h, %n frames, %b\n' "$OUT" 2>/dev/null | head -1
