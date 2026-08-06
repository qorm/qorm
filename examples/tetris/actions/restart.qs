# restart.qs — the action body; the shared core (fits/refreshView/refreshNext/spawn/lock) lives in lib.qs.

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
spawn()
refreshView()
