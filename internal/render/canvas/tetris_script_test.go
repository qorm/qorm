package canvas

// Regression tests for examples/tetris — the classic game written as a pure
// QORM app whose LOGIC lives in qscript actions (action JSON "script"),
// driven through the canvas engine: scene key bindings move and rotate the
// piece, the gravity timer advances it, rows clear and score, and the pause
// / game-over overlays render. Any script failure surfaces on
// rt.LastScriptError and fails the test.

import (
	"image"
	"path/filepath"
	"testing"
	"time"

	"github.com/qorm/qorm/internal/loader"
	"github.com/qorm/qorm/internal/model"
	"github.com/qorm/qorm/internal/runtime"
	"github.com/qorm/qorm/internal/theme"
)

// tetrisFixture loads examples/tetris into a headless engine. Any loader
// diagnostic fails the test: the example must compile clean — including
// every action's script.
func tetrisFixture(t *testing.T) (*Engine, *HeadlessSurface, *runtime.Runtime, *model.App) {
	t.Helper()
	app, err := loader.LoadDir(filepath.Join("..", "..", "..", "examples", "tetris"))
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
	return NewEngine(rt, SoftwareRenderer{}), NewHeadlessSurface(image.Pt(400, 820)), rt, app
}

// tetrisNoScriptErr fails the test when the last dispatched script action
// recorded a runtime error.
func tetrisNoScriptErr(t *testing.T, rt *runtime.Runtime) {
	t.Helper()
	if rt.LastScriptError != "" {
		t.Fatalf("script error: %s", rt.LastScriptError)
	}
}

func tetrisFind(ln *LayoutNode, id string) *LayoutNode {
	if ln == nil {
		return nil
	}
	if ln.Node != nil && ln.Node.ID == id {
		return ln
	}
	for _, c := range ln.Children {
		if f := tetrisFind(c, id); f != nil {
			return f
		}
	}
	return nil
}

func tetrisHasText(ln *LayoutNode, want string) bool {
	if ln == nil {
		return false
	}
	if ln.Text == want {
		return true
	}
	for _, c := range ln.Children {
		if tetrisHasText(c, want) {
			return true
		}
	}
	return false
}

// tetrisFilled counts non-empty board cells.
func tetrisFilled(rt *runtime.Runtime) int {
	n := 0
	for _, v := range rt.State["board"].([]any) {
		if f, _ := v.(float64); f > 0 {
			n++
		}
	}
	return n
}

func tetrisPiece(rt *runtime.Runtime) map[string]any {
	return rt.State["piece"].(map[string]any)
}

// The first frame mounts the whole playfield: the 200-cell board gridview,
// the 16-cell next preview, and the gravity timer keeping the frame loop
// alive.
func TestTetrisScriptFirstFrame(t *testing.T) {
	e, surf, rt, app := tetrisFixture(t)
	e.DrawFrame(surf)
	ln := Measure(app.EntryRoot(), rt, &e.Inter, 1)
	if g := tetrisFind(ln, "boardgrid"); g == nil || len(g.Children) != 200 {
		n := 0
		if g != nil {
			n = len(g.Children)
		}
		t.Fatalf("boardgrid children = %d, want 200", n)
	}
	if g := tetrisFind(ln, "nextgrid"); g == nil || len(g.Children) != 16 {
		t.Fatal("nextgrid must mount 16 cells")
	}
	if n := tetrisFilled(rt); n != 0 {
		t.Fatalf("board starts with %d filled cells, want 0", n)
	}
	if !e.Animating() {
		t.Fatal("the gravity timer must keep the loop alive while playing")
	}
}

