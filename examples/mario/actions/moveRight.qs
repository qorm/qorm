# moveRight.qs — keydown handler for the right arrow / D key. Mirrors
# moveLeft.qs; the physics step is the single source of truth for velocity.

if (state.status == "playing") {
  state.keys.right = true
  state.mario.dir = 1
}
