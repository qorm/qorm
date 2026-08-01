package widgets

// G2048 is the second game-mode showcase component (after tetris.go): the
// classic 4x4 sliding-tile game living behind the canvas widget seam
// (internal/render/canvas/widget.go). It composes two OPTIONAL extensions —
// InteractiveWidget (a press focuses the board, which is how it earns the
// keyboard) and KeyWidget (arrows slide, r restarts while focused).
// AnimatedWidget is reported but never animates: slides are instantaneous
// (no tween), so Animating is a constant false and the frame loop stays
// settled between key presses — the engine only repaints on dirty.
//
// The game RULES are pure Go (g2048Game below): no canvas, draw or runtime
// imports, deterministic under an injected seed — tests drive them directly.
// The widget struct only owns per-node state (map[node] per the widget-seam
// contract: the graph is rebuilt every frame, so state is keyed by the
// stable model pointer; the BEST score lives there too, surviving restarts)
// and translates the state into draw primitives.
//
// Tile and board colors are a FIXED palette, deliberately decoupled from the
// theme: players read tiles BY color (the cream-to-gold ladder is the game's
// vocabulary — a red 64, a gold 2048), so re-tinting them per OS light/dark
// would break tile recognition. Theme tokens are used only for the header
// chrome (SCORE/BEST labels and values), which is ordinary UI text.

import (
	"fmt"
	"image"
	"image/color"
	"math/rand/v2"
	"sync"
	"time"

	"github.com/qorm/qorm/internal/model"
	"github.com/qorm/qorm/internal/render/canvas"
	"github.com/qorm/qorm/internal/render/draw"
	"github.com/qorm/qorm/internal/runtime"
)

func init() {
	canvas.RegisterWidget("g2048", &G2048{games: map[*model.Node]*g2048State{}})
}

// ---------------------------------------------------------------------------
// Pure game rules (no rendering imports — deterministic, unit-testable).
// ---------------------------------------------------------------------------

const (
	g2048Size = 4    // board edge, in cells
	g2048Win  = 2048 // the tile that wins
)

// Slide directions.
const (
	g2048Left = iota
	g2048Right
	g2048Up
	g2048Down
)

// g2048Game is one board's full mutable state.
type g2048Game struct {
	board [g2048Size][g2048Size]int // 0 empty, else the tile value (2,4,8,...)
	rng   *rand.Rand

	score int

	won       bool // a g2048Win tile exists (the game CONTINUES past it)
	keepGoing bool // the player dismissed the WIN overlay
	over      bool // no empty cell and no legal merge
}

// newG2048Game deals a fresh game from a seed (injected — tests pass a fixed
// one for deterministic spawns): two tiles on an empty board.
func newG2048Game(seed int64) *g2048Game {
	g := &g2048Game{rng: rand.New(rand.NewPCG(uint64(seed), uint64(seed)^0x9e3779b97f4a7c15))}
	g.spawn()
	g.spawn()
	return g
}

// spawn drops one new tile (90% a 2, 10% a 4) on a random empty cell.
func (g *g2048Game) spawn() {
	var empty []image.Point
	for y := 0; y < g2048Size; y++ {
		for x := 0; x < g2048Size; x++ {
			if g.board[y][x] == 0 {
				empty = append(empty, image.Point{X: x, Y: y})
			}
		}
	}
	if len(empty) == 0 {
		return
	}
	c := empty[g.rng.IntN(len(empty))]
	v := 2
	if g.rng.IntN(10) == 0 {
		v = 4
	}
	g.board[c.Y][c.X] = v
}

// move slides the whole board one step in dir. A step that changes nothing is
// rejected (no spawn, no score); a real step scores its merges, spawns one
// new tile, then re-evaluates the win/lose flags. Returns whether the board
// changed.
func (g *g2048Game) move(dir int) bool {
	if g.over {
		return false
	}
	moved := false
	for i := 0; i < g2048Size; i++ {
		var line [g2048Size]int
		for j := 0; j < g2048Size; j++ {
			line[j] = g.at(dir, i, j)
		}
		out, gained, lineMoved := g2048Slide(line)
		if lineMoved {
			moved = true
		}
		g.score += gained
		for j := 0; j < g2048Size; j++ {
			g.put(dir, i, j, out[j])
		}
	}
	if !moved {
		return false
	}
	g.spawn()
	g.checkEnd()
	return true
}

// at reads the j-th cell of the i-th line in slide order for dir: sliding
// left reads a row left-to-right, right reads it right-to-left, up reads a
// column top-to-bottom, down reads it bottom-to-top.
func (g *g2048Game) at(dir, i, j int) int {
	switch dir {
	case g2048Left:
		return g.board[i][j]
	case g2048Right:
		return g.board[i][g2048Size-1-j]
	case g2048Up:
		return g.board[j][i]
	default: // g2048Down
		return g.board[g2048Size-1-j][i]
	}
}

