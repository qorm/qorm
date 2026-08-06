# slideLeft.qs — the action body; the shared 2048 core (expOf/refreshView/
# spawn/cellAt/slideLine/canMove/settle/slide) lives in lib.qs.

# Arrow left / swipe left: merge every row toward column 0. A won game keeps
# sliding.
if (state.status == "playing" || state.status == "won") { slide(0) }
