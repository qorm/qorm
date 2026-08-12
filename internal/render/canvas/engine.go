package canvas

import (
	"fmt"
	"image"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/qorm/qorm/internal/geom"
	"github.com/qorm/qorm/internal/model"
	"github.com/qorm/qorm/internal/op"
	"github.com/qorm/qorm/internal/render/graph"
	"github.com/qorm/qorm/internal/runtime"
	"github.com/qorm/qorm/internal/theme"
)

// Surface is a present target that owns the frame buffer the Renderer draws
// into. Platform windows implement it with zero-copy shared memory (darwin:
// NSBitmapImageRep; future: Windows DIB / X11 XImage); tests use
// HeadlessSurface. This is the platform seam of the engine — new platforms
// plug in here without touching layout, recording or rendering.
type Surface interface {
	// Size returns the surface dimensions in physical pixels.
	Size() image.Point
	// Scale returns the device-pixel ratio (1 = standard, 2 = Retina). The
	// engine lays out design pixels * Scale into the physical buffer.
	Scale() int
	// Backbuffer returns the buffer the next frame must be drawn into.
	Backbuffer() *image.RGBA
	// Present publishes the drawn backbuffer (swap + schedule display).
	Present()
}

// HeadlessSurface is an in-memory Surface for tests and headless runs: a
// single persistent buffer, no display. Frame() exposes the last rendered
// pixels; Presents counts published frames.
type HeadlessSurface struct {
	buf      *image.RGBA // physical pixels
	Presents int
	// ScaleFactor is the device-pixel ratio for tests (0/1 == 1).
	ScaleFactor int
	// Logical is the layout size in points (Size()). Zero → use the buffer
	// size (the scale-1 convenience: logical == physical).
	Logical image.Point
}

func NewHeadlessSurface(size image.Point) *HeadlessSurface {
	return &HeadlessSurface{buf: image.NewRGBA(image.Rect(0, 0, size.X, size.Y)), ScaleFactor: 1}
}

func (s *HeadlessSurface) Size() image.Point {
	if s.Logical.X > 0 && s.Logical.Y > 0 {
		return s.Logical
	}
	return s.buf.Rect.Size()
}
func (s *HeadlessSurface) Scale() int {
	if s.ScaleFactor < 1 {
		return 1
	}
	return s.ScaleFactor
}
func (s *HeadlessSurface) Backbuffer() *image.RGBA { return s.buf }
func (s *HeadlessSurface) Present()                { s.Presents++ }
func (s *HeadlessSurface) Frame() *image.RGBA      { return s.buf }

// Platform-agnostic input events. The host converts app.PointerEvent /
// app.KeyEvent into these, so the engine carries no platform dependency.
type PointerType uint8

const (
	PointerPress PointerType = iota
	PointerRelease
	PointerMove
)

type PointerInput struct {
	Type    PointerType
	X, Y    float64
	Buttons int
	// Right marks a right-button press/release — the onContextMenu seam
	// (engine.go dispatchContextMenu), set by the host for NSRightMouse*.
	Right bool
	// ID identifies the pointer/finger (0 = the mouse, 1+ = touch fingers on a
	// multi-touch host). The engine's single-pointer gestures ignore it; the
	// host's touch delivery (NSTouch) is the seam that fills it for two-finger
	// pinch and touch-drag gestures.
	ID int
}

type KeyInput struct {
	Key string // normalized name ("tab", "return", "a", …)
	// Rune is the printable character the key produces (0 = none). The host
	// fills it from the platform text-input channel (macOS -characters, the
	// future IME seam); the engine consumes it only while an input holds the
	// edit session (input.go). Hosts that send nothing fall back to named-key
	// ASCII there.
	Rune  rune
	Shift bool // shift modifier held
	Ctrl  bool // control modifier held
	Alt   bool // option/alt modifier held
	Meta  bool // command (macOS ⌘) / meta modifier held
	Down  bool // false = key up
}

// ScrollInput is a mouse-wheel or trackpad scroll, in physical pixels per
// notch/gesture tick (the host scales platform deltas like pointer coords).
// Ctrl marks a control-modified scroll — macOS trackpad pinch (which AppKit
// delivers as a precise scroll with the control flag) and Ctrl+wheel — which
// an infinite-canvas board treats as zoom rather than scroll/pan.
type ScrollInput struct {
	DX, DY float64
	Ctrl   bool
}

// FrameStats carries per-phase timings of one DrawFrame (Unity-style frame
// profiling, emitted to stderr when QORM_FRAME_STATS=1).
type FrameStats struct {
	LayoutRecord time.Duration // measure + layout + display-list recording
	Render       time.Duration // display list → pixels
	Present      time.Duration // buffer publish
	Total        time.Duration
}

// Graph exposes the most recently laid-out scene graph (the root node of the
// retained-mode tree RenderInto rebuilt on its last frame). Tests use it to
// assert the layout outcome; production code reads through the
// graph.SnapshotFor / WidgetFrames seams instead. May be nil before the first
// RenderInto or after the scene root was conditionally hidden.
func (e *Engine) Graph() graph.Node { return e.graphRoot }

//	input/state → layout → record (display list) → render
//
// It owns the interaction state, the (reused) display-list buffer and the
// last rendered graph. It does NOT own a Surface: the host renders into a
// buffer it provides (RenderInto) and presents it itself.
//
// Threading model (the only one that avoids the render/display data race that
// plagued the old background-render design): input (HandlePointer/HandleKey)
// and rendering (RenderInto) both run on the host's MAIN thread, so Inter,
// ops, graphRoot AND rt.State are all single-threaded. External work (HTTP/MCP
// handler goroutines) never touches rt.State directly: it goes through
// EnqueueMutation, which parks the closure on a queue the render thread drains
// at the next frame boundary (the top of RenderInto, before resolveTheme and
// layout read any state). The only cross-thread signals are that queue (qmu)
// and the dirty/animating atomics — RequestDraw/MarkDirty are plain atomic
// stores so they stay safe inside a dispatch on RenderInto's own call stack.
type Engine struct {
	RT       *runtime.Runtime
	Renderer Renderer
	Inter    Interaction

	qmu     sync.Mutex // guards pending — the only engine state touched off-thread
	pending []mutation

	StatsEnabled bool

	dirty     atomic.Bool // pending frame (input/state change)
	animating atomic.Bool // a tween is mid-flight; keep rendering

	ops        op.Ops
	graphRoot  graph.Node
	layoutRoot *LayoutNode // last laid-out tree (CollectMeasure style sidecar)
	lastScale  int         // device-pixel ratio used for last layout
	lastSize   image.Point // physical stage size of last layout
	lastRoot   *model.Node
	// lastOpsFP / lastBufSize skip software raster when a dirty frame
	// produces an identical display list into the same buffer size.
	lastOpsFP   uint64
	hasOpsFP    bool
	lastBufSize image.Point
	// lastNodeRects maps layout identity → previous absolute box for dirty
	// region partial redraw (union of old∪new for changed nodes).
	lastNodeRects map[string]image.Rectangle

	// lastPtr/hasPtr track the pointer's rest position from every
	// HandlePointer call: ScrollInput carries no coordinates, so wheel and
	// trackpad gestures hit-test where the cursor rests (scroll.go).
	lastPtr geom.Point
	hasPtr  bool

	// itemInstances is the repeat-instance sidecar from the latest layout
	// (list.go): instance-root graph node -> its index and item scope.
	// Rebuilt every frame alongside the graph it annotates.
	itemInstances map[graph.Node]itemInstance

	// timers is the live schedule of the scene's mounted timer nodes
	// (timer.go): the native engine has no client JS to run them, so the
	// frame loop itself dispatches their deadlines. Render-thread only.
	timers map[*model.Node]*sceneTimer

	themeFailed string // last theme name that failed to load (negative cache)
	// themeLoaded is the last REQUESTED name that loaded successfully (positive
	// cache). Tracked by requested name, not rt.Theme.Name: a skin file whose
	// inner "name" differs from its file name would otherwise reload — and
	// re-log — every frame.
	themeLoaded string
}

// mutation is one queued external state change (HTTP/MCP handler work) plus
// the channel that reports its completion back to the blocked caller.
type mutation struct {
	fn   func()
	done chan struct{}
}

func NewEngine(rt *runtime.Runtime, r Renderer) *Engine {
	if r == nil {
		r = SoftwareRenderer{}
	}
	e := &Engine{RT: rt, Renderer: r, StatsEnabled: os.Getenv("QORM_FRAME_STATS") == "1"}
	e.dirty.Store(true) // the first frame always renders
	return e
}

