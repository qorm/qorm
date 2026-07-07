#!/usr/bin/env bash
# attack.sh — integration test. Boots the WAF gateway in front of a mock
# backend, fires benign + malicious requests, and asserts the WAF verdicts.
# Requires openresty on PATH; skips cleanly if it's absent.
set -u
cd "$(dirname "$0")/.."   # deploy/waf
WAF_DIR="$PWD"

if ! command -v openresty >/dev/null 2>&1; then
  echo "SKIP: openresty not installed (brew install openresty)"; exit 0
fi

PORT=8899
BACK=8080
tmp="$(mktemp -d)"
mkdir -p "$tmp/logs"

# mock backend: always 200 "ok"
python3 -m http.server "$BACK" --bind 127.0.0.1 >/dev/null 2>&1 &
BACK_PID=$!

# an override server on $PORT proxying to the mock backend, WAF in detect->block
cat > "$tmp/test.conf" <<EOF
server {
    listen $PORT;
    access_by_lua_block { require("waf").access() }
    log_by_lua_block    { require("waf").finish() }
    location / { proxy_pass http://127.0.0.1:$BACK/; proxy_set_header X-Real-IP \$remote_addr; }
}
EOF
# run openresty with our nginx.conf but pointing conf.d at just the test server
cp "$WAF_DIR/nginx.conf" "$tmp/nginx.conf"
sed -i.bak "s#include conf.d/\*.conf;#include $tmp/test.conf;#" "$tmp/nginx.conf"

openresty -p "$WAF_DIR" -c "$tmp/nginx.conf" -g "pid $tmp/nginx.pid; error_log $tmp/logs/error.log notice;" &
sleep 1

fail=0
expect() { # name url expected_status
  code=$(curl -s -o /dev/null -w '%{http_code}' "http://127.0.0.1:$PORT$2")
  if [ "$code" = "$3" ]; then echo "  ok   $1 -> $code"; else echo "  FAIL $1 -> $code (want $3)"; fail=1; fi
}
expect "clean request"     "/index.html"                          200
expect "sqli union"        "/?id=1%20union%20select%20pass"       403
expect "xss script"        "/?q=%3Cscript%3Ealert(1)%3C/script%3E" 403
expect "path traversal"    "/../../etc/passwd"                    403
expect "dotfile probe"     "/.env"                                403
expect "admin probe"       "/wp-admin/"                           403
code=$(curl -s -o /dev/null -w '%{http_code}' -A "sqlmap/1.5" "http://127.0.0.1:$PORT/"); \
  { [ "$code" = 403 ] && echo "  ok   bad-ua sqlmap -> 403"; } || { echo "  FAIL bad-ua -> $code"; fail=1; }

# teardown
[ -f "$tmp/nginx.pid" ] && kill "$(cat "$tmp/nginx.pid")" 2>/dev/null
kill "$BACK_PID" 2>/dev/null
rm -rf "$tmp"
[ "$fail" = 0 ] && echo "attack.sh: all passed" || echo "attack.sh: FAILURES"
exit $fail
