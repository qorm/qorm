# tick.qs — the 60-fps physics frame. The scene's timer drives it while
# status is playing. The full physics step (gravity, horizontal, collision,
# enemy AI, pickup) lives in lib.qs as `physicsStep`; this file is the thin
# shim the timer dispatches.

physicsStep()
