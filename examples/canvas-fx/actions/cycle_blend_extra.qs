# Cycle extra mix-blend-mode. QSS .blendExtra rebinds mixBlendMode.
if (state.blendExtra == "difference") {
  state.blendExtra = "color-dodge"
} else {
  if (state.blendExtra == "color-dodge") { state.blendExtra = "multiply" } else { state.blendExtra = "difference" }
}
