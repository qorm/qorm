# Step skewX 0 → 12 → 24 → 0. Layout box stays 120x72.
if (state.skewDeg >= 24) {
  state.skewDeg = 0
} else {
  state.skewDeg = state.skewDeg + 12
}
