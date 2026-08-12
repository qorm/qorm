package canvas

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/qorm/qorm/internal/expr"
)

// tetrisAudioFiles is the clip set baked by tools/gentetrisaudio.
var tetrisAudioFiles = []string{
	"move.wav", "rotate.wav", "lock.wav", "drop.wav", "clear.wav",
	"tetris.wav", "levelup.wav", "gameover.wav", "pause.wav", "music.wav",
}

func TestTetrisAudioAssetsPresent(t *testing.T) {
	dir := filepath.Join("..", "..", "..", "examples", "tetris", "audio")
	for _, name := range tetrisAudioFiles {
		path := filepath.Join(dir, name)
		b, err := os.ReadFile(path)
		if err != nil {
			t.Errorf("missing %s: %v", name, err)
			continue
		}
		if len(b) < 44 || string(b[:4]) != "RIFF" || string(b[8:12]) != "WAVE" {
			t.Errorf("%s is not a RIFF/WAVE file (len=%d)", name, len(b))
		}
	}
}

func TestTetrisRestartTriggersAudio(t *testing.T) {
	_, _, rt, _ := tetrisFixture(t)
	log := hookAudioSeq(t)
	if err := rt.DispatchErr("restart", nil); err != nil {
		t.Fatalf("restart: %v", err)
	}
	tetrisNoScriptErr(t, rt)
	if !log.has("loop:audio/music.wav") {
		t.Fatalf("restart should playMusic, got %v", log.snapshot())
	}
}

func TestTetrisMoveRotateAudio(t *testing.T) {
	e, surf, rt, _ := tetrisFixture(t)
	e.DrawFrame(surf)
	log := hookAudioSeq(t)

	e.HandleKey(KeyInput{Key: "left", Down: true})
	tetrisNoScriptErr(t, rt)
	if !log.has("once:audio/move.wav") {
		t.Fatalf("successful left should play move.wav, got %v", log.snapshot())
	}

	log.reset()
	e.HandleKey(KeyInput{Key: "up", Down: true})
	tetrisNoScriptErr(t, rt)
	if !log.has("once:audio/rotate.wav") {
		t.Fatalf("successful rotate should play rotate.wav, got %v", log.snapshot())
	}

	// Walk into the wall: only the first few lefts succeed.
	log.reset()
	for i := 0; i < 10; i++ {
		e.HandleKey(KeyInput{Key: "left", Down: true})
	}
	tetrisNoScriptErr(t, rt)
	moves := log.countPrefix("once:audio/move.wav")
	if moves == 0 || moves > 4 {
		t.Fatalf("wall-limited lefts should play a few move SFX, got %d in %v", moves, log.snapshot())
	}

	// Gravity / soft drop must not spam the move click.
	log.reset()
	rt.Dispatch("tick", nil)
	rt.Dispatch("moveDown", nil)
	tetrisNoScriptErr(t, rt)
	if log.has("once:audio/move.wav") {
		t.Fatalf("tick/soft-drop must stay silent, got %v", log.snapshot())
	}
}

func TestTetrisLockAndClearAudio(t *testing.T) {
	_, _, rt, _ := tetrisFixture(t)
	log := hookAudioSeq(t)

	rt.Dispatch("hardDrop", nil)
	tetrisNoScriptErr(t, rt)
	if !log.has("once:audio/drop.wav") || !log.has("once:audio/lock.wav") {
		t.Fatalf("empty-board hard drop should play drop+lock, got %v", log.snapshot())
	}
	if log.has("once:audio/clear.wav") || log.has("once:audio/tetris.wav") {
		t.Fatalf("no-line drop should not clear, got %v", log.snapshot())
	}

	// Prefill row 19 except the I footprint (cols 3–6), then drop a single.
	rt.Dispatch("restart", nil)
	tetrisNoScriptErr(t, rt)
	board := rt.State["board"].([]any)
	for i := 190; i < 200; i++ {
		if i < 193 || i > 196 {
			board[i] = 1.0
		}
	}
	log.reset()
	rt.Dispatch("hardDrop", nil)
	tetrisNoScriptErr(t, rt)
	if !log.has("once:audio/drop.wav") || !log.has("once:audio/clear.wav") {
		t.Fatalf("single clear should play drop+clear, got %v", log.snapshot())
	}
	if log.has("once:audio/lock.wav") {
		t.Fatalf("a line clear should not also play lock.wav, got %v", log.snapshot())
	}
}

