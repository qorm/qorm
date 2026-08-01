package widgets

// G2048 tests split in two: the pure game rules (driven directly with fixed
// seeds) and the widget seams (a real engine routing pointer/key events and
// rasterizing into a headless surface).

import (
	"image"
	"testing"

	"github.com/qorm/qorm/internal/model"
	"github.com/qorm/qorm/internal/render/canvas"
	"github.com/qorm/qorm/internal/runtime"
	"github.com/qorm/qorm/internal/theme"
)

// freezeG2048 pins the seed seam (g2048Seed) for the test's duration.
func freezeG2048(t *testing.T, seed int64) {
	t.Helper()
	old := g2048Seed
	g2048Seed = func() int64 { return seed }
	t.Cleanup(func() { g2048Seed = old })
}

// g2048Engine builds an engine with a scene holding one g2048 node, behind a
// FRESH G2048 registration (restored on cleanup) so per-test games cannot
// leak into each other — the tetris_test.go convention.
func g2048Engine(t *testing.T) (*canvas.Engine, *canvas.HeadlessSurface, *G2048, *model.Node) {
	t.Helper()
	gw := &G2048{games: map[*model.Node]*g2048State{}}
	if old, ok := canvas.LookupWidget("g2048"); ok {
		t.Cleanup(func() { canvas.RegisterWidget("g2048", old) })
	}
	canvas.RegisterWidget("g2048", gw)
	node := &model.Node{Type: "g2048", ID: "game"}
	root := &model.Node{Type: "column", ID: "root", Children: []*model.Node{node}}
	app := &model.App{Entry: "main", Scenes: map[string]*model.Node{"main": root}}
	rt := runtime.New(app)
	rt.Theme = theme.GetDefault()
	e := canvas.NewEngine(rt, canvas.SoftwareRenderer{})
	return e, canvas.NewHeadlessSurface(image.Pt(400, 420)), gw, node
}

// g2048BoardsEqual reports whether two boards hold the same tiles.
func g2048BoardsEqual(a, b *g2048Game) bool {
	for y := 0; y < g2048Size; y++ {
		for x := 0; x < g2048Size; x++ {
			if a.board[y][x] != b.board[y][x] {
				return false
			}
		}
	}
	return true
}

// g2048Tiles counts the non-empty cells.
func g2048Tiles(g *g2048Game) int {
	n := 0
	for y := 0; y < g2048Size; y++ {
		for x := 0; x < g2048Size; x++ {
			if g.board[y][x] != 0 {
				n++
			}
		}
	}
	return n
}

func TestG2048Registered(t *testing.T) {
	if _, ok := canvas.LookupWidget("g2048"); !ok {
		t.Fatal("g2048 must be registered via this package's init")
	}
}

func TestG2048Slide(t *testing.T) {
	cases := []struct {
		name       string
		in         [g2048Size]int
		want       [g2048Size]int
		wantGained int
		wantMoved  bool
	}{
		// The one-merge-per-tile counterexample: [2,2,2,2] merges into two
		// 4s, never a single 8.
		{"pair pairs once", [4]int{2, 2, 2, 2}, [4]int{4, 4, 0, 0}, 8, true},
		{"gap closes first", [4]int{2, 0, 2, 4}, [4]int{4, 4, 0, 0}, 4, true},
		{"leftmost pair wins", [4]int{4, 4, 4, 0}, [4]int{8, 4, 0, 0}, 8, true},
		{"no triple merge", [4]int{2, 2, 4, 4}, [4]int{4, 8, 0, 0}, 12, true},
		{"slide without merge", [4]int{0, 0, 0, 2}, [4]int{2, 0, 0, 0}, 0, true},
		{"packed distinct stays", [4]int{2, 4, 8, 16}, [4]int{2, 4, 8, 16}, 0, false},
		{"empty stays", [4]int{0, 0, 0, 0}, [4]int{0, 0, 0, 0}, 0, false},
	}
	for _, c := range cases {
		got, gained, moved := g2048Slide(c.in)
		if got != c.want || gained != c.wantGained || moved != c.wantMoved {
			t.Errorf("%s: slide(%v) = (%v, %d, %v), want (%v, %d, %v)",
				c.name, c.in, got, gained, moved, c.want, c.wantGained, c.wantMoved)
		}
	}
}

