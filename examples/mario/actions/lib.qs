# lib.qs — NES Super Mario Bros. World 1-1 at 2x scale.
# Tile = 32 px (2x the NES 16 px). Small Mario hitbox 24x32; big 24x64.
# Physics numbers are 2x NES (px/frame at 60 Hz, converted with dt).
#
# Tiles:
#   . air
#   1 ground   2 brick   3 coin   4 flag
#   5 pipe top 6 pipe body
#   7 coin ?-block   q mushroom ?-block   8 used
#   b stair   c castle
#   d cloud   e bush   h hill

fn tileAt(x, y) {
  if (x < 0 || x >= state.levelW || y < 0 || y >= state.levelH) { return "." }
  return charAt(at(state.rows, y), x)
}

fn setTile(x, y, ch) {
  let row = at(state.rows, y)
  state.rows[y] = substring(row, 0, x) + ch + substring(row, x + 1)
  state.tilesDirty = true
}

fn tileSrc(ch) {
  if (ch == "1") { return "assets/ground.png" }
  if (ch == "2") { return "assets/brick.png" }
  if (ch == "3") { return "assets/coin.png" }
  if (ch == "4") { return "assets/flag.png" }
  if (ch == "5") { return "assets/pipe_top.png" }
  if (ch == "6") { return "assets/pipe_body.png" }
  if (ch == "7") { return "assets/question.png" }
  if (ch == "q") { return "assets/question.png" }
  if (ch == "8") { return "assets/used.png" }
  if (ch == "b") { return "assets/stair.png" }
  if (ch == "c") { return "assets/castle.png" }
  if (ch == "d") { return "assets/cloud.png" }
  if (ch == "e") { return "assets/bush.png" }
  if (ch == "h") { return "assets/hill.png" }
  return "assets/ground.png"
}

fn isSolid(t) {
  return t == "1" || t == "2" || t == "5" || t == "6" || t == "7" || t == "q" || t == "8" || t == "b" || t == "c"
}

fn marioW() { return 24 }
fn marioH() {
  if (state.mario.big) { return 64 }
  return 32
}

# Per-axis AABB slide. isVert=true uses pos as Y and fixed as X range.
fn resolveAxis(pos, delta, fixedLo, fixedHi, isVert, span) {
  let cell = state.cellSize
  if (delta == 0) { return [pos, false] }
  let newPos = pos + delta
  let lo = newPos
  let hi = newPos + span - 0.01
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
          if (delta > 0) { newPos = cx * cell - span } else { newPos = (cx + 1) * cell }
        }
      } else {
        if (isSolid(tileAt(cx, cy))) {
          hit = true
          if (delta > 0) { newPos = cx * cell - span } else { newPos = (cx + 1) * cell }
        }
      }
      cy = cy + 1
    }
    cx = cx + 1
  }
  return [newPos, hit]
}

fn win() {
  if (state.status == "won") { return }
  state.status = "won"
  stopMusic()
  playSound("audio/win.wav")
}

fn lose() {
  if (state.status == "dead") { return }
  state.status = "dead"
  state.mario.alive = false
  state.mario.vy = 0 - 480
  state.mario.onGround = false
  state.deathTimer = 90
  state.fxDeath = state.fxDeath + 1
  stopMusic()
  playSound("audio/death.wav")
}