func TestTetrisFourLineAndLevelUpAudio(t *testing.T) {
	_, _, rt, _ := tetrisFixture(t)

	// Vertical I occupies column x+2; spawn x=3 → column 5. Fill the bottom
	// four rows except that column so a rotate + hard drop is a Tetris.
	board := rt.State["board"].([]any)
	for row := 16; row < 20; row++ {
		for x := 0; x < 10; x++ {
			if x != 5 {
				board[row*10+x] = 1.0
			}
		}
	}
	log := hookAudioSeq(t)
	rt.Dispatch("rotate", nil)
	rt.Dispatch("hardDrop", nil)
	tetrisNoScriptErr(t, rt)
	if !log.has("once:audio/tetris.wav") {
		t.Fatalf("4-line clear should play tetris.wav, got %v", log.snapshot())
	}
	if log.has("once:audio/clear.wav") || log.has("once:audio/lock.wav") {
		t.Fatalf("Tetris should not also play clear/lock, got %v", log.snapshot())
	}

	// Nine lines already on the clock + one more single → level 2.
	rt.Dispatch("restart", nil)
	tetrisNoScriptErr(t, rt)
	rt.State["lines"] = 9.0
	rt.State["level"] = 1.0
	board = rt.State["board"].([]any)
	for i := 190; i < 200; i++ {
		if i < 193 || i > 196 {
			board[i] = 1.0
		}
	}
	log.reset()
	rt.Dispatch("hardDrop", nil)
	tetrisNoScriptErr(t, rt)
	if rt.State["level"] != 2.0 {
		t.Fatalf("level = %v, want 2 after 10 lines", rt.State["level"])
	}
	if !log.has("once:audio/clear.wav") || !log.has("once:audio/levelup.wav") {
		t.Fatalf("level-up single should play clear+levelup, got %v", log.snapshot())
	}
}

func TestTetrisPauseAndGameOverAudio(t *testing.T) {
	_, _, rt, _ := tetrisFixture(t)
	log := hookAudioSeq(t)

	rt.Dispatch("togglePause", nil)
	tetrisNoScriptErr(t, rt)
	if !log.has("stop") || !log.has("once:audio/pause.wav") {
		t.Fatalf("pause should stopMusic + pause.wav, got %v", log.snapshot())
	}

	log.reset()
	rt.Dispatch("togglePause", nil)
	tetrisNoScriptErr(t, rt)
	if !log.has("loop:audio/music.wav") {
		t.Fatalf("resume should playMusic, got %v", log.snapshot())
	}

	// Cell (4,1) blocks every spawn pose. Hard drop locks, then spawn tops out.
	rt.Dispatch("restart", nil)
	rt.State["board"].([]any)[14] = 1.0
	log.reset()
	rt.Dispatch("hardDrop", nil)
	tetrisNoScriptErr(t, rt)
	if rt.State["status"] != "over" {
		t.Fatalf("status = %v, want over", rt.State["status"])
	}
	if !log.has("stop") || !log.has("once:audio/gameover.wav") {
		t.Fatalf("top-out should stopMusic + gameover.wav, got %v", log.snapshot())
	}
}

// audioSeq records playSound / playMusic / stopMusic in order.
type audioSeq struct {
	mu    sync.Mutex
	calls []string
}

func hookAudioSeq(t *testing.T) *audioSeq {
	t.Helper()
	log := &audioSeq{}
	expr.SetAudioHandler(log)
	t.Cleanup(func() { expr.SetAudioHandler(nil) })
	return log
}

func (a *audioSeq) PlayOnce(src string) error { a.add("once:" + src); return nil }
func (a *audioSeq) PlayLoop(src string) error { a.add("loop:" + src); return nil }
func (a *audioSeq) Stop() error               { a.add("stop"); return nil }

func (a *audioSeq) add(s string) {
	a.mu.Lock()
	a.calls = append(a.calls, s)
	a.mu.Unlock()
}

func (a *audioSeq) reset() {
	a.mu.Lock()
	a.calls = a.calls[:0]
	a.mu.Unlock()
}

func (a *audioSeq) snapshot() []string {
	a.mu.Lock()
	defer a.mu.Unlock()
	out := append([]string(nil), a.calls...)
	return out
}

func (a *audioSeq) has(want string) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	for _, c := range a.calls {
		if c == want {
			return true
		}
	}
	return false
}

func (a *audioSeq) countPrefix(want string) int {
	a.mu.Lock()
	defer a.mu.Unlock()
	n := 0
	for _, c := range a.calls {
		if c == want || strings.HasPrefix(c, want) {
			n++
		}
	}
	return n
}
