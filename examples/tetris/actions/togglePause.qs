# P toggles between playing and paused; a finished game stays finished.
if (state.status == "playing") {
  state.status = "paused"
} else {
  if (state.status == "paused") { state.status = "playing" }
}