// Keys drive the piece through the scene's `keys` map into the script
// actions: arrows move and rotate, down locks on contact, and the gravity
// timer ticks the piece down without any key at all.
func TestTetrisScriptMovesAndGravity(t *testing.T) {
	e, surf, rt, _ := tetrisFixture(t)
	e.DrawFrame(surf)

	e.HandleKey(KeyInput{Key: "left", Down: true})
	if x := tetrisPiece(rt)["x"]; x != 2.0 {
		t.Fatalf("piece.x = %v after left, want 2", x)
	}
	e.HandleKey(KeyInput{Key: "right", Down: true})
	if x := tetrisPiece(rt)["x"]; x != 3.0 {
		t.Fatalf("piece.x = %v after right, want 3", x)
	}
	e.HandleKey(KeyInput{Key: "up", Down: true})
	if r := tetrisPiece(rt)["rot"]; r != 1.0 {
		t.Fatalf("piece.rot = %v after up, want 1", r)
	}
	e.HandleKey(KeyInput{Key: "z", Down: true})
	if r := tetrisPiece(rt)["rot"]; r != 0.0 {
		t.Fatalf("piece.rot = %v after z, want 0", r)
	}
	tetrisNoScriptErr(t, rt)

	// Gravity: force the timer's deadline and render — the piece moves down
	// one row with no key pressed.
	y0 := tetrisPiece(rt)["y"].(float64)
	for tm := range e.timers {
		e.timers[tm].nextFire = time.Now().Add(-time.Millisecond)
	}
	e.MarkDirty()
	e.DrawFrame(surf)
	tetrisNoScriptErr(t, rt)
	if y := tetrisPiece(rt)["y"]; y != y0+1 {
		t.Fatalf("piece.y = %v after timer tick, want %v", y, y0+1)
	}

	// Soft drops move one row each and lock on contact with the floor.
	for i := 0; i < 25; i++ {
		e.HandleKey(KeyInput{Key: "down", Down: true})
	}
	tetrisNoScriptErr(t, rt)
	if n := tetrisFilled(rt); n != 4 {
		t.Fatalf("filled cells = %d after down x25, want 4 (locked piece)", n)
	}
	if s := rt.State["status"]; s != "playing" {
		t.Fatalf("status = %v after floor lock, want playing", s)
	}
}

// A completed row clears and scores 100 x level; the board above it falls.
func TestTetrisScriptLineClearScores(t *testing.T) {
	e, surf, rt, _ := tetrisFixture(t)
	e.DrawFrame(surf)

	// Prefill row 19 except the I piece's footprint (cols 3-6 at spawn x=3),
	// then hard drop: the I completes the row and it clears.
	board := rt.State["board"].([]any)
	for i := 190; i < 200; i++ {
		if i < 193 || i > 196 {
			board[i] = 1.0
		}
	}
	e.HandleKey(KeyInput{Key: "space", Down: true})
	tetrisNoScriptErr(t, rt)
	if got := rt.State["lines"]; got != 1.0 {
		t.Fatalf("lines = %v, want 1", got)
	}
	if got := rt.State["score"]; got != 100.0 {
		t.Fatalf("score = %v, want 100 (single x level 1)", got)
	}
	if n := tetrisFilled(rt); n != 0 {
		t.Fatalf("filled = %d after the clear, want 0 (it was the only row)", n)
	}
}

// The view array the gridview renders is the board plus the falling piece:
// locking removes the piece's cells from the overlay and keeps them on the
// board.
func TestTetrisScriptViewOverlay(t *testing.T) {
	e, surf, rt, _ := tetrisFixture(t)
	e.DrawFrame(surf)

	view := rt.State["view"].([]any)
	// The I spawns covering cells 13-16 (x 3-6, y 1).
	for _, i := range []int{13, 14, 15, 16} {
		if view[i] != 1.0 {
			t.Fatalf("view[%d] = %v, want the falling piece overlaid", i, view[i])
		}
	}
	e.HandleKey(KeyInput{Key: "space", Down: true}) // hard drop + lock + spawn
	tetrisNoScriptErr(t, rt)
	if n := tetrisFilled(rt); n != 4 {
		t.Fatalf("filled = %d after the drop, want 4 (the locked I)", n)
	}
	view = rt.State["view"].([]any)
	board := rt.State["board"].([]any)
	overlaid := 0
	for i := range board {
		bf, _ := board[i].(float64)
		vf, _ := view[i].(float64)
		if vf > bf { // a view cell above the board value is the falling piece
			overlaid++
		}
	}
	if overlaid != 4 {
		t.Fatalf("view shows %d falling-piece cells, want 4", overlaid)
	}
}

