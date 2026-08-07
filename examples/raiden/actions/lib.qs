# lib.qs — the shared Raiden core. Reserved type:"scriptlib", spliced ahead
# of every action body. Keep this file to fn definitions + comments.
#
# The engine is coordinate-based (continuous pixel positions, not a tile
# grid). The play field is 320 wide × 560 tall. Sprites are 32×32; the
# player is drawn at 48×48. Collisions are circle-vs-circle distance checks
# (shmups use radial hitboxes, not AABB).
#
# Original designs only — no assets or audio from the 1990 arcade game are
# reproduced; the mechanics are re-implemented from scratch.

# ----- Constants -------------------------------------------------------------

fn fieldW() { return 320 }
fn fieldH() { return 560 }
fn playerSpeed() { return 5 }

# ----- Starfield (parallax background) --------------------------------------

fn initStars() {
  if (len(state.starsFar) > 0) { return }
  state.starsFar = []
  state.starsNear = []
  for i in range(40) {
    state.starsFar = push(state.starsFar, { x: mod(i * 13, 320), y: mod(i * 7, 560) })
  }
  for i in range(20) {
    state.starsNear = push(state.starsNear, { x: mod(i * 17, 320), y: mod(i * 11, 560) })
  }
}

fn scrollStars() {
  for s in state.starsFar {
    s.y = s.y + 1
    if (s.y >= 560) { s.y = 0 }
  }
  for s in state.starsNear {
    s.y = s.y + 3
    if (s.y >= 560) { s.y = 0 }
  }
}

# ----- Player movement (8-directional, held keys) ----------------------------

# movePlayer reads the held-key state and slides the ship. Diagonals move at
# 0.707× so the speed vector stays constant (classic shmup normalization).
fn movePlayer() {
  let k = state.keys
  let dx = 0
  let dy = 0
  if (k.left && !k.right) { dx = -1 }
  if (k.right && !k.left) { dx = 1 }
  if (k.up && !k.down) { dy = -1 }
  if (k.down && !k.up) { dy = 1 }
  if (dx != 0 && dy != 0) {
    # Diagonal: scale both axes so magnitude stays playerSpeed.
    dx = dx * 0.707
    dy = dy * 0.707
  }
  let sp = playerSpeed()
  state.player.x = clamp(state.player.x + dx * sp, 0, 264)
  state.player.y = clamp(state.player.y + dy * sp, 40, 400)
}

# ----- Weapons ---------------------------------------------------------------

# fireBullet: hold-to-fire, cooldown per level. Weapon levels:
#   1 = single Vulcan
#   2 = twin Vulcan
#   3 = triple spread (center + 2 angled)
#   4 = 5-way spread
#   5 = twin laser + side Vulcan
#   6 = max: triple laser + side shots
fn fireBullet() {
  state.fireTimer = state.fireTimer + 1
  if (state.fireTimer < 4) { return }
  state.fireTimer = 0
  let w = state.player.weapon
  let px = state.player.x + 28
  let py = state.player.y
  if (w == 1) {
    state.bullets = push(state.bullets, { x: px, y: py, dx: 0, dy: -9, type: 1 })
  } else if (w == 2) {
    state.bullets = push(state.bullets, { x: px - 6, y: py, dx: 0, dy: -9, type: 1 })
    state.bullets = push(state.bullets, { x: px + 6, y: py, dx: 0, dy: -9, type: 1 })
  } else if (w == 3) {
    state.bullets = push(state.bullets, { x: px, y: py, dx: 0, dy: -9, type: 1 })
    state.bullets = push(state.bullets, { x: px - 8, y: py + 4, dx: -1.5, dy: -8, type: 1 })
    state.bullets = push(state.bullets, { x: px + 8, y: py + 4, dx: 1.5, dy: -8, type: 1 })
  } else if (w == 4) {
    state.bullets = push(state.bullets, { x: px, y: py, dx: 0, dy: -9, type: 1 })
    state.bullets = push(state.bullets, { x: px - 8, y: py + 4, dx: -1.5, dy: -8, type: 1 })
    state.bullets = push(state.bullets, { x: px + 8, y: py + 4, dx: 1.5, dy: -8, type: 1 })
    state.bullets = push(state.bullets, { x: px - 12, y: py + 8, dx: -3, dy: -6, type: 1 })
    state.bullets = push(state.bullets, { x: px + 12, y: py + 8, dx: 3, dy: -6, type: 1 })
  } else if (w == 5) {
    state.bullets = push(state.bullets, { x: px - 4, y: py, dx: 0, dy: -10, type: 2 })
    state.bullets = push(state.bullets, { x: px + 4, y: py, dx: 0, dy: -10, type: 2 })
    state.bullets = push(state.bullets, { x: px - 10, y: py + 4, dx: -1.5, dy: -8, type: 1 })
    state.bullets = push(state.bullets, { x: px + 10, y: py + 4, dx: 1.5, dy: -8, type: 1 })
  } else {
    state.bullets = push(state.bullets, { x: px, y: py, dx: 0, dy: -10, type: 2 })
    state.bullets = push(state.bullets, { x: px - 6, y: py, dx: 0, dy: -10, type: 2 })
    state.bullets = push(state.bullets, { x: px + 6, y: py, dx: 0, dy: -10, type: 2 })
    state.bullets = push(state.bullets, { x: px - 12, y: py + 4, dx: -2, dy: -8, type: 1 })
    state.bullets = push(state.bullets, { x: px + 12, y: py + 4, dx: 2, dy: -8, type: 1 })
  }
  playSound("audio/shoot.wav")
}

