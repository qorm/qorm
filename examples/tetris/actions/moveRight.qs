# moveRight.qs — the action body; the shared core (fits/refreshView/refreshNext/spawn/lock) lives in lib.qs.

# Slide one column right when the target cells are free, then redraw the view.
if (state.status == "playing" && fits(state.piece.shapeIdx, state.piece.rot, state.piece.x + 1, state.piece.y)) {
  state.piece.x = state.piece.x + 1
  refreshView()
}
