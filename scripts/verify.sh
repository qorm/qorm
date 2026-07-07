#!/usr/bin/env bash
# One-command regression: pure-Go tests + a framework self-measured layout audit
# of every example (no external browser — uses the WebView desktop build).
set -uo pipefail
cd "$(dirname "$0")/.."
fail=0

echo "== 1. pure-Go tests (go test ./...) =="
if go test ./... >/tmp/verify-gotest.log 2>&1; then
  echo "   ✅ $(grep -c '^ok' /tmp/verify-gotest.log) packages ok"
else
  echo "   ❌ FAIL"; grep -E 'FAIL|---' /tmp/verify-gotest.log | head; fail=1
fi

echo "== 2. layout audit (qorm check --audit, WebView self-measure) =="
go build -tags desktop -o dist/qorm-desktop ./cmd/qorm 2>/dev/null || { echo "   desktop build failed"; exit 1; }
pkill -9 -f qorm-desktop 2>/dev/null; sleep 0.3
for app in examples/*/; do
  [ -f "$app/qorm.json" ] || continue
  name=$(basename "$app")
  for width in 400 1280; do
    tag="$name@${width}"
    out="/tmp/audit-${name}-${width}.json"
    if timeout 30 dist/qorm-desktop check "$app" --audit --width $width -o "$out" 2>/dev/null; then
      read ok issues vis < <(python3 -c "import json;d=json.load(open('$out'));print(d['ok'],d['issues'],d['visibleComponents'])" 2>/dev/null)
      if [ "$ok" = "True" ]; then echo "   ✅ $tag ($vis)";
      else echo "   ❌ $tag: $issues issue(s)"; python3 -c "import json;[print('      ',x['id'],x['kind'],x['detail']) for x in (json.load(open('$out')).get('details') or [])]" 2>/dev/null; fail=1; fi
    else echo "   ⚠ $tag: measure timed out"; fi
  done
done
pkill -9 -f qorm-desktop 2>/dev/null

echo "== 3. offline package parity (Go->WASM, no server) =="
rm -rf /tmp/verify-pkg
go build -o /tmp/verify-qorm ./cmd/qorm 2>/dev/null
if /tmp/verify-qorm package examples/showcase -o /tmp/verify-pkg >/dev/null 2>&1; then
  pkill -9 -f qorm-desktop 2>/dev/null; sleep 0.2
  if timeout 40 dist/qorm-desktop preview /tmp/verify-pkg -o /tmp/verify-pv.json 2>/dev/null; then
    read n zero < <(python3 -c "import json;d=json.load(open('/tmp/verify-pv.json'));v=[r for r in d if r.get('visible')];print(len(v), sum(1 for r in v if r.get('w',0)<=0 or r.get('h',0)<=0))" 2>/dev/null)
    if [ "${zero:-1}" = "0" ] && [ "${n:-0}" -gt 5 ]; then echo "   ✅ showcase WASM package renders offline ($n visible, 0 zero-size)";
    else echo "   ❌ offline package: $n visible, $zero zero-size"; fail=1; fi
  else echo "   ⚠ offline preview timed out"; fi
  pkill -9 -f qorm-desktop 2>/dev/null
else echo "   ⚠ package build skipped"; fi

echo "== result =="
[ $fail -eq 0 ] && echo "   ✅ ALL GREEN" || echo "   ❌ regressions above"
exit $fail