// put is at's write twin.
func (g *g2048Game) put(dir, i, j, v int) {
	switch dir {
	case g2048Left:
		g.board[i][j] = v
	case g2048Right:
		g.board[i][g2048Size-1-j] = v
	case g2048Up:
		g.board[j][i] = v
	default: // g2048Down
		g.board[g2048Size-1-j][i] = v
	}
}

// g2048Slide compacts one line toward index 0, merging equal neighbors at
// most once each, left to right (the standard rule: [2,2,2,2] becomes [4,4],
// never a single 8). It returns the new line, the points the merges scored,
// and whether anything changed.
func g2048Slide(line [g2048Size]int) (out [g2048Size]int, gained int, moved bool) {
	var vals []int
	for _, v := range line {
		if v != 0 {
			vals = append(vals, v)
		}
	}
	merged := make([]int, 0, g2048Size)
	for i := 0; i < len(vals); i++ {
		if i+1 < len(vals) && vals[i] == vals[i+1] {
			nv := vals[i] * 2
			merged = append(merged, nv)
			gained += nv
			i++ // the merged pair is consumed: no double merges
		} else {
			merged = append(merged, vals[i])
		}
	}
	for i := 0; i < g2048Size; i++ {
		if i < len(merged) {
			out[i] = merged[i]
		}
		if out[i] != line[i] {
			moved = true
		}
	}
	return out, gained, moved
}

// canMove reports whether any legal step remains: an empty cell, or two equal
// neighbors that could merge.
func (g *g2048Game) canMove() bool {
	for y := 0; y < g2048Size; y++ {
		for x := 0; x < g2048Size; x++ {
			v := g.board[y][x]
			if v == 0 {
				return true
			}
			if x+1 < g2048Size && g.board[y][x+1] == v {
				return true
			}
			if y+1 < g2048Size && g.board[y+1][x] == v {
				return true
			}
		}
	}
	return false
}

// checkEnd refreshes the win/lose flags after a step: a g2048Win tile wins
// (the game does NOT stop — won only raises the overlay until the player
// keeps going), and a board with no legal step is over.
func (g *g2048Game) checkEnd() {
	for y := 0; y < g2048Size; y++ {
		for x := 0; x < g2048Size; x++ {
			if g.board[y][x] >= g2048Win {
				g.won = true
			}
		}
	}
	g.over = !g.canMove()
}

// ---------------------------------------------------------------------------
// The widget: per-node state, draw-primitive rendering, input seams.
// ---------------------------------------------------------------------------

const (
	g2048CellDef = 64 // default cell edge, logical px (the `cell` prop)
	g2048GapDef  = 8  // default gap between cells, logical px (the `gap` prop)
	g2048HeaderH = 48 // score row above the board, logical px
)

// Fixed board and tile colors — the classic 2048 ladder, part of the game
// palette (see the file header), not the theme.
var (
	g2048BoardBg = color.RGBA{187, 173, 160, 255}
	g2048Empty   = color.RGBA{205, 193, 180, 255}
	g2048Dim     = color.RGBA{238, 228, 218, 178}
	g2048InkDark = color.RGBA{119, 110, 101, 255}
)

// g2048Seed is the test seam (the spinner.go spinNow convention): freeze it
// to make spawns deterministic.
var g2048Seed = func() int64 { return time.Now().UnixNano() }

// G2048 is the registered widget. All state is per node (games); a cached
// engine Interaction pointer per node lets Record read the live focus — the
// Widget seam carries no interaction state, so HandlePointer/HandleKey stash
// the pointer the engine passes in (its address is stable for the engine's
// lifetime), the textarea.go convention.
type G2048 struct {
	mu    sync.Mutex
	games map[*model.Node]*g2048State
}

type g2048State struct {
	g     *g2048Game
	best  int // survives restarts; the map[node] slot IS the persistence
	inter *canvas.Interaction
}

// stateFor returns (creating on first sight) the node's game state. Callers
// hold w.mu.
func (w *G2048) stateFor(n *model.Node) *g2048State {
	if st, ok := w.games[n]; ok {
		return st
	}
	st := &g2048State{g: newG2048Game(g2048Seed())}
	w.games[n] = st
	return st
}

// game exposes a node's live game (tests and the key handler).
func (w *G2048) game(n *model.Node) *g2048Game {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.stateFor(n).g
}

// Animating is a constant false: slides are instantaneous (no tween), so the
// frame loop never needs to keep ticking for this game — key presses dirty
// the engine and it repaints once.
func (w *G2048) Animating() bool { return false }

