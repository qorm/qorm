package widgets

// Tetris tests split in two: the pure game rules (driven directly with fixed
// seeds and frozen clocks) and the widget seams (a real engine routing
// pointer/key events and rasterizing into a headless surface).

import (
	"image"
	"image/color"
	"sort"
	"testing"
	"time"

	"github.com/qorm/qorm/internal/model"
	"github.com/qorm/qorm/internal/render/canvas"
	"github.com/qorm/qorm/internal/runtime"
	"github.com/qorm/qorm/internal/theme"
)

// freezeTetris pins the seed and clock seams (tetrisSeed/tetrisNow) for the
// test's duration.
func freezeTetris(t *testing.T, seed int64, now time.Time) {
	t.Helper()
	oldSeed, oldNow := tetrisSeed, tetrisNow
	tetrisSeed = func() int64 { return seed }
	tetrisNow = func() time.Time { return now }
	t.Cleanup(func() { tetrisSeed, tetrisNow = oldSeed, oldNow })
}

// tetrisEngine builds an engine with a scene holding one tetris node, behind
// a FRESH Tetris registration (restored on cleanup) so per-test games cannot
// leak into each other — Animating() is type-level and would otherwise see
// every game an earlier test left running.
func tetrisEngine(t *testing.T) (*canvas.Engine, *canvas.HeadlessSurface, *Tetris, *model.Node) {
	t.Helper()
	tw := &Tetris{games: map[*model.Node]*tetrisState{}}
	if old, ok := canvas.LookupWidget("tetris"); ok {
		t.Cleanup(func() { canvas.RegisterWidget("tetris", old) })
	}
	canvas.RegisterWidget("tetris", tw)
	node := &model.Node{Type: "tetris", ID: "game"}
	root := &model.Node{Type: "column", ID: "root", Children: []*model.Node{node}}
	app := &model.App{Entry: "main", Scenes: map[string]*model.Node{"main": root}}
	rt := runtime.New(app)
	rt.Theme = theme.GetDefault()
	e := canvas.NewEngine(rt, canvas.SoftwareRenderer{})
	return e, canvas.NewHeadlessSurface(image.Pt(400, 420)), tw, node
}

func pointSet(cells [4]image.Point) map[image.Point]bool {
	s := map[image.Point]bool{}
	for _, c := range cells {
		s[c] = true
	}
	return s
}

func TestTetrisRegistered(t *testing.T) {
	if _, ok := canvas.LookupWidget("tetris"); !ok {
		t.Fatal("tetris must be registered via this package's init")
	}
}

func TestTetrisRotationShapes(t *testing.T) {
	// The T piece through its four clockwise turns: down, left, up, right.
	want := [4][4]image.Point{
		{{0, 0}, {1, 0}, {2, 0}, {1, 1}},
		{{2, 0}, {2, 1}, {2, 2}, {1, 1}},
		{{0, 2}, {1, 2}, {2, 2}, {1, 1}},
		{{0, 0}, {0, 1}, {0, 2}, {1, 1}},
	}
	for rot := 0; rot < 4; rot++ {
		got := pointSet(tetrisCellsAt(pieceT, rot, 0, 0))
		for _, c := range want[rot] {
			if !got[c] {
				t.Errorf("T rot %d missing cell %v (got %v)", rot, c, got)
			}
		}
		if len(got) != 4 {
			t.Errorf("T rot %d has %d distinct cells, want 4", rot, len(got))
		}
	}
	// Four quarter-turns return to the base orientation.
	base, full := pointSet(tetrisCellsAt(pieceT, 0, 0, 0)), pointSet(tetrisCellsAt(pieceT, 4, 0, 0))
	for c := range base {
		if !full[c] {
			t.Errorf("T rot 4 must equal rot 0, missing %v", c)
		}
	}
	// The I piece rotates from a horizontal bar to a vertical one.
	gotI := pointSet(tetrisCellsAt(pieceI, 1, 0, 0))
	for _, c := range [4]image.Point{{2, 0}, {2, 1}, {2, 2}, {2, 3}} {
		if !gotI[c] {
			t.Errorf("I rot 1 missing cell %v (got %v)", c, gotI)
		}
	}
	// Rotation composes with the origin offset.
	off := pointSet(tetrisCellsAt(pieceO, 0, 4, 7))
	if !off[image.Point{4, 7}] || !off[image.Point{5, 8}] {
		t.Errorf("O cells must translate by the origin: %v", off)
	}
}

