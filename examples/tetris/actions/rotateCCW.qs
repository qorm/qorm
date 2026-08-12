# rotateCCW.qs — the action body; the shared core (fits/refreshView/refreshNext/spawn/lock) lives in lib.qs.

# Rotate counter-clockwise when the rotated cells are free, then redraw the view.
if (state.status == "playing" && fits(state.piece.shapeIdx, mod(state.piece.rot + 3, 4), state.piece.x, state.piece.y)) {
  state.piece.rot = mod(state.piece.rot + 3, 4)
  refreshView()
  playSound("audio/rotate.wav")
}