func TestG2048MoveDirections(t *testing.T) {
	// One column/row with a merge, the rest empty: each direction must slide
	// toward its own edge, score the merge, and spawn exactly one new tile.
	setup := func() *g2048Game {
		g := newG2048Game(1)
		g.board = [g2048Size][g2048Size]int{}
		g.score = 0
		return g
	}

	t.Run("up", func(t *testing.T) {
		g := setup()
		g.board[0][0], g.board[1][0], g.board[3][0] = 2, 2, 2
		if !g.move(g2048Up) {
			t.Fatal("up must move")
		}
		if g.board[0][0] != 4 || g.board[1][0] != 2 || g.board[2][0] != 0 || g.board[3][0] != 0 {
			t.Errorf("column 0 after up = %d %d %d %d, want 4 2 0 0",
				g.board[0][0], g.board[1][0], g.board[2][0], g.board[3][0])
		}
		if g.score != 4 {
			t.Errorf("score = %d, want 4", g.score)
		}
		if n := g2048Tiles(g); n != 3 {
			t.Errorf("tiles = %d, want 3 (2 merged results + 1 spawn)", n)
		}
	})

	t.Run("down", func(t *testing.T) {
		g := setup()
		g.board[0][0], g.board[1][0], g.board[3][0] = 2, 2, 2
		if !g.move(g2048Down) {
			t.Fatal("down must move")
		}
		if g.board[3][0] != 4 || g.board[2][0] != 2 || g.board[0][0] != 0 || g.board[1][0] != 0 {
			t.Errorf("column 0 after down = %d %d %d %d, want 0 0 2 4",
				g.board[0][0], g.board[1][0], g.board[2][0], g.board[3][0])
		}
		if g.score != 4 {
			t.Errorf("score = %d, want 4", g.score)
		}
	})

	t.Run("left", func(t *testing.T) {
		g := setup()
		g.board[0][0], g.board[0][1], g.board[0][3] = 2, 2, 2
		if !g.move(g2048Left) {
			t.Fatal("left must move")
		}
		if g.board[0] != [4]int{4, 2, 0, 0} {
			t.Errorf("row 0 after left = %v, want [4 2 0 0]", g.board[0])
		}
	})

	t.Run("right", func(t *testing.T) {
		g := setup()
		g.board[0][0], g.board[0][1], g.board[0][3] = 2, 2, 2
		if !g.move(g2048Right) {
			t.Fatal("right must move")
		}
		if g.board[0] != [4]int{0, 0, 2, 4} {
			t.Errorf("row 0 after right = %v, want [0 0 2 4]", g.board[0])
		}
	})
}

func TestG2048NoMoveStays(t *testing.T) {
	g := newG2048Game(1)
	g.board = [g2048Size][g2048Size]int{{2, 4, 8, 16}}
	g.score = 0
	before := g.board
	if g.move(g2048Left) {
		t.Fatal("sliding a packed distinct row left must not move")
	}
	if g.board != before {
		t.Errorf("a rejected move must not change the board: %v", g.board[0])
	}
	if g.score != 0 {
		t.Errorf("a rejected move must not score, score = %d", g.score)
	}
	if n := g2048Tiles(g); n != 4 {
		t.Errorf("a rejected move must not spawn, tiles = %d, want 4", n)
	}
}

func TestG2048DeterministicSeed(t *testing.T) {
	g1, g2 := newG2048Game(42), newG2048Game(42)
	if !g2048BoardsEqual(g1, g2) {
		t.Fatal("same seed must deal the same opening pair")
	}
	for i, dir := range []int{g2048Left, g2048Up, g2048Right, g2048Down, g2048Left, g2048Down} {
		g1.move(dir)
		g2.move(dir)
		if !g2048BoardsEqual(g1, g2) {
			t.Fatalf("move %d diverged under the same seed", i)
		}
	}
	// Spawns are always 2 or 4.
	g := newG2048Game(7)
	for y := 0; y < g2048Size; y++ {
		for x := 0; x < g2048Size; x++ {
			if v := g.board[y][x]; v != 0 && v != 2 && v != 4 {
				t.Errorf("opening spawn = %d, want 2 or 4", v)
			}
		}
	}
}