fn moveBullets() {
  let live = []
  for b in state.bullets {
    b.x = b.x + b.dx
    b.y = b.y + b.dy
    if (b.y >= -16 && b.x >= -16 && b.x <= 336) { live = push(live, b) }
  }
  state.bullets = live
}

# bulletHitbox: laser (type 2) hits slightly wider than Vulcan.
fn bulletHitbox(t) {
  if (t == 2) { return 14 }
  return 10
}

# ----- Bomb (screen-clear, Raiden signature) ---------------------------------

# useBomb clears every enemy + enemy bullet on screen, deals 20 damage to the
# boss, and triggers a white flash. Costs one bomb; ignored at zero.
fn useBomb() {
  if (state.player.bombs <= 0 || state.status != "playing") { return }
  state.player.bombs = state.player.bombs - 1
  state.bombFlash = 8
  for e in state.enemies {
    if (e.alive) {
      e.alive = false
      state.explosions = push(state.explosions, { x: e.x, y: e.y, t: 0 })
      state.score = state.score + 50
    }
  }
  if (state.boss.alive) {
    state.boss.hp = state.boss.hp - 20
    state.explosions = push(state.explosions, { x: state.boss.x, y: state.boss.y, t: 0 })
    if (state.boss.hp <= 0) { bossDefeated() }
  }
  playSound("audio/explode.wav")
}

fn tickBombFlash() {
  if (state.bombFlash > 0) { state.bombFlash = state.bombFlash - 1 }
}

# ----- Enemy formations (curved dive paths, Raiden signature) ----------------

# spawnFormation sends a wave of N small fighters entering from the top and
# diving along a sine curve toward the player. Each enemy carries a path
# phase so moveEnemies can advance it. This reproduces the classic "formation
# dive" feel instead of random single drops.
fn spawnFormation() {
  let n = 4
  let side = mod(state.tick, 2)
  let startX = 40
  let fdir = 1
  if (side == 1) {
    startX = 280
    fdir = -1
  }
  for i in range(n) {
    state.enemies = push(state.enemies, {
      x: startX, y: -20 - i * 28,
      type: 1, alive: true,
      form: 1, phase: i * 0.6, dir: fdir,
      drops: 0
    })
  }
}

# spawnSingle drops one heavier enemy (bomber 2 / heli 3 / turret 4).
fn spawnSingle() {
  let roll = mod(state.tick * 7, 100)
  let t = 2
  if (roll < 40) { t = 2 }
  else if (roll < 75) { t = 3 }
  else { t = 4 }
  let ex = mod(state.tick * 37, 288) + 16
  let drops = 0
  if (t == 3 && mod(state.tick, 2) == 0) { drops = 1 }
  if (t == 2 && mod(state.tick, 3) == 0) { drops = 1 }
  state.enemies = push(state.enemies, {
    x: ex, y: -32, type: t, alive: true,
    form: 0, phase: 0, dir: 1, drops: drops
  })
}

