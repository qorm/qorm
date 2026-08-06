# slideRight.qs — the action body; the shared 2048 core (expOf/refreshView/
# spawn/cellAt/slideLine/canMove/settle/slide) lives in lib.qs.

# Arrow right / swipe right: merge every row toward column 3. A won game keeps sliding.
if (state.status == "playing" || state.status == "won") { slide(1) }
