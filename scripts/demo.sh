#!/usr/bin/env bash
# QORM live human-AI collaboration demo — a self-driving recording aid.
#
# It starts a shared session (qorm run) and then plays a scripted series of AI
# edits over MCP, with pauses, so you can screen-record the wow moment: the app
# changes live in your browser and an "AI edited" toast shows who did it.
#
#   ./scripts/demo.sh            # uses examples/counter on port 8899
#   ./scripts/demo.sh examples/todo 8899
set -euo pipefail

APP="${1:-examples/counter}"
PORT="${2:-8899}"
URL="http://127.0.0.1:${PORT}"
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

echo "building qorm..."
go build -o /tmp/qorm-demo ./cmd/qorm

/tmp/qorm-demo run "$APP" --port "$PORT" >/tmp/qorm-demo.log 2>&1 &
SRV=$!
trap 'kill $SRV 2>/dev/null || true' EXIT
sleep 2
curl -s "$URL/" >/dev/null || true   # render once so handlers/measure are live

cat <<BANNER

  ------------------------------------------------------------------
   QORM live collaboration demo
   1. Open  $URL  in your browser.
   2. Start your screen recorder.
   3. Press Enter here — the "AI" will edit the app on a timer.
  ------------------------------------------------------------------
BANNER
read -r _

# mcp CALL '<tool>' '<json-args>'  -> prints the tool result text
mcp() {
  curl -s -X POST "$URL/mcp" -H 'Content-Type: application/json' \
    -d "{\"jsonrpc\":\"2.0\",\"id\":1,\"method\":\"tools/call\",\"params\":{\"name\":\"$1\",\"arguments\":$2}}"
}
# apply a single setProp edit through the review-bound preview -> apply gate
edit_prop() { # target key value
  local ops="[{\"op\":\"setProp\",\"target\":\"$1\",\"key\":\"$2\",\"value\":\"$3\"}]"
  local tok
  tok=$(mcp qorm_preview_patch "{\"ops\":$ops}" \
        | python3 -c 'import sys,json; r=json.load(sys.stdin); t=r["result"]["content"][0]["text"]; print(json.loads(t).get("previewToken",""))')
  mcp qorm_apply_patch "{\"ops\":$ops,\"previewToken\":\"$tok\"}" >/dev/null
}

say() { printf '\n  >> %s\n' "$1"; sleep "${2:-2.5}"; }

say "AI: bumping the counter to 41 (watch the number + the toast)"
mcp qorm_set_state '{"path":"count","value":41}' >/dev/null

say "AI: setting the status line"
mcp qorm_set_state '{"path":"status","value":"edited live by AI"}' >/dev/null

say "AI: retitling the app (a reviewed apply_patch)"
edit_prop title text "COUNTER · live with AI"

say "AI: recoloring the title"
edit_prop title style "color:#34c759;font-size:14px;font-weight:800;text-align:center;"

say "AI: reading what the human just did (qorm_activity)"
mcp qorm_activity '{}' | python3 -c 'import sys,json; r=json.load(sys.stdin); print("     activity:", r["result"]["content"][0]["text"][:200])' 2>/dev/null || true

say "done — stop the recording. (Ctrl-C to quit; the server stops with it.)" 3
echo "  the app is still live at $URL"
wait $SRV