// A spawn collision tops out: status flips to over, the overlay renders, and
// the loop settles (the gravity timer hides). R restarts from the manifest.
func TestTetrisScriptTopOutAndRestart(t *testing.T) {
	e, surf, rt, app := tetrisFixture(t)
	e.DrawFrame(surf)

	// Cell (4,1) is covered by EVERY tetromino's spawn pose (x=3, rot=0), so
	// blocking it tops out whatever piece the LCG serves next — and a single
	// blocked cell never completes a row for the lock to clear.
	rt.State["board"].([]any)[14] = 1.0
	e.HandleKey(KeyInput{Key: "space", Down: true}) // locks, then the spawn fails
	tetrisNoScriptErr(t, rt)
	if s := rt.State["status"]; s != "over" {
		t.Fatalf("status = %v, want over", s)
	}
	e.DrawFrame(surf)
	ln := Measure(app.EntryRoot(), rt, &e.Inter, 1)
	if !tetrisHasText(ln, "GAME OVER") {
		t.Fatal("GAME OVER overlay must render when status=over")
	}
	e.DrawFrame(surf)
	if e.Animating() {
		t.Fatal("the loop must settle once the game is over")
	}

	e.HandleKey(KeyInput{Key: "r", Down: true})
	tetrisNoScriptErr(t, rt)
	if s := rt.State["status"]; s != "playing" {
		t.Fatalf("status = %v after restart, want playing", s)
	}
	if n := tetrisFilled(rt); n != 0 {
		t.Fatalf("filled = %d after restart, want 0", n)
	}
	if got := rt.State["score"]; got != 0.0 {
		t.Fatalf("score = %v after restart, want 0", got)
	}
}

// Pause freezes the piece (keys ignored), shows its overlay, and resumes.
func TestTetrisScriptPauseResume(t *testing.T) {
	e, surf, rt, app := tetrisFixture(t)
	e.DrawFrame(surf)

	e.HandleKey(KeyInput{Key: "p", Down: true})
	if s := rt.State["status"]; s != "paused" {
		t.Fatalf("status = %v, want paused", s)
	}
	ln := Measure(app.EntryRoot(), rt, &e.Inter, 1)
	if !tetrisHasText(ln, "PAUSED") {
		t.Fatal("PAUSED overlay must render when status=paused")
	}
	y := tetrisPiece(rt)["y"]
	e.HandleKey(KeyInput{Key: "down", Down: true})
	if tetrisPiece(rt)["y"] != y {
		t.Fatal("keys must not move the piece while paused")
	}
	e.HandleKey(KeyInput{Key: "p", Down: true})
	tetrisNoScriptErr(t, rt)
	if s := rt.State["status"]; s != "playing" {
		t.Fatalf("status = %v after resume, want playing", s)
	}
}

// lockAndSpawn is a dispatchable action of its own (hosts, MCP agents): the
// falling piece merges into the board and the next one enters.
func TestTetrisScriptLockAndSpawnAction(t *testing.T) {
	_, _, rt, _ := tetrisFixture(t)
	first := tetrisPiece(rt)["shapeIdx"]
	rt.Dispatch("lockAndSpawn", nil)
	tetrisNoScriptErr(t, rt)
	if n := tetrisFilled(rt); n != 4 {
		t.Fatalf("filled = %d after lockAndSpawn, want 4", n)
	}
	if tetrisPiece(rt)["shapeIdx"] == first && rt.State["nextIdx"] == first {
		t.Fatal("lockAndSpawn must advance the piece queue")
	}
	if y := tetrisPiece(rt)["y"]; y != 0.0 {
		t.Fatalf("piece.y = %v after spawn, want 0", y)
	}
}