func TestTetrisCollision(t *testing.T) {
	g := newTetrisGame(1)
	// Walls and floor.
	if !g.collides(pieceT, 0, -1, 0) {
		t.Error("T at x=-1 must collide with the left wall")
	}
	if !g.collides(pieceT, 0, tetrisCols-2, 0) {
		t.Error("T past the right wall must collide")
	}
	if g.collides(pieceT, 0, 3, 18) {
		t.Error("T resting on the floor must not collide")
	}
	if !g.collides(pieceT, 0, 3, 19) {
		t.Error("T below the floor must collide")
	}
	// Sideways moves respect the walls.
	g.kind, g.rot, g.ax, g.ay = pieceT, 0, 0, 5
	if g.move(-1) || g.ax != 0 {
		t.Error("move left through the wall must fail in place")
	}
	g.ax = tetrisCols - 3
	if g.move(1) || g.ax != tetrisCols-3 {
		t.Error("move right through the wall must fail in place")
	}
	// Locked cells block: the T bump at (4,1) covers the marked cell.
	g.board[1][4] = 1
	if !g.collides(pieceT, 0, 3, 0) {
		t.Error("T spawn over a locked cell must collide")
	}
	g.board[1][4] = 0
	if g.collides(pieceT, 0, 3, 0) {
		t.Error("T spawn over empty cells must not collide")
	}
}

func TestTetrisLockClearScoreLevel(t *testing.T) {
	t.Run("two lines at level 1 score 300 and level up at 10 lines", func(t *testing.T) {
		g := newTetrisGame(1)
		for y := 18; y <= 19; y++ {
			for x := 0; x < 8; x++ {
				g.board[y][x] = 1
			}
		}
		g.kind, g.rot, g.ax, g.ay = pieceO, 0, 8, 18
		g.lines = 8
		g.lock()
		if g.score != 300 {
			t.Errorf("2-line clear at level 1: score = %d, want 300", g.score)
		}
		if g.lines != 10 || g.level != 2 {
			t.Errorf("after 8+2 lines: lines=%d level=%d, want 10/2", g.lines, g.level)
		}
		for x := 0; x < tetrisCols; x++ {
			if g.board[19][x] != 0 || g.board[18][x] != 0 {
				t.Fatalf("cleared rows must be empty, row18=%v row19=%v", g.board[18], g.board[19])
			}
		}
		if g.over {
			t.Error("lock on a low stack must not end the game")
		}
	})

	t.Run("four lines score 800 times level", func(t *testing.T) {
		g := newTetrisGame(1)
		g.lines, g.level = 10, 2
		for y := 16; y <= 19; y++ {
			for x := 0; x < tetrisCols; x++ {
				if x != 5 {
					g.board[y][x] = 1
				}
			}
		}
		// Vertical I fills column 5, rows 16..19.
		g.kind, g.rot, g.ax, g.ay = pieceI, 1, 3, 16
		g.lock()
		if g.score != 1600 {
			t.Errorf("4-line clear at level 2: score = %d, want 1600", g.score)
		}
		if g.lines != 14 {
			t.Errorf("lines = %d, want 14", g.lines)
		}
	})

	t.Run("one line scores 100", func(t *testing.T) {
		g := newTetrisGame(1)
		for x := 0; x < 8; x++ {
			g.board[19][x] = 1
		}
		g.kind, g.rot, g.ax, g.ay = pieceO, 0, 8, 18
		g.lock()
		if g.score != 100 || g.lines != 1 || g.level != 1 {
			t.Errorf("1-line clear: score=%d lines=%d level=%d, want 100/1/1", g.score, g.lines, g.level)
		}
	})
}

func TestTetrisTopOut(t *testing.T) {
	g := newTetrisGame(1)
	for x := 0; x < tetrisCols; x++ {
		g.board[0][x], g.board[1][x] = 1, 1
	}
	g.spawn()
	if !g.over {
		t.Error("a spawn colliding with the stack must top out the game")
	}
}

func TestTetrisHardDrop(t *testing.T) {
	g := newTetrisGame(7)
	dropped, spawned := g.kind, g.next
	g.hardDrop()
	// The dropped piece's cells are locked into the stack, resting on the floor.
	locked := 0
	maxY := -1
	for y := 0; y < tetrisRows; y++ {
		for x := 0; x < tetrisCols; x++ {
			if g.board[y][x] == dropped+1 {
				locked++
				if y > maxY {
					maxY = y
				}
			}
		}
	}
	if locked != 4 {
		t.Errorf("hard drop must lock 4 cells, found %d", locked)
	}
	if maxY != tetrisRows-1 {
		t.Errorf("hard drop must rest on the floor (max locked y = %d, want %d)", maxY, tetrisRows-1)
	}
	if g.score != 0 {
		t.Errorf("no rows cleared: score = %d, want 0", g.score)
	}
	if g.over {
		t.Error("hard drop onto an empty board must not end the game")
	}
	if g.kind != spawned {
		t.Errorf("after the lock the previewed piece must spawn (kind = %d, want %d)", g.kind, spawned)
	}
}

