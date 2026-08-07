# lib.qs — the shared Tetris core: the reserved library file the loader
# collects as type:"scriptlib" and the runtime prepends to EVERY script
# action at dispatch. Keep this file to fn definitions and comments — it is
# spliced ahead of each action's body, so any top-level statement here would
# run before every action.
#
# The board is a flat 10x20 row-major array; state.view is the board plus
# the falling piece, ready for the scene's gridview. SHAPES lives in
# globalState.initial (see qorm.json) — the engine initialises it before
# the first action runs, so fits/refreshView/refreshNext/lock can just read
# `state.SHAPES[si][rot][y*4+x]`. No `let SHAPES = ...` here, no init step
# in restart.

fn cellOf(si, r, x, y) {
  let s = at(at(state.SHAPES, si), r)
  return charAt(s, y * 4 + x) != "."
}

fn fits(si, r, px, py) {
  for ry in range(4) {
    for rx in range(4) {
      if (cellOf(si, r, rx, ry) == 1) {
        let x = px + rx
        let y = py + ry
        if (x < 0 || x >= 10 || y >= 20) { return false }
        if (y >= 0 && at(state.board, y * 10 + x) > 0) { return false }
      }
    }
  }
  return true
}

fn refreshView() {
  let v = concat(state.board)
  for ry in range(4) {
    for rx in range(4) {
      if (cellOf(state.piece.shapeIdx, state.piece.rot, rx, ry) == 1) {
        let y = state.piece.y + ry
        if (y >= 0) { v[y * 10 + state.piece.x + rx] = state.piece.color }
      }
    }
  }
  state.view = v
}

fn refreshNext() {
  let v = fill(16, 0)
  for ry in range(4) {
    for rx in range(4) {
      if (cellOf(state.nextIdx, 0, rx, ry) == 1) { v[ry * 4 + rx] = state.nextIdx + 1 }
    }
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
  for ry in range(4) {
    for rx in range(4) {
      if (cellOf(state.piece.shapeIdx, state.piece.rot, rx, ry) == 1) {
        let y = state.piece.y + ry
        if (y >= 0) { state.board[y * 10 + state.piece.x + rx] = state.piece.color }
      }
    }
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

# tickStep is one gravity step: slide down when the cells below are free,
# otherwise lock the piece and spawn the next. tick (the timer) and moveDown
# (soft drop) both drive it; hardDrop drives its two halves at full travel.
fn tickStep() {
  if (fits(state.piece.shapeIdx, state.piece.rot, state.piece.x, state.piece.y + 1)) {
    state.piece.y = state.piece.y + 1
    refreshView()
  } else {
    lock()
  }
}
# ---- end shared core ------------------------------------------------------
