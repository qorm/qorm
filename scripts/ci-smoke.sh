#!/usr/bin/env bash
# ci-smoke.sh — CI smoke test: boot the counter example headless and drive it
# over HTTP.
#
#   Usage: ci-smoke.sh <qorm-binary>
#
# Starts `<qorm-binary> run examples/counter --no-open --port 18080`, waits for
# the page to come up, then POSTs an increment event (with the page-embedded
# X-Qorm-Token) and asserts via GET /dev/state that count went 0 -> 1.
#
# Written for the lowest common denominator of the three GitHub Actions
# runners (macOS/BSD tools, Linux/GNU tools, Windows git-bash): no GNU-only
# flags, plain curl/grep/sed only.
set -u

BIN="${1:-}"
if [ -z "$BIN" ]; then
  echo "usage: $0 <qorm-binary>" >&2
  exit 2
fi
# On Windows the build output is qorm.exe; accept either name.
if [ ! -x "$BIN" ] && [ -x "${BIN}.exe" ]; then
  BIN="${BIN}.exe"
fi
if [ ! -x "$BIN" ]; then
  echo "error: qorm binary not found/executable: $BIN" >&2
  exit 2
fi

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
APP_DIR="$ROOT/examples/counter"
PORT=18080
WORK="${RUNNER_TEMP:-${TMPDIR:-/tmp}}"
LOG="$WORK/qorm-smoke-server.log"
BODY="$WORK/qorm-smoke-index.html"

SERVER_PID=""
fail() {
  echo "FAIL: $*" >&2
  echo "---- server log (tail) ----" >&2
  tail -n 50 "$LOG" >&2 || true
  echo "---------------------------" >&2
  exit 1
}
cleanup() {
  if [ -n "$SERVER_PID" ]; then
    kill "$SERVER_PID" 2>/dev/null || true
    wait "$SERVER_PID" 2>/dev/null || true
  fi
}
trap cleanup EXIT

echo "==> starting: $BIN run $APP_DIR --no-open --port $PORT"
"$BIN" run "$APP_DIR" --no-open --port "$PORT" >"$LOG" 2>&1 &
SERVER_PID=$!

# The server falls back to an ephemeral port if 18080 is taken; read the port
# it actually bound from the startup banner once it appears.
BASE=""
i=0
while [ $i -lt 60 ]; do  # 60 * 0.5s = 30s
  if ! kill -0 "$SERVER_PID" 2>/dev/null; then
    fail "server process exited early"
  fi
  URL_LINE=$(grep 'running at http' "$LOG" 2>/dev/null | head -n 1 || true)
  if [ -n "$URL_LINE" ]; then
    BASE=$(echo "$URL_LINE" | sed 's/.*running at \(http:\/\/[0-9.]*:[0-9]*\)\/.*/\1/')
  fi
  if [ -n "$BASE" ]; then
    CODE=$(curl -s -o "$BODY" -w '%{http_code}' "$BASE/" || echo 000)
    if [ "$CODE" = "200" ]; then
      break
    fi
  fi
  sleep 0.5
  i=$((i + 1))
done
if [ $i -ge 60 ]; then
  fail "server did not serve GET / with 200 within 30s (base=$BASE)"
fi
echo "==> GET $BASE/ -> 200"

# The rendered page must actually contain the counter app.
if ! grep -qi 'counter' "$BODY"; then
  fail "index page does not mention 'counter'"
fi
echo "==> index page contains counter marker"

# /event is human-only: it requires the token the server embedded in the page
# as  var __tok='...'.  Extract it and dispatch handler 1 (the '+' button).
TOK=$(grep -o "__tok='[^']*'" "$BODY" | head -n 1 | sed "s/__tok='//;s/'//")
if [ -z "$TOK" ]; then
  fail "could not extract __tok event token from index page"
fi
echo "==> extracted event token"

EV_CODE=$(curl -s -o "$WORK/qorm-smoke-event.out" -w '%{http_code}' \
  -X POST "$BASE/event" \
  -H 'Content-Type: application/json' \
  -H "X-Qorm-Token: $TOK" \
  -d '{"h":1,"inputs":{}}' || echo 000)
if [ "$EV_CODE" != "200" ]; then
  fail "POST /event returned $EV_CODE (expected 200)"
fi
echo "==> POST /event (h=1, increment) -> 200"

STATE=$(curl -s "$BASE/dev/state" || true)
echo "==> GET /dev/state -> $STATE"
if ! echo "$STATE" | grep -q '"count":1[,}]'; then
  fail "expected \"count\":1 in /dev/state, got: $STATE"
fi

echo "PASS: counter served, event dispatched, state count=1"
exit 0
