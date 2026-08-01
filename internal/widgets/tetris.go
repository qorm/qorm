package widgets

// Tetris is the game-mode showcase component: the classic 10x20 falling-block
// game living behind the canvas widget seam (internal/render/canvas/widget.go).
// It composes three OPTIONAL extensions at once — InteractiveWidget (a press
// focuses the board, which is how it earns the keyboard), KeyWidget (arrows /
// space / p / r drive the active piece while focused), and AnimatedWidget (the
// frame loop stays alive while a game is running, and gravity advances off the
// wall clock inside Measure/Record, the spinner.go clock convention).
//
// The game RULES are pure Go (tetrisGame below): no canvas, draw or runtime
// imports, deterministic under an injected seed — tests drive them directly.
// The widget struct only owns per-node state (map[node] per the widget-seam
// contract: the graph is rebuilt every frame, so state is keyed by the stable
// model pointer; entries for unmounted nodes linger, one map slot each, same
// trade-off as spinStarts) and translates the state into draw primitives.
//
// Piece and board colors are a FIXED palette, deliberately decoupled from the
// theme: players read pieces BY color (the guideline colors are the game's
// vocabulary — a cyan I, a yellow O), so re-tinting them per OS light/dark
// would break piece recognition. Theme tokens are used only for the sidebar
// chrome (labels/values), which is ordinary UI text.

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
	canvas.RegisterWidget("tetris", &Tetris{games: map[*model.Node]*tetrisState{}})
}

// ---------------------------------------------------------------------------
// Pure game rules (no rendering imports — deterministic, unit-testable).
// ---------------------------------------------------------------------------

const (
	tetrisCols = 10
	tetrisRows = 20
)

// Piece kinds, in the order the palette below follows.
const (
	pieceI = iota
	pieceO
	pieceT
	pieceS
	pieceZ
	pieceJ
	pieceL
)

// tetrisPiece is one tetromino: cells of the base orientation inside a
// size×size rotation box (I:4, O:2, the rest:3), plus its fixed color.
type tetrisPiece struct {
	size  int
	cells [4]image.Point
	col   color.RGBA
}

// tetrisPieces holds the seven standard shapes in their standard
// (guideline-style) colors — see the file header for why these are fixed
// rather than themed.
var tetrisPieces = [7]tetrisPiece{
	{4, [4]image.Point{{0, 1}, {1, 1}, {2, 1}, {3, 1}}, color.RGBA{0, 240, 240, 255}}, // I cyan
	{2, [4]image.Point{{0, 0}, {1, 0}, {0, 1}, {1, 1}}, color.RGBA{240, 240, 0, 255}}, // O yellow
	{3, [4]image.Point{{0, 0}, {1, 0}, {2, 0}, {1, 1}}, color.RGBA{160, 0, 240, 255}}, // T purple
	{3, [4]image.Point{{1, 0}, {2, 0}, {0, 1}, {1, 1}}, color.RGBA{0, 240, 0, 255}},   // S green
	{3, [4]image.Point{{0, 0}, {1, 0}, {1, 1}, {2, 1}}, color.RGBA{240, 0, 0, 255}},   // Z red
	{3, [4]image.Point{{0, 0}, {0, 1}, {1, 1}, {2, 1}}, color.RGBA{0, 0, 240, 255}},   // J blue
	{3, [4]image.Point{{2, 0}, {0, 1}, {1, 1}, {2, 1}}, color.RGBA{240, 160, 0, 255}}, // L orange
}

// tetrisLineScore maps a clear of 1..4 rows to its base points (× level).
var tetrisLineScore = [5]int{0, 100, 300, 500, 800}

// tetrisFallInterval is the gravity period at a level: 800ms at level 1,
// 70ms faster per level, floored at 100ms.
func tetrisFallInterval(level int) time.Duration {
	ms := 800 - (level-1)*70
	if ms < 100 {
		ms = 100
	}
	return time.Duration(ms) * time.Millisecond
}