# moveEnemies advances every live enemy. Formation fighters (form==1) follow
# a sine dive; singles move per type.
fn moveEnemies() {
  for e in state.enemies {
    if (!e.alive) { continue }
    if (e.form == 1) {
      # Formation dive: descend + sweep in a sine wave across the field.
      e.y = e.y + 3
      e.phase = e.phase + 0.08
      e.x = e.x + e.dir * sin(e.phase) * 3
      if (e.x < 8) { e.x = 8 }
      if (e.x > 312) { e.x = 312 }
    } else if (e.type == 4) {
      e.y = 444
    } else {
      e.y = e.y + 2
      if (e.type == 1) {
        e.x = e.x + e.dir * 2
        if (e.x < 0 || e.x > 304) { e.dir = 0 - e.dir }
      }
      if (e.type == 3) {
        e.x = e.x + sin(e.phase) * 2
        e.phase = e.phase + 0.1
      }
    }
    if (e.y > 470) { e.alive = false }
  }
}

fn pruneEnemies() {
  let live = []
  for e in state.enemies {
    if (e.alive) { live = push(live, e) }
  }
  state.enemies = live
}

# enemyFire lets ground turrets and bombers shoot back — a Raiden staple.
# Turrets (type 4) fire an AIMED shot at the player every 20 ticks; bombers
# (type 2) drop a straight bullet every 24 ticks while on screen.
fn enemyFire() {
  let pcx = state.player.x + 24
  let pcy = state.player.y + 24
  for e in state.enemies {
    if (!e.alive) { continue }
    if (e.y < 0) { continue }
    if (e.type == 4 && mod(state.tick, 20) == 0) {
      let ang = atan2(pcy - e.y, pcx - e.x)
      state.bullets = push(state.bullets, {
        x: e.x, y: e.y - 8,
        dx: cos(ang) * 3.5, dy: sin(ang) * 3.5,
        type: 9
      })
    }
    if (e.type == 2 && mod(state.tick, 24) == 0) {
      state.bullets = push(state.bullets, {
        x: e.x, y: e.y + 12,
        dx: 0, dy: 4,
        type: 9
      })
    }
  }
}

# ----- Boss ------------------------------------------------------------------

# spawnBoss activates the end-of-stage boss. Big HP, slides side to side at
# the top, fires aimed shots (handled in bossFire).
fn spawnBoss() {
  state.boss = { alive: true, x: 160, y: -60, hp: 60, phase: 0 }
  playSound("audio/music.wav")
}

fn moveBoss() {
  if (!state.boss.alive) { return }
  # Slide in from the top to y=60, then patrol side to side.
  if (state.boss.y < 60) {
    state.boss.y = state.boss.y + 2
  } else {
    state.boss.phase = state.boss.phase + 0.03
    state.boss.x = 160 + sin(state.boss.phase) * 100
    # Boss fires an aimed spread every so often.
    if (mod(state.tick, 6) == 0) { bossFire() }
  }
}

# bossFire spawns enemy bullets aimed at the player in a 3-way fan.
fn bossFire() {
  let bx = state.boss.x
  let by = state.boss.y + 20
  let px = state.player.x + 28
  let py = state.player.y + 28
  let ang = atan2(py - by, px - bx)
  for i in range(3) {
    let a = ang + (i - 1) * 0.25
    state.bullets = push(state.bullets, {
      x: bx, y: by,
      dx: cos(a) * 4, dy: sin(a) * 4,
      type: 9
    })
  }
}

fn bossDefeated() {
  state.boss.alive = false
  state.score = state.score + 5000
  for i in range(6) {
    state.explosions = push(state.explosions, {
      x: state.boss.x + mod(i * 23, 48) - 24,
      y: state.boss.y + mod(i * 31, 48) - 24,
      t: 0
    })
  }
  playSound("audio/explode.wav")
  state.status = "won"
  stopMusic()
  playSound("audio/win.wav")
}

# ----- Power-ups (P weapon / B bomb / medal score) ---------------------------

# dropPowerUp: type 1=P (weapon), 2=B (bomb), 3=medal (500 pts).
fn dropPowerUp(x, y) {
  let r = mod(state.tick * 11, 100)
  let pt = 3
  if (r < 50) { pt = 1 }
  else if (r < 75) { pt = 2 }
  state.powerups = push(state.powerups, { x: x, y: y, type: pt, alive: true })
}

fn movePowerUps() {
  let live = []
  for p in state.powerups {
    p.y = p.y + 2
    if (p.y < 470) { live = push(live, p) }
  }
  state.powerups = live
}

