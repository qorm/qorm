# restart.qs — the action body; the shared core lives in lib.qs.
# Back to a fresh run: the level restored from state.rows0 (the immutable
# copy — coins Mario picked are re-placed), Mario at the start, the goomba
# patrolling, counters zeroed. The scene's onEnter points here, so first
# load opens ready to play.

state.rows = concat(state.rows0)
state.mario = { "x": 1, "y": 9, "rise": 0, "onGround": true }
state.goomba = { "x": 8, "y": 9, "dir": -1, "alive": true }
state.coins = 0
state.score = 0
state.status = "playing"
refreshView()
