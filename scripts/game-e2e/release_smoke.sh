#!/usr/bin/env bash
set -euo pipefail

# Minimal post-deploy smoke for qorm.com/games.
# Fast path: mario scroll + raiden fire button still work.
# Optional deep pass: FULL=1 adds the 4-game stability rotation.
#
#   ./scripts/game-e2e/release_smoke.sh
#   FULL=1 ./scripts/game-e2e/release_smoke.sh

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$ROOT"

need_py() {
  python3 - <<'PY'
import importlib, sys
mods = ["playwright.sync_api", "PIL"]
missing = []
for name in mods:
    try:
        importlib.import_module(name)
    except Exception:
        missing.append(name)
if missing:
    print("missing python modules: " + ", ".join(missing), file=sys.stderr)
    sys.exit(1)
PY
}

echo "==> checking python deps"
need_py

echo "==> mario smoke"
python3 "$ROOT/scripts/game-e2e/verify_mario_walk.py"

echo "==> raiden smoke"
python3 "$ROOT/scripts/game-e2e/verify_raiden_keys.py"

if [ "${FULL:-0}" = "1" ]; then
  echo "==> 4-game stability"
  python3 "$ROOT/scripts/game-e2e/stability_4game_cycles.py"
fi

echo "OK: games smoke passed"