// MarkDirty / RequestDraw flag the engine as needing a redraw. Safe to call
// from any goroutine (atomic store; never locks — see Engine comment).
func (e *Engine) MarkDirty()   { e.dirty.Store(true) }
func (e *Engine) RequestDraw() { e.dirty.Store(true) }

// Dirty reports whether a redraw is pending (input or state change).
func (e *Engine) Dirty() bool { return e.dirty.Load() }

// Animating reports whether a tween is still in flight OR a declarative timer
// is mounted and waiting for its next deadline. The host's idle loop only
// calls RenderInto when this is true, so timers (which start off un-fired
// with `nextFire = now + every` — see tickTimer) MUST register here the
// instant they show up in the mounted scene. Without this, a game whose
// first frame only sets state would tick zero times: the engine renders
// once (the first call here observes the timer node and returns), the
// timer is now pending but `Dirty` is false, the host idles forever, and
// the game freezes on its initial frame.
func (e *Engine) Animating() bool {
	if e.animating.Load() {
		return true
	}
	return e.timersPending()
}

// EnqueueMutation hands an external state change (an HTTP/MCP handler's work)
// to the render thread and blocks until it has run, preserving the caller's
// request/response semantics. The closure runs at the next frame boundary —
// the top of RenderInto — so rt.State stays single-threaded (see the Engine
// comment). It must NOT be called from the render thread itself, which would
// wait for its own frame and deadlock. The host must keep the render loop
// ticking; when that loop stops the process is exiting (window closed), so a
// blocked caller dies with it instead of deadlocking a shut-down host.
func (e *Engine) EnqueueMutation(fn func()) {
	done := make(chan struct{})
	e.qmu.Lock()
	e.pending = append(e.pending, mutation{fn: fn, done: done})
	e.qmu.Unlock()
	// Wake the render loop: the queue only drains inside RenderInto.
	e.dirty.Store(true)
	<-done
}

// drainMutations applies every queued external mutation. RenderInto calls it
// at the frame boundary, before anything below reads rt.State for the new
// frame.
func (e *Engine) drainMutations() {
	e.qmu.Lock()
	q := e.pending
	e.pending = nil
	e.qmu.Unlock()
	for _, m := range q {
		m.fn()
		close(m.done)
	}
}

// RenderInto runs one staged frame into target (size/scale describe it). It
// returns true if it actually rendered (caller should then present target) and
// the frame stats. It is a no-op when neither dirty nor animating. Must be
// called from the main thread.
func (e *Engine) RenderInto(size image.Point, scale int, target *image.RGBA) (bool, FrameStats) {
	var st FrameStats
	rt := e.RT
	if rt == nil || rt.App == nil || target == nil {
		return false, st
	}

	// Frame boundary: apply queued external mutations on this (render) thread,
	// before resolveTheme/layout read rt.State for the new frame.
	e.drainMutations()

	// Scene enter hooks drain here too — the runtime marks pendingEnter on
	// creation/navigation and each host picks its drain point (the server
	// drains before rendering); the canvas host had none, so onEnter data
	// (seeded lists, loaded state) never reached the native window. Drain
	// before the frame so the hook's writes are exactly what it lays out.
	if rt.PendingEnter() {
		rt.RunPendingEnter()
		e.dirty.Store(true)
	}

	if !e.dirty.Load() && !e.animating.Load() {
		return false, st
	}
	// Consume the frame request UP FRONT, not at the tail: a state change that
	// lands MID-frame — the physics pass dispatching a collision action whose
	// render step calls Commit → RequestDraw, or an enqueue racing this frame —
	// re-flags dirty and must produce the NEXT frame. Clearing at the tail (the
	// old code) ate that flag, so the frame carrying the new state was lost
	// until some unrelated event dirtied the engine again.
	e.dirty.Store(false)

	start := time.Now()
	e.resolveTheme()

	// Reset interaction identities when the scene tree changed (scene switch
	// or hot reload): the model pointers in Inter could alias retired nodes.
	// The timer schedule keys by the same pointers, so it resets with them.
	root := e.sceneRoot()
	if root != e.lastRoot {
		e.Inter = Interaction{}
		e.lastRoot = root
		e.timers = nil
	}

	// Scene timers (the native scheduler for the declarative timer node):
	// fire every deadline that passed since the last frame, BEFORE layout, so
	// the state a tick writes is what this frame records.
	e.tickTimers(root)

	// Scroll momentum: advance every active viewport's inertia by one frame
	// before layout, so the new offsets are what this frame records.
	if e.applyScrollMomentum(time.Now()) {
		e.dirty.Store(true)
	}
	if e.applyBoardMomentum(time.Now()) {
		e.dirty.Store(true)
	}

	// Layout + record. The display list is rebuilt from scratch each frame;
	// Reset keeps the backing array, so steady-state recording allocates
	// nothing. (Before the Engine owned this, ops accumulated forever.)
	t0 := time.Now()
	e.ops.Reset()
	rootNode, needsRedraw, instances, layoutRoot := layout(&e.ops, root, size, rt, &e.Inter, scale)
	e.graphRoot = rootNode
	e.layoutRoot = layoutRoot
	e.lastScale = scale
	e.lastSize = size
	e.itemInstances = instances
	st.LayoutRecord = time.Since(t0)

	e.physics(rootNode)

	// Motion onComplete (timeline/fx): measure queues invokes; dispatch here
	// after layout so actions see the settled frame and can re-dirty.
	e.drainMotionCompletes()

	// dirty was consumed at the top of the frame; animation keeps the loop
	// ticking until the tweens settle (no separate timer goroutine — the host
	// polls). Registered AnimatedWidgets (spinner) never settle on their own,
	// so the scene scan keeps the loop alive for them too; a mounted scene
	// timer (timer.go) keeps it alive until its last deadline.
	// A live edit session on the built-in input keeps the loop ticking so the
	// caret can blink (input.go caretVisible) — the browser repaints its text
	// cursor the same way. The textarea's caret is static, so it does not
	// waste frames; both settle as soon as the session closes.
	stillAnimating := needsRedraw || e.sceneAnimating() || e.timersPending() ||
		e.hasScrollMomentum() || e.hasBoardMomentum() || flipStillRunning() ||
		(e.Inter.Input != nil && e.Inter.Input.Node.Type == "input")
	e.animating.Store(stillAnimating)

	// Skip software raster when the display list is identical to the last
	// frame and the buffer size matches. Layout still ran (physics, timers,
	// graph for hit-testing). Still return true so the host Presents — the
	// buffer already holds the correct pixels; MarkDirty / animating loops
	// keep their present contract without re-rasterizing.
	fp := e.ops.Fingerprint()
	bufSize := target.Bounds().Size()
	if e.hasOpsFP && fp == e.lastOpsFP && bufSize == e.lastBufSize {
		st.Total = time.Since(start)
		if e.StatsEnabled {
			fmt.Fprintf(os.Stderr, "[qorm frame] layout+record=%s render=skipped total=%s\n",
				st.LayoutRecord, st.Total)
		}
		return true, st
	}

	// Dirty-region partial redraw (SoftwareRenderer.Dirty): when layout boxes
	// moved, only re-raster the pad-expanded union. Empty Dirty = full frame.
	dirty := e.computeDirtyRect(layoutRoot, target.Bounds())
	t1 := time.Now()
	switch r := e.Renderer.(type) {
	case SoftwareRenderer:
		r.Dirty = dirty
		r.Render(&e.ops, target)
	case *SoftwareRenderer:
		r.Dirty = dirty
		r.Render(&e.ops, target)
	default:
		e.Renderer.Render(&e.ops, target)
	}
	st.Render = time.Since(t1)
	e.lastOpsFP = fp
	e.hasOpsFP = true
	e.lastBufSize = bufSize
	e.snapshotNodeRects(layoutRoot)

	st.Total = time.Since(start)
	if e.StatsEnabled {
		extra := ""
		if !dirty.Empty() {
			extra = fmt.Sprintf(" dirty=%v", dirty)
		}
		fmt.Fprintf(os.Stderr, "[qorm frame] layout+record=%s render=%s total=%s%s\n",
			st.LayoutRecord, st.Render, st.Total, extra)
	}
	return true, st
}

