package canvas

import (
	"fmt"
	"image"
	"os"
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
	buf      *image.RGBA
	Presents int
	// ScaleFactor is the device-pixel ratio for tests (0/1 == 1). The buffer
	// is expected to already be in physical pixels (size == logical * Scale).
	ScaleFactor int
}

func NewHeadlessSurface(size image.Point) *HeadlessSurface {
	return &HeadlessSurface{buf: image.NewRGBA(image.Rect(0, 0, size.X, size.Y)), ScaleFactor: 1}
}

func (s *HeadlessSurface) Size() image.Point       { return s.buf.Rect.Size() }
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
//	input/state → layout → record (display list) → render → present
//
// It owns the interaction state, the (reused) display-list buffer and the
// last rendered graph. Threading contract: all Engine methods are called from
// a single goroutine (the host's event loop); OnRedraw fires from a timer
// goroutine and the host must marshal it back.
type Engine struct {
	RT       *runtime.Runtime
	Renderer Renderer
	Surface  Surface
	Inter    Interaction

	// OnRedraw is invoked (from a separate goroutine, ~16ms later) when an
	// animation is mid-flight and another frame is needed.
	OnRedraw func()

	StatsEnabled bool

	// dirty marks a pending frame: input handlers and MarkDirty set it,
	// DrawFrame clears it. A clean engine skips the whole frame.
	dirty bool

	ops       op.Ops
	graphRoot graph.Node
	lastRoot  *model.Node
}

func NewEngine(rt *runtime.Runtime, r Renderer, s Surface) *Engine {
	if r == nil {
		r = SoftwareRenderer{}
	}
	return &Engine{
		RT:           rt,
		Renderer:     r,
		Surface:      s,
		StatsEnabled: os.Getenv("QORM_FRAME_STATS") == "1",
		dirty:        true, // the first frame always renders
	}
}

// MarkDirty flags the engine as needing a redraw (external state change).
func (e *Engine) MarkDirty() { e.dirty = true }

// DrawFrame runs one staged frame. Safe to call whenever a redraw is needed
// (state change, input, animation tick); when nothing changed since the last
// frame (clean) it is a no-op — no layout, render or present.
func (e *Engine) DrawFrame() FrameStats {
	var st FrameStats
	rt := e.RT
	if rt == nil || rt.App == nil {
		return st
	}
	if !e.dirty {
		return FrameStats{}
	}
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
	rootNode, needsRedraw := Layout(&e.ops, root, e.Surface.Size(), rt, &e.Inter, e.Surface.Scale())
	e.graphRoot = rootNode
	st.LayoutRecord = time.Since(t0)

	e.physics(rootNode)

	t1 := time.Now()
	e.Renderer.Render(&e.ops, e.Surface.Backbuffer())
	st.Render = time.Since(t1)

	t2 := time.Now()
	e.Surface.Present()
	st.Present = time.Since(t2)

	st.Total = time.Since(start)
	if e.StatsEnabled {
		fmt.Fprintf(os.Stderr, "[qorm frame] layout+record=%s render=%s present=%s total=%s\n",
			st.LayoutRecord, st.Render, st.Present, st.Total)
	}

	// This frame consumed the dirty state…
	e.dirty = false

	// Animation continuation: ~60fps ticker until tweens settle. The
	// continuation re-marks dirty so its DrawFrame actually renders.
	// (Vsync / dirty-tree scheduling is a later milestone.)
	if needsRedraw {
		e.dirty = true
		if e.OnRedraw != nil {
			cb := e.OnRedraw
			go func() {
				time.Sleep(16 * time.Millisecond)
				cb()
			}()
		}
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
	hit := e.graphRoot.HitTest(geom.Point{X: p.X, Y: p.Y})
	redraw := false

	switch {
	case p.Type == PointerPress:
		if tgt := VisualTarget(hit); tgt != nil {
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
	}

	// Bubble up the tree until a handler is found.
	dispatched := false
	for hit != nil {
		var evt *model.Invoke
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
		e.dirty = true
	}
	return changed
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
			e.Inter.Focused = NextFocus(Focusables(e.sceneRoot()), e.Inter.Focused, !k.Shift)
			handled = true
		case "return", "space":
			if f := e.Inter.Focused; f != nil && f.OnPress != nil {
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
	e.dirty = true
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

// resolveTheme implements dynamic theme switching: state.theme names a
// themes/<name>.json skin; without one the default theme applies.
// (Fragility noted: the path is cwd-relative — GetDefault covers failure.)
func (e *Engine) resolveTheme() {
	rt := e.RT
	themeName, ok := rt.State["theme"].(string)
	if ok && themeName != "" && (rt.Theme == nil || rt.Theme.Name != themeName) {
		if t, err := theme.LoadTheme(fmt.Sprintf("themes/%s.json", themeName)); err == nil {
			rt.Theme = t
		}
	} else if !ok && rt.Theme == nil {
		rt.Theme = theme.GetDefault()
	}
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
