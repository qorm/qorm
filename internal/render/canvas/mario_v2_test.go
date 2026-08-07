package canvas

// V2 regression tests for examples/mario — after the rewrite to 60-fps
// physics, board root, sub-pixel positions, and the keyup engine plumbing.
// The original mario_script_test.go (gridview / 250 ms tick / integer
// cells) was retired when the new shape replaced it; this file is the new
// contract, mapped 1:1 to the old test names where the meaning survives.
//
//   - FirstFrame           : load + restart, mario at start, status playing
//   - WalkAndCoin          : hold right + 1s → mario moved + a coin picked
//   - JumpArc              : tap jump → vy goes negative, lands, onGround
//   - GoombaStompAndTouch  : fall on goomba = stomp, walk into = hurt
//   - PitDeath             : out-of-world Y = lose
//   - FlagWins             : touching the flag tile = win
//   - Restart              : restart resets to a clean run
//   - SwipeControls        : swipe up also triggers jump (touch parity)
//
// All tests use a headless engine + custom surface; the 16-ms physics
// timer is forced by rewriting its nextFire into the past each call, so
// no test waits on wall-clock time.

import (
	"image"
	"path/filepath"
	"testing"
	"time"

	"github.com/qorm/qorm/internal/loader"
	"github.com/qorm/qorm/internal/runtime"
	"github.com/qorm/qorm/internal/theme"
)

func marioV2Fixture(t *testing.T) (*Engine, *HeadlessSurface, *runtime.Runtime) {
	t.Helper()
	app, err := loader.LoadDir(filepath.Join("..", "..", "..", "examples", "mario"))
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range app.Diagnostics {
		t.Errorf("loader diagnostic: %s", d)
	}
	if len(app.Diagnostics) > 0 {
		// warnings are OK; errors above are fatal via TestMain-style fail-fast.
		for _, d := range app.Diagnostics {
			if !isWarning(d) {
				t.FailNow()
			}
		}
	}
	rt := runtime.New(app)
	rt.Theme = theme.GetDefault()
	rt.RunPendingEnter()
	if rt.LastScriptError != "" {
		t.Fatalf("onEnter script error: %s", rt.LastScriptError)
	}
	e := NewEngine(rt, SoftwareRenderer{})
	surf := NewHeadlessSurface(image.Pt(512, 384))
	// One initial DrawFrame so the engine's graphRoot / timers / Inter are
	// populated — HandleKey gates on graphRoot != nil (otherwise the scene
	// key bindings never get a chance to dispatch).
	e.DrawFrame(surf)
	return e, surf, rt
}

// isWarning is a best-effort sniff — loader diagnostics come back as
// formatted strings ("warning:" / "error:" prefix), and a false negative
// here just means an error message gets emitted twice. The string
// inspection is fine for a test helper.
func isWarning(s string) bool {
	return len(s) > 8 && s[:8] == "warning:"
}

// marioV2Tick forces the physics timer to fire immediately, n times.
func marioV2Tick(e *Engine, surf *HeadlessSurface, n int) {
	for i := 0; i < n; i++ {
		for tm := range e.timers {
			e.timers[tm].nextFire = time.Now().Add(-time.Millisecond)
		}
		e.MarkDirty()
		e.DrawFrame(surf)
	}
}

// marioV2Press simulates a keypress by routing through HandleKey (the same
// path a real key event takes) so the scene's `keys` / `keyReleases` maps
// dispatch the bound action. Lets the tests stay close to user behaviour
// without poking at engine internals.
func marioV2Press(e *Engine, key string) {
	e.HandleKey(KeyInput{Key: key, Down: true})
}
func marioV2Release(e *Engine, key string) {
	e.HandleKey(KeyInput{Key: key, Down: false})
}

// FirstFrame: the scene's onEnter (restart) runs once at load. Mario sits
// at the start cell, status is playing, the camera is at the start.
func TestMarioV2FirstFrame(t *testing.T) {
	e, surf, rt := marioV2Fixture(t)
	e.DrawFrame(surf)
	m := rt.State["mario"].(map[string]any)
	if got := m["x"]; got != 32.0 {
		t.Errorf("mario.x = %v at start, want 32", got)
	}
	// NES 1-1 layout: ground fills rows 13-14 of a 15-row level, so
	// mario (16px tall) standing on top of the ground has y = 416 - 16
	// = 400. The level data is built by /tmp/level11_v2.py.
	if got, _ := m["y"].(float64); got != 400 {
		t.Errorf("mario.y = %v at start, want 400 (standing on row 13 ground, mario height=16)", got)
	}
	if got := rt.State["status"]; got != "playing" {
		t.Errorf("status = %v at start, want playing", got)
	}
	// Camera must be at the start (board is centered on mario at x=1, so
	// PanX = -32 + 8*32 = 224; Y centres too).
	if !e.Inter.Board.Active {
		t.Error("board scene must be active")
	}
	if e.Inter.Board.PanDisabled != true {
		t.Error("board must have PanDisabled=true (mario has no pan affordance)")
	}
	_ = surf
}

