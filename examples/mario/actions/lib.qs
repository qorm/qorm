# lib.qs — the shared Mario core: the reserved library file the loader
# collects as type:"scriptlib" and the runtime prepends to EVERY script
# action at dispatch. Keep this file to fn definitions and comments — it is
# spliced ahead of each action's body, so any top-level statement here would
# run before every action.
#
# The level is a 16x12 grid kept as 16-char strings in state.rows (row 0 is
# the TOP). Tile glyphs: '.' air, '1' ground, '2' brick, '3' coin, '4' flag.
# Ground/brick are SOLID; coin and flag are pickups Mario walks through.
# Entities (mario, the goomba) are objects overlaid on the tiles at render.

# tileAt returns the level glyph at (x, y); out of bounds reads as air.
fn tileAt(x, y) {
  if (x < 0 || x > 15 || y < 0 || y > 11) { return "." }
  return charAt(at(state.rows, y), x)
}

# isSolid reports whether (x, y) blocks movement: bricks and ground do, and
# the two side walls (x out of range) keep Mario on screen. Above the level
# is open (jumps can crest the top row); below it is open too — that is how
# a pit kills.
fn isSolid(x, y) {
  if (x < 0 || x > 15) { return true }
  if (y < 0 || y > 11) { return false }
  let t = tileAt(x, y)
  return t == "1" || t == "2"
}

# setTile rewrites one cell of the level (used to pluck a coin). Strings are
# immutable, so the row is rebuilt with two substring() slices around the
# replacement glyph.
fn setTile(x, y, ch) {
  let row = at(state.rows, y)
  state.rows[y] = substring(row, 0, x) + ch + substring(row, x + 1)
}

# goombaAt reports whether the live goomba occupies (x, y).
fn goombaAt(x, y) {
  return state.goomba.alive && state.goomba.x == x && state.goomba.y == y
}

# win / lose settle the run; the scene's overlays bind to state.status.
fn win() { state.status = "won" }
fn lose() { state.status = "dead" }

# collect handles whatever Mario stands in: a coin is picked (tile cleared,
# score +100), the flag ends the level. Called after every Mario move.
fn collect() {
  let t = tileAt(state.mario.x, state.mario.y)
  if (t == "3") {
    setTile(state.mario.x, state.mario.y, ".")
    state.coins = state.coins + 1
    state.score = state.score + 100
  }
  if (t == "4") {
    state.score = state.score + 500
    win()
  }
}

# touchGoomba resolves Mario sharing the goomba's cell from the SIDE (a walk
# into it): that is always fatal. Stomps are resolved in gravity() instead,
# because they depend on Mario arriving from above.
fn touchGoomba() {
  if (state.goomba.alive && state.goomba.x == state.mario.x && state.goomba.y == state.mario.y) {
    lose()
  }
}

# gravity advances Mario one vertical step. While a jump's rise budget lasts
# Mario moves up (a ceiling spends the budget); otherwise he falls one cell
# unless ground — or the live goomba — is directly below. Landing on the
# goomba from above is a STOMP: it dies for 200 and Mario bounces a cell.
# Falling out the bottom of the level is fatal.
fn gravity() {
  if (state.mario.rise > 0) {
    if (!isSolid(state.mario.x, state.mario.y - 1)) {
      state.mario.y = state.mario.y - 1
    }
    state.mario.rise = state.mario.rise - 1
    state.mario.onGround = false
  } else {
    if (isSolid(state.mario.x, state.mario.y + 1)) {
      state.mario.onGround = true
    } else {
      if (goombaAt(state.mario.x, state.mario.y + 1)) {
        state.goomba.alive = false
        state.score = state.score + 200
        state.mario.rise = 1
      } else {
        state.mario.y = state.mario.y + 1
        state.mario.onGround = false
      }
    }
  }
  if (state.mario.y > 11) { lose() }
}

# stepGoomba walks the goomba one cell in its direction; it turns around at a
# wall, the level edge, or a ledge (no ground under the next cell), so it
# paces its platform and never walks off.
fn stepGoomba() {
  if (!state.goomba.alive) { return }
  let nx = state.goomba.x + state.goomba.dir
  if (isSolid(nx, state.goomba.y) || !isSolid(nx, state.goomba.y + 1)) {
    state.goomba.dir = 0 - state.goomba.dir
  } else {
    state.goomba.x = nx
  }
}

# refreshView flattens the level into state.view (row-major color indexes)
# and overlays the goomba and Mario on top — the gridview renders that array.
fn refreshView() {
  let v = []
  for y in range(12) {
    for x in range(16) {
      let t = tileAt(x, y)
      let c = 0
      if (t == "1") { c = 1 }
      if (t == "2") { c = 2 }
      if (t == "3") { c = 3 }
      if (t == "4") { c = 4 }
      v = push(v, c)
    }
  }
  if (state.goomba.alive) { v[state.goomba.y * 16 + state.goomba.x] = 6 }
  if (state.mario.y >= 0 && state.mario.y < 12) {
    v[state.mario.y * 16 + state.mario.x] = 5
  }
  state.view = v
}