func TestG2048WinKeepsPlaying(t *testing.T) {
	g := newG2048Game(1)
	g.board = [g2048Size][g2048Size]int{{1024, 1024}}
	g.move(g2048Left)
	if g.board[0][0] != g2048Win {
		t.Fatalf("1024+1024 must merge into %d, got %d", g2048Win, g.board[0][0])
	}
	if !g.won {
		t.Error("reaching 2048 must set won")
	}
	if g.score != g2048Win {
		t.Errorf("score = %d, want %d", g.score, g2048Win)
	}
	if g.over {
		t.Error("winning must not end the game — it continues")
	}
	// The rules keep sliding after a win (the overlay is widget-level).
	if !g.move(g2048Down) {
		t.Error("a won game must keep accepting moves")
	}
}

func TestG2048LoseDetection(t *testing.T) {
	// A full checkerboard: no empty cell, no equal neighbors — dead.
	g := newG2048Game(1)
	for y := 0; y < g2048Size; y++ {
		for x := 0; x < g2048Size; x++ {
			if (x+y)%2 == 0 {
				g.board[y][x] = 2
			} else {
				g.board[y][x] = 4
			}
		}
	}
	if g.canMove() {
		t.Error("a full checkerboard must have no legal move")
	}
	g.checkEnd()
	if !g.over {
		t.Error("checkEnd on a dead board must end the game")
	}
	// One equal neighbor pair brings it back to life.
	g.board[0][1] = 2
	if !g.canMove() {
		t.Error("an adjacent equal pair must be a legal move")
	}
	g.checkEnd()
	if g.over {
		t.Error("a board with a legal merge must not be over")
	}
	// An over game rejects further slides.
	if g2 := newG2048Game(1); func() bool { g2.over = true; return g2.move(g2048Left) }() {
		t.Error("an over game must reject moves")
	}
}

func TestG2048KeyRoutingThroughEngine(t *testing.T) {
	freezeG2048(t, 7)
	e, surf, gw, node := g2048Engine(t)
	e.DrawFrame(surf)
	g := gw.game(node)

	// Without focus the widget never sees keys: the board stays put.
	before := g.board
	e.HandleKey(canvas.KeyInput{Key: "left", Down: true})
	if g.board != before {
		t.Fatal("an unfocused g2048 must not receive keys")
	}

	// A press focuses the board (engine pointer semantics).
	e.HandlePointer(canvas.PointerInput{Type: canvas.PointerPress, X: 30, Y: 60})
	if e.Inter.Focused != node {
		t.Fatalf("press must focus the g2048 node, focused = %v", e.Inter.Focused)
	}

	// Focused arrows slide in all four directions; the board changes and the
	// score only ever grows.
	g.board = [g2048Size][g2048Size]int{{2, 0, 0, 2}, {0, 2, 0, 0}, {0, 0, 0, 0}, {4, 0, 4, 0}}
	for _, key := range []string{"left", "up", "right", "down"} {
		prev := g.board
		e.HandleKey(canvas.KeyInput{Key: key, Down: true})
		if g.board == prev {
			t.Errorf("%s must slide the board", key)
		}
	}

	// r deals a fresh game any time (two tiles, zero score), BEST survives.
	g.score = 128
	gw.mu.Lock()
	gw.games[node].best = 128
	gw.mu.Unlock()
	e.HandleKey(canvas.KeyInput{Key: "r", Down: true})
	g2 := gw.game(node)
	if g2 == g || g2048Tiles(g2) != 2 || g2.score != 0 {
		t.Errorf("r must deal a fresh two-tile game, got tiles=%d score=%d", g2048Tiles(g2), g2.score)
	}
	gw.mu.Lock()
	best := gw.games[node].best
	gw.mu.Unlock()
	if best != 128 {
		t.Errorf("BEST must survive a restart, best = %d, want 128", best)
	}

	// Escape blurs back out (the engine's generic path) and keys stop flowing.
	e.HandleKey(canvas.KeyInput{Key: "escape", Down: true})
	if e.Inter.Focused != nil {
		t.Error("escape must blur the focused g2048")
	}
}