fn physicsStep() {
  if (state.status == "won") { return }
  # Fixed 1/60. Wall-clock dt made enemies stutter whenever a frame
  # ran long (dirty-rect misses, audio, …).
  let dt = 1 / 60
  state.lastTickMs = num(now())
  unstickMario()

  if (state.status == "dead" || !state.mario.alive) {
    state.mario.vy = state.mario.vy + 3150 * dt
    if (state.mario.vy > 480) { state.mario.vy = 480 }
    state.mario.y = state.mario.y + state.mario.vy * dt
    state.deathTimer = state.deathTimer - 1
    if (state.deathTimer <= 0) { state.deathDone = true }
    return
  }

  # NES TIME ticks ~2.5/s (400 ≈ 160 s of play).
  state.timeLeft = state.timeLeft - dt * 2.5
  if (state.timeLeft <= 0) {
    state.timeLeft = 0
    lose()
    return
  }

  if (state.mario.invuln > 0) { state.mario.invuln = state.mario.invuln - 1 }
  if (state.bumpT > 0) { state.bumpT = state.bumpT - 1 }

  let k = state.keys
  let acc = 166
  let maxVx = 187
  if (k.run) {
    acc = 396
    maxVx = 307
  }
  let ax = 0
  if (k.left && !k.right) { ax = 0 - acc }
  if (k.right && !k.left) { ax = acc }

  if (ax != 0 && state.mario.vx != 0 && (ax > 0) != (state.mario.vx > 0)) {
    # Skid: reverse direction while moving.
    state.mario.vx = state.mario.vx + ax * 2 * dt
  } else if (ax != 0) {
    state.mario.vx = state.mario.vx + ax * dt
  } else {
    let dec = 367 * dt
    if (state.mario.vx > dec) { state.mario.vx = state.mario.vx - dec } else {
      if (state.mario.vx < 0 - dec) { state.mario.vx = state.mario.vx + dec } else { state.mario.vx = 0 }
    }
  }
  if (state.mario.vx > maxVx) { state.mario.vx = maxVx }
  if (state.mario.vx < 0 - maxVx) { state.mario.vx = 0 - maxVx }
  if (state.mario.vx != 0) { state.mario.dir = state.mario.vx > 0 ? 1 : 0 - 1 }

  if (!state.mario.onGround) {
    # 2x NES: hold ~4.4 tiles, tap ~2.2. Old 1120/3150 was 3.2 / 1.1 tiles
    # and a tap could not reach a ? block (96px above small Mario's head).
    let g = 2080
    if (k.jumpHold && state.mario.vy < 0) { g = 1040 }
    state.mario.vy = state.mario.vy + g * dt
    if (state.mario.vy > 560) { state.mario.vy = 560 }
  }

  let cell = state.cellSize
  let mW = marioW()
  let mH = marioH()
  if (state.mario.onGround && abs(state.mario.vx) > 8) {
    state.mario.walkPhase = (state.mario.walkPhase + 1) % 16
  } else {
    if (state.mario.onGround) { state.mario.walkPhase = 0 }
  }

  let horizRes = resolveAxis(state.mario.x, state.mario.vx * dt, state.mario.y, state.mario.y + mH, false, mW)
  state.mario.x = at(horizRes, 0)
  if (at(horizRes, 1)) { state.mario.vx = 0 }

  let vertRes = resolveAxis(state.mario.y, state.mario.vy * dt, state.mario.x, state.mario.x + mW, true, mH)
  state.mario.y = at(vertRes, 0)
  if (at(vertRes, 1)) {
    if (state.mario.vy > 0) {
      state.mario.onGround = true
    } else {
      bumpBelow()
    }
    state.mario.vy = 0
  } else {
    let footCx = floor((state.mario.x + mW / 2) / cell)
    # Pixel just below the exclusive AABB bottom — the ground cell.
    let footCy = floor((state.mario.y + mH) / cell)
    if (isSolid(tileAt(footCx, footCy))) {
      state.mario.onGround = true
      state.mario.y = footCy * cell - mH
      state.mario.vy = 0
    } else {
      state.mario.onGround = false
    }
  }

  if (state.mario.x < 0) { state.mario.x = 0
    state.mario.vx = 0 }
  let maxX = state.levelW * cell - mW
  if (state.mario.x > maxX) { state.mario.x = maxX
    state.mario.vx = 0 }
  if (state.mario.y > state.levelH * cell) { lose() }

  collectTiles()
  stepEnemies(dt)
  touchEnemies()
  stepPowerups(dt)
}

