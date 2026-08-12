# P toggles between playing and paused; a finished game stays finished.
if (state.status == "playing") {
  state.status = "paused"
  stopMusic()
  playSound("audio/pause.wav")
} else {
  if (state.status == "paused") {
    state.status = "playing"
    playMusic("audio/music.wav")
  }
}