// computeDirtyRect returns the screen-space region that must be re-rasterized.
// Empty means "full frame" (SoftwareRenderer treats empty Dirty as full).
// Set QORM_FULL_RASTER=1 to always full-frame (debugging / golden isolation).
func (e *Engine) computeDirtyRect(root *LayoutNode, bounds image.Rectangle) image.Rectangle {
	if os.Getenv("QORM_FULL_RASTER") == "1" {
		return image.Rectangle{}
	}
	if e.lastNodeRects == nil || root == nil || e.lastBufSize != bounds.Size() {
		return image.Rectangle{} // full
	}
	var dirty image.Rectangle
	var walk func(ln *LayoutNode)
	walk = func(ln *LayoutNode) {
		if ln == nil || ln.Node == nil {
			return
		}
		key := ln.Node.ID
		if key == "" {
			key = fmt.Sprintf("_%p", ln.Node)
		}
		if ln.ItemIndex != 0 {
			key += fmt.Sprintf("@%d", ln.ItemIndex)
		}
		cur := image.Rect(ln.AbsX, ln.AbsY, ln.AbsX+ln.Width, ln.AbsY+ln.Height)
		if prev, ok := e.lastNodeRects[key]; ok {
			if prev != cur {
				dirty = dirty.Union(prev).Union(cur)
			}
		} else {
			// New node — mark its box dirty.
			dirty = dirty.Union(cur)
		}
		for _, c := range ln.Children {
			walk(c)
		}
	}
	walk(root)
	// Nodes removed this frame: their previous rects must clear.
	seen := map[string]bool{}
	var mark func(ln *LayoutNode)
	mark = func(ln *LayoutNode) {
		if ln == nil || ln.Node == nil {
			return
		}
		key := ln.Node.ID
		if key == "" {
			key = fmt.Sprintf("_%p", ln.Node)
		}
		if ln.ItemIndex != 0 {
			key += fmt.Sprintf("@%d", ln.ItemIndex)
		}
		seen[key] = true
		for _, c := range ln.Children {
			mark(c)
		}
	}
	mark(root)
	for k, prev := range e.lastNodeRects {
		if !seen[k] {
			dirty = dirty.Union(prev)
		}
	}
	if dirty.Empty() {
		// Ops fingerprint changed but layout boxes did not (text/color only).
		// Fall back to full redraw — partial would miss in-box content changes.
		return image.Rectangle{}
	}
	// Pad for shadows / blur / AA.
	const pad = 24
	dirty = image.Rect(dirty.Min.X-pad, dirty.Min.Y-pad, dirty.Max.X+pad, dirty.Max.Y+pad).Intersect(bounds)
	// If dirty covers most of the stage, full redraw is cheaper/simpler.
	if dirty.Dx()*dirty.Dy() > bounds.Dx()*bounds.Dy()*3/4 {
		return image.Rectangle{}
	}
	return dirty
}

// snapshotNodeRects stores this frame's absolute boxes for the next dirty pass.
func (e *Engine) snapshotNodeRects(root *LayoutNode) {
	next := make(map[string]image.Rectangle)
	var walk func(ln *LayoutNode)
	walk = func(ln *LayoutNode) {
		if ln == nil || ln.Node == nil {
			return
		}
		key := ln.Node.ID
		if key == "" {
			key = fmt.Sprintf("_%p", ln.Node)
		}
		if ln.ItemIndex != 0 {
			key += fmt.Sprintf("@%d", ln.ItemIndex)
		}
		next[key] = image.Rect(ln.AbsX, ln.AbsY, ln.AbsX+ln.Width, ln.AbsY+ln.Height)
		for _, c := range ln.Children {
			walk(c)
		}
	}
	walk(root)
	e.lastNodeRects = next
}

// sceneAnimating reports whether any VISIBLE mounted node is a registered
// AnimatedWidget still asking for frames — the engine keeps ticking without
// knowing the widget's type. Hidden nodes (if/visible/show flip or an
// unselected when branch) don't count: a spinner on an inactive tab must
// not spin the loop at full speed (R7).
func (e *Engine) sceneAnimating() bool {
	rt := e.RT
	found := false
	var walk func(n *model.Node)
	walk = func(n *model.Node) {
		if n == nil || found || !nodeVisible(n, rt) {
			return
		}
		if w, ok := LookupWidget(n.Type); ok {
			if aw, yes := w.(AnimatedWidget); yes && aw.Animating() {
				found = true
				return
			}
		}
		if n.Type == "when" {
			walk(whenBranch(n, rt))
			return
		}
		for _, c := range n.Children {
			walk(c)
		}
	}
	walk(e.sceneRoot())
	return found
}

// DrawFrame renders into the given surface's buffer and presents it — a thin
// convenience used by headless tests that still drive a Surface.
func (e *Engine) DrawFrame(s Surface) FrameStats {
	rendered, st := e.RenderInto(s.Size(), s.Scale(), s.Backbuffer())
	if rendered {
		t0 := time.Now()
		s.Present()
		st.Present = time.Since(t0)
		st.Total += st.Present
	}
	return st
}

