# restart shares expOf/refreshView/spawn with the slide actions: qscript v1
# has no cross-action calls (scripts cannot dispatch other actions), so each
# action carries the helpers it uses. See slideLeft.qs for what they do.

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

# Back to a fresh game: empty board, zeroed score, the LCG reseeded, then the
# two opening tiles. The scene's onEnter points here too, so first load gets
# the same opening. state.best survives: it is the across-games high score.
state.board = fill(16, 0)
state.score = 0
state.status = "playing"
state.rng = 424242
spawn()
spawn()
refreshView()
