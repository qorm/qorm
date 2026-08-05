# ---- shared 2048 core -------------------------------------------------------
# Identical in slideLeft.qs, slideRight.qs, slideUp.qs and slideDown.qs:
# qscript v1 has no cross-action calls (scripts cannot dispatch other
# actions), so every slide action carries the same helpers. The board is a
# flat 4x4 row-major array of tile values (0 = empty); state.view holds each
# cell's exponent (0 = empty, else log2 of the value) so the scene can index
# state.colors / state.labels without a pow() builtin.

# expOf computes log2(v) by repeated halving, clamped to the last
# colors/labels slot so 4096+ tiles reuse the "super" entry.
fn expOf(v) {
  let e = 0
  let n = v
  while (n > 1) {
    n = n / 2
    e = e + 1
  }
  let cap = len(state.labels) - 1
  if (e > cap) { e = cap }
  return e
}

fn refreshView() {
  let v = fill(16, 0)
  for i in range(16) { v[i] = expOf(at(state.board, i)) }
  state.view = v
}

# spawn drops one tile (90% a 2, 10% a 4) on a random empty cell, driven by a
# Park-Miller LCG kept in state.rng (x <- x*48271 mod 2^31-1: exact in
# float64, and the only randomness the game gets — scripts have no clock/IO).
fn spawn() {
  let empty = []
  for i in range(16) {
    if (at(state.board, i) == 0) { empty = concat(empty, i) }
  }
  if (len(empty) > 0) {
    state.rng = mod(state.rng * 48271, 2147483647)
    let cell = at(empty, mod(state.rng, len(empty)))
    state.rng = mod(state.rng * 48271, 2147483647)
    let v = 2
    if (mod(state.rng, 10) == 0) { v = 4 }
    state.board[cell] = v
  }
}

# cellAt maps (direction, line, slot) to a board index. Slots are ordered
# from the edge the tiles slide TOWARD, so every direction reduces to the
# same front-merge: dir 0 left (rows), 1 right (rows reversed), 2 up
# (columns), 3 down (columns reversed).
fn cellAt(dir, k, j) {
  if (dir == 0) { return k * 4 + j }
  if (dir == 1) { return k * 4 + (3 - j) }
  if (dir == 2) { return j * 4 + k }
  return (3 - j) * 4 + k
}

# slideLine compacts one 4-tile sequence toward its front: zeros drop out,
# then equal neighbours merge once each, scanning from the front — so
# [2,2,2,2] becomes [4,4,0,0], never [8,0,0,0]. Returns [padded4, gained]
# where gained is the sum of the merged values (the score delta).
fn slideLine(line) {
  let vals = []
  for j in range(4) {
    let v = at(line, j)
    if (v != 0) { vals = concat(vals, v) }
  }
  let out = []
  let gained = 0
  let j = 0
  while (j < len(vals)) {
    if (j + 1 < len(vals) && at(vals, j) == at(vals, j + 1)) {
      let m = at(vals, j) * 2
      out = concat(out, m)
      gained = gained + m
      j = j + 2
    } else {
      out = concat(out, at(vals, j))
      j = j + 1
    }
  }
  while (len(out) < 4) { out = concat(out, 0) }
  return [out, gained]
}

# canMove reports whether any slide could still change the board: an empty
# cell, or an equal horizontal/vertical neighbour pair.
fn canMove() {
  for i in range(16) {
    if (at(state.board, i) == 0) { return true }
  }
  for r in range(4) {
    for c in range(3) {
      if (at(state.board, r * 4 + c) == at(state.board, r * 4 + c + 1)) { return true }
      if (at(state.board, c * 4 + r) == at(state.board, (c + 1) * 4 + r)) { return true }
    }
  }
  return false
}

# settle decides the game state after every slide attempt: no legal move ends
# the game (even when the attempted slide changed nothing); otherwise a fresh
# 2048 wins but stays playable.
fn settle() {
  if (!canMove()) {
    state.status = "over"
  } else {
    if (state.status == "playing") {
      for i in range(16) {
        if (at(state.board, i) == 2048) { state.status = "won" }
      }
    }
  }
}

# slide moves the whole board along dir. Only a board that actually changed
# scores the merges and spawns a new tile; the win/lose check runs on every
# attempt so a stuck full board ends even on a no-op key press.
fn slide(dir) {
  let nb = concat(state.board)
  let gained = 0
  for k in range(4) {
    let res = slideLine([at(state.board, cellAt(dir, k, 0)), at(state.board, cellAt(dir, k, 1)), at(state.board, cellAt(dir, k, 2)), at(state.board, cellAt(dir, k, 3))])
    let out = at(res, 0)
    gained = gained + at(res, 1)
    for j in range(4) { nb[cellAt(dir, k, j)] = at(out, j) }
  }
  let changed = false
  for i in range(16) {
    if (at(nb, i) != at(state.board, i)) { changed = true }
  }
  if (changed) {
    state.board = nb
    state.score = state.score + gained
    if (state.score > state.best) { state.best = state.score }
    spawn()
    refreshView()
  }
  settle()
}
# ---- end shared core --------------------------------------------------------

# Arrow down: merge every column toward row 3. A won game keeps sliding.
if (state.status == "playing" || state.status == "won") { slide(3) }
