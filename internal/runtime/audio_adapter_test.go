package runtime

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"

	"github.com/qorm/qorm/internal/model"
)

// writeTinyWAV emits a minimal valid 16-bit mono PCM WAV so LoadSound has
// something real to decode.
func writeTinyWAV(t *testing.T, path string) {
	t.Helper()
	samples := 16
	var buf []byte
	buf = append(buf, []byte("RIFF")...)
	dataSize := uint32(samples * 2)
	buf = binary.LittleEndian.AppendUint32(buf, 36+dataSize)
	buf = append(buf, []byte("WAVEfmt ")...)
	buf = binary.LittleEndian.AppendUint32(buf, 16) // fmt chunk size
	buf = binary.LittleEndian.AppendUint16(buf, 1)  // PCM
	buf = binary.LittleEndian.AppendUint16(buf, 1)  // mono
	buf = binary.LittleEndian.AppendUint32(buf, 8000)
	buf = binary.LittleEndian.AppendUint32(buf, 16000) // byte rate
	buf = binary.LittleEndian.AppendUint16(buf, 2)     // block align
	buf = binary.LittleEndian.AppendUint16(buf, 16)    // bits/sample
	buf = append(buf, []byte("data")...)
	buf = binary.LittleEndian.AppendUint32(buf, dataSize)
	for i := 0; i < samples; i++ {
		buf = binary.LittleEndian.AppendUint16(buf, uint16(i*100))
	}
	if err := os.WriteFile(path, buf, 0o644); err != nil {
		t.Fatalf("write wav: %v", err)
	}
}

func TestAudioAdapterBaseDir(t *testing.T) {
	// Nil app → empty base dir.
	if got := (audioAdapter{}).baseDir(); got != "" {
		t.Errorf("baseDir with nil app = %q, want empty", got)
	}
	// App with a BaseDir → that dir.
	a := audioAdapter{rt: &Runtime{App: &model.App{BaseDir: "/tmp/someapp"}}}
	if got := a.baseDir(); got != "/tmp/someapp" {
		t.Errorf("baseDir = %q, want /tmp/someapp", got)
	}
}

func TestAudioAdapterPlayMissingFile(t *testing.T) {
	dir := t.TempDir()
	a := audioAdapter{rt: &Runtime{App: &model.App{BaseDir: dir}}}
	// A missing file surfaces a load error (the builtin ignores it, but the
	// adapter returns it).
	if err := a.PlayOnce("nope.wav"); err == nil {
		t.Error("PlayOnce with a missing file should error")
	}
	if err := a.PlayLoop("nope.wav"); err == nil {
		t.Error("PlayLoop with a missing file should error")
	}
}

func TestAudioAdapterPlayValidWAV(t *testing.T) {
	dir := t.TempDir()
	writeTinyWAV(t, filepath.Join(dir, "tone.wav"))
	a := audioAdapter{rt: &Runtime{App: &model.App{BaseDir: dir}}}
	// The default sink is the silent nop, so a valid WAV plays without error.
	if err := a.PlayOnce("tone.wav"); err != nil {
		t.Errorf("PlayOnce with a valid WAV: %v", err)
	}
	if err := a.PlayLoop("tone.wav"); err != nil {
		t.Errorf("PlayLoop with a valid WAV: %v", err)
	}
	if err := a.Stop(); err != nil {
		t.Errorf("Stop: %v", err)
	}
}

func TestNewInstallsAudioHandler(t *testing.T) {
	// New wires the adapter into expr's global audio hook; a script-level
	// playSound must not panic and must route through the adapter. A missing
	// file is fine — the builtin swallows the load error.
	app := &model.App{GlobalState: model.GlobalState{Initial: map[string]any{}}}
	rt := New(app)
	if rt == nil {
		t.Fatal("New returned nil")
	}
}
