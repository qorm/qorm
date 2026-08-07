package audio

import (
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
)

// writeTestWAV encodes a minimal valid 16-bit mono PCM WAV with a few sine
// samples — enough to exercise the decoder.
func writeTestWAV(t *testing.T, path string, samples int) {
	t.Helper()
	var buf bytes.Buffer
	// RIFF header
	buf.WriteString("RIFF")
	dataSize := uint32(samples * 2)
	fileSize := uint32(36) + dataSize
	_ = binary.Write(&buf, binary.LittleEndian, fileSize)
	buf.WriteString("WAVE")
	// fmt chunk
	buf.WriteString("fmt ")
	_ = binary.Write(&buf, binary.LittleEndian, uint32(16)) // chunk size
	_ = binary.Write(&buf, binary.LittleEndian, uint16(1))  // PCM
	_ = binary.Write(&buf, binary.LittleEndian, uint16(1))  // mono
	_ = binary.Write(&buf, binary.LittleEndian, uint32(44100))
	_ = binary.Write(&buf, binary.LittleEndian, uint32(88200)) // byte rate
	_ = binary.Write(&buf, binary.LittleEndian, uint16(2))     // block align
	_ = binary.Write(&buf, binary.LittleEndian, uint16(16))    // bits per sample
	// data chunk
	buf.WriteString("data")
	_ = binary.Write(&buf, binary.LittleEndian, dataSize)
	for i := 0; i < samples; i++ {
		v := int16(i % 256)
		_ = binary.Write(&buf, binary.LittleEndian, v)
	}
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		t.Fatalf("write test wav: %v", err)
	}
}

func TestLoadSoundReadsWAV(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tone.wav")
	writeTestWAV(t, path, 64)

	snd, err := LoadSound(dir, "tone.wav")
	if err != nil {
		t.Fatalf("LoadSound: %v", err)
	}
	if snd.SampleRate != 44100 {
		t.Errorf("SampleRate = %d, want 44100", snd.SampleRate)
	}
	if snd.Channels != 1 {
		t.Errorf("Channels = %d, want 1", snd.Channels)
	}
	if snd.BitsPerSample != 16 {
		t.Errorf("BitsPerSample = %d, want 16", snd.BitsPerSample)
	}
	if len(snd.Samples) != 128 {
		t.Errorf("Samples = %d bytes, want 128", len(snd.Samples))
	}
}

func TestLoadSoundRejectsEmptySrc(t *testing.T) {
	if _, err := LoadSound("/tmp", ""); err == nil {
		t.Fatal("LoadSound(\"\") should error")
	}
}

func TestLoadSoundRejectsAbsolutePath(t *testing.T) {
	if _, err := LoadSound("/tmp", "/etc/passwd"); err == nil {
		t.Fatal("LoadSound with absolute path should error")
	}
}

func TestLoadSoundRejectsPathTraversal(t *testing.T) {
	if _, err := LoadSound("/tmp", "../../etc/passwd"); err == nil {
		t.Fatal("LoadSound with ../ should error")
	}
}

func TestLoadSoundRejectsRemoteSrc(t *testing.T) {
	if _, err := LoadSound("/tmp", "https://example.com/x.wav"); err == nil {
		t.Fatal("LoadSound with https:// should error")
	}
}

func TestLoadSoundRejectsMissingFile(t *testing.T) {
	if _, err := LoadSound(t.TempDir(), "missing.wav"); err == nil {
		t.Fatal("LoadSound with missing file should error")
	}
}

func TestLoadSoundRejectsBadWAV(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.wav")
	if err := os.WriteFile(path, []byte("not a wav file"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadSound(dir, "bad.wav"); err == nil {
		t.Fatal("LoadSound with bad wav should error")
	}
}

func TestRegisterSinkSwaps(t *testing.T) {
	RegisterSink(nil) // restore default
	defer RegisterSink(nil)
	s := ActiveSink()
	if _, ok := s.(nopSink); !ok {
		t.Errorf("default sink = %T, want nopSink", s)
	}
}

func TestNopSinkPlaysAndStops(t *testing.T) {
	var s nopSink
	if err := s.Play(nil, false); err != nil {
		t.Errorf("nopSink.Play: %v", err)
	}
	if err := s.Stop(); err != nil {
		t.Errorf("nopSink.Stop: %v", err)
	}
}

func TestResolveSrcRequiresBaseDir(t *testing.T) {
	if _, err := resolveSrc("", "foo.wav"); err == nil {
		t.Fatal("resolveSrc with empty baseDir should error")
	}
}

// Round-trip: encode a Sound to a temp WAV via encodeWAV, then re-decode it
// via LoadSound and check the bytes match.
func TestEncodeWAVRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "in.wav")
	writeTestWAV(t, path, 32)

	snd, err := LoadSound(dir, "in.wav")
	if err != nil {
		t.Fatalf("LoadSound: %v", err)
	}

	outPath := filepath.Join(dir, "out.wav")
	f, err := os.Create(outPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := encodeWAV(f, snd); err != nil {
		f.Close()
		t.Fatalf("encodeWAV: %v", err)
	}
	f.Close()

	snd2, err := LoadSound(dir, "out.wav")
	if err != nil {
		t.Fatalf("LoadSound (roundtrip): %v", err)
	}
	if snd2.SampleRate != snd.SampleRate || snd2.Channels != snd.Channels ||
		snd2.BitsPerSample != snd.BitsPerSample || len(snd2.Samples) != len(snd.Samples) {
		t.Errorf("roundtrip mismatch: %+v vs %+v", snd, snd2)
	}
	for i, b := range snd.Samples {
		if snd2.Samples[i] != b {
			t.Errorf("sample[%d] = %d, want %d", i, snd2.Samples[i], b)
			break
		}
	}
}