// HandlePointer processes one pointer event against the last rendered frame.
// It updates interaction state and dispatches handlers; the return reports
// whether visuals changed and the host should DrawFrame.
func (e *Engine) HandlePointer(p PointerInput) bool {
	rt := e.RT
	if rt == nil || e.graphRoot == nil {
		return false
	}
	// Input must arrive in physical pixels (the host multiplies logical points
	// by its scale before calling us — the engine is scale-agnostic).
	hit := e.graphRoot.HitTest(geom.Point{X: p.X, Y: p.Y})
	e.lastPtr = geom.Point{X: p.X, Y: p.Y}
	e.hasPtr = true

	// A new press abandons any in-flight cross-panel drag: either the release
	// already dropped it on a dragtarget, or it was released over nothing.
	if p.Type == PointerPress && e.Inter.Drag.Active {
		e.Inter.Drag = DragState{}
	}
	// A new press also settles any unsettled swipe candidate before the
	// gesture paths below claim the stream — the recognizer re-arms only from
	// a press that reaches the generic path (see there), so a captured
	// gesture can never leave a stale candidate behind.
	if p.Type == PointerPress {
		e.Inter.Swipe.Armed = false
	}

	// A right-button press dispatches the nearest onContextMenu handler up the
	// hit chain (the browser's contextmenu event) before anything else.
	if p.Type == PointerPress && p.Right {
		if e.dispatchContextMenu(hit, rt) {
			e.dirty.Store(true)
			return true
		}
	}

	// A drag-to-reorder gesture owns the stream while in flight: the dragged
	// item's target slot follows the pointer in item-height steps (one grid
	// row per crossAxisCount for gridviews), and the release dispatches
	// onReorder {from, to} only when the slot actually moved.
	if e.Inter.Reorder.Active {
		switch {
		case p.Type == PointerMove && p.Buttons > 0:
			e.Inter.Reorder.To = reorderTarget(&e.Inter.Reorder, p.Y)
			e.dirty.Store(true)
			return true
		case p.Type == PointerRelease:
			if e.Inter.Reorder.To != e.Inter.Reorder.From {
				e.dispatchReorder()
			}
			e.Inter.Reorder = ReorderState{}
			e.dirty.Store(true)
			return true
		default:
			// A button-less move mid-drag (capture lost, cancelled) drops the
			// gesture — a stale flag would hijack the NEXT drag (the same
			// hazard the board pan defends against).
			if p.Type == PointerMove && p.Buttons == 0 {
				e.Inter.Reorder = ReorderState{}
			}
		}
	}
	// A press on an item of a reorderable list arms a PENDING reorder without
	// claiming the press — the item's own tap and handlers still work, and the
	// gesture activates only once the drag passes the start slop.
	if p.Type == PointerPress {
		e.armReorder(hit, rt)
	}
	if e.Inter.Reorder.Pending {
		if p.Type == PointerMove && p.Buttons > 0 {
			if math.Hypot(p.X-e.Inter.Reorder.PressX, p.Y-e.Inter.Reorder.PressY) > reorderStartSlop {
				e.Inter.Reorder.Active = true
				e.Inter.Reorder.Pending = false
				e.Inter.Pressed = nil // steal any capture the press opened
				e.Inter.Reorder.To = reorderTarget(&e.Inter.Reorder, p.Y)
				e.dirty.Store(true)
				return true
			}
		} else if p.Type == PointerRelease || (p.Type == PointerMove && p.Buttons == 0) {
			e.Inter.Reorder = ReorderState{} // a tap, or the press was cancelled
		}
	}

	// Arm / re-arm pending scroll drag on every press that hits a scroll
	// viewport (including InteractiveWidget children). Subsequent moves past
	// slop steal the stream from the child (handleScrollDrag).
	if p.Type == PointerPress {
		e.armScrollDragFromHit(hit)
		e.Inter.ScrollDrag.StartX, e.Inter.ScrollDrag.StartY = p.X, p.Y
		e.Inter.ScrollDrag.LastX, e.Inter.ScrollDrag.LastY = p.X, p.Y
	}

	// An in-flight touch-drag scroll owns the stream (same priority band as
	// board pan): finger deltas move the content with rubber-band overscroll.
	// Also consulted while an InteractiveWidget holds Pressed, so a vertical
	// drag on a list tile becomes a scroll instead of a stuck press.
	if e.Inter.ScrollDrag.Pending || e.Inter.ScrollDrag.Active {
		if e.handleScrollDrag(p) {
			return true
		}
	}

	// An in-flight board pan owns the stream: the canvas follows the pointer
	// before any widget sees the move, so dragging across a note doesn't fight
	// the pan. Ends on release — or on a button-less move, which means the drag
	// was cancelled/lost mid-flight (a stale Panning would otherwise hijack the
	// NEXT note drag into a pan from a dead PanStart).
	if e.Inter.Board.Active && e.Inter.Board.Panning {
		switch {
		case p.Type == PointerRelease:
			e.Inter.Board.Panning = false
			// Start the coast phase: capture the drag velocity so the canvas
			// decelerates naturally instead of stopping dead at release.
			e.Inter.Board.PanMomActive = true
			e.Inter.Board.PanMomLast = time.Now()
			e.dirty.Store(true)
			return true
		case p.Type == PointerMove && p.Buttons > 0:
			// Track velocity: delta from the last move, smoothed with EMA so
			// a single jittery event doesn't spike the coast speed.
			now := time.Now()
			dx := p.X - e.Inter.Board.PanStart.X + e.Inter.Board.PanOrigin.X - e.Inter.Board.PanX
			dy := p.Y - e.Inter.Board.PanStart.Y + e.Inter.Board.PanOrigin.Y - e.Inter.Board.PanY
			e.Inter.Board.PanX = e.Inter.Board.PanOrigin.X + (p.X - e.Inter.Board.PanStart.X)
			e.Inter.Board.PanY = e.Inter.Board.PanOrigin.Y + (p.Y - e.Inter.Board.PanStart.Y)
			// EMA: 20% new speed, 80% old — smooth but responsive.
			dt := now.Sub(e.Inter.Board.PanMomLast).Seconds()
			if dt > 0 && dt < 0.5 {
				frames := dt * 60 // normalize to 60fps
				if frames > 0 {
					sx := dx / frames
					sy := dy / frames
					e.Inter.Board.PanMomVX = 0.2*sx + 0.8*e.Inter.Board.PanMomVX
					e.Inter.Board.PanMomVY = 0.2*sy + 0.8*e.Inter.Board.PanMomVY
				}
			}
			e.Inter.Board.PanMomLast = now
			e.dirty.Store(true)
			return true
		default:
			// A hover move or a fresh press during a pan: drop the drag state
			// without consuming the event, so normal dispatch resumes.
			e.Inter.Board.Panning = false
			e.Inter.Board.PanMomActive = false
		}
	}

	// A press-drag text selection owns the stream while it is in flight: the
	// caret follows the pointer (clamped at the buffer ends), extending the
	// selection from the press anchor. A button-less move or release ends it.
	if s := e.Inter.Input; s != nil && s.Selecting {
		switch {
		case p.Type == PointerMove && p.Buttons > 0:
			if m := e.inputMetricsFromGraph(s.Node); m != nil {
				s.Cursor = caretIndexFromPointer(m, s, p.X, p.Y)
				if s.Cursor < s.Anchor {
					s.SelStart, s.SelEnd = s.Cursor, s.Anchor
				} else {
					s.SelStart, s.SelEnd = s.Anchor, s.Cursor
				}
				e.dirty.Store(true)
				return true
			}
		case p.Type == PointerRelease || (p.Type == PointerMove && p.Buttons == 0):
			s.Selecting = false
		}
	}

	// Registered interactive widgets claim their own event stream first: a
	// pressed widget node keeps capture until release (drag), and a fresh hit
	// routes to the nearest registered-interactive ancestor. Consumed events
	// never reach the generic press/hover path.
	if e.Inter.Pressed != nil {
		if w, ok := LookupWidget(e.Inter.Pressed.Type); ok {
			if iw, yes := w.(InteractiveWidget); yes {
				frame := image.Rectangle{}
				if g := e.findGroupByModel(e.Inter.Pressed); g != nil {
					b := g.GetBBox()
					frame = image.Rect(int(b.MinX), int(b.MinY), int(b.MaxX), int(b.MaxY))
				}
				redraw := iw.HandlePointer(e.Inter.Pressed, rt, p, &e.Inter, frame)
				if p.Type == PointerRelease {
					e.Inter.Pressed = nil
				}
				// A widget's redraw return must reach the dirty flag —
				// RenderInto gates on it, so without this a widget-driven
				// change (a drag's thumb move) never repaints.
				if redraw {
					e.dirty.Store(true)
				}
				return redraw
			}
		}
	}
	if iw, m, frame := interactiveWidgetAt(hit); iw != nil {
		// A drag release prefers the nearest drop target over any inner
		// interactive widget: a drop zone must not be swallowed by its own
		// children (a dragtarget wrapping an interactive child still drops).
		if p.Type == PointerRelease && e.Inter.Drag.Active {
			if dt, dm, df := dropTargetAt(hit); dt != nil {
				iw, m, frame = dt, dm, df
			}
		}
		// A press on an interactive widget focuses it (pointer semantics, no
		// ring) — the keyboard seam (KeyWidget) and the edit-session funnel
		// both hang off this identity. Disabled widgets take no focus.
		if p.Type == PointerPress && !nodeDisabled(m, rt) {
			e.Inter.Focused = m
			e.Inter.FocusVisible = false
			// The repeat-instance scope (for a widget press inside a list):
			// the item bindings a handler like draggable's data payload needs.
			e.Inter.PressedItem = e.itemIndexOf(hit)
			e.Inter.PressedScope = e.itemScopeOf(hit)
		}
		// Widget-routed events must not starve the hover bookkeeping: without
		// this, Inter.Hovered never moves onto an InteractiveWidget, so its
		// theme hover style (applyInteractiveOverlay) and cursor hint were
		// dead over every widget (select, checkbox, slider, …).
		hoverMoved := p.Type == PointerMove && p.Buttons == 0 && e.updateHover(hit)
		redraw := iw.HandlePointer(m, rt, p, &e.Inter, frame)
		// The widget may have moved focus (a textarea focuses itself on
		// press): open/close the edit session through the same funnel the
		// generic press path uses (input.go).
		e.syncEditSession()
		// A press on the editable positions the caret at the click and arms
		// drag-select (single click), selects the word (double) or the line
		// (triple).
		if p.Type == PointerPress {
			e.placeCaretFromPointer()
		}
		// Same dirty propagation as the capture path above: a toggle's state
		// flip is invisible until the next frame without it.
		if redraw || hoverMoved {
			e.dirty.Store(true)
		}
		return redraw || hoverMoved
	}

	// A blank-space press on an active board starts a pan (drag the empty
	// canvas). A press on anything with a handler — a note's onTouchMove, a
	// pressable, an interactive widget — falls through to the normal path so
	// the note drags instead of the board. A board with `disablePan: true`
	// (side-scroller follow-cam) skips the manual pan — the user has no
	// pan affordance, and the engine's camera follow owns the offset.
	if e.Inter.Board.Active && !e.Inter.Board.PanDisabled && p.Type == PointerPress && boardBlank(hit, rt) {
		e.Inter.Board.PanMomActive = false // cancel any coast from a prior drag
		e.Inter.Board.Panning = true
		e.Inter.Board.PanStart = geom.Point{X: p.X, Y: p.Y}
		e.Inter.Board.PanOrigin = geom.Point{X: e.Inter.Board.PanX, Y: e.Inter.Board.PanY}
		// Blank-space press blurs focus (HTML parity), like the generic press
		// path's empty hit does below.
		e.Inter.Focused, e.Inter.FocusVisible = nil, false
		e.Inter.FocusedItem = 0
		e.syncEditSession()
		e.dirty.Store(true)
		return true
	}

	redraw := false

	switch {
	case p.Type == PointerPress:
		if tgt := VisualTarget(hit, rt); tgt != nil {
			e.Inter.Pressed = tgt
			// Pointer-driven focus never shows the keyboard ring
			// (:focus-visible semantics).
			e.Inter.Focused = tgt
			e.Inter.FocusVisible = false
			// Which repeat instance was hit (0 outside a list): the identity
			// alone names the shared template pointer (list.go).
			idx := e.itemIndexOf(hit)
			e.Inter.PressedItem = idx
			e.Inter.FocusedItem = idx
			e.Inter.PressedScope = e.itemScopeOf(hit)
		} else {
			// Pressing blank space blurs the focused node (HTML parity: focus
			// returns to the body) and ends any input edit session.
			e.Inter.Focused, e.Inter.FocusVisible = nil, false
			e.Inter.FocusedItem = 0
		}
		e.syncEditSession()
		// A press on the editable positions the caret at the click and arms
		// drag-select / double-click word / triple-click line.
		if e.Inter.Input != nil {
			e.placeCaretFromPointer()
		}
		// A press on blank scene area arms the scene-swipe recognizer; a press
		// that found a pressable is a tap on it, and a press on a reorderable
		// item belongs to the reorder gesture — neither starts a swipe.
		e.Inter.Swipe = SwipeTrack{Armed: e.Inter.Pressed == nil && !e.Inter.Reorder.Pending, X: p.X, Y: p.Y}
		redraw = true
	case p.Type == PointerRelease:
		if e.Inter.Pressed != nil {
			e.Inter.Pressed = nil
			e.Inter.PressedItem = 0
			e.Inter.PressedScope = nil
			redraw = true
		}
	case p.Type == PointerMove && p.Buttons == 0:
		// A move that did NOT change the hovered node requests no redraw —
		// otherwise setAcceptsMouseMovedEvents turns every pixel of cursor
		// drift into a full frame, churning the loop and starving the main
		// (event-loop) thread. Drag moves (Buttons>0) are handled below.
		redraw = e.updateHover(hit)
	}

	// A release that travelled past the swipe slop in one dominant direction
	// dispatches the scene's bound swipe action — the touch counterpart of
	// the scene's `keys`, and like keys it needs no focus and no handler on
	// any node. It fires only when nothing else claimed the gesture (every
	// competing path returns early or disarms the tracker), and it outranks
	// the onTouchEnd bubble below: a swipe is a whole-gesture verdict.
	if p.Type == PointerRelease && e.Inter.Swipe.Armed {
		e.Inter.Swipe.Armed = false
		if action, ok := rt.SwipeAction(swipeDirection(p.X-e.Inter.Swipe.X, p.Y-e.Inter.Swipe.Y)); ok {
			e.dispatch(&model.Invoke{Name: action}, nil)
			e.dirty.Store(true)
			return true
		}
	}

	// Bubble up the tree until a handler is found. A node whose style marks it
	// `disabled` is transparent to activation (the web renderer gives it
	// pointer-events:none): its own handlers are skipped and the event falls
	// through to the next ancestor.
	dispatched := false
	for hit != nil {
		var evt *model.Invoke
		var seeds map[string]any
		if !nodeDisabled(hit.Base().Model, rt) {
			switch {
			case p.Type == PointerPress:
				evt = hit.Base().OnPress
				if evt == nil {
					evt = hit.Base().OnTouchStart
					seeds = e.touchSeeds(p)
				}
			case p.Type == PointerRelease:
				evt = hit.Base().OnTouchEnd
				if evt != nil {
					seeds = e.touchSeeds(p)
				}
			case p.Type == PointerMove && p.Buttons > 0:
				evt = hit.Base().OnTouchMove
				if evt != nil {
					seeds = e.touchSeeds(p)
				}
			}
		}
		if evt != nil {
			e.dispatchScoped(evt, seeds, e.itemScopeOf(hit))
			dispatched = true
			break
		}
		if hit.Base().Parent != nil {
			hit = hit.Base().Parent
		} else {
			hit = nil
		}
	}

	changed := redraw || dispatched
	if changed {
		e.dirty.Store(true)
	}
	return changed
}

