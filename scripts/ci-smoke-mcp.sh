#!/usr/bin/env bash
# ci-smoke-mcp.sh — MCP policy smoke: inspect works, dispatch denied under
# preview-only manifest policy, preview_patch still allowed.
#
# Usage: ci-smoke-mcp.sh <qorm-binary>
set -u

BIN="${1:-}"
if [ -z "$BIN" ]; then
  echo "usage: $0 <qorm-binary>" >&2
  exit 2
fi
if [ ! -x "$BIN" ] && [ -x "${BIN}.exe" ]; then
  BIN="${BIN}.exe"
fi
if [ ! -x "$BIN" ]; then
  echo "error: qorm binary not found/executable: $BIN" >&2
  exit 2
fi

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
WORK="${RUNNER_TEMP:-${TMPDIR:-/tmp}}"
APP_DIR="$WORK/qorm-smoke-mcp-app"
LOG="$WORK/qorm-smoke-mcp.log"
PORT=18081

rm -rf "$APP_DIR"
mkdir -p "$APP_DIR/scenes" "$APP_DIR/actions"
cp "$ROOT/examples/counter/qorm.json" "$APP_DIR/"
cp "$ROOT/examples/counter/scenes/main.json" "$APP_DIR/scenes/"
cp "$ROOT/examples/counter/actions/"*.json "$APP_DIR/actions/"

# Inject preview-only policy (legacy counter stays full-access in repo).
python3 - <<'PY' "$APP_DIR/qorm.json"
import json, sys
path = sys.argv[1]
with open(path) as f:
    doc = json.load(f)
doc["agent"] = {"policy": {"level": "preview-only"}}
with open(path, "w") as f:
    json.dump(doc, f, indent=2)
    f.write("\n")
PY

SERVER_PID=""
fail() { echo "FAIL: $*" >&2; tail -n 40 "$LOG" >&2 || true; exit 1; }
cleanup() {
  if [ -n "$SERVER_PID" ]; then
    kill "$SERVER_PID" 2>/dev/null || true
    wait "$SERVER_PID" 2>/dev/null || true
  fi
}
trap cleanup EXIT

echo "==> starting with preview-only policy on port $PORT"
"$BIN" run "$APP_DIR" --no-open --port "$PORT" >"$LOG" 2>&1 &
SERVER_PID=$!

BASE=""
i=0
while [ $i -lt 60 ]; do
  if ! kill -0 "$SERVER_PID" 2>/dev/null; then
    fail "server exited early"
  fi
  URL_LINE=$(grep 'running at http' "$LOG" 2>/dev/null | head -n 1 || true)
  if [ -n "$URL_LINE" ]; then
    BASE=$(echo "$URL_LINE" | sed 's/.*running at \(http:\/\/[0-9.]*:[0-9]*\)\/.*/\1/')
  fi
  if [ -n "$BASE" ]; then
    CODE=$(curl -s -o /dev/null -w '%{http_code}' "$BASE/" || echo 000)
    if [ "$CODE" = "200" ]; then
      break
    fi
  fi
  sleep 0.5
  i=$((i + 1))
done
[ -n "$BASE" ] || fail "server did not start"

mcp() {
  curl -s -X POST "$BASE/mcp" -H 'Content-Type: application/json' -d "$1"
}

INSPECT=$(mcp '{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"qorm_inspect","arguments":{}}}')
echo "$INSPECT" | grep -q 'agentPolicy' || fail "inspect missing agentPolicy: $INSPECT"
echo "$INSPECT" | grep -q 'preview-only' || fail "inspect missing preview-only level: $INSPECT"

DISPATCH=$(mcp '{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"qorm_dispatch","arguments":{"action":"increment","args":{"count":0}}}}')
echo "$DISPATCH" | grep -q 'agent policy denied' || fail "dispatch should be policy-denied: $DISPATCH"

PREVIEW=$(mcp '{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"qorm_preview_patch","arguments":{"ops":[{"op":"setProp","target":"title","key":"text","value":"SMOKE"}]}}}')
echo "$PREVIEW" | grep -q '"isError":false' || echo "$PREVIEW" | grep -qv '"error"' || fail "preview_patch should succeed: $PREVIEW"

echo "PASS: MCP manifest policy smoke"
exit 0