fn collectTiles() {
  let cell = state.cellSize
  let mW = marioW()
  let mH = marioH()
  let mcx = floor((state.mario.x + mW / 2) / cell)
  let mcyCenter = floor((state.mario.y + mH / 2) / cell)
  let mcyHead = floor(state.mario.y / cell)
  let mcyFoot = floor((state.mario.y + mH - 1) / cell)
  let spots = [mcyCenter, mcyHead, mcyFoot]
  let i = 0
  while (i < 3) {
    let cy = at(spots, i)
    let t = tileAt(mcx, cy)
    if (t == "3") {
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
    if (t == "4" || t == "c") { win() }
    i = i + 1
  }
}

fn bumpBelow() {
  let cell = state.cellSize
  let mcy = floor((state.mario.y - 1) / cell)
  let mcx0 = floor(state.mario.x / cell)
  let mcx1 = floor((state.mario.x + marioW() - 1) / cell)
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
    } else {
      if (t == "7") {
        setTile(c, mcy, "8")
        state.bumpCX = c
        state.bumpCY = mcy
        state.bumpT = 8
        state.coins = state.coins + 1
        state.score = state.score + 200
        state.fxCoin = state.fxCoin + 1
        playSound("audio/coin.wav")
      } else {
        if (t == "q") {
          setTile(c, mcy, "8")
          state.bumpCX = c
          state.bumpCY = mcy
          state.bumpT = 8
          spawnMushroom(c, mcy)
          playSound("audio/powerup_appear.wav")
        } else {
          if (t == "8") {
            state.bumpCX = c
            state.bumpCY = mcy
            state.bumpT = 8
            playSound("audio/bump.wav")
          }
        }
      }
    }
    c = c + 1
  }
}

fn spawnMushroom(cx, cy) {
  let p = { x: num(cx) * state.cellSize, y: num(cy) * state.cellSize - 32, vx: 60, type: "mushroom", alive: true }
  state.powerups = push(state.powerups, p)
}

fn stepEnemies(dt) {
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
    if (e.shell && e.vx == 0) { g = g + 1
      continue }
    let nx = e.x + e.vx * dt
    let dir = e.vx < 0 ? 0 - 1 : 1
    let aheadX = floor((e.x + (dir > 0 ? 31 : 0)) / cell) + dir
    let aheadY = floor((e.y + 16) / cell)
    if (isSolid(tileAt(aheadX, aheadY))) {
      e.vx = 0 - e.vx
    } else {
      e.x = nx
    }
    let bottomCX = floor((e.x + 16) / cell)
    let bottomCY = floor((e.y + 32) / cell)
    if (!isSolid(tileAt(bottomCX, bottomCY))) {
      e.y = e.y + 240 * dt
    }
    if (e.y > state.levelH * cell) { e.alive = false }
    # Moving shell hits other enemies.
    if (e.shell && abs(e.vx) > 10) {
      let h = 0
      while (h < len(state.goombas)) {
        let o = at(state.goombas, h)
        if (h != g && o.alive && o.squash == 0) {
          if (e.x < o.x + 32 && e.x + 32 > o.x && e.y < o.y + 32 && e.y + 32 > o.y) {
            o.alive = false
            state.score = state.score + 100
            playSound("audio/stomp.wav")
          }
        }
        h = h + 1
      }
    }
    g = g + 1
  }
}