func TestTetrisGravityClock(t *testing.T) {
	g := newTetrisGame(1)
	t0 := time.Unix(1000, 0)
	g.lastFall = t0
	y0 := g.ay

	g.advanceGravity(t0.Add(799 * time.Millisecond))
	if g.ay != y0 {
		t.Errorf("799ms < level-1 interval: ay = %d, want %d", g.ay, y0)
	}
	g.advanceGravity(t0.Add(800 * time.Millisecond))
	if g.ay != y0+1 {
		t.Errorf("one interval elapsed: ay = %d, want %d", g.ay, y0+1)
	}
	g.advanceGravity(t0.Add(2400 * time.Millisecond))
	if g.ay != y0+3 {
		t.Errorf("two more intervals: ay = %d, want %d", g.ay, y0+3)
	}

	// Interval table: 800ms at level 1, -70ms per level, floored at 100ms.
	for level, want := range map[int]time.Duration{1: 800, 2: 730, 11: 100, 12: 100} {
		if got := tetrisFallInterval(level); got != want*time.Millisecond {
			t.Errorf("fall interval at level %d = %v, want %v", level, got, want*time.Millisecond)
		}
	}

	// A paused game holds still and re-arms its clock instead of bursting.
	g.paused = true
	g.advanceGravity(t0.Add(10 * time.Second))
	if g.ay != y0+3 {
		t.Errorf("paused: ay = %d, want %d", g.ay, y0+3)
	}
	if g.lastFall != t0.Add(10*time.Second) {
		t.Errorf("paused clock must track the wall clock, lastFall = %v", g.lastFall)
	}
}

func TestTetrisDeterministicSeed(t *testing.T) {
	g1, g2 := newTetrisGame(42), newTetrisGame(42)
	if g1.kind != g2.kind || g1.next != g2.next {
		t.Fatalf("same seed must deal the same opening: (%d,%d) vs (%d,%d)", g1.kind, g1.next, g2.kind, g2.next)
	}
	for i := 0; i < 14; i++ {
		if a, b := g1.pull(), g2.pull(); a != b {
			t.Fatalf("pull %d diverged: %d vs %d", i, a, b)
		}
	}
	// The bag is a permutation of all seven pieces.
	g3 := newTetrisGame(3)
	g3.bag = nil
	var bag []int
	for i := 0; i < 7; i++ {
		bag = append(bag, g3.pull())
	}
	sort.Ints(bag)
	for i, v := range bag {
		if v != i {
			t.Fatalf("7-bag must be a permutation of 0..6, got %v", bag)
		}
	}
}

func TestTetrisKeyRoutingThroughEngine(t *testing.T) {
	freezeTetris(t, 7, time.Unix(1000, 0))
	e, surf, tw, node := tetrisEngine(t)
	e.DrawFrame(surf)
	g := tw.game(node)

	// Without focus the widget never sees keys: the piece stays put.
	ax0 := g.ax
	e.HandleKey(canvas.KeyInput{Key: "left", Down: true})
	if g.ax != ax0 {
		t.Fatal("an unfocused tetris must not receive keys")
	}

	// A press focuses the board (engine pointer semantics).
	e.HandlePointer(canvas.PointerInput{Type: canvas.PointerPress, X: 30, Y: 30})
	if e.Inter.Focused != node {
		t.Fatalf("press must focus the tetris node, focused = %v", e.Inter.Focused)
	}

	// Focused keys drive the piece.
	e.HandleKey(canvas.KeyInput{Key: "left", Down: true})
	if g.ax != ax0-1 {
		t.Errorf("left: ax = %d, want %d", g.ax, ax0-1)
	}
	e.HandleKey(canvas.KeyInput{Key: "right", Down: true})
	if g.ax != ax0 {
		t.Errorf("right: ax = %d, want %d", g.ax, ax0)
	}
	e.HandleKey(canvas.KeyInput{Key: "x", Down: true})
	if g.rot != 1 {
		t.Errorf("x must rotate clockwise: rot = %d, want 1", g.rot)
	}
	e.HandleKey(canvas.KeyInput{Key: "z", Down: true})
	if g.rot != 0 {
		t.Errorf("z must rotate back: rot = %d, want 0", g.rot)
	}
	e.HandleKey(canvas.KeyInput{Key: "down", Down: true})
	if g.ay != 1 {
		t.Errorf("down must soft-drop one row: ay = %d, want 1", g.ay)
	}

	// Space hard-drops and locks.
	e.HandleKey(canvas.KeyInput{Key: "space", Down: true})
	locked := 0
	for y := 0; y < tetrisRows; y++ {
		for x := 0; x < tetrisCols; x++ {
			if g.board[y][x] != 0 {
				locked++
			}
		}
	}
	if locked != 4 {
		t.Errorf("space must hard-drop and lock 4 cells, found %d", locked)
	}

	// Pause freezes the piece; game keys fall through without effect.
	e.HandleKey(canvas.KeyInput{Key: "p", Down: true})
	if !g.paused {
		t.Fatal("p must pause the game")
	}
	ax, ay := g.ax, g.ay
	e.HandleKey(canvas.KeyInput{Key: "left", Down: true})
	e.HandleKey(canvas.KeyInput{Key: "space", Down: true})
	if g.ax != ax || g.ay != ay {
		t.Error("a paused game must ignore movement keys")
	}
	e.HandleKey(canvas.KeyInput{Key: "p", Down: true})
	if g.paused {
		t.Error("p must resume the game")
	}

	// r on a finished game deals a fresh one.
	g.over = true
	e.HandleKey(canvas.KeyInput{Key: "r", Down: true})
	if g2 := tw.game(node); g2 == g || g2.over || g2.score != 0 {
		t.Errorf("r must restart a finished game, got %+v", g2)
	}

	// Escape blurs back out (the engine's generic path) and keys stop flowing.
	e.HandleKey(canvas.KeyInput{Key: "escape", Down: true})
	if e.Inter.Focused != nil {
		t.Error("escape must blur the focused tetris")
	}
}

