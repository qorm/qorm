# restart.qs — back to a fresh run: the level is restored from state.rows0
# (the pristine copy — coins Mario picked are re-placed), entities cleared,
# counters zeroed, the camera back to the start. The scene's onEnter
# points here, so first load opens ready to play.

state.rows = concat(state.rows0)
# Mario (16px tall) starts standing on top of the 2-row ground at
# rows 13-14 (y=416), so his y = 416 - 16 = 400. The starting x=32
# is cell 1 in the NES 1-1 layout — the camera-relative "mario at
# the left edge of the world" position from the original.
state.mario = { x: 32.0, y: 400.0, vx: 0.0, vy: 0.0, onGround: true, big: false, alive: true, dir: 1 }
state.goombas = [
  { x: 560.0,  y: 400.0, vx: -16.0, alive: true, walkPhase: 0 },
  { x: 736.0,  y: 400.0, vx: -16.0, alive: true, walkPhase: 0 },
  { x: 912.0,  y: 400.0, vx: -16.0, alive: true, walkPhase: 0 },
  { x: 1216.0, y: 400.0, vx: -16.0, alive: true, walkPhase: 0 },
  { x: 1504.0, y: 400.0, vx: -16.0, alive: true, walkPhase: 0 }
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
playMusic("audio/music.wav")
# Compute the initial camera + visible tiles so the first frame
# (before any client has connected and the 16ms timer starts firing)
# already has the right camera position. Without this the cameraX /
# cameraY fallback in the renderer would be 0/0 and the mario sprite
# would render at his raw world position — clipped off the left edge
# of the 512x480 stage.
buildViewTiles()
let camX = state.mario.x - 8 * state.cellSize
let camY = state.mario.y - 6 * state.cellSize
if (camX < 0) { camX = 0 }
state.cameraX = 0 - camX
state.cameraY = 0 - camY
