# jump.qs — the jump KEYDOWN handler. The jump KEYUP handler
# (stopJump.qs) just clears the hold flag — the physics step uses the
# flag to apply reduced gravity while the button is held, the classic
# "variable jump height" trick (a tap gives a short hop, a hold gives a
# full jump).

if (state.status == "playing" && state.mario.onGround && state.mario.alive) {
  state.mario.vy = -480
  state.mario.onGround = false
  state.keys.jump = true
  state.keys.jumpHold = true
  state.fxJump = state.fxJump + 1
  playSound("audio/jump.wav")
}
