# restart.qs — back to a fresh run: the level is restored from state.rows0
# (the pristine copy — coins Mario picked are re-placed), entities cleared,
# counters zeroed, the camera back to the start. The scene's onEnter
# points here, so first load opens ready to play.

state.rows = concat(state.rows0)
# Small Mario is 32 px tall (2x NES). Ground top is row 13 (y=416),
# so standing y = 416 - 32 = 384. x=32 is one tile in from the left.
state.mario = { x: 32.0, y: 384.0, vx: 0.0, vy: 0.0, onGround: true, big: false, alive: true, dir: 1, walkPhase: 0, invuln: 0 }
# NES 1-1 pacing: goombas first, then a green Koopa. vx is 2x NES walk.
state.goombas = [
  { x: 704.0,  y: 384.0, vx: -32.0, alive: true, walkPhase: 0, type: "goomba", shell: false, squash: 0 },
  { x: 864.0,  y: 384.0, vx: -32.0, alive: true, walkPhase: 0, type: "goomba", shell: false, squash: 0 },
  { x: 1408.0, y: 384.0, vx: -32.0, alive: true, walkPhase: 0, type: "koopa", shell: false, squash: 0 },
  { x: 2112.0, y: 384.0, vx: -32.0, alive: true, walkPhase: 0, type: "goomba", shell: false, squash: 0 },
  { x: 2304.0, y: 384.0, vx: -32.0, alive: true, walkPhase: 0, type: "goomba", shell: false, squash: 0 }
]
state.powerups = []
state.particles = []
state.bumpBlocks = []
state.keys = { left: false, right: false, jump: false, jumpHold: false, run: false }
state.coins = 0
state.score = 0
state.lives = 3
state.timeLeft = 400
state.status = "playing"
state.lastTickMs = 0
state.deathTimer = 0
state.deathDone = false
state.fxDeath = 0
state.fxCoin = 0
state.fxStomp = 0
state.fxJump = 0
state.fxHurt = 0
state.bumpT = 0
state.bumpCX = 0
state.bumpCY = 0
state.tilesDirty = true
state.viewX0 = -1
# Bump so cameraLockLeft / dead-zone sticky pan rewind to the start.
state.cameraGen = state.cameraGen + 1
playMusic("audio/music.wav")
state.cameraX = 0
state.cameraY = 0