// propInvoke converts a {name, args} prop object (the onContextMenu and
// onDismissed spellings) into a dispatchable Invoke, or nil when the prop is
// absent or nameless.
func propInvoke(raw any) *model.Invoke {
	m, ok := raw.(map[string]any)
	if !ok {
		return nil
	}
	inv := &model.Invoke{}
	if s, ok := m["name"].(string); ok {
		inv.Name = s
	}
	if args, ok := m["args"].(map[string]any); ok {
		inv.Args = make(map[string]string, len(args))
		for k, v := range args {
			inv.Args[k] = fmt.Sprint(v)
		}
	}
	if inv.Name == "" {
		return nil
	}
	return inv
}

// dispatchContextMenu walks the hit chain for the first node with an
// onContextMenu prop and dispatches it (the browser's contextmenu event),
// reporting whether any handler consumed the press.
func (e *Engine) dispatchContextMenu(hit graph.Node, rt *runtime.Runtime) bool {
	for n := hit; n != nil; {
		if m := n.Base().Model; m != nil && !nodeDisabled(m, rt) {
			if raw, ok := m.Prop("onContextMenu"); ok {
				if inv := propInvoke(raw); inv != nil {
					e.dispatch(inv, nil)
					return true
				}
			}
		}
		p := n.Base().Parent
		if p == nil {
			break
		}
		n = p
	}
	return false
}

// reorderStartSlop is how far a press must drag before a pending reorder
// activates (the HTML path's 5px threshold — taps and small nudges are taps).
const reorderStartSlop = 5.0

// swipeMinDist is how far (physical px) a press must travel before its
// release counts as a scene swipe at all — taps, restless holds and tiny
// nudges stay taps. swipeAxisDominance is how much larger the dominant axis
// must be than the other for the travel to read as a cardinal direction; a
// near-diagonal drag is ambiguous and dispatches nothing.
const (
	swipeMinDist       = 24.0
	swipeAxisDominance = 1.3
)

// swipeDirection classifies a press→release travel into "left" / "right" /
// "up" / "down", or "" when it is not a swipe (too short, or no dominant
// axis). The engine's swipe recognizer (HandlePointer) feeds the result to
// the scene's "swipes" bindings.
func swipeDirection(dx, dy float64) string {
	ax, ay := math.Abs(dx), math.Abs(dy)
	if math.Max(ax, ay) < swipeMinDist {
		return ""
	}
	if ax > ay*swipeAxisDominance {
		if dx > 0 {
			return "right"
		}
		return "left"
	}
	if ay > ax*swipeAxisDominance {
		if dy > 0 {
			return "down"
		}
		return "up"
	}
	return ""
}

// reorderTarget maps a pointer Y to the target slot: whole row steps of the
// dragged item's height (grid rows counting crossAxisCount columns), clamped
// to the data length.
func reorderTarget(r *ReorderState, y float64) int {
	rows := int(math.Round((y - r.PressY) / r.ItemH))
	to := r.From + rows*r.Cols
	if to < 0 {
		to = 0
	}
	if to > r.Count-1 {
		to = r.Count - 1
	}
	return to
}

// armReorder arms a PENDING reorder when the press lands on an item of the
// FIRST list up the hit chain (a nested list stops the walk — the outer list
// must never reorder with an inner item's index) whose reorderable + onReorder
// props are set. It does NOT claim the press: the gesture activates only after
// the drag passes reorderStartSlop, so taps and the item's own handlers keep
// working (an item with a press/touch handler or an interactive child is left
// alone entirely). The list data is evaluated top-level.
func (e *Engine) armReorder(hit graph.Node, rt *runtime.Runtime) {
	for n := hit; n != nil; {
		b := n.Base()
		if m := b.Model; m != nil && (m.Type == "list" || m.Type == "gridview") {
			// The hit's own list is the first one on the chain — check it and
			// STOP: a nested list (or a non-reorderable inner list) must not
			// fall through to an outer list with the wrong item index.
			if raw, ok := m.Prop("reorderable"); ok && truthy(evalStyleProp(raw, rt)) {
				if _, has := m.Prop("onReorder"); has {
					// Only a hit inside an actual item instance arms the
					// gesture (a press in the list's padding/gap hits the list
					// container itself, which is not a drag).
					if inst, ok := e.itemInstanceOf(hit); ok {
						idx := inst.index
						count := len(listData(m, rt, nil))
						if count > 1 && m.Template != nil && !reorderableItemInteractive(hit, m, rt) {
							var itemH float64
							if g := e.findGroupByModelIndex(m.Template, idx); g != nil {
								bb := g.GetBBox()
								itemH = bb.MaxY - bb.MinY
							}
							if itemH > 0 {
								cols := 1
								if m.Type == "gridview" {
									if c := gridColumns(m); c > 1 {
										cols = c
									}
								}
								e.Inter.Reorder = ReorderState{
									Pending: true, List: m, From: idx, To: idx,
									PressX: e.lastPtr.X, PressY: e.lastPtr.Y,
									ItemH: itemH, Count: count, Cols: cols,
								}
							}
						}
					}
				}
			}
			return // the first list up the chain is the item's own: stop here
		}
		p := b.Parent
		if p == nil {
			break
		}
		n = p
	}
}