func TestTetrisRenderSmoke(t *testing.T) {
	freezeTetris(t, 7, time.Unix(1000, 0))
	e, surf, _, _ := tetrisEngine(t)
	e.DrawFrame(surf) // first frame: the graph must exist before input hit-tests it
	e.HandlePointer(canvas.PointerInput{Type: canvas.PointerPress, X: 30, Y: 30})
	e.HandleKey(canvas.KeyInput{Key: "space", Down: true}) // lock a piece at the bottom
	e.DrawFrame(surf)
	img := surf.Frame()

	// The board border ring along the top edge (stroke over the dark board).
	borderPx := 0
	for x := 0; x < tetrisCols*tetrisCellDef; x++ {
		if c := img.RGBAAt(x, 0); c != tetrisBoardBg {
			borderPx++
		}
	}
	if borderPx == 0 {
		t.Error("no border pixels along the board's top edge")
	}

	// The hard-dropped piece: palette-colored pixels in the bottom rows.
	piecePx := 0
	for y := 16 * tetrisCellDef; y < tetrisRows*tetrisCellDef; y++ {
		for x := 0; x < tetrisCols*tetrisCellDef; x++ {
			c := img.RGBAAt(x, y)
			for _, p := range tetrisPieces {
				if nearColor(c, p.col, 12) {
					piecePx++
					break
				}
			}
		}
	}
	if piecePx < 400 { // 4 cells x ~16x16px, minus the 1px grid insets
		t.Errorf("locked piece pixels = %d, want a piece's worth (>= 400)", piecePx)
	}

	// The sidebar's SCORE label: ink pixels in its strip (theme textSecondary
	// on the light scene background).
	inkPx := 0
	for y := 106; y < 121; y++ {
		for x := 186; x < 262; x++ {
			if c := img.RGBAAt(x, y); c.R < 200 && c.G < 200 && c.B < 200 {
				inkPx++
			}
		}
	}
	if inkPx == 0 {
		t.Error("no SCORE label pixels in the sidebar")
	}
}

func TestTetrisPauseStopsFrameLoop(t *testing.T) {
	freezeTetris(t, 7, time.Unix(1000, 0))
	e, surf, tw, node := tetrisEngine(t)
	e.DrawFrame(surf)
	if !e.Animating() {
		t.Fatal("a running game must keep the frame loop alive")
	}

	e.HandlePointer(canvas.PointerInput{Type: canvas.PointerPress, X: 30, Y: 30})
	e.HandleKey(canvas.KeyInput{Key: "p", Down: true})
	e.DrawFrame(surf)
	if e.Animating() {
		t.Error("a paused game must settle the frame loop")
	}

	e.HandleKey(canvas.KeyInput{Key: "p", Down: true})
	e.DrawFrame(surf)
	if !e.Animating() {
		t.Error("a resumed game must keep the frame loop alive")
	}

	tw.game(node).over = true
	e.MarkDirty()
	e.DrawFrame(surf)
	if e.Animating() {
		t.Error("a finished game must settle the frame loop")
	}
}

// nearColor reports whether two colors agree within a per-channel tolerance.
func nearColor(a, b color.RGBA, tol uint8) bool {
	d := func(x, y uint8) uint8 {
		if x > y {
			return x - y
		}
		return y - x
	}
	return d(a.R, b.R) <= tol && d(a.G, b.G) <= tol && d(a.B, b.B) <= tol && d(a.A, b.A) <= tol
}
