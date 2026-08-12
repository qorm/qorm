package canvas

// Regression tests for examples/g2048 — the classic sliding-tile game as a
// pure QORM app whose LOGIC lives in qscript actions (actions/*.qs), driven
// through the canvas engine: the scene's onEnter deals the two opening
// tiles, arrow keys slide and merge the board ([2,2,2,2] -> [4,4] per move,
// never [8]), merges score, a fresh tile spawns only after a move that
// changed the board, a stuck board flips status to over and renders its
// overlay, and R restarts while keeping best. Any script failure surfaces on
// rt.LastScriptError and fails the test.

import (
	"image"
	"path/filepath"
	"testing"

	"github.com/qorm/qorm/internal/loader"
	"github.com/qorm/qorm/internal/model"
	"github.com/qorm/qorm/internal/runtime"
	"github.com/qorm/qorm/internal/theme"
)

// g2048Fixture loads examples/g2048 into a headless engine and runs the
// scene's onEnter once (hosts call RunPendingEnter at their render choke
// point; the entry hook is restart, which deals the two opening tiles). Any
// loader diagnostic fails the test: the example must compile clean —
// including every action's script.
func g2048Fixture(t *testing.T) (*Engine, *HeadlessSurface, *runtime.Runtime, *model.App) {
	t.Helper()
	app, err := loader.LoadDir(filepath.Join("..", "..", "..", "examples", "g2048"))
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
	return NewEngine(rt, SoftwareRenderer{}), NewHeadlessSurface(image.Pt(400, 700)), rt, app
}

func g2048Board(rt *runtime.Runtime) []any { return rt.State["board"].([]any) }

// g2048Filled counts non-empty board cells.
func g2048Filled(rt *runtime.Runtime) int {
	n := 0
	for _, v := range g2048Board(rt) {
		if f, _ := v.(float64); f > 0 {
			n++
		}
	}
	return n
}

// g2048SetBoard zeroes the board, then writes vals from cell 0 row-major —
// the precondition for asserting a slide's exact merge semantics.
func g2048SetBoard(rt *runtime.Runtime, vals ...float64) {
	b := g2048Board(rt)
	for i := range b {
		b[i] = 0.0
	}
	for i, v := range vals {
		b[i] = v
	}
}

// The entry scene's onEnter (restart) deals exactly two opening tiles on a
// fresh board, and the first frame mounts the whole 4x4 gridview — empty
// slots included — with the dealt tiles' labels visible.
func TestG2048ScriptFirstFrame(t *testing.T) {
	e, surf, rt, app := g2048Fixture(t)
	if n := g2048Filled(rt); n != 2 {
		t.Fatalf("filled = %d after onEnter, want 2 opening tiles", n)
	}
	if s := rt.State["status"]; s != "playing" {
		t.Fatalf("status = %v after onEnter, want playing", s)
	}
	e.DrawFrame(surf)
	ln := Measure(app.EntryRoot(), rt, &e.Inter, 1)
	g := tetrisFind(ln, "boardgrid")
	if g == nil || len(g.Children) != 16 {
		n := 0
		if g != nil {
			n = len(g.Children)
		}
		t.Fatalf("boardgrid children = %d, want 16", n)
	}
	if !tetrisHasText(ln, "SCORE") {
		t.Fatal("the SCORE readout must render")
	}
	// The LCG seed in the manifest deals a 2 and a 4; both labels show.
	if !tetrisHasText(g, "2") || !tetrisHasText(g, "4") {
		t.Fatal("the two opening tiles must render their values")
	}
}

