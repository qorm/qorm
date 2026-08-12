# lib.qs — the mavis core. Reserved `type:"scriptlib"`, spliced ahead
# of every action's body. Keep this file to fn definitions + comments;
# top-level statements here would run before every action.
#
# World is a levelW × levelH grid of 32-px cells, kept as 211-char strings
# in `state.rows` (row 0 = top, row levelH-1 = ground). Mario sprite is
# 16×16, AABB-centered in the 32-px cell. Tile glyphs:
#   .  air
#   1  ground (solid, indestructible)
#   2  brick (solid; small mario nudges, big mario breaks)
#   3  coin (pickup)
#   4  flag pole
#   5  pipe top (solid, indestructible)
#   6  pipe body (solid)
#   7  question block (solid; first hit → powerup, then '8' used)
#   8  used question block (solid, inert)
#   b  stair (solid)
#   d  cloud, e  bush, h  hill (non-solid decoration; mario passes through)
#
# Physics: mario.x/y are physical px. The engine's timer is 16ms (60fps)
# and computes dt from wall-clock so the world is frame-rate independent.

# ----- Tile helpers ----------------------------------------------------------

fn tileAt(x, y) {
  if (x < 0 || x >= state.levelW || y < 0 || y >= state.levelH) { return "." }
  return charAt(at(state.rows, y), x)
}

fn setTile(x, y, ch) {
  let row = at(state.rows, y)
  state.rows[y] = substring(row, 0, x) + ch + substring(row, x + 1)
}

fn isSolid(t) {
  return t == "1" || t == "2" || t == "5" || t == "6" || t == "7" || t == "8" || t == "b" || t == "c"
}

fn isDecorative(t) {
  return t == "d" || t == "e" || t == "h"
}

# ----- AABB collision (one axis at a time) ----------------------------------
# resolveAxis returns [newPos, hitSolid] after sliding past the solid
# cells the sprite would otherwise overlap. Called once per axis: horizontal
# then vertical, with the OTHER axis's already-resolved coordinate. This
# is the standard platformer trick — without per-axis resolution the sprite
# can catch on a corner.
fn resolveAxis(pos, delta, fixedLo, fixedHi, isVert) {
  let cell = state.cellSize
  let marioW = 16
  let marioH = 16
  if (delta == 0) { return [pos, false] }
  let newPos = pos + delta
  let lo = newPos
  let hi = newPos + (isVert ? marioH : marioW) - 0.01
  let c0 = floor(lo / cell)
  let c1 = floor(hi / cell)
  let co0 = floor(fixedLo / cell)
  let co1 = floor((fixedHi - 0.01) / cell)
  let hit = false
  let cx = c0
  while (cx <= c1) {
    let cy = co0
    while (cy <= co1) {
      if (isVert) {
        if (isSolid(tileAt(cy, cx))) {
          hit = true
          if (delta > 0) {
            newPos = cx * cell - marioH
          } else {
            newPos = (cx + 1) * cell
          }
        }
      } else {
        if (isSolid(tileAt(cx, cy))) {
          hit = true
          if (delta > 0) {
            newPos = cx * cell - marioW
          } else {
            newPos = (cx + 1) * cell
          }
        }
      }
      cy = cy + 1
    }
    cx = cx + 1
  }
  return [newPos, hit]
}

# ----- Game rules ------------------------------------------------------------

fn win() {
  state.status = "won"
  stopMusic()
  playSound("audio/win.wav")
}

fn lose() {
  if (state.status == "dead") { return }
  state.status = "dead"
  state.mario.alive = false
  state.mario.vy = -300
  state.mario.onGround = false
  state.deathTimer = 60
  state.fxDeath = state.fxDeath + 1
  stopMusic()
  playSound("audio/death.wav")
}

