let i = mod(state.heroFxToken, 4)
if (i == 0) {
  state.fxMode = "wobble"
}
if (i == 1) {
  state.fxMode = "shake"
}
if (i == 2) {
  state.fxMode = "punch"
}
if (i == 3) {
  state.fxMode = "burst"
}
state.heroFxToken = state.heroFxToken + 1
