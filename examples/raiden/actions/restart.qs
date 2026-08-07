# restart.qs — fresh run: ship back at start, everything cleared, counters
# zeroed, status playing, background music on loop.

state.player = { x: 144, y: 400, lives: 3, invuln: 0, weapon: 1, bombs: 2 }
state.bullets = []
state.enemies = []
state.explosions = []
state.powerups = []
state.stars = []
state.starsFar = []
state.starsNear = []
state.groundScroll = 0
state.terrainType = 0
state.keys = { left: false, right: false, up: false, down: false, fire: false }
state.score = 0
state.tick = 0
state.spawnTimer = 30
state.fireTimer = 0
state.bombFlash = 0
state.boss = { alive: false, x: 160, y: -60, hp: 0, phase: 0 }
state.status = "playing"
playMusic("audio/music.wav")