# ----- Main physics step -----------------------------------------------------
# physicsStep advances the world by one tick. The tick is driven by a 16-ms
# timer (60 fps) declared in scenes/main.json.
fn physicsStep() {
  if (state.status == "won") { return }
  let now = num(now())
  let dt = (now - state.lastTickMs) / 1000
  if (dt <= 0 || dt > 0.1) { dt = 1 / 60 }
  state.lastTickMs = now

  # lose() sets status=dead immediately; keep ticking so the NES bounce
  # (upward vy, then free-fall) can play and deathDone can arm the overlay.
  if (state.status == "dead" || !state.mario.alive) {
    state.mario.vy = state.mario.vy + 800 * dt
    state.mario.y = state.mario.y + state.mario.vy * dt
    state.deathTimer = state.deathTimer - 1
    if (state.deathTimer <= 0) { state.deathDone = true }
    return
  }

  # Timer countdown
  state.timeLeft = state.timeLeft - dt
  if (state.timeLeft <= 0) { state.timeLeft = 0
    lose()
  }

  if (state.mario.invuln > 0) { state.mario.invuln = state.mario.invuln - 1 }
  if (state.bumpT > 0) { state.bumpT = state.bumpT - 1 }

  # Horizontal: input → acceleration, friction when no key is held.
  let k = state.keys
  let ax = 0
  if (k.left && !k.right) { ax = -1200 }
  if (k.right && !k.left) { ax = 1200 }
  state.mario.vx = state.mario.vx + ax * dt
  if (ax == 0) {
    state.mario.vx = state.mario.vx * 0.85
    if (abs(state.mario.vx) < 4) { state.mario.vx = 0 }
  }
  let maxVx = 120
  if (k.run) { maxVx = 180 }
  if (state.mario.vx > maxVx) { state.mario.vx = maxVx }
  if (state.mario.vx < 0 - maxVx) { state.mario.vx = 0 - maxVx }
  if (state.mario.vx != 0) { state.mario.dir = state.mario.vx > 0 ? 1 : 0 - 1 }

  # Vertical: gravity, reduced while jump is held.
  if (!state.mario.onGround) {
    let g = 1400
    if (k.jumpHold && state.mario.vy < 0) { g = 500 }
    state.mario.vy = state.mario.vy + g * dt
    if (state.mario.vy > 600) { state.mario.vy = 600 }
  }

  # Apply movement one axis at a time, with collision.
  let cell = state.cellSize
  let mW = 16
  let mH = 16
  # Walk animation phase (cycles 0..3 every 8 frames when on ground moving)
  if (state.mario.onGround && state.mario.vx != 0) {
    state.mario.walkPhase = (state.mario.walkPhase + 1) % 16
  } else {
    state.mario.walkPhase = 0
  }

  # Horizontal pass.
  let dx = state.mario.vx * dt
  let horizRes = resolveAxis(state.mario.x, dx, state.mario.y, state.mario.y + mH, false)
  state.mario.x = at(horizRes, 0)
  if (at(horizRes, 1)) { state.mario.vx = 0 }
  # Vertical pass.
  let dy = state.mario.vy * dt
  let vertRes = resolveAxis(state.mario.y, dy, state.mario.x, state.mario.x + mW, true)
  state.mario.y = at(vertRes, 0)
  if (at(vertRes, 1)) {
    if (state.mario.vy > 0) {
      state.mario.onGround = true
    } else {
      bumpBelow()
    }
    state.mario.vy = 0
  } else {
    let footCx = floor((state.mario.x + 8) / cell)
    let footCy = floor((state.mario.y + mH) / cell)
    if (isSolid(tileAt(footCx, footCy))) {
      state.mario.onGround = true
    } else {
      state.mario.onGround = false
    }
  }

  # Clamp to level bounds.
  if (state.mario.x < 0) { state.mario.x = 0
    state.mario.vx = 0 }
  if (state.mario.x > (state.levelW - 1) * cell) { state.mario.x = (state.levelW - 1) * cell
    state.mario.vx = 0 }

  # Out-of-world fall = death.
  if (state.mario.y > state.levelH * cell) { lose() }

  # Coin + flag pickup: check the cell at mario's center.
  let mcx = floor((state.mario.x + mW / 2) / cell)
  let mcyCenter = floor((state.mario.y + mH / 2) / cell)
  let mcyHead = floor(state.mario.y / cell) - 1
  let mcyFoot = floor((state.mario.y + mH) / cell)
  let collected = false
  let tCenter = tileAt(mcx, mcyCenter)
  let tHead = tileAt(mcx, mcyHead)
  let tFoot = tileAt(mcx, mcyFoot)
  if (tCenter == "3" || tHead == "3" || tFoot == "3") {
    let cy = tCenter == "3" ? mcyCenter : (tHead == "3" ? mcyHead : mcyFoot)
    setTile(mcx, cy, ".")
    state.coins = state.coins + 1
    state.score = state.score + 200
    state.fxCoin = state.fxCoin + 1
    playSound("audio/coin.wav")
    if (state.coins >= 100) {
      state.coins = state.coins - 100
      state.lives = state.lives + 1
      playSound("audio/1up.wav")
    }
  }
  if (tCenter == "4" || tHead == "4" || tFoot == "4" || tCenter == "c" || tHead == "c" || tFoot == "c") { win() }

  # Enemy interaction.
  stepGoombas(dt)
  touchEnemies()
  stepPowerups(dt)

  # Camera follow (writes state.cameraX/Y for buildViewTiles; the canvas
  # engine computes its own PanX from mario, so both renderers stay in
  # sync). Center on mario, clamp to the level bounds so the camera
  # never shows sky past the left/right edge of the level.
  let halfW = floor(state.viewportW / 2) * cell
  let halfH = floor(state.viewportH / 2) * cell
  let camX = state.mario.x - halfW
  let camY = state.mario.y - halfH
  let maxCamX = max(0, (state.levelW - state.viewportW) * cell)
  let maxCamY = max(0, (state.levelH - state.viewportH) * cell)
  if (camX < 0) { camX = 0 }
  if (camX > maxCamX) { camX = maxCamX }
  if (camY < 0) { camY = 0 }
  if (camY > maxCamY) { camY = maxCamY }
  state.cameraX = 0 - camX
  state.cameraY = 0 - camY

  # Flatten the visible viewport into a list the scene renders.
  buildViewTiles()
}

