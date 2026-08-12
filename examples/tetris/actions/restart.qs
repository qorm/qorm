# restart.qs — the action body; the shared core (SHAPES/fits/refreshView/refreshNext/spawn/lock) lives in lib.qs.

# Back to a fresh game: empty board, zeroed counters, the LCG reseeded from
# now() (ms since epoch) so each restart plays a different piece sequence,
# then the same spawn the manifest's initial state encodes (I piece first).
state.board = fill(200, 0)
state.score = 0
state.lines = 0
state.level = 1
state.rng = mod(now(), 2147483646) + 1
state.nextIdx = 0
state.status = "playing"
state.fxClear = 0
state.fxLock = 0
state.fxDrop = 0
state.fxSpawn = 0
state.fxOver = 0
state.fxTetris = 0
state.fxKind = 0
state.clearName = ""
state.flashOn = false
playMusic("audio/music.wav")
spawn()
refreshView()
