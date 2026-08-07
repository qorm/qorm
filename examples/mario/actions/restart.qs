# restart.qs — back to a fresh run: the level is restored from state.rows0
# (the pristine copy — coins Mario picked are re-placed), entities cleared,
# counters zeroed, the camera back to the start. The scene's onEnter
# points here, so first load opens ready to play.

state.rows = concat(state.rows0)
state.mario = { x: 32.0, y: 292.0, vx: 0.0, vy: 0.0, onGround: true, big: false, alive: true, dir: 1 }
state.goombas = []
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
playMusic("audio/music.wav")
# Compute the initial camera + visible tiles so the first frame
# (before any client has connected and the 16ms timer starts firing)
# already has the right camera position. Without this the cameraX /
# cameraY fallback in the renderer would be 0/0 and the mario sprite
# would render at his raw world position — clipped off the left edge
# of the 512x384 stage.
buildViewTiles()
let camX = state.mario.x - 8 * state.cellSize
let camY = state.mario.y - 6 * state.cellSize
if (camX < 0) { camX = 0 }
state.cameraX = 0 - camX
state.cameraY = 0 - camY
