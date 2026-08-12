# Cycle tint hex. White would be identity; we stay on saturated colours.
if (state.tintHex == "#ff6b6b") {
  state.tintHex = "#4ecdc4"
} else {
  if (state.tintHex == "#4ecdc4") { state.tintHex = "#ffe66d" } else { state.tintHex = "#ff6b6b" }
}
