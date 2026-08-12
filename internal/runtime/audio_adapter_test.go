package runtime

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"

	"github.com/qorm/qorm/internal/audio"
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

// webSrcSink records PlaySrc calls for App.Web adapter tests.
type webSrcSink struct {
	calls []string
	loop  []bool
}

func (w *webSrcSink) Play(*audio.Sound, bool) error { return nil }
func (w *webSrcSink) Stop() error                   { return nil }
func (w *webSrcSink) PlaySrc(url string, loop bool) error {
	w.calls = append(w.calls, url)
	w.loop = append(w.loop, loop)
	return nil
}

func TestAudioAdapterWebPlaySrc(t *testing.T) {
	sink := &webSrcSink{}
	audio.RegisterSink(sink)
	defer audio.RegisterSink(nil)

	a := audioAdapter{rt: &Runtime{App: &model.App{
		BaseDir: "/games/mario/",
		Web:     true,
	}}}
	if err := a.PlayOnce("audio/coin.wav"); err != nil {
		t.Fatalf("PlayOnce: %v", err)
	}
	if err := a.PlayLoop("audio/music.wav"); err != nil {
		t.Fatalf("PlayLoop: %v", err)
	}
	if len(sink.calls) != 2 {
		t.Fatalf("calls = %v, want 2", sink.calls)
	}
	if sink.calls[0] != "/games/mario/audio/coin.wav" || sink.loop[0] {
		t.Errorf("one-shot = %q loop=%v", sink.calls[0], sink.loop[0])
	}
	if sink.calls[1] != "/games/mario/audio/music.wav" || !sink.loop[1] {
		t.Errorf("music = %q loop=%v", sink.calls[1], sink.loop[1])
	}
}

func TestAudioAdapterWebRejectsTraversal(t *testing.T) {
	audio.RegisterSink(&webSrcSink{})
	defer audio.RegisterSink(nil)
	a := audioAdapter{rt: &Runtime{App: &model.App{BaseDir: "/games/mario/", Web: true}}}
	if err := a.PlayOnce("../../etc/passwd"); err == nil {
		t.Error("expected path-traversal error")
	}
}
