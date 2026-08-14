package canvas

import (
	"image"
	"path/filepath"
	"testing"

	"github.com/qorm/platform/internal/loader"
	"github.com/qorm/platform/internal/runtime"
	"github.com/qorm/platform/internal/theme"
)

// boardExampleFixture loads examples/board (the sticky-note whiteboard) into a
// headless engine; any loader diagnostic fails the test.
func boardExampleFixture(t *testing.T) (*Engine, *HeadlessSurface, *runtime.Runtime) {
	t.Helper()
	app, err := loader.LoadDir(filepath.Join("..", "..", "..", "examples", "board"))
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
	return NewEngine(rt, SoftwareRenderer{}), NewHeadlessSurface(image.Pt(900, 700)), rt
}

// The board example's full loop: notes render at their state positions, a drag
// moves the NOTE (not the board), a blank-space drag pans, and ctrl-scroll
// zooms — the touch seeds arrive in board space so zoom never skews a drag.
func TestBoardExampleEndToEnd(t *testing.T) {
	e, s, rt := boardExampleFixture(t)
	e.DrawFrame(s)
	if !e.Inter.Board.Active {
		t.Fatal("board scene must be active")
	}
	if e.Inter.Board.Zoom != 1 {
		t.Fatalf("initial zoom = %v, want 1", e.Inter.Board.Zoom)
	}

	// n1 starts at board (90,90) size 210x130. Press inside at (150,120) and
	// drag +30/+20 screen px. Board zoom 1 / pan 0, so the seeded pointer is
	// the board-space position and the note moves by exactly the screen delta.
	e.HandlePointer(PointerInput{Type: PointerPress, X: 150, Y: 120, Buttons: 1})
	if e.Inter.Board.Panning {
		t.Fatal("pressing a note must not start a board pan")
	}
	e.HandlePointer(PointerInput{Type: PointerMove, X: 180, Y: 140, Buttons: 1})
	if got := rt.State["n1x"]; got != float64(120) {
		t.Errorf("after drag n1x = %v, want 120", got)
	}
	if got := rt.State["n1y"]; got != float64(110) {
		t.Errorf("after drag n1y = %v, want 110", got)
	}
	e.HandlePointer(PointerInput{Type: PointerRelease, X: 180, Y: 140})

	// A blank-space drag pans the canvas 1:1.
	e.HandlePointer(PointerInput{Type: PointerPress, X: 700, Y: 600, Buttons: 1})
	e.HandlePointer(PointerInput{Type: PointerMove, X: 720, Y: 620, Buttons: 1})
	if e.Inter.Board.PanX != 20 || e.Inter.Board.PanY != 20 {
		t.Errorf("blank drag pan = (%v,%v), want (20,20)", e.Inter.Board.PanX, e.Inter.Board.PanY)
	}
	e.HandlePointer(PointerInput{Type: PointerRelease, X: 720, Y: 620})

	// Ctrl-scroll (pinch / Ctrl+wheel) zooms the board.
	e.HandlePointer(PointerInput{Type: PointerMove, X: 450, Y: 350})
	e.HandleScroll(ScrollInput{DY: 120, Ctrl: true})
	if z := e.Inter.Board.Zoom; z < 1.09 || z > 1.11 {
		t.Errorf("zoom = %v, want ~1.1", z)
	}
}
