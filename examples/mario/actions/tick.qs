# tick.qs — one physics frame; the shared core lives in lib.qs. The scene's
# timer drives it while status is playing.

if (state.status == "playing") {
  gravity()
  if (state.status == "playing") {
    stepGoomba()
    touchGoomba()
  }
  collect()
  refreshView()
}
