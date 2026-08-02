# fits + refreshView, shared verbatim with the other tetris actions (qscript
# v1 has no cross-action calls, so each action carries the helpers it uses).

fn fits(si, r, px, py) {
  let base = si * 16 + r * 4
  for i in range(4) {
    let c = at(state.flat, base + i)
    let x = px + at(c, 0)
    let y = py + at(c, 1)
    if (x < 0 || x >= 10 || y >= 20) { return false }
    if (y >= 0 && at(state.board, y * 10 + x) > 0) { return false }
  }
  return true
}

fn refreshView() {
  let v = concat(state.board)
  let base = state.piece.shapeIdx * 16 + state.piece.rot * 4
  for i in range(4) {
    let c = at(state.flat, base + i)
    let y = state.piece.y + at(c, 1)
    if (y >= 0) { v[y * 10 + state.piece.x + at(c, 0)] = state.piece.color }
  }
  state.view = v
}

if (state.status == "playing" && fits(state.piece.shapeIdx, mod(state.piece.rot + 3, 4), state.piece.x, state.piece.y)) {
  state.piece.rot = mod(state.piece.rot + 3, 4)
  refreshView()
}