// Arrow keys dispatch through the scene's `keys` map into the slide scripts:
// equal neighbours merge once per move ([2,2,2,2] -> [4,4,0,0], scoring
// 4+4), best tracks score, and exactly one new tile spawns because the board
// changed. A slide that changes nothing spawns nothing.
func TestG2048ScriptMergeScores(t *testing.T) {
	e, surf, rt, _ := g2048Fixture(t)
	e.DrawFrame(surf)

	g2048SetBoard(rt, 2, 2, 2, 2)
	rt.State["score"] = 0.0
	e.HandleKey(KeyInput{Key: "left", Down: true})
	tetrisNoScriptErr(t, rt)
	b := g2048Board(rt)
	if b[0] != 4.0 || b[1] != 4.0 {
		t.Fatalf("row after left = %v, want [4 4 ...] (one merge per tile)", b[:4])
	}
	if got := rt.State["score"]; got != 8.0 {
		t.Fatalf("score = %v, want 8 (4+4)", got)
	}
	if got := rt.State["best"]; got != 8.0 {
		t.Fatalf("best = %v, want 8", got)
	}
	if n := g2048Filled(rt); n != 3 {
		t.Fatalf("filled = %d, want 3 (two 4s + one spawn)", n)
	}

	// Already packed against the left edge: a no-op slide spawns nothing.
	g2048SetBoard(rt, 2)
	e.HandleKey(KeyInput{Key: "left", Down: true})
	tetrisNoScriptErr(t, rt)
	if n := g2048Filled(rt); n != 1 {
		t.Fatalf("filled = %d after a no-op slide, want 1 (no spawn)", n)
	}
	if got := rt.State["score"]; got != 8.0 {
		t.Fatalf("score = %v after a no-op slide, want unchanged 8", got)
	}
}

// The four directions share one front-merge core, mapped per key: right
// merges toward column 3, up merges a column toward row 0, down drops a tile
// to the last row.
func TestG2048ScriptDirections(t *testing.T) {
	e, surf, rt, _ := g2048Fixture(t)
	e.DrawFrame(surf)

	g2048SetBoard(rt, 2, 2, 2, 2)
	e.HandleKey(KeyInput{Key: "right", Down: true})
	tetrisNoScriptErr(t, rt)
	if b := g2048Board(rt); b[2] != 4.0 || b[3] != 4.0 {
		t.Fatalf("row after right = %v, want [.. 4 4]", b[:4])
	}

	g2048SetBoard(rt, 2, 0, 0, 0, 2) // two 2s stacked in column 0
	e.HandleKey(KeyInput{Key: "up", Down: true})
	tetrisNoScriptErr(t, rt)
	if b := g2048Board(rt); b[0] != 4.0 {
		t.Fatalf("column after up: b[0] = %v, want 4", b[0])
	}

	g2048SetBoard(rt, 2)
	e.HandleKey(KeyInput{Key: "down", Down: true})
	tetrisNoScriptErr(t, rt)
	if b := g2048Board(rt); b[12] != 2.0 {
		t.Fatalf("column after down: b[12] = %v, want 2 (bottom row)", b[12])
	}
}

// Merging a 2048 flips status to won and renders the overlay — and the game
// keeps going: slides still dispatch and status stays won.
func TestG2048ScriptWinKeepsPlaying(t *testing.T) {
	e, surf, rt, app := g2048Fixture(t)
	e.DrawFrame(surf)

	g2048SetBoard(rt, 1024, 1024)
	e.HandleKey(KeyInput{Key: "left", Down: true})
	tetrisNoScriptErr(t, rt)
	if s := rt.State["status"]; s != "won" {
		t.Fatalf("status = %v, want won", s)
	}
	if b := g2048Board(rt); b[0] != 2048.0 {
		t.Fatalf("b[0] = %v, want 2048", b[0])
	}
	e.DrawFrame(surf)
	ln := Measure(app.EntryRoot(), rt, &e.Inter, 1)
	if !tetrisHasText(ln, "YOU WIN") {
		t.Fatal("YOU WIN overlay must render when status=won")
	}

	e.HandleKey(KeyInput{Key: "right", Down: true})
	tetrisNoScriptErr(t, rt)
	if s := rt.State["status"]; s != "won" {
		t.Fatalf("status = %v after sliding on, want still won", s)
	}
}