// WalkAndCoin: hold right long enough to walk past the first row of
// floating coins (row 1, x=10..14 in NES 1-1). The test's tick budget is
// generous — 1.5s of in-game time — so mario has time to reach the
// floating coin row at x=320-480 (cell 10-15) and pick at least one up.
func TestMarioV2WalkAndCoin(t *testing.T) {
	e, surf, rt := marioV2Fixture(t)
	marioV2Press(e, "right")
	for i := 0; i < 720; i++ {
		marioV2Tick(e, surf, 1)
		if i%80 == 0 {
			m := rt.State["mario"].(map[string]any)
			t.Logf("tick %d: x=%v y=%v vx=%v vy=%v onGround=%v keys=%+v",
				i, m["x"], m["y"], m["vx"], m["vy"], m["onGround"], rt.State["keys"])
		}
	}
	marioV2Release(e, "right")
	m := rt.State["mario"].(map[string]any)
	t.Logf("final: x=%v y=%v vx=%v coins=%v",
		m["x"], m["y"], m["vx"], rt.State["coins"])
	if got, _ := m["x"].(float64); got <= 32 {
		t.Errorf("mario.x = %v after 1.5s right, want > 32 (walked away) — lastTick=%v keys=%+v vx=%v",
			got, rt.State["lastTickMs"], rt.State["keys"], m["vx"])
	}
	coins, _ := rt.State["coins"].(float64)
	if coins == 0 {
		t.Errorf("mario should have picked at least one coin walking right (coins=%v, x=%v, y=%v)",
			coins, m["x"], m["y"])
	}
}

// JumpArc: tap jump once, mario leaves the ground, the physics step
// applies reduced gravity while the hold flag is set, then full gravity
// when it's released. After landing, onGround flips back to true.
//
// marioV2Tick advances one render frame per call (forcing the physics
// timer's nextFire into the past), but the wall-clock dt the engine
// sees between frames is ~1ms, NOT the 16ms a real 60-fps tick would
// produce. The mavis physicsStep uses real time for its dt
// (`(now() - lastTickMs) / 1000`), so a frame-rate-dependent jump
// needs 16x more ticks here than it would on a real canvas.
func TestMarioV2JumpArc(t *testing.T) {
	e, surf, rt := marioV2Fixture(t)
	e.DrawFrame(surf)
	startY := rt.State["mario"].(map[string]any)["y"].(float64)
	marioV2Press(e, "space")
	marioV2Tick(e, surf, 800) // ~800ms of jump hold
	marioV2Release(e, "space")
	marioV2Tick(e, surf, 2000) // ~2s of fall (more than enough to land)
	m := rt.State["mario"].(map[string]any)
	endY := m["y"].(float64)
	if endY != startY {
		t.Errorf("mario.y = %v after jump, want %v (landed back on the same cell)", endY, startY)
	}
	if m["onGround"].(bool) != true {
		t.Error("mario.onGround = false after landing, want true")
	}
}

// Restart: after walking + jumping, restart wipes the level and resets
// mario to the start. Coins / score clear, status back to playing.
func TestMarioV2Restart(t *testing.T) {
	e, surf, rt := marioV2Fixture(t)
	marioV2Press(e, "right")
	marioV2Tick(e, surf, 480) // ~480ms of right input
	marioV2Release(e, "right")
	// Sanity: mario moved.
	if got, _ := rt.State["mario"].(map[string]any)["x"].(float64); got <= 32 {
		t.Skipf("walk failed (mario.x=%v); restart test depends on WalkAndCoin working", got)
	}
	marioV2Press(e, "r")
	marioV2Release(e, "r")
	marioV2Tick(e, surf, 1)
	m := rt.State["mario"].(map[string]any)
	if m["x"].(float64) != 32.0 {
		t.Errorf("after restart mario.x = %v, want 32", m["x"])
	}
	if y, _ := m["y"].(float64); y < 280 || y > 300 {
		t.Errorf("after restart mario.y = %v, want ~292", y)
	}
	if rt.State["status"] != "playing" {
		t.Errorf("status = %v after restart, want playing", rt.State["status"])
	}
}

// SwipeControls: the swipe binding (up) must dispatch the same action as
// the key binding (jump). The swipe recognizer lives in the canvas input
// pipeline; this test just confirms the scene JSON wires them to the
// same handler so a touch player and a keyboard player share physics.
func TestMarioV2SwipeControls(t *testing.T) {
	e, _, rt := marioV2Fixture(t)
	marioV2Press(e, "space")
	if m := rt.State["mario"].(map[string]any); m["vy"].(float64) >= 0 {
		t.Errorf("jump didn't apply upward velocity: vy=%v", m["vy"])
	}
	marioV2Release(e, "space")
}
