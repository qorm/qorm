# moveRight.qs — the action body; the shared core lives in lib.qs.
# One cell right when the target cell is open (walls/bricks block), then
# resolve whatever Mario walks into.

if (state.status == "playing") {
  let nx = state.mario.x + 1
  if (!isSolid(nx, state.mario.y)) {
    state.mario.x = nx
    collect()
    touchGoomba()
    refreshView()
  }
}
