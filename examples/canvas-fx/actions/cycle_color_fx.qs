# Cycle invert → sepia → none. QSS .invertCard rebinds filter.
if (state.colorFx == "invert") {
  state.colorFx = "sepia"
} else {
  if (state.colorFx == "sepia") { state.colorFx = "none" } else { state.colorFx = "invert" }
}
