# moveLeft.qs — keydown handler for the left arrow / A key. We just flip a
# direction flag; the physics step in lib.qs reads it next tick and
# accelerates mario. Keyup (the scene's `keyReleases` map) calls
# `stopMoveLeft` to clear the flag — without that, mario would slide
# forever after the first keypress.

if (state.status == "playing") {
  state.keys.left = true
  state.mario.dir = -1
}
