package canvas

// Regression tests for examples/mario — a small platformer written as a pure
// QORM app whose LOGIC lives in qscript (actions/*.qs, shared core in
// actions/lib.qs), driven through the canvas engine: arrow keys and swipes
// move and jump, the physics timer applies gravity, coins score, a patrolling
// goomba is deadly from the side but stompable from above, pits kill, and the
// flag at the course's end wins. Any script failure surfaces on
// rt.LastScriptError and fails the test.

import (
	"image"
	"path/filepath"
	"testing"
	"time"

	"github.com/qorm/qorm/internal/loader"
	"github.com/qorm/qorm/internal/runtime"
	"github.com/qorm/qorm/internal/theme"
)

// marioFixture loads examples/mario into a headless engine and runs the
// scene's onEnter once (restart resets the level and deals Mario his start).
// Any loader diagnostic fails the test: the example must compile clean —
// including the shared lib and every action's script.
func marioFixture(t *testing.T) (*Engine, *HeadlessSurface, *runtime.Runtime) {
	t.Helper()
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
	if rt.LastScriptError != "" {
		t.Fatalf("onEnter script error: %s", rt.LastScriptError)
	}
	return NewEngine(rt, SoftwareRenderer{}), NewHeadlessSurface(image.Pt(500, 760)), rt
}

func marioOf(rt *runtime.Runtime) map[string]any { return rt.State["mario"].(map[string]any) }
func goombaOf(rt *runtime.Runtime) map[string]any { return rt.State["goomba"].(map[string]any) }

// marioTicks forces the physics timer's deadline n times and renders, so one
// gravity step elapses per call without waiting on real time.
func marioTicks(e *Engine, surf *HeadlessSurface, n int) {
	for i := 0; i < n; i++ {
		for tm := range e.timers {
			e.timers[tm].nextFire = time.Now().Add(-time.Millisecond)
		}
		e.MarkDirty()
		e.DrawFrame(surf)
	}
}

// The first frame mounts the whole 16x12 board and resets to a fresh run.
func TestMarioScriptFirstFrame(t *testing.T) {
	e, surf, rt := marioFixture(t)
	e.DrawFrame(surf)
	if got := marioOf(rt)["x"]; got != 1.0 {
		t.Fatalf("mario.x = %v at start, want 1", got)
	}
	if s := rt.State["status"]; s != "playing" {
		t.Fatalf("status = %v at start, want playing", s)
	}
	ln := Measure(e.RT.App.EntryRoot(), rt, &e.Inter, 1)
	g := tetrisFind(ln, "boardgrid")
	if g == nil || len(g.Children) != 192 {
		n := 0
		if g != nil {
			n = len(g.Children)
		}
		t.Fatalf("boardgrid children = %d, want 192", n)
	}
}

// Keys dispatch through the scene's `keys` map: walking right onto the coin
// at (2,9) picks it up (tile cleared, +100), and a wall/brick blocks further
// movement.
func TestMarioScriptWalkAndCoin(t *testing.T) {
	e, surf, rt := marioFixture(t)
	e.DrawFrame(surf)

	e.HandleKey(KeyInput{Key: "right", Down: true})
	tetrisNoScriptErr(t, rt)
	if x := marioOf(rt)["x"]; x != 2.0 {
		t.Fatalf("mario.x = %v after right, want 2", x)
	}
	if got := rt.State["coins"]; got != 1.0 {
		t.Fatalf("coins = %v, want 1", got)
	}
	if got := rt.State["score"]; got != 100.0 {
		t.Fatalf("score = %v, want 100", got)
	}
	// Walking left back to the start and into the wall stops at x0.
	e.HandleKey(KeyInput{Key: "left", Down: true})
	e.HandleKey(KeyInput{Key: "left", Down: true})
	e.HandleKey(KeyInput{Key: "left", Down: true})
	tetrisNoScriptErr(t, rt)
	if x := marioOf(rt)["x"]; x != 0.0 {
		t.Fatalf("mario.x = %v after walking into the wall, want 0", x)
	}
}

// Jumping spends a 3-cell rise budget against gravity, then Mario falls back
// to the ground — the physics timer drives the whole arc with no further input.
func TestMarioScriptJumpArc(t *testing.T) {
	e, surf, rt := marioFixture(t)
	e.DrawFrame(surf)

	y0 := marioOf(rt)["y"].(float64)
	e.HandleKey(KeyInput{Key: "up", Down: true})
	tetrisNoScriptErr(t, rt)
	if r := marioOf(rt)["rise"]; r != 3.0 {
		t.Fatalf("rise = %v after jump, want 3", r)
	}
	marioTicks(e, surf, 1)
	tetrisNoScriptErr(t, rt)
	if y := marioOf(rt)["y"]; y != y0-1 {
		t.Fatalf("mario.y = %v one tick into the jump, want %v", y, y0-1)
	}
	// Run the arc to completion: Mario lands back on the ground.
	marioTicks(e, surf, 10)
	tetrisNoScriptErr(t, rt)
	if y := marioOf(rt)["y"]; y != y0 {
		t.Fatalf("mario.y = %v after the arc, want back at %v", y, y0)
	}
	if g := marioOf(rt)["onGround"]; g != true {
		t.Fatalf("onGround = %v after landing, want true", g)
	}
	// A grounded Mario can jump; the flag is still far, so nothing wins yet.
	if s := rt.State["status"]; s != "playing" {
		t.Fatalf("status = %v mid-course, want playing", s)
	}
}