// A full board with no equal neighbours is game over — even when the
// attempted slide changes nothing — the overlay renders, and the arrows go
// dead. R restarts: zeroed score, two fresh tiles, best preserved.
func TestG2048ScriptGameOverAndRestart(t *testing.T) {
	e, surf, rt, app := g2048Fixture(t)
	e.DrawFrame(surf)

	// 2/4 checkerboard: full, and no horizontal/vertical pair is equal.
	b := g2048Board(rt)
	for i := range b {
		if (i/4+i%4)%2 == 0 {
			b[i] = 2.0
		} else {
			b[i] = 4.0
		}
	}
	e.HandleKey(KeyInput{Key: "left", Down: true})
	tetrisNoScriptErr(t, rt)
	if s := rt.State["status"]; s != "over" {
		t.Fatalf("status = %v, want over", s)
	}
	e.DrawFrame(surf)
	ln := Measure(app.EntryRoot(), rt, &e.Inter, 1)
	if !tetrisHasText(ln, "GAME OVER") {
		t.Fatal("GAME OVER overlay must render when status=over")
	}

	stuck := g2048Board(rt)[0]
	e.HandleKey(KeyInput{Key: "left", Down: true})
	tetrisNoScriptErr(t, rt)
	if g2048Board(rt)[0] != stuck || g2048Filled(rt) != 16 {
		t.Fatal("arrows must not move the board once the game is over")
	}

	rt.State["best"] = 100.0
	e.HandleKey(KeyInput{Key: "r", Down: true})
	tetrisNoScriptErr(t, rt)
	if s := rt.State["status"]; s != "playing" {
		t.Fatalf("status = %v after restart, want playing", s)
	}
	if got := rt.State["score"]; got != 0.0 {
		t.Fatalf("score = %v after restart, want 0", got)
	}
	if n := g2048Filled(rt); n != 2 {
		t.Fatalf("filled = %d after restart, want 2 opening tiles", n)
	}
	if got := rt.State["best"]; got != 100.0 {
		t.Fatalf("best = %v after restart, want preserved 100", got)
	}
}

// g2048Swipe drags from (x0,y0) to (x1,y1) through the engine's pointer
// stream: press, one move, release — the shape of a finger flick.
func g2048Swipe(e *Engine, x0, y0, x1, y1 float64) {
	e.HandlePointer(PointerInput{Type: PointerPress, X: x0, Y: y0, Buttons: 1})
	e.HandlePointer(PointerInput{Type: PointerMove, X: x1, Y: y1, Buttons: 1})
	e.HandlePointer(PointerInput{Type: PointerRelease, X: x1, Y: y1})
}

// The scene's `swipes` map is the touch counterpart of its `keys`: a
// press-drag-release flick in a dominant direction dispatches the bound
// slide — same merges, same spawn rules as the arrows (assertions mirror the
// key tests above). This is how the same game JSON plays on a phone.
func TestG2048ScriptSwipeSlides(t *testing.T) {
	e, surf, rt, _ := g2048Fixture(t)
	e.DrawFrame(surf)

	g2048SetBoard(rt, 2, 2, 2, 2)
	rt.State["score"] = 0.0
	g2048Swipe(e, 300, 400, 120, 405) // flick left
	tetrisNoScriptErr(t, rt)
	if b := g2048Board(rt); b[0] != 4.0 || b[1] != 4.0 {
		t.Fatalf("row after swipe-left = %v, want [4 4 ...]", b[:4])
	}
	if got := rt.State["score"]; got != 8.0 {
		t.Fatalf("score = %v after swipe-left, want 8 (4+4)", got)
	}

	g2048SetBoard(rt, 2, 0, 0, 0, 2)  // two 2s stacked in column 0
	g2048Swipe(e, 200, 480, 205, 240) // flick up
	tetrisNoScriptErr(t, rt)
	if b := g2048Board(rt); b[0] != 4.0 {
		t.Fatalf("column after swipe-up: b[0] = %v, want 4", b[0])
	}

	g2048SetBoard(rt, 2)
	g2048Swipe(e, 200, 240, 208, 500) // flick down
	tetrisNoScriptErr(t, rt)
	if b := g2048Board(rt); b[12] != 2.0 {
		t.Fatalf("column after swipe-down: b[12] = %v, want 2 (bottom row)", b[12])
	}

	g2048SetBoard(rt, 2, 2)
	g2048Swipe(e, 120, 400, 320, 395) // flick right
	tetrisNoScriptErr(t, rt)
	if b := g2048Board(rt); b[3] != 4.0 {
		t.Fatalf("row after swipe-right = %v, want [.. 4]", b[:4])
	}
}

