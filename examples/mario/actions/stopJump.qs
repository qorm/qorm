# stopJump.qs — jump KEYUP handler. The hold flag is the lever the
# physics step uses to reduce gravity during ascent; clearing it returns
# gravity to the falling rate, so a tap gives a short hop, a hold gives
# a full jump. Setting `jump = false` here too means a fresh keydown is
# the only path to the next jump — the physics step's onGround check
# covers the airborne case, but this makes the contract obvious.

state.keys.jump = false
state.keys.jumpHold = false
