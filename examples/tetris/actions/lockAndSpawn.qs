# ---- shared tetris core ---------------------------------------------------
# Identical in tick.qs, moveDown.qs, hardDrop.qs, lockAndSpawn.qs and
# restart.qs: qscript v1 has no cross-action calls (scripts cannot dispatch
# other actions), so every action that touches the board carries the same
# helpers. The board is a flat 10x20 row-major array; state.flat holds the 7
# pieces x 4 rotations x 4 cells as [x, y] pairs; state.view is the board
# plus the falling piece, ready for the scene's gridview.

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

fn refreshNext() {
  let v = fill(16, 0)
  let base = state.nextIdx * 16
  for i in range(4) {
    let c = at(state.flat, base + i)
    v[at(c, 1) * 4 + at(c, 0)] = state.nextIdx + 1
  }
  state.nextView = v
}

# spawn moves nextIdx into the falling piece and draws the following piece
# with a Park-Miller LCG kept in state.rng (x <- x*48271 mod 2^31-1: exact in
# float64, and the only randomness the game gets — scripts have no clock/IO).
# A piece that cannot enter the board tops the game out.
fn spawn() {
  state.piece.shapeIdx = state.nextIdx
  state.piece.rot = 0
  state.piece.x = 3
  state.piece.y = 0
  state.piece.color = state.nextIdx + 1
  state.rng = mod(state.rng * 48271, 2147483647)
  state.nextIdx = mod(state.rng, 7)
  refreshNext()
  if (!fits(state.piece.shapeIdx, 0, 3, 0)) { state.status = "over" }
}

# lock writes the falling piece into the board, clears full rows (100/300/
# 500/800 x level for 1/2/3/4 rows, a level every 10 lines) and spawns next.
fn lock() {
  let base = state.piece.shapeIdx * 16 + state.piece.rot * 4
  for i in range(4) {
    let c = at(state.flat, base + i)
    let y = state.piece.y + at(c, 1)
    if (y >= 0) { state.board[y * 10 + state.piece.x + at(c, 0)] = state.piece.color }
  }
  let kept = []
  let cleared = 0
  for row in range(20) {
    let full = true
    for x in range(10) {
      if (at(state.board, row * 10 + x) == 0) { full = false }
    }
    if (full) {
      cleared = cleared + 1
    } else {
      for x in range(10) { kept = concat(kept, at(state.board, row * 10 + x)) }
    }
  }
  if (cleared > 0) {
    state.board = concat(fill(cleared * 10, 0), kept)
    state.lines = state.lines + cleared
    state.score = state.score + at([0, 100, 300, 500, 800], cleared) * state.level
    state.level = floor(state.lines / 10) + 1
  }
  spawn()
  refreshView()
}
# ---- end shared core ------------------------------------------------------

# The locking half on its own, dispatchable by hosts and agents (MCP): merge
# the falling piece, clear and score lines, spawn the next one.
if (state.status == "playing") { lock() }
