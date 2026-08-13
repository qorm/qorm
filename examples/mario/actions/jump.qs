# jump.qs — the jump KEYDOWN handler. The jump KEYUP handler
# (stopJump.qs) just clears the hold flag — the physics step uses the
# flag to apply reduced gravity while the button is held, the classic
# "variable jump height" trick (a tap gives a short hop, a hold gives a
# full jump).

if (state.status == "playing" && state.mario.alive && !state.keys.jumpHold) {
  unstickMario()
}
if (state.status == "playing" && state.mario.onGround && state.mario.alive && !state.keys.jumpHold) {
  let spd = abs(state.mario.vx)
  let jv = 540
  if (spd > 140) { jv = 580 }
  if (spd > 240) { jv = 620 }
  state.mario.vy = 0 - jv
  state.mario.onGround = false
  state.keys.jump = true
  state.keys.jumpHold = true
  state.fxJump = state.fxJump + 1
  playSound("audio/jump.wav")
}