// Falling onto the goomba from above stomps it (+200, it dies, Mario bounces);
// walking into it from the side is fatal.
func TestMarioScriptGoombaStompAndTouch(t *testing.T) {
	e, surf, rt := marioFixture(t)
	e.DrawFrame(surf)

	// Park the goomba under a falling Mario.
	g := goombaOf(rt)
	g["x"], g["y"], g["dir"] = 7.0, 9.0, 0.0
	m := marioOf(rt)
	m["x"], m["y"], m["onGround"] = 7.0, 7.0, false
	marioTicks(e, surf, 3)
	tetrisNoScriptErr(t, rt)
	if g["alive"] != false {
		t.Fatalf("goomba.alive = %v after a stomp, want false", g["alive"])
	}
	if got := rt.State["score"]; got != 200.0 {
		t.Fatalf("score = %v after a stomp, want 200", got)
	}

	// Restart, then walk Mario into a live goomba from the side.
	rt.Dispatch("restart", nil)
	g = goombaOf(rt)
	g["x"], g["y"], g["dir"] = 4.0, 9.0, 0.0
	m = marioOf(rt)
	m["x"], m["y"] = 3.0, 9.0
	e.HandleKey(KeyInput{Key: "right", Down: true})
	tetrisNoScriptErr(t, rt)
	if s := rt.State["status"]; s != "dead" {
		t.Fatalf("status = %v after walking into the goomba, want dead", s)
	}
}

// Falling off the bottom of the level (a pit) is fatal.
func TestMarioScriptPitDeath(t *testing.T) {
	e, surf, rt := marioFixture(t)
	e.DrawFrame(surf)

	m := marioOf(rt)
	m["x"], m["y"] = 5.0, 9.0 // directly over the pit at x5
	marioTicks(e, surf, 6)
	tetrisNoScriptErr(t, rt)
	if s := rt.State["status"]; s != "dead" {
		t.Fatalf("status = %v after falling into the pit, want dead", s)
	}
}

// Reaching the flag pole ends the course and renders the overlay.
func TestMarioScriptFlagWins(t *testing.T) {
	e, surf, rt := marioFixture(t)
	e.DrawFrame(surf)

	m := marioOf(rt)
	m["x"], m["y"] = 14.0, 9.0 // one cell left of the pole
	e.HandleKey(KeyInput{Key: "right", Down: true})
	tetrisNoScriptErr(t, rt)
	if s := rt.State["status"]; s != "won" {
		t.Fatalf("status = %v at the flag, want won", s)
	}
	e.DrawFrame(surf)
	ln := Measure(e.RT.App.EntryRoot(), rt, &e.Inter, 1)
	if !tetrisHasText(ln, "COURSE CLEAR!") {
		t.Fatal("COURSE CLEAR! overlay must render when status=won")
	}
}

// Restart restores a fresh run: coin re-placed, counters zeroed, Mario home.
func TestMarioScriptRestart(t *testing.T) {
	e, surf, rt := marioFixture(t)
	e.DrawFrame(surf)

	// Pick the coin, score, then restart.
	e.HandleKey(KeyInput{Key: "right", Down: true})
	tetrisNoScriptErr(t, rt)
	rt.Dispatch("restart", nil)
	tetrisNoScriptErr(t, rt)
	if got := rt.State["coins"]; got != 0.0 {
		t.Fatalf("coins = %v after restart, want 0", got)
	}
	if got := rt.State["score"]; got != 0.0 {
		t.Fatalf("score = %v after restart, want 0", got)
	}
	if x := marioOf(rt)["x"]; x != 1.0 {
		t.Fatalf("mario.x = %v after restart, want 1", x)
	}
	if s := rt.State["status"]; s != "playing" {
		t.Fatalf("status = %v after restart, want playing", s)
	}
	if goombaOf(rt)["alive"] != true {
		t.Fatal("goomba must be alive again after restart")
	}
}

// Swipes are the touch counterpart of the arrows: a right swipe moves Mario,
// an up swipe arms a jump.
func TestMarioScriptSwipeControls(t *testing.T) {
	e, surf, rt := marioFixture(t)
	e.DrawFrame(surf)

	// Coordinates land squarely on the (now larger) board — the prior
	// coordinates targeted the older 20-px-cell layout.
	g2048Swipe(e, 80, 300, 280, 305) // swipe right (dx > 0)
	tetrisNoScriptErr(t, rt)
	if x := marioOf(rt)["x"]; x != 2.0 {
		t.Fatalf("mario.x = %v after swipe-right, want 2", x)
	}
	g2048Swipe(e, 200, 320, 205, 200) // swipe up
	tetrisNoScriptErr(t, rt)
	if r := marioOf(rt)["rise"]; r != 3.0 {
		t.Fatalf("rise = %v after swipe-up, want 3 (jump armed)", r)
	}
}
