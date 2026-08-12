# Step uniform scale 0.8 → 1.6 then wrap. Transform does not change layout.
if (state.xformScale >= 1.5) { state.xformScale = 0.8 } else { state.xformScale = state.xformScale + 0.2 }
