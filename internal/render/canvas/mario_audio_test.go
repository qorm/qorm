package canvas

import (
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/qorm/qorm/internal/expr"
	"github.com/qorm/qorm/internal/loader"
	"github.com/qorm/qorm/internal/runtime"
	"github.com/qorm/qorm/internal/theme"
)

// TestMarioRestartTriggersAudio hooks a recording audio handler AFTER the
// runtime is built (the runtime installs its own adapter in New()), then
// dispatches restart to drive playMusic.
func TestMarioRestartTriggersAudio(t *testing.T) {
	app, err := loader.LoadDir(filepath.Join("..", "..", "..", "examples", "mario"))
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range app.Diagnostics {
		t.Errorf("loader diagnostic: %s", d)
	}
	if len(app.Diagnostics) != 0 {
		t.FailNow()
	}
	rt := runtime.New(app)
	rt.Theme = theme.GetDefault()
	rt.RunPendingEnter()

	// After runtime.New() set its own adapter, swap ours in.
	var calls atomic.Int32
	expr.SetAudioHandler(&recordingAudio{onCall: func(op, src string) {
		calls.Add(1)
		t.Logf("audio call: %s %q", op, src)
	}})
	defer expr.SetAudioHandler(nil)

	// Dispatch restart — should call playMusic("audio/music.wav").
	if err := rt.DispatchErr("restart", nil); err != nil {
		t.Fatalf("restart dispatch: %v", err)
	}
	if got := calls.Load(); got == 0 {
		t.Error("restart did not invoke any audio calls (expected playMusic)")
	}
}

// recordingAudio records every call so tests can assert what the runtime
// asked the audio layer to do.
type recordingAudio struct {
	onCall func(op, src string)
}

func (r *recordingAudio) PlayOnce(src string) error { r.onCall("once", src); return nil }
func (r *recordingAudio) PlayLoop(src string) error { r.onCall("loop", src); return nil }
func (r *recordingAudio) Stop() error                { r.onCall("stop", ""); return nil }