// A tap (travel below the slop) and a near-diagonal flick (no dominant axis)
// are NOT swipes: the board neither slides nor spawns a tile.
func TestG2048ScriptSwipeRejectsTapsAndDiagonals(t *testing.T) {
	e, surf, rt, _ := g2048Fixture(t)
	e.DrawFrame(surf)

	// A lone tile mid-board would MOVE under any cardinal slide, so an
	// unchanged board proves nothing dispatched.
	g2048SetBoard(rt, 0, 0, 0, 0, 0, 2)
	g2048Swipe(e, 200, 400, 206, 403) // a tap
	g2048Swipe(e, 200, 400, 290, 480) // ambiguous diagonal
	tetrisNoScriptErr(t, rt)
	if b := g2048Board(rt); b[5] != 2.0 || b[4] != 0.0 {
		t.Fatalf("board moved under a tap/diagonal: %v", b[:8])
	}
	if n := g2048Filled(rt); n != 1 {
		t.Fatalf("filled = %d, want 1 (no slide, no spawn)", n)
	}
}

// swipeDirection is the recognizer's verdict table: distance floor, axis
// dominance, and the sign of the dominant travel.
func TestSwipeDirection(t *testing.T) {
	cases := []struct {
		dx, dy float64
		want   string
	}{
		{-100, 5, "left"}, {100, -5, "right"}, {3, -100, "up"}, {-3, 100, "down"},
		{-10, 0, ""},  // below the distance floor
		{100, 90, ""}, // diagonal: no dominant axis
		{0, 0, ""},    // no travel at all
	}
	for _, c := range cases {
		if got := swipeDirection(c.dx, c.dy); got != c.want {
			t.Errorf("swipeDirection(%v, %v) = %q, want %q", c.dx, c.dy, got, c.want)
		}
	}
}

func g2048Token(rt *runtime.Runtime, key string) float64 {
	switch v := rt.State[key].(type) {
	case float64:
		return v
	case int:
		return float64(v)
	}
	return 0
}

// TestG2048MotionFx: a merge slide bumps fxMerge / cellGen / mergeMask.
func TestG2048MotionFx(t *testing.T) {
	_, _, rt, _ := g2048Fixture(t)
	g2048SetBoard(rt, 2, 2, 0, 0)
	rt.State["cellGen"] = make([]any, 16)
	rt.State["mergeMask"] = make([]any, 16)
	for i := 0; i < 16; i++ {
		rt.State["cellGen"].([]any)[i] = 0.0
		rt.State["mergeMask"].([]any)[i] = 0.0
	}
	merge0 := g2048Token(rt, "fxMerge")
	rt.Dispatch("slideLeft", nil)
	if rt.LastScriptError != "" {
		t.Fatalf("slideLeft: %s", rt.LastScriptError)
	}
	if g2048Token(rt, "fxMerge") <= merge0 {
		t.Fatalf("merge should bump fxMerge, got %v board=%v", rt.State["fxMerge"], rt.State["board"])
	}
	if g2048Token(rt, "fxKind") != 2 {
		t.Fatalf("fxKind after merge = %v, want 2", rt.State["fxKind"])
	}
	mask, _ := rt.State["mergeMask"].([]any)
	merged := false
	for _, v := range mask {
		if f, _ := v.(float64); f != 0 {
			merged = true
		}
	}
	if !merged {
		t.Fatal("mergeMask should mark the merged cell")
	}
}