# bumpBelow handles mario bonking a block from below. The block cell is one
# row above mario's head; identify it, then spawn a powerup (from a `7`
# question block on the first hit) or destroy it (brick, if big).
fn bumpBelow() {
  let cell = state.cellSize
  let mH = 16
  let mcy = floor((state.mario.y - 1) / cell)
  let mcx0 = floor(state.mario.x / cell)
  let mcx1 = floor((state.mario.x + 16 - 1) / cell)
  let c = mcx0
  while (c <= mcx1) {
    let t = tileAt(c, mcy)
    if (t == "2") {
      state.bumpCX = c
      state.bumpCY = mcy
      state.bumpT = 8
      if (state.mario.big) {
        setTile(c, mcy, ".")
        state.score = state.score + 50
      } else {
        playSound("audio/bump.wav")
      }
    } else if (t == "7") {
      setTile(c, mcy, "8")
      state.bumpCX = c
      state.bumpCY = mcy
      state.bumpT = 8
      spawnMushroom(c, mcy)
      state.score = state.score + 200
      playSound("audio/powerup_appear.wav")
    } else if (t == "8") {
      state.bumpCX = c
      state.bumpCY = mcy
      state.bumpT = 8
      playSound("audio/bump.wav")
    }
    c = c + 1
  }
}

fn spawnMushroom(cx, cy) {
  let p = { x: num(cx) * state.cellSize,
            y: num(cy) * state.cellSize,
            vx: 60,
            type: "mushroom",
            alive: true,
            dead: false }
  state.powerups = push(state.powerups, p)
}

# stepGoombas walks every live goomba. Each one moves along its platform
# and turns at walls / ledges.
fn stepGoombas(dt) {
  let cell = state.cellSize
  let g = 0
  while (g < len(state.goombas)) {
    let e = at(state.goombas, g)
    if (!e.alive) { g = g + 1
      continue }
    if (e.squash > 0) {
      e.squash = e.squash - 1
      if (e.squash <= 0) { e.alive = false }
      g = g + 1
      continue
    }
    e.walkPhase = (e.walkPhase + 1) % 16
    let nx = e.x + e.vx * dt
    # Check the cell in front of the goomba, in the goomba's body row.
    # Using the goomba's BOTTOM row (y+cell)/cell would always be the ground
    # row, so the wall check fires every frame and the goomba oscillates
    # in place. The body row is floor(y/cell) — for a 32-px goomba at y=400
    # (sitting on the row-13 ground) the body is in row 12, and a wall at
    # row 12 actually blocks horizontal motion.
    let dir = e.vx < 0 ? 0 - 1 : 1
    let aheadX = floor((e.x + (dir > 0 ? cell - 1 : 0)) / cell) + dir
    let aheadY = floor(e.y / cell)
    let wallAhead = isSolid(tileAt(aheadX, aheadY))
    if (wallAhead) {
      e.vx = 0 - e.vx
    } else if (e.x >= 0 && e.x < state.levelW * cell) {
      e.x = nx
    } else {
      e.vx = 0 - e.vx
    }
    # Settle vertically onto the platform below.
    let ey = e.y
    let bottomCX = floor((e.x + cell / 2) / cell)
    let bottomCY = floor((ey + cell) / cell)
    if (!isSolid(tileAt(bottomCX, bottomCY))) {
      # No platform directly below — goomba falls.
      ey = ey + 80 * dt
    }
    e.y = ey
    # Remove goombas that fell off the world.
    if (e.y > state.levelH * cell) { e.alive = false }
    g = g + 1
  }
}

