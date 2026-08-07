package audio

import (
	"path/filepath"
	"testing"
)

func TestSmokeLoadMarioCoin(t *testing.T) {
	dir := filepath.Join("..", "..", "examples", "mario")
	snd, err := LoadSound(dir, "audio/coin.wav")
	if err != nil {
		t.Fatalf("LoadSound: %v", err)
	}
	if len(snd.Samples) == 0 {
		t.Fatal("coin.wav decoded to no samples")
	}
	t.Logf("Loaded coin.wav: rate=%d ch=%d bps=%d samples=%d",
		snd.SampleRate, snd.Channels, snd.BitsPerSample, len(snd.Samples))
}