fn touchEnemies() {
  let mW = marioW()
  let mH = marioH()
  let mx = state.mario.x
  let my = state.mario.y
  let g = 0
  while (g < len(state.goombas)) {
    let e = at(state.goombas, g)
    if (!e.alive || e.squash > 0) { g = g + 1
      continue }
    if (mx < e.x + 32 && mx + mW > e.x && my < e.y + 32 && my + mH > e.y) {
      if (e.shell && e.vx == 0) {
        e.vx = state.mario.dir * 320
        playSound("audio/stomp.wav")
      } else {
        if (state.mario.vy > 40 && my + mH - e.y < 20) {
          if (e.type == "koopa" && !e.shell) {
            e.shell = true
            e.vx = 0
            e.type = "shell"
          } else {
            e.squash = 12
            e.vx = 0
          }
          state.score = state.score + 100
          state.mario.vy = 0 - 240
          state.fxStomp = state.fxStomp + 1
          playSound("audio/stomp.wav")
        } else {
          if (!(e.shell && abs(e.vx) < 10)) { hurtMario() }
        }
      }
    }
    g = g + 1
  }
  let p = 0
  while (p < len(state.powerups)) {
    let u = at(state.powerups, p)
    if (u.alive && mx < u.x + 32 && mx + mW > u.x && my < u.y + 32 && my + mH > u.y) {
      u.alive = false
      growMario()
      state.score = state.score + 1000
      playSound("audio/powerup.wav")
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
    let nx = u.x + u.vx * dt
    let dir = u.vx < 0 ? 0 - 1 : 1
    let ax = floor((nx + (dir > 0 ? 32 : 0)) / cell) + dir
    let ay = floor((u.y + 24) / cell)
    if (isSolid(tileAt(ax, ay))) { u.vx = 0 - u.vx } else { u.x = nx }
    let cy = floor((u.x + 16) / cell)
    if (!isSolid(tileAt(cy, floor((u.y + 32) / cell)))) { u.y = u.y + 240 * dt }
    p = p + 1
  }
}

fn unstickMario() {
  if (!state.mario.alive) { return }
  let cell = state.cellSize
  let cx = floor((state.mario.x + marioW() / 2) / cell)
  let mH = marioH()
  let i = 0
  while (i < 4) {
    let inside = floor((state.mario.y + mH - 1) / cell)
    if (!isSolid(tileAt(cx, inside))) { return }
    state.mario.y = inside * cell - mH
    state.mario.vy = 0
    state.mario.onGround = true
    i = i + 1
  }
}

fn growMario() {
  if (state.mario.big) { return }
  state.mario.big = true
  unstickMario()
}

fn shrinkMario() {
  if (!state.mario.big) { return }
  state.mario.big = false
  state.mario.y = state.mario.y + 32
}

fn hurtMario() {
  if (state.mario.invuln > 0) { return }
  if (state.mario.big) {
    shrinkMario()
    state.mario.invuln = 120
    state.fxHurt = state.fxHurt + 1
    playSound("audio/powerdown.wav")
  } else {
    lose()
  }
}

fn buildViewTiles() {
  let cell = state.cellSize
  let vw = state.viewportW
  let camX = max(0, state.mario.x - 5 * cell)
  let need0 = max(0, floor(camX / cell) - 1)
  let need1 = min(state.levelW, need0 + vw + 3)
  # Keep a 4-column slack so running does not rebuild every tile column
  # (that hitch is what made the board feel stuck while the camera moved).
  if (len(state.viewTiles) > 0 && state.bumpT == 0 && state.viewBump == 0 && state.tilesDirty != true && need0 >= state.viewX0 && need1 <= state.viewX1) {
    return
  }
  let slop = 4
  let x0 = max(0, need0 - slop)
  let x1 = min(state.levelW, need1 + slop)
  let n = 0
  let y = 0
  while (y < state.levelH) {
    let row = at(state.rows, y)
    let x = x0
    while (x < x1) {
      if (charAt(row, x) != ".") { n = n + 1 }
      x = x + 1
    }
    y = y + 1
  }
  let tiles = fill(n, 0)
  let i = 0
  y = 0
  while (y < state.levelH) {
    let row = at(state.rows, y)
    let x = x0
    while (x < x1) {
      let ch = charAt(row, x)
      if (ch != ".") {
        let ty = num(y) * cell
        if (state.bumpT > 0 && x == state.bumpCX && y == state.bumpCY) { ty = ty - 6 }
        tiles[i] = { x: num(x) * cell, y: ty, kind: ch, src: tileSrc(ch) }
        i = i + 1
      }
      x = x + 1
    }
    y = y + 1
  }
  state.viewTiles = tiles
  state.viewX0 = x0
  state.viewX1 = x1
  state.viewBump = state.bumpT
  state.tilesDirty = false
}