// tetrisCellsAt returns the board coordinates of a piece's cells at a
// rotation (0..3 clockwise turns within the size box — wall kicks are
// deliberately NOT implemented, a documented simplification) and origin.
func tetrisCellsAt(kind, rot, ox, oy int) [4]image.Point {
	p := tetrisPieces[kind]
	var out [4]image.Point
	for i, c := range p.cells {
		x, y := c.X, c.Y
		for r := 0; r < rot; r++ {
			x, y = p.size-1-y, x // one clockwise quarter-turn
		}
		out[i] = image.Point{X: ox + x, Y: oy + y}
	}
	return out
}

// tetrisGame is one board's full mutable state.
type tetrisGame struct {
	board [tetrisRows][tetrisCols]int // 0 empty, else piece kind+1
	rng   *rand.Rand
	bag   []int // 7-bag randomizer tail

	kind   int // active piece kind
	rot    int
	ax, ay int // active box origin on the board
	next   int

	score int
	lines int
	level int

	over   bool
	paused bool

	lastFall time.Time // wall clock of the last gravity step
}

// newTetrisGame deals a fresh game from a seed (injected — tests pass a
// fixed one for deterministic piece sequences).
func newTetrisGame(seed int64) *tetrisGame {
	g := &tetrisGame{rng: rand.New(rand.NewPCG(uint64(seed), uint64(seed)^0x9e3779b97f4a7c15)), level: 1}
	g.next = g.pull()
	g.spawn()
	return g
}

// pull draws the next piece kind from the 7-bag, refilling and shuffling the
// bag when it runs out.
func (g *tetrisGame) pull() int {
	if len(g.bag) == 0 {
		g.bag = []int{pieceI, pieceO, pieceT, pieceS, pieceZ, pieceJ, pieceL}
		g.rng.Shuffle(len(g.bag), func(i, j int) { g.bag[i], g.bag[j] = g.bag[j], g.bag[i] })
	}
	v := g.bag[0]
	g.bag = g.bag[1:]
	return v
}

// spawn makes the previewed piece active at the top centre and previews the
// next. A spawn colliding with the locked stack tops the game out.
func (g *tetrisGame) spawn() {
	g.kind = g.next
	g.next = g.pull()
	g.rot = 0
	g.ax = (tetrisCols - tetrisPieces[g.kind].size) / 2
	g.ay = 0
	if g.collides(g.kind, g.rot, g.ax, g.ay) {
		g.over = true
	}
}

// collides reports whether the piece at (ox,oy)/rot overlaps a wall, the
// floor, or a locked cell. Cells above row 0 are legal (spawn headroom).
func (g *tetrisGame) collides(kind, rot, ox, oy int) bool {
	for _, c := range tetrisCellsAt(kind, rot, ox, oy) {
		if c.X < 0 || c.X >= tetrisCols || c.Y >= tetrisRows {
			return true
		}
		if c.Y >= 0 && g.board[c.Y][c.X] != 0 {
			return true
		}
	}
	return false
}

// cells returns the active piece's current board cells.
func (g *tetrisGame) cells() [4]image.Point {
	return tetrisCellsAt(g.kind, g.rot, g.ax, g.ay)
}

// move shifts the active piece sideways when the target cells are free.
func (g *tetrisGame) move(dx int) bool {
	if g.collides(g.kind, g.rot, g.ax+dx, g.ay) {
		return false
	}
	g.ax += dx
	return true
}

// rotate turns the active piece (dir ±1) when the rotated cells are free —
// no wall kicks (documented simplification).
func (g *tetrisGame) rotate(dir int) bool {
	nr := (g.rot + dir + 4) % 4
	if g.collides(g.kind, nr, g.ax, g.ay) {
		return false
	}
	g.rot = nr
	return true
}

// step applies one gravity (or soft-drop) row: the piece descends, or locks
// into the stack when it cannot.
func (g *tetrisGame) step() {
	if g.over {
		return
	}
	if !g.collides(g.kind, g.rot, g.ax, g.ay+1) {
		g.ay++
		return
	}
	g.lock()
}

// hardDrop slams the active piece straight down and locks it.
func (g *tetrisGame) hardDrop() {
	if g.over {
		return
	}
	for !g.collides(g.kind, g.rot, g.ax, g.ay+1) {
		g.ay++
	}
	g.lock()
}

