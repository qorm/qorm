# stopLeft.qs — keyup handler for left arrow / A. Clears the
# direction flag so the physics step applies friction (frame-by-frame
# velocity decay) instead of accelerating.

state.keys.left = false
