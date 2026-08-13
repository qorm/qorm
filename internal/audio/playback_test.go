//go:build darwin || linux
// +build darwin linux

package audio

import (
	"path/filepath"
	"testing"
	"time"
)

// TestStdoutSinkPlaysAndStops drives the real platform sink for ~50ms and
// stops it. Skipped silently when no audio tool (afplay/aplay/paplay) is
// installed. On CI without audio, this is just a "doesn't deadlock" check.
func TestStdoutSinkPlaysAndStops(t *testing.T) {
	dir := filepath.Join("..", "..", "examples", "mario")
	snd, err := LoadSound(dir, "audio/coin.wav")
	if err != nil {
		t.Skipf("mario coin.wav not available: %v", err)
	}
	sink := &StdoutSink{}
	if err := sink.Play(snd, false); err != nil {
		t.Skipf("afplay/aplay not available: %v", err)
	}
	// Brief playback — give the OS tool time to actually start the audio.
	time.Sleep(50 * time.Millisecond)
	if err := sink.Stop(); err != nil {
		t.Errorf("Stop: %v", err)
	}
}

// One-shot SFX must not replace the looping music process. Mario jump used
// to kill BGM because StdoutSink had a single afplay slot.
func TestStdoutSinkSFXDoesNotReplaceMusic(t *testing.T) {
	dir := filepath.Join("..", "..", "examples", "mario")
	music, err := LoadSound(dir, "audio/music.wav")
	if err != nil {
		t.Skipf("mario music.wav: %v", err)
	}
	sfx, err := LoadSound(dir, "audio/jump.wav")
	if err != nil {
		t.Skipf("mario jump.wav: %v", err)
	}
	sink := &StdoutSink{}
	if err := sink.Play(music, true); err != nil {
		t.Skipf("afplay/aplay: %v", err)
	}
	// Headless CI has the audio tool but no device: the music process dies
	// almost immediately (and the sink stops respawning — see playMusic).
	// Give it time to declare itself; the slot invariant below only means
	// something while music actually plays.
	time.Sleep(150 * time.Millisecond)
	sink.mu.Lock()
	musicDone := sink.musicDone
	sink.mu.Unlock()
	select {
	case <-musicDone:
		t.Skipf("music process exited immediately — no audio device on this host")
	default:
	}
	sink.mu.Lock()
	musicCmd := sink.music
	sink.mu.Unlock()
	if musicCmd == nil {
		t.Fatal("loop play did not start a music process")
	}
	if err := sink.Play(sfx, false); err != nil {
		t.Fatalf("sfx play: %v", err)
	}
	sink.mu.Lock()
	still := sink.music == musicCmd
	nsfx := len(sink.sfx)
	sink.mu.Unlock()
	if !still {
		t.Error("oneshot SFX replaced the music process")
	}
	if nsfx == 0 {
		t.Error("oneshot SFX did not start an sfx process")
	}
	if err := sink.Stop(); err != nil {
		t.Errorf("Stop: %v", err)
	}
}