// lock writes the active piece into the stack, sweeps full rows (scoring
// them at the CURRENT level, then levelling every 10 lines), and spawns the
// next piece.
func (g *tetrisGame) lock() {
	for _, c := range g.cells() {
		if c.Y < 0 {
			g.over = true // locked above the skyline
			continue
		}
		g.board[c.Y][c.X] = g.kind + 1
	}
	if g.over {
		return
	}
	if cleared := g.sweep(); cleared > 0 {
		g.score += tetrisLineScore[cleared] * g.level
		g.lines += cleared
		g.level = 1 + g.lines/10
	}
	g.spawn()
}

// sweep removes full rows, dropping the stack above them, and returns how
// many cleared.
func (g *tetrisGame) sweep() int {
	cleared := 0
	for y := tetrisRows - 1; y >= 0; y-- {
		full := true
		for x := 0; x < tetrisCols; x++ {
			if g.board[y][x] == 0 {
				full = false
				break
			}
		}
		if !full {
			continue
		}
		cleared++
		for yy := y; yy > 0; yy-- {
			g.board[yy] = g.board[yy-1]
		}
		g.board[0] = [tetrisCols]int{}
		y++ // re-check the row that just dropped into this slot
	}
	return cleared
}

// advanceGravity steps the piece down once per fall interval, measured off
// the wall clock (the widget calls this from Measure/Record every frame).
// Paused and finished games hold still, and their clock keeps resetting so
// resuming never discharges a burst of missed steps.
func (g *tetrisGame) advanceGravity(now time.Time) {
	if g.over || g.paused {
		g.lastFall = now
		return
	}
	iv := tetrisFallInterval(g.level)
	for !g.over && !now.Before(g.lastFall.Add(iv)) {
		g.lastFall = g.lastFall.Add(iv)
		g.step()
	}
}

// ---------------------------------------------------------------------------
// The widget: per-node state, draw-primitive rendering, input seams.
// ---------------------------------------------------------------------------

const (
	tetrisCellDef = 18 // default cell edge, logical px (the `cell` prop)
	tetrisSideW   = 90 // sidebar width, logical px
)

// Fixed board chrome colors — part of the game palette (see the file
// header), not the theme.
var (
	tetrisBoardBg = color.RGBA{18, 20, 26, 255}
	tetrisBorder  = color.RGBA{70, 75, 85, 255}
	tetrisDim     = color.RGBA{10, 12, 16, 178}
)

// tetrisNow / tetrisSeed are the test seams (the spinner.go spinNow
// convention): freeze them to make frames and piece sequences deterministic.
var (
	tetrisNow  = time.Now
	tetrisSeed = func() int64 { return time.Now().UnixNano() }
)

// Tetris is the registered widget. All state is per node (games); a cached
// engine Interaction pointer per node lets Record read the live focus — the
// Widget seam carries no interaction state, so HandlePointer/HandleKey stash
// the pointer the engine passes in (its address is stable for the engine's
// lifetime), the textarea.go convention.
type Tetris struct {
	mu    sync.Mutex
	games map[*model.Node]*tetrisState
}

type tetrisState struct {
	g     *tetrisGame
	inter *canvas.Interaction
}

// stateFor returns (creating on first sight) the node's game state. Callers
// hold t.mu.
func (t *Tetris) stateFor(n *model.Node) *tetrisState {
	if st, ok := t.games[n]; ok {
		return st
	}
	st := &tetrisState{g: newTetrisGame(tetrisSeed())}
	st.g.lastFall = tetrisNow()
	t.games[n] = st
	return st
}

// game exposes a node's live game (tests and the key handler).
func (t *Tetris) game(n *model.Node) *tetrisGame {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.stateFor(n).g
}

// Animating keeps the frame loop alive while ANY mounted tetris node is mid
// game — the engine consults the registered (shared) instance, not a node,
// so the answer is type-level. Paused and topped-out games settle: the loop
// stops ticking until input dirties the engine again.
func (t *Tetris) Animating() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	for _, st := range t.games {
		if st.g != nil && !st.g.over && !st.g.paused {
			return true
		}
	}
	return false
}

