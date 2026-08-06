# restart.qs — the action body; the shared 2048 core (expOf/refreshView/
# spawn/…) lives in lib.qs.

# Back to a fresh game: empty board, zeroed score, the LCG reseeded, then the
# two opening tiles. The scene's onEnter points here too, so first load gets
# the same opening. state.best survives: it is the across-games high score.
state.board = fill(16, 0)
state.score = 0
state.status = "playing"
state.rng = 424242
spawn()
spawn()
refreshView()
