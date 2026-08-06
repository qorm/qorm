# lockAndSpawn.qs — the action body; the shared core (fits/refreshView/refreshNext/spawn/lock) lives in lib.qs.

# The locking half on its own, dispatchable by hosts and agents (MCP): merge
# the falling piece, clear and score lines, spawn the next one.
if (state.status == "playing") { lock() }
