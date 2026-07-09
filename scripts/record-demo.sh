#!/usr/bin/env bash
# Record the QORM human-AI collaboration demo GIF.
# 1. Launch the real native desktop app (App + DevTool windows visible to user)
# 2. User clicks buttons naturally on the native app (generating "you" logs)
# 3. AI edits via MCP (generating "agent" logs)
# 4. QORM's built-in shot captures App and DevTool separately, ImageMagick stitches them
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"; cd "$ROOT"
PORT=8866; U="http://127.0.0.1:$PORT"; APP="${1:-examples/counter}"

CGO_ENABLED=1 go build -tags desktop -o /tmp/qorm-shot ./cmd/qorm
rm -rf /tmp/qframes; mkdir -p /tmp/qframes

# Launch native desktop app — user sees both App window and DevTool window
/tmp/qorm-shot run "$APP" --port "$PORT" --app >/tmp/qrec.log 2>&1 & SRV=$!
trap 'kill $SRV 2>/dev/null || true' EXIT
sleep 4.0

mcp(){ curl -s -X POST "$U/mcp" -H 'Content-Type: application/json' -d "{\"jsonrpc\":\"2.0\",\"id\":1,\"method\":\"tools/call\",\"params\":{\"name\":\"$1\",\"arguments\":$2}}"; }
editprop(){ ops="[{\"op\":\"setProp\",\"target\":\"$1\",\"key\":\"$2\",\"value\":\"$3\"}]"; tok=$(mcp qorm_preview_patch "{\"ops\":$ops}"|python3 -c 'import sys,json;print(json.loads(json.load(sys.stdin)["result"]["content"][0]["text"])["previewToken"])'); mcp qorm_apply_patch "{\"ops\":$ops,\"previewToken\":\"$tok\"}">/dev/null; }

FN=0
# Capture one frame: shot the App page and DevTool page separately via qorm shot,
# then stitch them side-by-side with ImageMagick
shot(){
  sleep 1.0
  local app_png="/tmp/qframes/app_$(printf %03d $FN).png"
  local log_png="/tmp/qframes/log_$(printf %03d $FN).png"
  local out="/tmp/qframes/f$(printf %03d $FN).png"
  # Use QORM's built-in WebKit shot to capture each page (same-origin, no iframe issues)
  /tmp/qorm-shot shot --url "$U/" -o "$app_png" --width 360 --height 620 >/dev/null 2>&1
  /tmp/qorm-shot shot --url "$U/logwindow" -o "$log_png" --width 420 --height 620 >/dev/null 2>&1
  # Stitch side-by-side with a dark gap
  magick "$app_png" "$log_png" +append -bordercolor '#0b0c0f' -border 10x10 "$out"
  FN=$((FN+1))
  # Duplicate frame for pacing
  cp "$out" "/tmp/qframes/f$(printf %03d $FN).png"
  FN=$((FN+1))
}

echo "================================================================"
echo " QORM 人机协作 Demo 录屏"
echo " 请在弹出的原生 App 窗口里点击几次按钮（加/减），"
echo " 右侧 DevTool 窗口会实时显示 'you' 的操作日志。"
echo " 完成后回到终端按 [Enter]，AI 将接管编辑并录入所有帧。"
echo "================================================================"

# Capture initial state (before any interaction)
shot

# Human clicks. Interactive by default (click the native window). With
# QORM_DEMO_AUTO=1 the script plays the human itself over the same
# token-authenticated /event channel a browser click uses — it scrapes the
# page token exactly like the browser would. That is only legitimate here
# because the recorder IS the human operator on their own machine.
if [ "${QORM_DEMO_AUTO:-}" = "1" ]; then
  TOK=$(curl -s "$U/" | grep -o "var __tok='[^']*'" | cut -d"'" -f2)
  click(){ curl -s -X POST "$U/event" -H "X-Qorm-Token: $TOK" -H 'Content-Type: application/json' -d "{\"h\":$1,\"inputs\":{}}" >/dev/null; }
  click 1; sleep 0.3; click 1; sleep 0.3; click 0; sleep 0.3; click 1
else
  read -r -p "按下回车键继续..."
fi

# Capture after user interaction (DevTool now shows user's click logs)
shot
shot

# AI takes over — each MCP call generates "agent" logs in DevTool
mcp qorm_set_state '{"path":"count","value":7}'>/dev/null;   shot
mcp qorm_set_state '{"path":"status","value":"AI is editing…"}'>/dev/null; shot
mcp qorm_set_state '{"path":"count","value":42}'>/dev/null;  shot
editprop title text "COUNTER · live with AI";                shot
editprop title style "color:#34c759;font-size:14px;font-weight:800;text-align:center;letter-spacing:2px;"; shot
shot

# Save final frame as the static logwindow screenshot
cp "/tmp/qframes/f$(printf %03d $((FN-1))).png" assets/screenshots/logwindow.png
echo "wrote assets/screenshots/logwindow.png"

# Compile GIF
magick -delay 65 -loop 0 /tmp/qframes/f*.png -resize 580 -layers optimize assets/qorm-demo.gif
echo "wrote assets/qorm-demo.gif"
