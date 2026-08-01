package canvas

import (
	"fmt"
	"image"
	"os"
	"path/filepath"
	"regexp"
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
}

type KeyInput struct {
	Key   string // normalized name ("tab", "return", "a", …)
	Shift bool
	Down  bool // false = key up
}

// ScrollInput is a mouse-wheel or trackpad scroll, in physical pixels per
// notch/gesture tick (the host scales platform deltas like pointer coords).
type ScrollInput struct {
	DX, DY float64
}

// FrameStats carries per-phase timings of one DrawFrame (Unity-style frame
// profiling, emitted to stderr when QORM_FRAME_STATS=1).
type FrameStats struct {
	LayoutRecord time.Duration // measure + layout + display-list recording
	Render       time.Duration // display list → pixels
	Present      time.Duration // buffer publish
	Total        time.Duration
}

// Engine drives the native backend's staged frame loop:
//
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

	ops       op.Ops
	graphRoot graph.Node
	lastRoot  *model.Node

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

// Animating reports whether a tween is still in flight (the host should keep
// ticking until this is false).
func (e *Engine) Animating() bool { return e.animating.Load() }

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
	root := e.sceneRoot()
	if root != e.lastRoot {
		e.Inter = Interaction{}
		e.lastRoot = root
	}

	// Layout + record. The display list is rebuilt from scratch each frame;
	// Reset keeps the backing array, so steady-state recording allocates
	// nothing. (Before the Engine owned this, ops accumulated forever.)
	t0 := time.Now()
	e.ops.Reset()
	rootNode, needsRedraw := Layout(&e.ops, root, size, rt, &e.Inter, scale)
	e.graphRoot = rootNode
	st.LayoutRecord = time.Since(t0)

	e.physics(rootNode)

	t1 := time.Now()
	e.Renderer.Render(&e.ops, target)
	st.Render = time.Since(t1)

	st.Total = time.Since(start)
	if e.StatsEnabled {
		fmt.Fprintf(os.Stderr, "[qorm frame] layout+record=%s render=%s total=%s\n",
			st.LayoutRecord, st.Render, st.Total)
	}

	// dirty was consumed at the top of the frame; animation keeps the loop
	// ticking until the tweens settle (no separate timer goroutine — the host
	// polls).
	e.animating.Store(needsRedraw)
	return true, st
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
	redraw := false

	switch {
	case p.Type == PointerPress:
		if tgt := VisualTarget(hit, rt); tgt != nil {
			e.Inter.Pressed = tgt
			// Pointer-driven focus never shows the keyboard ring
			// (:focus-visible semantics).
			e.Inter.Focused = tgt
			e.Inter.FocusVisible = false
		}
		redraw = true
	case p.Type == PointerRelease:
		if e.Inter.Pressed != nil {
			e.Inter.Pressed = nil
			redraw = true
		}
	case p.Type == PointerMove && p.Buttons == 0:
		if hm := ModelOf(hit); hm != e.Inter.Hovered {
			// Hover actions resolve through the live graph — graph identities
			// go stale as soon as a redraw rebuilds the tree. Parent walks
			// must guard the typed *Group (a nil *Group in an interface is
			// non-nil — walking past the root would panic).
			if old := e.findGroupByModel(e.Inter.Hovered); old != nil {
				for n := old; n != nil; {
					if n.Base().OnHoverOut != nil {
						e.dispatch(n.Base().OnHoverOut, nil)
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
					e.dispatch(n.Base().OnHoverIn, nil)
					break
				}
				if p := n.Base().Parent; p != nil {
					n = p
				} else {
					break
				}
			}
			e.Inter.Hovered = hm
			redraw = true
		}
		// A move that did NOT change the hovered node requests no redraw —
		// otherwise setAcceptsMouseMovedEvents turns every pixel of cursor
		// drift into a full frame, churning the loop and starving the main
		// (event-loop) thread. Drag moves (Buttons>0) are handled below.
	}

	// Bubble up the tree until a handler is found. A node whose style marks it
	// `disabled` is transparent to activation (the web renderer gives it
	// pointer-events:none): its own handlers are skipped and the event falls
	// through to the next ancestor.
	dispatched := false
	for hit != nil {
		var evt *model.Invoke
		if !nodeDisabled(hit.Base().Model, rt) {
			if p.Type == PointerPress {
				evt = hit.Base().OnPress
				if evt == nil {
					evt = hit.Base().OnTouchStart
				}
			} else if p.Type == PointerRelease {
				evt = hit.Base().OnTouchEnd
			} else if p.Type == PointerMove && p.Buttons > 0 {
				evt = hit.Base().OnTouchMove
			}
		}
		if evt != nil {
			e.dispatch(evt, nil)
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

// HandleScroll processes one scroll/wheel event against the last rendered
// frame. Scroll containers are not implemented yet (third milestone), so this
// is deliberately a no-op that reports no visual change — it exists so the
// host routes ScrollInput into the engine instead of silently dropping it.
func (e *Engine) HandleScroll(s ScrollInput) bool {
	return false
}

// HandleKey processes one keyboard event: Tab traversal, Enter/Space
// activation, Escape clears focus, everything else dispatches onKeyDown /
// onKeyUp (focused node first, bubbling to the scene root). Always returns
// true — focus/ring/activation visuals may have changed.
func (e *Engine) HandleKey(k KeyInput) bool {
	rt := e.RT
	if rt == nil || e.graphRoot == nil {
		return false
	}
	handled := false
	if k.Down {
		switch k.Key {
		case "tab":
			e.Inter.FocusVisible = true
			e.Inter.Focused = NextFocus(Focusables(e.sceneRoot(), rt), e.Inter.Focused, !k.Shift)
			handled = true
		case "return", "space":
			// Re-check at dispatch time: the focused node may have become
			// disabled or unmounted (if/visible/show flip, when-branch switch)
			// since it gained focus.
			if f := e.Inter.Focused; f != nil && f.OnPress != nil && !nodeDisabled(f, rt) && nodeMounted(e.sceneRoot(), f, rt) {
				e.dispatch(f.OnPress, nil)
				handled = true
			}
		case "escape":
			if e.Inter.Focused != nil {
				e.Inter.Focused, e.Inter.FocusVisible = nil, false
				handled = true
			}
		}
	}
	if !handled {
		start := e.findGroupByModel(e.Inter.Focused)
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
				e.dispatch(evt, map[string]any{"key": k.Key})
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
	if e.themeLoaded == themeName {
		return // already active (keyed by requested name, not the file's inner name)
	}
	if rt.Theme != nil && rt.Theme.Name == themeName {
		e.themeLoaded = themeName // adopted a theme set elsewhere; stop probing
		return
	}
	if e.themeFailed == themeName {
		return // known-bad name; retried only when state.theme changes
	}
	if !validThemeName.MatchString(themeName) {
		e.themeFailed = themeName
		fmt.Fprintf(os.Stderr, "[qorm theme] rejected theme name %q (skin ids match %s)\n", themeName, validThemeName)
		return
	}
	for _, dir := range e.themeDirs() {
		if t, err := theme.LoadTheme(filepath.Join(dir, themeName+".json")); err == nil {
			rt.Theme = t
			e.themeLoaded = themeName
			e.themeFailed = ""
			fmt.Fprintf(os.Stderr, "[qorm theme] loaded %q\n", themeName)
			return
		}
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

// dispatch evaluates invoke args against live state and runs the action.
// seeds pre-populates args (e.g. the pressed key); author args win.
func (e *Engine) dispatch(evt *model.Invoke, seeds map[string]any) {
	if evt == nil {
		return
	}
	args := make(map[string]any, len(evt.Args)+len(seeds))
	for k, v := range seeds {
		args[k] = v
	}
	for k, v := range evt.Args {
		args[k] = runtime.EvalBinding(v, map[string]any{"state": e.RT.State})
	}
	e.RT.Dispatch(evt.Name, args)
}

// findGroupByModel locates the graph node built from model node m.
func (e *Engine) findGroupByModel(m *model.Node) graph.Node {
	if m == nil || e.graphRoot == nil {
		return nil
	}
	var walk func(n graph.Node) graph.Node
	walk = func(n graph.Node) graph.Node {
		if n == nil {
			return nil
		}
		if n.Base().Model == m {
			return n
		}
		if g, ok := n.(*graph.Group); ok {
			for _, c := range g.Children {
				if hit := walk(c); hit != nil {
					return hit
				}
			}
		}
		return nil
	}
	return walk(e.graphRoot)
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