// Measure reports the content size: the score header plus the board
// (g2048Size cells + surrounding gaps) square, at the device scale.
func (w *G2048) Measure(n *model.Node, rt *runtime.Runtime, _ map[string]any, scale int) (wd, ht int) {
	if scale < 1 {
		scale = 1
	}
	cell, gap := g2048Geom(n)
	board := g2048Size*cell + (g2048Size+1)*gap
	return board * scale, (g2048HeaderH + board) * scale
}

// g2048Geom reads the `cell` and `gap` props (logical px), clamped to sane
// bands: scene JSON feeds them and the header proportion assumes them.
func g2048Geom(n *model.Node) (cell, gap int) {
	cell = int(propNumDefault(n, "cell", g2048CellDef))
	if cell < 16 {
		cell = 16
	}
	if cell > 128 {
		cell = 128
	}
	gap = int(propNumDefault(n, "gap", g2048GapDef))
	if gap < 2 {
		gap = 2
	}
	if gap > 32 {
		gap = 32
	}
	return cell, gap
}

// g2048TileColors maps a tile value to its fixed background and foreground —
// the classic ladder: dark ink on the cream 2/4, light ink from 8 up.
func g2048TileColors(v int) (bg, fg color.RGBA) {
	dark := g2048InkDark
	light := color.RGBA{249, 246, 242, 255}
	switch v {
	case 2:
		return color.RGBA{238, 228, 218, 255}, dark
	case 4:
		return color.RGBA{237, 224, 200, 255}, dark
	case 8:
		return color.RGBA{242, 177, 121, 255}, light
	case 16:
		return color.RGBA{245, 149, 99, 255}, light
	case 32:
		return color.RGBA{246, 124, 95, 255}, light
	case 64:
		return color.RGBA{246, 94, 59, 255}, light
	case 128:
		return color.RGBA{237, 207, 114, 255}, light
	case 256:
		return color.RGBA{237, 204, 97, 255}, light
	case 512:
		return color.RGBA{237, 200, 80, 255}, light
	case 1024:
		return color.RGBA{237, 197, 63, 255}, light
	case 2048:
		return color.RGBA{237, 194, 46, 255}, light
	default: // beyond 2048: the super-tile near-black
		return color.RGBA{60, 58, 50, 255}, light
	}
}

// g2048FontSize scales the tile number to the digit count: two-digit tiles
// get the big face, four-digit ones shrink to fit.
func g2048FontSize(cell, v int) float64 {
	digits := 1
	for x := v; x >= 10; x /= 10 {
		digits++
	}
	switch {
	case digits <= 2:
		return float64(cell) * 0.44
	case digits == 3:
		return float64(cell) * 0.36
	default:
		return float64(cell) * 0.28
	}
}

