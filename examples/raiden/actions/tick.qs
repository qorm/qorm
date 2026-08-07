# tick.qs — one physics frame. The scene's timer drives it while status is
# playing. Movement is hold-based (keys.* booleans) so this step reads them
# and slides the ship; firing is likewise held.

if (state.status == "playing") {
  state.tick = state.tick + 1
  initStars()
  scrollStars()
  scrollGround()
  movePlayer()
  if (state.keys.fire) { fireBullet() }
  moveBullets()
  moveEnemies()
  enemyFire()
  moveBoss()
  movePowerUps()
  pruneEnemies()
  pruneExplosions()
  checkHits()
  checkPowerUps()
  checkPlayerHits()
  tickInvuln()
  tickBombFlash()

  # Spawn director: formations early, singles mid, boss near the end.
  state.spawnTimer = state.spawnTimer - 1
  if (state.spawnTimer <= 0) {
    if (state.tick < 200) {
      spawnFormation()
      state.spawnTimer = 22
    } else if (state.tick < 500) {
      if (mod(state.tick, 60) < 30) { spawnFormation() } else { spawnSingle() }
      state.spawnTimer = 16
    } else if (!state.boss.alive && state.status == "playing") {
      spawnBoss()
      state.spawnTimer = 999
    } else {
      state.spawnTimer = 30
    }
  }
}