fn touchEnemies() {
  let cell = state.cellSize
  let mW = 16
  let mH = 16
  let mx = state.mario.x
  let my = state.mario.y
  let g = 0
  while (g < len(state.goombas)) {
    let e = at(state.goombas, g)
    if (!e.alive || e.squash > 0) { g = g + 1
      continue }
    if (mx < e.x + cell && mx + mW > e.x && my < e.y + cell && my + mH > e.y) {
      # Stomp: mario falling onto goomba from above.
      if (state.mario.vy > 0 && my + mH - e.y < cell / 2) {
        e.squash = 10
        e.vx = 0
        state.score = state.score + 100
        state.mario.vy = -240
        state.fxStomp = state.fxStomp + 1
        playSound("audio/stomp.wav")
      } else {
        hurtMario()
        g = len(state.goombas)
      }
    }
    g = g + 1
  }
  # Powerup pickup.
  let p = 0
  while (p < len(state.powerups)) {
    let u = at(state.powerups, p)
    if (!u.alive) { p = p + 1
      continue }
    if (mx < u.x + cell && mx + mW > u.x && my < u.y + cell && my + mH > u.y) {
      u.alive = false
      if (u.type == "mushroom") {
        state.mario.big = true
        state.score = state.score + 1000
        playSound("audio/powerup.wav")
      }
    }
    p = p + 1
  }
}

fn stepPowerups(dt) {
  let cell = state.cellSize
  let p = 0
  while (p < len(state.powerups)) {
    let u = at(state.powerups, p)
    if (!u.alive) { p = p + 1
      continue }
    if (u.type == "mushroom") {
      let nx = u.x + u.vx * dt
      let dir = u.vx < 0 ? 0 - 1 : 1
      let ax = floor((nx + (dir > 0 ? cell : 0)) / cell) + dir
      let ay = floor((u.y + cell - 1) / cell)
      if (isSolid(tileAt(ax, ay))) {
        u.vx = 0 - u.vx
      } else {
        u.x = nx
      }
      # Settle onto platform.
      let uy = u.y
      let cy = floor((u.x + cell / 2) / cell)
      while (!isSolid(tileAt(cy, floor((uy + cell) / cell))) && uy < state.levelH * cell) {
        uy = uy + 2
      }
      u.y = uy
    }
    p = p + 1
  }
}

fn hurtMario() {
  if (state.mario.big) {
    state.mario.big = false
    state.mario.invuln = 30
    state.fxHurt = state.fxHurt + 1
    playSound("audio/powerdown.wav")
  } else {
    lose()
  }
}

# buildViewTiles flattens the visible viewport (camera-followed) into
# state.viewTiles — a list of {x, y, kind} the scene's tile lists render.
fn buildViewTiles() {
  let cell = state.cellSize
  let vw = state.viewportW
  let vh = state.viewportH
  let camX = max(0, min(state.levelW * cell, state.mario.x - vw / 2 * cell))
  let camY = max(0, min(state.levelH * cell, state.mario.y - vh / 2 * cell))
  let x0 = max(0, floor(camX / cell) - 1)
  let y0 = max(0, floor(camY / cell) - 1)
  let x1 = min(state.levelW, x0 + vw + 2)
  let y1 = min(state.levelH, y0 + vh + 2)
  let tiles = []
  let y = y0
  while (y < y1) {
    let row = at(state.rows, y)
    let x = x0
    while (x < x1) {
      let ch = charAt(row, x)
      if (ch != ".") {
        let ty = num(y) * cell
        if (state.bumpT > 0 && x == state.bumpCX && y == state.bumpCY) {
          ty = ty - 6
        }
        tiles = push(tiles, { x: num(x) * cell, y: ty, kind: ch })
      }
      x = x + 1
    }
    y = y + 1
  }
  state.viewTiles = tiles
}