// reorderableItemInteractive reports whether the hit chain up to (but
// excluding) the list carries a press/touch handler or an interactive widget —
// such an item keeps its own taps and is not reorderable (the drag would
// double-fire the handler).
func reorderableItemInteractive(hit graph.Node, list *model.Node, rt *runtime.Runtime) bool {
	for n := hit; n != nil; {
		b := n.Base()
		if b.Model == list {
			break
		}
		if m := b.Model; m != nil && !nodeDisabled(m, rt) {
			if m.OnPress != nil || m.OnTouchStart != nil || m.OnTouchMove != nil || m.OnTouchEnd != nil {
				return true
			}
			if w, ok := LookupWidget(m.Type); ok {
				if _, yes := w.(InteractiveWidget); yes {
					return true
				}
			}
		}
		p := b.Parent
		if p == nil {
			break
		}
		n = p
	}
	return false
}

// dispatchReorder fires the reorderable list's onReorder handler with
// {from, to} (the author re-sorts their data; the list re-renders).
func (e *Engine) dispatchReorder() {
	r := e.Inter.Reorder
	if r.List == nil {
		return
	}
	if raw, ok := r.List.Prop("onReorder"); ok {
		if inv := propInvoke(raw); inv != nil {
			args := map[string]any{"from": r.From, "to": r.To}
			for k, v := range inv.Args {
				args[k] = v
			}
			e.RT.Dispatch(inv.Name, args)
		}
	}
}

// dropTargetAt walks up from hit for the nearest node whose registered widget
// is a DropTargetWidget, so a drag release can be routed there even when an
// inner interactive widget is closer.
func dropTargetAt(hit graph.Node) (InteractiveWidget, *model.Node, image.Rectangle) {
	for hit != nil {
		if m := hit.Base().Model; m != nil {
			if w, ok := LookupWidget(m.Type); ok {
				if _, yes := w.(DropTargetWidget); yes {
					b := hit.GetBBox()
					return w.(InteractiveWidget), m, image.Rect(int(b.MinX), int(b.MinY), int(b.MaxX), int(b.MaxY))
				}
			}
		}
		p := hit.Base().Parent
		if p == nil {
			break
		}
		hit = p
	}
	return nil, nil, image.Rectangle{}
}

// notifyWidgetFocused tells an interactive widget that keyboard focus landed
// on it (the Tab path, which never routes through HandlePointer): the widget
// caches the engine interaction so its Record can reach the live edit session.
func (e *Engine) notifyWidgetFocused(m *model.Node) {
	if w, ok := LookupWidget(m.Type); ok {
		if fw, yes := w.(FocusHookWidget); yes {
			fw.OnFocused(m, &e.Inter)
		}
	}
}

// updateHover moves the hovered identity to the node at hit, dispatching
// hoverOut on the old chain and hoverIn on the new, and reports whether the
// identity changed (the caller turns a change into a redraw). Extracted from
// HandlePointer's generic move case so the interactive-widget branch can run
// the same bookkeeping for widget-routed moves.
func (e *Engine) updateHover(hit graph.Node) bool {
	hm := ModelOf(hit)
	hi := e.itemIndexOf(hit)
	// Two instances of one template share the model pointer, so the
	// instance index is part of the hovered identity: sliding from item 2
	// to item 3 of the same template IS a hover change.
	if hm == e.Inter.Hovered && hi == e.Inter.HoveredItem {
		return false
	}
	// Hover actions resolve through the live graph — graph identities
	// go stale as soon as a redraw rebuilds the tree. Parent walks
	// must guard the typed *Group (a nil *Group in an interface is
	// non-nil — walking past the root would panic).
	if old := e.findGroupByModelIndex(e.Inter.Hovered, e.Inter.HoveredItem); old != nil {
		for n := old; n != nil; {
			if n.Base().OnHoverOut != nil {
				e.dispatchScoped(n.Base().OnHoverOut, nil, e.itemScopeOf(n))
				break
			}
			if p := n.Base().Parent; p != nil {
				n = p
			} else {
				break
			}
		}
	}
	for n := hit; n != nil; {
		if n.Base().OnHoverIn != nil {
			e.dispatchScoped(n.Base().OnHoverIn, nil, e.itemScopeOf(n))
			break
		}
		if p := n.Base().Parent; p != nil {
			n = p
		} else {
			break
		}
	}
	e.Inter.Hovered = hm
	e.Inter.HoveredItem = hi
	return true
}

// HandleScroll processes one scroll/wheel event against the last rendered
// frame: the gesture hit-tests where the pointer rests (ScrollInput carries
// no coordinates, so the engine tracks it from pointer moves), then walks the
// ancestor chain applying the delta to each `scroll` viewport's offset —
// clamped to [0, contentHeight-viewportHeight], the unconsumed remainder
// bubbling outward to nested viewports (the web's scroll chaining). A
// consumed delta requests a redraw; a gesture over no viewport — or one with
// nothing left to consume — reports no change. Vertical only for now.
func (e *Engine) HandleScroll(s ScrollInput) bool {
	rt := e.RT
	if rt == nil || e.graphRoot == nil || !e.hasPtr {
		return false
	}
	// A board's control-modified scroll (trackpad pinch, Ctrl+wheel) zooms at
	// the cursor — never chained into viewport scrolling.
	if e.Inter.Board.Active && s.Ctrl {
		return e.boardZoom(e.lastPtr, s.DY)
	}
	hit := e.graphRoot.HitTest(e.lastPtr)
	dx, dy := s.DX, s.DY
	changed := false
	for n := hit; n != nil && (dx != 0 || dy != 0); {
		if g, ok := n.(*graph.Group); ok {
			if m := g.Base().Model; m != nil && isScrollType(m.Type) {
				beforeX, beforeY := dx, dy
				dx, dy = e.scrollViewport(g, m, dx, dy)
				changed = changed || dx != beforeX || dy != beforeY
			}
		}
		p := n.Base().Parent
		if p == nil {
			break
		}
		n = p
	}
	// An unconsumed wheel/trackpad scroll over a board pans the canvas — the
	// board is the outer fallback consumer of scroll, so a wheel over empty
	// whiteboard space drags the canvas instead of doing nothing. The sign
	// mirrors scrollViewport (positive dy = toward content bottom → content
	// shifts up, hence the subtract).
	if (dx != 0 || dy != 0) && e.Inter.Board.Active {
		e.Inter.Board.PanX -= dx
		e.Inter.Board.PanY -= dy
		changed = true
	}
	if changed {
		e.dirty.Store(true)
	}
	return changed
}

// boardBlank reports whether the hit (and every ancestor) is bare board
// background — no pressable, touch handler or interactive widget anywhere up
// the chain — so the board may claim a blank-space press as a pan. Mirrors the
// dispatch walk's conditions (disabled nodes are transparent).
func boardBlank(hit graph.Node, rt *runtime.Runtime) bool {
	for hit != nil {
		b := hit.Base()
		if m := b.Model; m != nil && !nodeDisabled(m, rt) {
			if m.OnPress != nil || m.OnTouchStart != nil || m.OnTouchMove != nil || m.OnTouchEnd != nil {
				return false
			}
			if w, ok := LookupWidget(m.Type); ok {
				if _, yes := w.(InteractiveWidget); yes {
					return false
				}
			}
		}
		p := b.Parent
		if p == nil {
			break
		}
		hit = p
	}
	return true
}