fn checkPowerUps() {
  let live = []
  for p in state.powerups {
    if (abs(p.x - (state.player.x + 28)) < 30 && abs(p.y - (state.player.y + 28)) < 30) {
      p.alive = false
      if (p.type == 1) {
        if (state.player.weapon < 6) {
          state.player.weapon = state.player.weapon + 1
        } else {
          state.score = state.score + 1000
        }
        playSound("audio/powerup.wav")
      } else if (p.type == 2) {
        if (state.player.bombs < 7) { state.player.bombs = state.player.bombs + 1 }
        playSound("audio/powerup.wav")
      } else {
        state.score = state.score + 500
        playSound("audio/hit.wav")
      }
    }
    if (p.alive) { live = push(live, p) }
  }
  state.powerups = live
}

# ----- Collisions ------------------------------------------------------------

fn checkHits() {
  let hitBullets = []
  for bi in range(len(state.bullets)) {
    let b = at(state.bullets, bi)
    if (b.type == 9) { continue }
    let br = bulletHitbox(b.type)
    for ei in range(len(state.enemies)) {
      let e = at(state.enemies, ei)
      if (!e.alive) { continue }
      if (abs(b.x - e.x) < br && abs(b.y - e.y) < br) {
        let pts = 100
        if (e.type == 2) { pts = 250 }
        if (e.type == 3) { pts = 500 }
        state.score = state.score + pts
        e.alive = false
        hitBullets = push(hitBullets, bi)
        state.explosions = push(state.explosions, { x: e.x, y: e.y, t: 0 })
        if (e.drops == 1) { dropPowerUp(e.x, e.y) }
        playSound("audio/hit.wav")
      }
    }
    # Boss hit.
    if (state.boss.alive && abs(b.x - state.boss.x) < 40 && abs(b.y - state.boss.y) < 30) {
      state.boss.hp = state.boss.hp - 1
      hitBullets = push(hitBullets, bi)
      if (state.boss.hp <= 0) { bossDefeated() }
    }
  }
  if (len(hitBullets) == 0) { return }
  let kept = []
  for bi in range(len(state.bullets)) {
    let drop = false
    for h in hitBullets { if (h == bi) { drop = true } }
    if (!drop) { kept = push(kept, at(state.bullets, bi)) }
  }
  state.bullets = kept
}

fn pruneExplosions() {
  let live = []
  for x in state.explosions {
    x.t = x.t + 1
    if (x.t < 4) { live = push(live, x) }
  }
  state.explosions = live
}

# checkPlayerHits: enemy bodies and enemy bullets (type 9) both hurt.
fn checkPlayerHits() {
  if (state.player.invuln > 0) { return }
  let pcx = state.player.x + 28
  let pcy = state.player.y + 28
  # Enemy bullets.
  let kept = []
  for b in state.bullets {
    if (b.type == 9) {
      if (abs(b.x - pcx) < 14 && abs(b.y - pcy) < 14) {
        playerHit()
      } else {
        kept = push(kept, b)
      }
    } else {
      kept = push(kept, b)
    }
  }
  state.bullets = kept
  if (state.player.invuln > 0) { return }
  # Enemy bodies.
  for e in state.enemies {
    if (!e.alive) { continue }
    if (abs(e.x - pcx) < 20 && abs(e.y - pcy) < 20) {
      e.alive = false
      playerHit()
    }
  }
}

fn playerHit() {
  state.player.lives = state.player.lives - 1
  state.player.invuln = 60
  state.explosions = push(state.explosions, { x: state.player.x + 28, y: state.player.y + 28, t: 0 })
  playSound("audio/explode.wav")
  if (state.player.lives <= 0) {
    state.status = "dead"
    stopMusic()
    playSound("audio/death.wav")
    if (state.score > state.hiScore) { state.hiScore = state.score }
  } else {
    state.player.weapon = 1
  }
}

fn tickInvuln() {
  if (state.player.invuln > 0) { state.player.invuln = state.player.invuln - 1 }
}

# ----- Terrain scroll (land/water alternation) -------------------------------

# scrollGround advances the bottom terrain band and alternates its type:
# terrainType 0 = land, 1 = water. Swaps every 300 ticks so the field reads
# as a scrolling landscape, a signature Raiden backdrop.
fn scrollGround() {
  state.groundScroll = state.groundScroll + 2
  if (state.groundScroll >= 32) { state.groundScroll = 0 }
  if (mod(state.tick, 300) == 0 && state.tick > 0) {
    state.terrainType = 1 - state.terrainType
  }
}

# ----- Small helpers ---------------------------------------------------------

fn abs(v) {
  if (v < 0) { return 0 - v }
  return v
}

fn clamp(v, lo, hi) {
  if (v < lo) { return lo }
  if (v > hi) { return hi }
  return v
}