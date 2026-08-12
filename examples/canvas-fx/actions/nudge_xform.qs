# Rotate 15deg and toggle flipX. Layout box stays 56x56.
state.xformRot = state.xformRot + 15
if (state.xformRot >= 360) { state.xformRot = 0 }
state.xformFlip = !state.xformFlip
