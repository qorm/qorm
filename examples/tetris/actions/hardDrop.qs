# hardDrop.qs — the action body; the shared core (fits/refreshView/refreshNext/spawn/lock) lives in lib.qs.

# A hard drop is tickStep's two halves at full travel: slide to the floor
# (at most 19 rows from any spawn), then lock and spawn.
if (state.status == "playing") {
  for i in range(20) {
    if (fits(state.piece.shapeIdx, state.piece.rot, state.piece.x, state.piece.y + 1)) {
      state.piece.y = state.piece.y + 1
    }
  }
  lock()
}
