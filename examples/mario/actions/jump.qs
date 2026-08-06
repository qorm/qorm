# jump.qs — the action body; the shared core lives in lib.qs.
# Mario leaves the ground with a rise budget of 3 cells; gravity() spends it
# one cell per tick before he starts to fall. Only a grounded Mario can jump.

if (state.status == "playing" && state.mario.onGround) {
  state.mario.rise = 3
  state.mario.onGround = false
}