// boardZoom zooms the board around the pointer so the content under the cursor
// stays fixed (zoom-to-cursor): pan' = C − (C − pan)·(z/old). dy is the scroll
// delta in physical px — one wheel notch ≈ 120px and each notch scales ~1.1×,
// positive dy zooming in. Returns whether the zoom actually changed (clamped
// at the range ends → no redraw).
func (e *Engine) boardZoom(cursor geom.Point, dy float64) bool {
	b := &e.Inter.Board
	if b.Zoom <= 0 {
		b.Zoom = 1
	}
	z := clampFloat(b.Zoom*math.Pow(1.1, dy/120), minBoardZoom, maxBoardZoom)
	if z == b.Zoom {
		return false
	}
	ratio := z / b.Zoom
	b.Zoom = z
	b.PanX = cursor.X - (cursor.X-b.PanX)*ratio
	b.PanY = cursor.Y - (cursor.Y-b.PanY)*ratio
	e.dirty.Store(true)
	return true
}

// clampFloat bounds v to [lo, hi].
func clampFloat(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// HandleKey processes one keyboard event: a focused input consumes text /
// cursor / delete keys for editing (input.go); otherwise Tab traversal,
// Enter/Space activation, Escape clears focus, everything else dispatches
// onKeyDown / onKeyUp (focused node first, bubbling to the scene root).
// Always returns true — focus/ring/activation visuals may have changed.
func (e *Engine) HandleKey(k KeyInput) bool {
	rt := e.RT
	if rt == nil || e.graphRoot == nil {
		return false
	}
	handled := false
	if k.Down {
		// A focused key widget (game, rich editor) owns the keyboard before
		// anything else — escape passes through to the generic blur below.
		if f := e.Inter.Focused; f != nil && k.Key != "escape" {
			if w, ok := LookupWidget(f.Type); ok {
				if kw, yes := w.(KeyWidget); yes {
					consumed, redraw := kw.HandleKey(f, rt, k, &e.Inter)
					if redraw {
						e.dirty.Store(true)
					}
					if consumed {
						return true
					}
				}
			}
		}
		// A focused input owns the keyboard: text, cursor and delete keys edit
		// its buffer (input.go handleEditKey); tab and escape keep their focus
		// semantics below and close the session via syncEditSession.
		if k.Key != "tab" && k.Key != "escape" {
			handled = e.handleEditKey(k)
		}
		if !handled {
			switch k.Key {
			case "tab":
				e.Inter.FocusVisible = true
				// Focusables walks the model tree, which holds a list's
				// template once — never the repeat instances — so keyboard
				// focus always lands outside lists and the item companion is 0.
				list := Focusables(e.sceneRoot(), rt)
				// Advance past focusables with no rendered instance (an empty
				// list's template, a conditionally-hidden node): focus must
				// land somewhere visible, so the ring actually draws.
				f := NextFocus(list, e.Inter.Focused, !k.Shift)
				for range list {
					if f == nil {
						break
					}
					if e.findGroupByModelIndex(f, 0) != nil {
						break
					}
					f = NextFocus(list, f, !k.Shift)
				}
				e.Inter.Focused = f
				e.Inter.FocusedItem = 0
				if e.Inter.Focused != nil {
					// Keyboard focus does not route through the widget's
					// HandlePointer, so tell a FocusHookWidget it now owns the
					// interaction (it caches the live edit session).
					e.notifyWidgetFocused(e.Inter.Focused)
					// Scroll the newly focused node into view (a focus ring on a
					// clipped node is invisible until the viewport scrolls it in).
					e.ensureFocusVisible(e.Inter.Focused)
				}
				e.syncEditSession()
				handled = true
			case "return", "space":
				// Re-check at dispatch time: the focused node may have become
				// disabled or unmounted (if/visible/show flip, when-branch switch)
				// since it gained focus.
				if f := e.Inter.Focused; f != nil && f.OnPress != nil && !nodeDisabled(f, rt) && nodeMounted(e.sceneRoot(), f, rt) {
					e.dispatchScoped(f.OnPress, nil, e.itemScopeOf(e.findGroupByModelIndex(f, e.Inter.FocusedItem)))
					handled = true
				}
			case "escape":
				if e.Inter.Focused != nil {
					e.Inter.Focused, e.Inter.FocusVisible = nil, false
					e.Inter.FocusedItem = 0
					e.syncEditSession()
					handled = true
				}
			}
		}
	}
	// Scene-level key bindings (the scene's own `keys` map — games and
	// keyboard-driven apps): key → action, no focus required. Generic
	// handlers in the k.Down branch above win any conflict (tab/return/
	// escape keep their meaning); the map fills the rest. On keyup
	// (k.Down == false) the keyReleases map is tried so a "hold to move"
	// game can clear its direction flag without a focused widget;
	// otherwise the bubbling onKeyUp on a focused widget picks it up.
	//
	// Run AFTER the k.Down branch and OUTSIDE its `if k.Down` guard —
	// otherwise a keyup event skips the keyReleases map entirely and the
	// game's "hold to run" never gets its stopMove/release action. The
	// `!handled` short-circuit means a focused widget that consumed the
	// event in the k.Down branch still wins (a focused text field keeps
	// its delete key from leaking to the scene).
	if !handled {
		if !k.Down {
			if action, ok := rt.KeyReleaseAction(k.Key); ok {
				e.dispatch(&model.Invoke{Name: action}, nil)
				e.dirty.Store(true)
				handled = true
			}
		} else if action, ok := rt.KeyAction(k.Key); ok {
			e.dispatch(&model.Invoke{Name: action}, nil)
			e.dirty.Store(true)
			handled = true
		}
	}
	if !handled {
		start := e.findGroupByModelIndex(e.Inter.Focused, e.Inter.FocusedItem)
		if start == nil {
			start = e.graphRoot
		}
		for n := start; n != nil; {
			var evt *model.Invoke
			if k.Down {
				evt = n.Base().OnKeyDown
			} else {
				evt = n.Base().OnKeyUp
			}
			if evt != nil {
				e.dispatchScoped(evt, map[string]any{"key": k.Key}, e.itemScopeOf(n))
				break
			}
			if p := n.Base().Parent; p != nil {
				n = p
			} else {
				break
			}
		}
	}
	e.dirty.Store(true)
	return true
}

// sceneRoot resolves the root node of the active scene.
func (e *Engine) sceneRoot() *model.Node {
	root := e.RT.App.EntryRoot()
	if sceneID := e.RT.CurrentScene(); sceneID != "" {
		if sc := e.RT.App.Scenes[sceneID]; sc != nil {
			root = sc
		}
	}
	return root
}

// validThemeName gates what state.theme may name: skin ids only (the shipped
// skins are apple-light/apple-dark/win11-light/win11-dark). state.theme is
// writable by any action and by MCP set_state — unsanitized it was spliced
// straight into a file path, so "../../evil" loaded arbitrary JSON as a skin.
var validThemeName = regexp.MustCompile(`^[a-z0-9-]+$`)

// SetThemeLoaded pre-seeds the internal theme cache so resolveTheme skips
// its os.ReadFile probe. The WASM build calls this to inject a theme loaded
// from the doc list (no filesystem in the browser).
func (e *Engine) SetThemeLoaded(name string) {
	e.themeLoaded = name
	e.themeFailed = ""
}

// resolveTheme implements dynamic theme switching: state.theme names a
// themes/<name>.json skin; without one the default theme applies. The name is
// sanitized (no path traversal) and resolved against the app's own themes/
// directory first, then the working directory's — the HTML backend maps the
// same names onto built-in palettes and never reads the disk. A failed load
// is negatively cached (themeFailed): the failure logs once and is not
// retried until state.theme changes, instead of spinning disk + stderr every
// frame of an animation.
func (e *Engine) resolveTheme() {
	rt := e.RT
	themeName, ok := rt.State["theme"].(string)
	if !ok || themeName == "" {
		if rt.Theme == nil {
			rt.Theme = theme.GetDefault()
		}
		return
	}
	// Built-in aliases, mirroring the HTML shell (render/theme.go
	// ThemeVarsFor): "apple"/"auto" (and "" above) mean the default palette,
	// "dark" the dark one, "material" the Material light one. Resolve them to
	// the corresponding skin name and fall through the NORMAL probe — disk
	// file when present, built-in adopt otherwise — so an alias lands exactly
	// where an explicit skin name lands (no dual-source drift, and the widely
	// used theme:"apple" never spams the FAILED log).
	switch themeName {
	case "apple", "auto":
		themeName = "apple-light"
	case "dark":
		themeName = "apple-dark"
	case "material":
		themeName = "win11-light"
	}
	if e.themeLoaded == themeName {
		return // already active (keyed by requested name, not the file's inner name)
	}
	if e.themeFailed == themeName {
		return // known-bad name; retried only when state.theme changes
	}
	if !validThemeName.MatchString(themeName) {
		e.themeFailed = themeName
		fmt.Fprintf(os.Stderr, "[qorm theme] rejected theme name %q (skin ids match %s)\n", themeName, validThemeName)
		return
	}
	// A skin file on disk is AUTHORITATIVE: load it even when the current
	// theme already carries this name. The built-in default and
	// themes/<name>.json legitimately differ (the JSON ships component styles
	// the in-code default lacks — button shadow, animated_container radius),
	// and the first frame must look identical to a later re-toggle.
	for _, dir := range e.themeDirs() {
		if t, err := theme.LoadTheme(filepath.Join(dir, themeName+".json")); err == nil {
			rt.Theme = t
			e.themeLoaded = themeName
			e.themeFailed = ""
			fmt.Fprintf(os.Stderr, "[qorm theme] loaded %q\n", themeName)
			return
		}
	}
	// No skin file by this name: the default palette always exists in code,
	// so falling back to it is not a failure (HTML serves it unconditionally
	// for ""/apple/auto) — and a round trip dark → apple-light must work
	// without a themes/ directory.
	if themeName == "apple-light" {
		rt.Theme = theme.GetDefault()
		e.themeLoaded = themeName
		e.themeFailed = ""
		return
	}
	// No skin file by this name: adopt the current theme when it already
	// matches (the built-in default with no themes/ dir on disk), else fail
	// once and keep what we have.
	if rt.Theme != nil && rt.Theme.Name == themeName {
		e.themeLoaded = themeName
		return
	}
	e.themeFailed = themeName
	if rt.Theme == nil {
		rt.Theme = theme.GetDefault()
	}
	fmt.Fprintf(os.Stderr, "[qorm theme] FAILED to load %q, keeping %q\n", themeName, rt.Theme.Name)
}

// themeDirs lists the directories searched for <name>.json, in order: the
// app's own themes/ directory (app-shipped skins) first, then the working
// directory's (the repo/dev layout, where qorm run is invoked from a
// directory containing themes/).
func (e *Engine) themeDirs() []string {
	if bd := e.RT.App.BaseDir; bd != "" {
		return []string{filepath.Join(bd, "themes"), "themes"}
	}
	return []string{"themes"}
}

// drainMotionCompletes runs timeline onComplete actions queued during measure
// (fx has no onComplete — only timelineOnComplete / onComplete on timeline nodes).
func (e *Engine) drainMotionCompletes() {
	if e == nil || len(e.Inter.MotionCompletes) == 0 {
		return
	}
	q := e.Inter.MotionCompletes
	e.Inter.MotionCompletes = nil
	for _, pc := range q {
		e.dispatch(pc.inv, pc.seeds)
		e.dirty.Store(true)
	}
}

// dispatch evaluates invoke args against live state and runs the action.
// seeds pre-populates args (e.g. the pressed key); author args win.
func (e *Engine) dispatch(evt *model.Invoke, seeds map[string]any) {
	e.dispatchScoped(evt, seeds, nil)
}

// dispatchScoped is dispatch with a repeat scope: for a handler declared
// inside a list template, scope is the instance's item scope, so arg (and
// action-name) bindings like {{item.id}} or {{index}} evaluate against the
// item that was actually hit — the canvas mirror of the HTML path's
// Handler.Scope capture (render.go:824, consumed at server.go:1336).
func (e *Engine) dispatchScoped(evt *model.Invoke, seeds, scope map[string]any) {
	if evt == nil {
		return
	}
	ctx := map[string]any{"state": e.RT.State}
	for k, v := range scope {
		ctx[k] = v
	}
	name := evt.Name
	if strings.Contains(name, "{{") {
		// The action name may itself be a binding (the HTML renderer resolves
		// it at handler registration, render.go:820); with a repeat scope this
		// is how items name per-row actions.
		name = runtime.Stringify(runtime.EvalBinding(name, ctx))
	}
	args := make(map[string]any, len(evt.Args)+len(seeds))
	for k, v := range seeds {
		args[k] = v
	}
	for k, v := range evt.Args {
		args[k] = runtime.EvalBinding(v, ctx)
	}
	e.RT.Dispatch(name, args)
}

// touchSeeds seeds a touch handler's args with the pointer position. On a
// board the coordinates are mapped to BOARD space (inverse of the live pan/
// zoom), so a note-drag action works in board units and zooms cancel out —
// dragging the same screen distance moves the note the same board distance at
// any zoom. Outside a board the raw physical px pass through.
func (e *Engine) touchSeeds(p PointerInput) map[string]any {
	x, y := p.X, p.Y
	if e.Inter.Board.Active {
		if z := e.Inter.Board.Zoom; z > 0 {
			x = (x - e.Inter.Board.PanX) / z
			y = (y - e.Inter.Board.PanY) / z
		}
	}
	return map[string]any{"x": x, "y": y}
}

// itemInstanceOf resolves the repeat instance a graph node belongs to by
// walking its ancestors to the first sidecar entry the layout recorded —
// the innermost list wins, matching the scope shadowing rules (list.go).
func (e *Engine) itemInstanceOf(n graph.Node) (itemInstance, bool) {
	for n != nil {
		if inst, ok := e.itemInstances[n]; ok {
			return inst, true
		}
		p := n.Base().Parent
		if p == nil {
			return itemInstance{}, false
		}
		n = p
	}
	return itemInstance{}, false
}

// itemIndexOf returns the hit's repeat index, 0 outside any list — the
// companion half of the (node, index) interaction identity.
func (e *Engine) itemIndexOf(n graph.Node) int {
	if inst, ok := e.itemInstanceOf(n); ok {
		return inst.index
	}
	return 0
}

// itemScopeOf returns the item scope a hit's handlers evaluate against, nil
// outside any list.
func (e *Engine) itemScopeOf(n graph.Node) map[string]any {
	if inst, ok := e.itemInstanceOf(n); ok {
		return inst.vars
	}
	return nil
}

// findGroupByModel locates the graph node built from model node m — the first
// match in layout order, which is instance 0 when m is a repeat template.
func (e *Engine) findGroupByModel(m *model.Node) graph.Node {
	return e.findGroupByModelIndex(m, 0)
}

// findGroupByModelIndex is findGroupByModel selecting the itemIndex-th match:
// a repeat template's model pointer backs one graph node per instance, and
// the interaction companions (PressedItem & friends) say which one is meant.
func (e *Engine) findGroupByModelIndex(m *model.Node, itemIndex int) graph.Node {
	if m == nil || e.graphRoot == nil {
		return nil
	}
	var found graph.Node
	var walk func(n graph.Node) bool // true = stop the search
	walk = func(n graph.Node) bool {
		if n == nil {
			return false
		}
		if n.Base().Model == m && !n.Base().Overlay {
			if itemIndex <= 0 {
				found = n
				return true
			}
			itemIndex--
		}
		if g, ok := n.(*graph.Group); ok {
			for _, c := range g.Children {
				if walk(c) {
					return true
				}
			}
		}
		return false
	}
	walk(e.graphRoot)
	return found
}

// physics runs the AABB collision pass for nodes with OnCollide
// (moved unchanged from the former window loop).
func (e *Engine) physics(rootNode graph.Node) {
	rt := e.RT
	isDirty, _ := rt.State["_physics_dirty"].(bool)
	if rootNode != nil && !isDirty {
		var colliders []graph.Node
		var collect func(n graph.Node)
		collect = func(n graph.Node) {
			if n == nil {
				return
			}
			if n.Base().OnCollide != nil {
				colliders = append(colliders, n)
			}
			if g, ok := n.(*graph.Group); ok {
				for _, child := range g.Children {
					collect(child)
				}
			}
		}
		collect(rootNode)

		hasCollision := false
		for i := 0; i < len(colliders); i++ {
			for j := i + 1; j < len(colliders); j++ {
				if colliders[i].GetBBox().Intersects(colliders[j].GetBBox()) {
					hasCollision = true
					a := colliders[i]
					args := make(map[string]any)
					for k, v := range a.Base().OnCollide.Args {
						args[k] = runtime.EvalBinding(v, map[string]any{"state": rt.State})
					}
					// Lock physics to prevent an infinite trigger loop this tick.
					rt.State["_physics_dirty"] = true
					rt.Dispatch(a.Base().OnCollide.Name, args)
					break
				}
			}
			if hasCollision {
				break
			}
		}
	} else if rt.State["_physics_dirty"] != nil && rt.State["_physics_dirty"].(bool) {
		rt.State["_physics_dirty"] = false
	}
}