// Measure reports the content size: board (tetrisCols×cell) plus sidebar
// wide, board (tetrisRows×cell) tall, at the device scale — and ticks the
// clock so gravity keeps pace with the frame loop.
func (t *Tetris) Measure(n *model.Node, rt *runtime.Runtime, _ map[string]any, scale int) (w, h int) {
	if scale < 1 {
		scale = 1
	}
	cell := tetrisCell(n)
	t.mu.Lock()
	st := t.stateFor(n)
	st.g.advanceGravity(tetrisNow())
	t.mu.Unlock()
	return (tetrisCols*cell + tetrisSideW) * scale, tetrisRows * cell * scale
}

// tetrisCell reads the `cell` prop (logical px per board cell), clamped to a
// sane band: scene JSON feeds it and the sidebar proportion assumes it.
func tetrisCell(n *model.Node) int {
	c := int(propNumDefault(n, "cell", tetrisCellDef))
	if c < 4 {
		c = 4
	}
	if c > 64 {
		c = 64
	}
	return c
}

// Record paints the frame: board backdrop, the locked stack, the active
// piece, the border, the sidebar (NEXT preview + SCORE/LINES/LEVEL), and any
// overlay (game over / paused / unfocused hint) — all draw primitives.
func (t *Tetris) Record(ln *canvas.LayoutNode, rt *runtime.Runtime, scale int) draw.Node {
	if ln.Width <= 0 || ln.Height <= 0 {
		return nil
	}
	if scale < 1 {
		scale = 1
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	st := t.stateFor(ln.Node)
	st.g.advanceGravity(tetrisNow())
	g := st.g
	focused := st.inter != nil && st.inter.Focused == ln.Node

	cell := tetrisCell(ln.Node) * scale
	bw := float64(tetrisCols * cell)
	bh := float64(tetrisRows * cell)

	root := draw.NewGroup()

	bg := draw.NewRect()
	bg.Width, bg.Height = bw, bh
	bg.Fill = tetrisBoardBg
	root.AddChild(bg)

	for y := 0; y < tetrisRows; y++ {
		for x := 0; x < tetrisCols; x++ {
			if v := g.board[y][x]; v != 0 {
				root.AddChild(tetrisCellRect(x, y, cell, scale, tetrisPieces[v-1].col))
			}
		}
	}
	if !g.over {
		for _, c := range g.cells() {
			if c.Y >= 0 {
				root.AddChild(tetrisCellRect(c.X, c.Y, cell, scale, tetrisPieces[g.kind].col))
			}
		}
	}

	border := draw.NewRect()
	border.Width, border.Height = bw, bh
	border.Stroke = tetrisBorder
	border.StrokeWidth = float64(scale)
	root.AddChild(border)

	// Sidebar: theme-colored UI text (unlike the fixed game palette).
	ink := themeColor(rt, "text", color.RGBA{29, 29, 31, 255})
	dim := themeColor(rt, "textSecondary", color.RGBA{134, 134, 139, 255})
	sx := bw + float64(8*scale)
	y := float64(8 * scale)
	lfs := float64(10 * scale)
	root.AddChild(tetrisText("NEXT", sx, y, lfs, dim))
	y += lfs*1.2 + float64(4*scale)
	for _, c := range tetrisCellsAt(g.next, 0, 0, 0) {
		r := draw.NewRect()
		r.X = sx + float64(c.X*cell) + float64(scale)
		r.Y = y + float64(c.Y*cell) + float64(scale)
		r.Width = float64(cell - 2*scale)
		r.Height = float64(cell - 2*scale)
		r.Fill = tetrisPieces[g.next].col
		root.AddChild(r)
	}
	y += float64(4*cell) + float64(12*scale) // reserve the full 4-row box: no jitter
	y = tetrisStat(root, "SCORE", fmt.Sprintf("%d", g.score), sx, y, scale, dim, ink)
	y = tetrisStat(root, "LINES", fmt.Sprintf("%d", g.lines), sx, y, scale, dim, ink)
	tetrisStat(root, "LEVEL", fmt.Sprintf("%d", g.level), sx, y, scale, dim, ink)

	// Overlays, in priority order: the ended game, the paused game, then the
	// not-yet-focused hint (the board runs behind a translucent dim either
	// way — it is a game, it does not freeze just because it lost focus).
	title, sub := "", ""
	switch {
	case g.over:
		title, sub = "GAME OVER", "PRESS R TO RESTART"
	case g.paused:
		title, sub = "PAUSED", "PRESS P TO RESUME"
	case !focused:
		title = "CLICK TO PLAY"
	}
	if title != "" {
		veil := draw.NewRect()
		veil.Width, veil.Height = bw, bh
		veil.Fill = tetrisDim
		root.AddChild(veil)
		tfs := float64(16 * scale)
		tt := tetrisText(title, (bw-canvas.MeasureText(title, tfs))/2, bh/2-tfs*1.2, tfs, color.RGBA{255, 255, 255, 255})
		tt.FontWeight = 700
		root.AddChild(tt)
		if sub != "" {
			sfs := float64(10 * scale)
			root.AddChild(tetrisText(sub, (bw-canvas.MeasureText(sub, sfs))/2, bh/2+float64(4*scale), sfs, color.RGBA{220, 224, 230, 255}))
		}
	}
	return root
}

// tetrisCellRect is one board cell, inset 1 device px so the grid reads.
func tetrisCellRect(x, y, cell, scale int, col color.RGBA) *draw.Rect {
	r := draw.NewRect()
	r.X = float64(x*cell + scale)
	r.Y = float64(y*cell + scale)
	r.Width = float64(cell - 2*scale)
	r.Height = float64(cell - 2*scale)
	r.Fill = col
	return r
}

// tetrisText builds one text run.
func tetrisText(content string, x, y, fs float64, fill color.RGBA) *draw.Text {
	t := draw.NewText()
	t.Content = content
	t.FontSize = fs
	t.Fill = fill
	t.X = x
	t.Y = y
	return t
}

// tetrisStat paints a label/value pair and returns the next row's y.
func tetrisStat(g *draw.Group, label, value string, x, y float64, scale int, dim, ink color.RGBA) float64 {
	lfs := float64(10 * scale)
	vfs := float64(15 * scale)
	g.AddChild(tetrisText(label, x, y, lfs, dim))
	y += lfs*1.2 + float64(2*scale)
	v := tetrisText(value, x, y, vfs, ink)
	v.FontWeight = 600
	g.AddChild(v)
	return y + vfs*1.2 + float64(14*scale)
}

// HandlePointer takes focus on press (pointer semantics, no ring) and caches
// the engine interaction so Record can read the live focus each frame. The
// engine already focuses a pressed interactive widget before routing here;
// setting it again keeps the widget correct standalone (switch.go does the
// same). There is no drag in this game — nothing else to handle.
func (t *Tetris) HandlePointer(n *model.Node, rt *runtime.Runtime, p canvas.PointerInput, inter *canvas.Interaction, _ image.Rectangle) bool {
	t.mu.Lock()
	t.stateFor(n).inter = inter
	t.mu.Unlock()
	if p.Type != canvas.PointerPress {
		return false
	}
	inter.Focused = n
	inter.FocusVisible = false
	return true
}

// HandleKey drives the active piece: left/right/down move, up/x rotate
// clockwise, z counter-clockwise, space hard-drops, p pauses/resumes, r
// restarts a finished game. Game keys report consumed with a redraw;
// anything else falls through to the engine's generic key handling. Escape
// never reaches here — the engine blurs on it first (the KeyWidget contract).
func (t *Tetris) HandleKey(n *model.Node, rt *runtime.Runtime, k canvas.KeyInput, inter *canvas.Interaction) (consumed, redraw bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	st := t.stateFor(n)
	st.inter = inter
	g := st.g

	switch k.Key {
	case "p":
		if g.over {
			return false, false
		}
		g.paused = !g.paused
		if !g.paused {
			g.lastFall = tetrisNow() // resume on a fresh interval
		}
		return true, true
	case "r":
		if !g.over {
			return false, false
		}
		st.g = newTetrisGame(tetrisSeed())
		st.g.lastFall = tetrisNow()
		return true, true
	}
	if g.over || g.paused {
		return false, false
	}
	switch k.Key {
	case "left":
		g.move(-1)
	case "right":
		g.move(1)
	case "down":
		g.step()
	case "up", "x":
		g.rotate(1)
	case "z":
		g.rotate(-1)
	case "space":
		g.hardDrop()
	default:
		return false, false
	}
	return true, true
}