// Record paints the frame: the score header (SCORE/BEST, theme-colored UI
// text), the rounded board backdrop, the empty cells, the valued tiles with
// their numbers, and any overlay (win / game over / unfocused hint) — all
// draw primitives.
func (w *G2048) Record(ln *canvas.LayoutNode, rt *runtime.Runtime, scale int) draw.Node {
	if ln.Width <= 0 || ln.Height <= 0 {
		return nil
	}
	if scale < 1 {
		scale = 1
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	st := w.stateFor(ln.Node)
	g := st.g
	focused := st.inter != nil && st.inter.Focused == ln.Node

	cl, gp := g2048Geom(ln.Node)
	cell, gap := cl*scale, gp*scale
	bw := float64(g2048Size*cell + (g2048Size+1)*gap)
	by := float64(g2048HeaderH * scale)

	root := draw.NewGroup()

	// Header: SCORE at the left, BEST at the right — theme chrome.
	ink := themeColor(rt, "text", color.RGBA{29, 29, 31, 255})
	dim := themeColor(rt, "textSecondary", color.RGBA{134, 134, 139, 255})
	lfs := float64(10 * scale)
	vfs := float64(18 * scale)
	gx := float64(gap)
	root.AddChild(g2048Text("SCORE", gx, float64(2*scale), lfs, dim))
	sv := g2048Text(fmt.Sprintf("%d", g.score), gx, float64(2*scale)+lfs*1.2, vfs, ink)
	sv.FontWeight = 700
	root.AddChild(sv)
	bv := fmt.Sprintf("%d", st.best)
	bx := bw - gx - canvas.MeasureText(bv, vfs)
	bl := bw - gx - canvas.MeasureText("BEST", lfs)
	root.AddChild(g2048Text("BEST", bl, float64(2*scale), lfs, dim))
	bt := g2048Text(bv, bx, float64(2*scale)+lfs*1.2, vfs, ink)
	bt.FontWeight = 700
	root.AddChild(bt)

	// Board: rounded backdrop, then one rounded slot per cell.
	bg := draw.NewRect()
	bg.Y = by
	bg.Width, bg.Height = bw, bw
	bg.Fill = g2048BoardBg
	bg.BorderRadius = float64(6 * scale)
	root.AddChild(bg)
	for y := 0; y < g2048Size; y++ {
		for x := 0; x < g2048Size; x++ {
			cx := float64(gap + x*(cell+gap))
			cy := by + float64(gap+y*(cell+gap))
			v := g.board[y][x]
			r := draw.NewRect()
			r.X, r.Y = cx, cy
			r.Width, r.Height = float64(cell), float64(cell)
			r.BorderRadius = float64(4 * scale)
			if v == 0 {
				r.Fill = g2048Empty
				root.AddChild(r)
				continue
			}
			tbg, tfg := g2048TileColors(v)
			r.Fill = tbg
			root.AddChild(r)
			s := fmt.Sprintf("%d", v)
			tfs := g2048FontSize(cell, v)
			tt := g2048Text(s, cx+(float64(cell)-canvas.MeasureText(s, tfs))/2, cy+(float64(cell)-tfs*1.2)/2, tfs, tfg)
			tt.FontWeight = 700
			root.AddChild(tt)
		}
	}

	// Overlays, in priority order: the lost game, the reached 2048 (until the
	// player keeps going), then the not-yet-focused hint.
	title, sub := "", ""
	switch {
	case g.over:
		title, sub = "GAME OVER", "R: restart"
	case g.won && !g.keepGoing:
		title, sub = "WIN", "R: restart   arrows: keep going"
	case !focused:
		title = "CLICK TO PLAY"
	}
	if title != "" {
		veil := draw.NewRect()
		veil.Y = by
		veil.Width, veil.Height = bw, bw
		veil.Fill = g2048Dim
		veil.BorderRadius = float64(6 * scale)
		root.AddChild(veil)
		tfs := float64(20 * scale)
		tt := g2048Text(title, (bw-canvas.MeasureText(title, tfs))/2, by+bw/2-tfs*1.2, tfs, g2048InkDark)
		tt.FontWeight = 700
		root.AddChild(tt)
		if sub != "" {
			sfs := float64(10 * scale)
			root.AddChild(g2048Text(sub, (bw-canvas.MeasureText(sub, sfs))/2, by+bw/2+float64(6*scale), sfs, g2048InkDark))
		}
	}
	return root
}

// g2048Text builds one text run.
func g2048Text(content string, x, y, fs float64, fill color.RGBA) *draw.Text {
	t := draw.NewText()
	t.Content = content
	t.FontSize = fs
	t.Fill = fill
	t.X = x
	t.Y = y
	return t
}

// HandlePointer takes focus on press (pointer semantics, no ring) and caches
// the engine interaction so Record can read the live focus each frame. The
// engine already focuses a pressed interactive widget before routing here;
// setting it again keeps the widget correct standalone (switch.go does the
// same). There is no drag in this game — nothing else to handle.
func (w *G2048) HandlePointer(n *model.Node, rt *runtime.Runtime, p canvas.PointerInput, inter *canvas.Interaction, _ image.Rectangle) bool {
	w.mu.Lock()
	w.stateFor(n).inter = inter
	w.mu.Unlock()
	if p.Type != canvas.PointerPress {
		return false
	}
	inter.Focused = n
	inter.FocusVisible = false
	return true
}

// HandleKey drives the board: the four arrows slide, r deals a fresh game
// (the BEST score survives in the node slot). Reaching 2048 raises the WIN
// overlay; the next arrow dismisses it and slides on (keep going). Arrow
// keys are consumed even when the slide changes nothing — they just skip the
// redraw; anything else falls through to the engine's generic key handling.
// Escape never reaches here — the engine blurs on it first (the KeyWidget
// contract).
func (w *G2048) HandleKey(n *model.Node, rt *runtime.Runtime, k canvas.KeyInput, inter *canvas.Interaction) (consumed, redraw bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	st := w.stateFor(n)
	st.inter = inter
	g := st.g

	if k.Key == "r" {
		st.g = newG2048Game(g2048Seed())
		return true, true
	}
	if g.over {
		return false, false
	}
	var dir int
	switch k.Key {
	case "left":
		dir = g2048Left
	case "right":
		dir = g2048Right
	case "up":
		dir = g2048Up
	case "down":
		dir = g2048Down
	default:
		return false, false
	}
	if g.won && !g.keepGoing {
		g.keepGoing = true // the first arrow past the WIN overlay keeps going
	}
	moved := g.move(dir)
	if g.score > st.best {
		st.best = g.score
	}
	return true, moved
}