func TestG2048BestUpdates(t *testing.T) {
	freezeG2048(t, 7)
	e, surf, gw, node := g2048Engine(t)
	e.DrawFrame(surf)
	e.HandlePointer(canvas.PointerInput{Type: canvas.PointerPress, X: 30, Y: 60})
	g := gw.game(node)
	g.board = [g2048Size][g2048Size]int{{4, 4, 8, 8}}
	e.HandleKey(canvas.KeyInput{Key: "left", Down: true})
	if g.score != 24 {
		t.Fatalf("two merges (8+16) must score 24, got %d", g.score)
	}
	gw.mu.Lock()
	best := gw.games[node].best
	gw.mu.Unlock()
	if best != 24 {
		t.Errorf("BEST = %d, want the score 24", best)
	}
	// A smaller score later must not lower BEST.
	g.score = 8
	e.HandleKey(canvas.KeyInput{Key: "up", Down: true})
	gw.mu.Lock()
	best = gw.games[node].best
	gw.mu.Unlock()
	if best != 24 {
		t.Errorf("BEST must be monotonic, best = %d, want 24", best)
	}
}

func TestG2048RenderSmoke(t *testing.T) {
	freezeG2048(t, 7)
	e, surf, _, _ := g2048Engine(t)
	e.DrawFrame(surf) // first frame: the graph must exist before input hit-tests it
	e.HandlePointer(canvas.PointerInput{Type: canvas.PointerPress, X: 30, Y: 60})
	e.DrawFrame(surf)
	img := surf.Frame()

	// The rounded board backdrop: its color fills the gap bands.
	boardPx := 0
	for y := g2048HeaderH; y < g2048HeaderH+g2048Size*g2048CellDef+(g2048Size+1)*g2048GapDef; y++ {
		for x := 0; x < g2048Size*g2048CellDef+(g2048Size+1)*g2048GapDef; x++ {
			if c := img.RGBAAt(x, y); nearColor(c, g2048BoardBg, 4) {
				boardPx++
			}
		}
	}
	if boardPx == 0 {
		t.Error("no board backdrop pixels")
	}

	// Empty cells: the fixed empty-slot color must tile most of the board.
	emptyPx := 0
	for y := g2048HeaderH + g2048GapDef; y < g2048HeaderH+g2048GapDef+g2048CellDef; y++ {
		for x := g2048GapDef; x < g2048GapDef+g2048CellDef; x++ {
			if c := img.RGBAAt(x, y); nearColor(c, g2048Empty, 4) {
				emptyPx++
			}
		}
	}
	if emptyPx < 2000 {
		t.Errorf("empty-cell pixels in the first slot = %d, want a cell's worth (>= 2000)", emptyPx)
	}

	// Valued tiles: the two opening spawns paint the 2/4 palette somewhere.
	tilePx := 0
	c2, _ := g2048TileColors(2)
	c4, _ := g2048TileColors(4)
	for y := g2048HeaderH; y < g2048HeaderH+g2048Size*g2048CellDef+(g2048Size+1)*g2048GapDef; y++ {
		for x := 0; x < g2048Size*g2048CellDef+(g2048Size+1)*g2048GapDef; x++ {
			if c := img.RGBAAt(x, y); nearColor(c, c2, 4) || nearColor(c, c4, 4) {
				tilePx++
			}
		}
	}
	if tilePx < 4000 { // 2 tiles x ~60x60px inside the rounded corners
		t.Errorf("valued-tile pixels = %d, want two tiles' worth (>= 4000)", tilePx)
	}

	// The header's SCORE label: ink pixels in its strip (theme textSecondary
	// on the light scene background).
	inkPx := 0
	for y := 2; y < 16; y++ {
		for x := g2048GapDef; x < 60; x++ {
			if c := img.RGBAAt(x, y); c.R < 200 && c.G < 200 && c.B < 200 {
				inkPx++
			}
		}
	}
	if inkPx == 0 {
		t.Error("no SCORE label pixels in the header")
	}
}

// TestG2048AnimatingSettled pins the no-animation contract: slides are
// instantaneous, so the frame loop never runs for this game.
func TestG2048AnimatingSettled(t *testing.T) {
	freezeG2048(t, 7)
	e, surf, _, _ := g2048Engine(t)
	e.DrawFrame(surf)
	if e.Animating() {
		t.Error("g2048 must never keep the frame loop alive")
	}
	e.HandlePointer(canvas.PointerInput{Type: canvas.PointerPress, X: 30, Y: 60})
	e.HandleKey(canvas.KeyInput{Key: "left", Down: true})
	e.DrawFrame(surf)
	if e.Animating() {
		t.Error("g2048 must stay settled after a slide")
	}
}
