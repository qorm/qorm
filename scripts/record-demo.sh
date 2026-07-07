#!/usr/bin/env bash
# Record the QORM human-AI collaboration demo GIF — using QORM itself (no browser
# automation). It drives edits over MCP and captures each state with `qorm shot`
# (an offscreen WebKit render), injecting the "AI edited" toast, then assembles a
# GIF with ImageMagick. macOS + `-tags desktop` (WebKit) required.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"; cd "$ROOT"
PORT=8866; U="http://127.0.0.1:$PORT"; APP="${1:-examples/counter}"
go build -o /tmp/qc ./cmd/qorm
CGO_ENABLED=1 go build -tags desktop -o /tmp/qorm-shot ./cmd/qorm
rm -rf /tmp/qframes; mkdir -p /tmp/qframes
/tmp/qc run "$APP" --port "$PORT" >/tmp/qrec.log 2>&1 & SRV=$!
trap 'kill $SRV 2>/dev/null || true' EXIT
sleep 2
mcp(){ curl -s -X POST "$U/mcp" -H 'Content-Type: application/json' -d "{\"jsonrpc\":\"2.0\",\"id\":1,\"method\":\"tools/call\",\"params\":{\"name\":\"$1\",\"arguments\":$2}}"; }
editprop(){ ops="[{\"op\":\"setProp\",\"target\":\"$1\",\"key\":\"$2\",\"value\":\"$3\"}]"; tok=$(mcp qorm_preview_patch "{\"ops\":$ops}"|python3 -c 'import sys,json;print(json.loads(json.load(sys.stdin)["result"]["content"][0]["text"])["previewToken"])'); mcp qorm_apply_patch "{\"ops\":$ops,\"previewToken\":\"$tok\"}">/dev/null; }
FN=0
shot(){ curl -s "$U/" >/tmp/qrec.html
  if [ -n "${1:-}" ]; then T='<div id="qorm-presence" class="show"><svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M13 2L4 14h7l-1 8 9-12h-7z"/></svg><span>AI edited &middot; '"$1"'</span></div>'
    python3 -c "h=open('/tmp/qrec.html').read();open('/tmp/qrec.html','w').write(h.replace('</body>','''$T</body>'''))"; fi
  for k in 1 2; do /tmp/qorm-shot shot --html /tmp/qrec.html -o "/tmp/qframes/f$(printf %03d $FN).png" --width 440 --height 620 >/dev/null 2>&1; FN=$((FN+1)); done; }
shot ""; shot ""
mcp qorm_set_state '{"path":"count","value":7}'>/dev/null;   shot "set_state count = 7"
mcp qorm_set_state '{"path":"status","value":"AI is editing…"}'>/dev/null; shot "set_state status"
mcp qorm_set_state '{"path":"count","value":42}'>/dev/null;  shot "set_state count = 42"
editprop title text "COUNTER · live with AI";                shot "apply_patch (UI edit)"
editprop title style "color:#34c759;font-size:14px;font-weight:800;text-align:center;letter-spacing:2px;"; shot "apply_patch (recolor)"
shot ""; shot ""
magick -delay 65 -loop 0 /tmp/qframes/f*.png -resize 380 -layers optimize assets/qorm-demo.gif
echo "wrote assets/qorm-demo.gif"
