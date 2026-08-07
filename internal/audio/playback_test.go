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
