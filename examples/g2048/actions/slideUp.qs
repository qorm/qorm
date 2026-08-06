# slideUp.qs — the action body; the shared 2048 core (expOf/refreshView/
# spawn/cellAt/slideLine/canMove/settle/slide) lives in lib.qs.

# Arrow up / swipe up: merge every column toward row 0. A won game keeps sliding.
if (state.status == "playing" || state.status == "won") { slide(2) }
