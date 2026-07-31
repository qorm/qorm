//go:build !desktop && darwin

package main

import (
	"fmt"
	"image"
	"net"
	"sync"
	"time"

	"github.com/qorm/qorm/internal/app"
	"github.com/qorm/qorm/internal/geom"
	"github.com/qorm/qorm/internal/model"
	"github.com/qorm/qorm/internal/op"
	"github.com/qorm/qorm/internal/render/canvas"
	"github.com/qorm/qorm/internal/render/graph"
	"github.com/qorm/qorm/internal/runtime"
	"github.com/qorm/qorm/internal/server"
	"github.com/qorm/qorm/internal/theme"
)

func launchWindow(srv *server.Server, ln net.Listener, url, title string) bool {
	aw := srv.AppWindow()
	ww, hh := aw.Width, aw.Height
	if ww == 0 {
		ww = 400
	}
	if hh == 0 {
		hh = 820
	}

	// Canvas mode: the HTTP listener is never served (app.Main blocks), so
	// there are no browser clients or SSE subscribers — skip HTML rendering.
	srv.SetCanvasHost(true)

	win := app.NewWindow(title, ww, hh)

	go func() {
		var ops op.Ops

		// Cross-frame interaction state: pressed/hovered/focused identities,
		// keyed by *model.Node (stable between frames; the graph is rebuilt
		// every layout and re-stamped from this state).
		inter := &canvas.Interaction{}
		var lastRoot *model.Node
		var currentGraphNode graph.Node

		// drawFrame runs from the event loop AND from the animation
		// continuation goroutine — serialize all frame work (ops, graph,
		// currentGraphNode) under one lock.
		var drawMu sync.Mutex
		redraw := func(rt *runtime.Runtime) {
			drawMu.Lock()
			currentGraphNode = drawFrame(srv, win, &ops, rt, inter, &lastRoot, ww, hh)
			drawMu.Unlock()
		}

		srv.OnStateChange = redraw

		// findGroupByModel locates the graph node built from model node m
		// (used to bubble keyboard/hover events from a stable identity).
		var findGroupByModel func(n graph.Node, m *model.Node) graph.Node
		findGroupByModel = func(n graph.Node, m *model.Node) graph.Node {
			if n == nil || m == nil {
				return nil
			}
			if n.Base().Model == m {
				return n
			}
			if g, ok := n.(*graph.Group); ok {
				for _, c := range g.Children {
					if hit := findGroupByModel(c, m); hit != nil {
						return hit
					}
				}
			}
			return nil
		}

		for e := range win.Events() {
			switch e := e.(type) {
			case app.FrameEvent:
				// Ensure initial draw triggers
				redraw(srv.Runtime())

			case app.KeyEvent:
				rt := srv.Runtime()
				if currentGraphNode == nil {
					continue
				}
				handled := false
				if e.Type == app.KeyDown {
					switch e.Key {
					case "tab":
						inter.FocusVisible = true
						inter.Focused = canvas.NextFocus(canvas.Focusables(sceneRoot(rt)), inter.Focused, !e.Shift)
						handled = true
					case "return", "space":
						if f := inter.Focused; f != nil && f.OnPress != nil {
							dispatchInvoke(rt, f.OnPress, nil)
							handled = true
						}
					case "escape":
						if inter.Focused != nil {
							inter.Focused, inter.FocusVisible = nil, false
							handled = true
						}
					}
				}
				if !handled {
					// Deliver to the focused node first, then bubble up to the
					// scene root; the first handler found wins. The pressed key
					// name is injected as a "key" arg unless the author defined one.
					start := findGroupByModel(currentGraphNode, inter.Focused)
					if start == nil {
						start = currentGraphNode
					}
					for n := start; n != nil; n = n.Base().Parent {
						var evt *model.Invoke
						if e.Type == app.KeyDown {
							evt = n.Base().OnKeyDown
						} else {
							evt = n.Base().OnKeyUp
						}
						if evt != nil {
							dispatchInvoke(rt, evt, map[string]any{"key": e.Key})
							handled = true
							break
						}
					}
				}
				redraw(rt) // always: focus ring / activation may have changed

			case app.PointerEvent:
				rt := srv.Runtime()
				if currentGraphNode == nil {
					continue
				}
				p := geom.Point{X: float64(e.Position.X), Y: float64(e.Position.Y)}
				hit := currentGraphNode.HitTest(p)

				redrawNeeded := false
				switch {
				case e.Type == app.PointerPress:
					if tgt := canvas.VisualTarget(hit); tgt != nil {
						inter.Pressed = tgt
						// Pointer-driven focus never shows the keyboard ring
						// (:focus-visible semantics).
						inter.Focused = tgt
						inter.FocusVisible = false
					}
					redrawNeeded = true
				case e.Type == app.PointerRelease:
					if inter.Pressed != nil {
						inter.Pressed = nil
						redrawNeeded = true
					}
				case e.Type == app.PointerMove && e.Buttons == 0:
					if hm := canvas.ModelOf(hit); hm != inter.Hovered {
						// Fire hover actions via the freshly built graph chain —
						// graph identities go stale as soon as a redraw rebuilds
						// the tree, so always resolve through currentGraphNode.
						if old := findGroupByModel(currentGraphNode, inter.Hovered); old != nil {
							for n := old; n != nil; n = n.Base().Parent {
								if n.Base().OnHoverOut != nil {
									dispatchInvoke(rt, n.Base().OnHoverOut, nil)
									break
								}
							}
						}
						for n := hit; n != nil; n = n.Base().Parent {
							if n.Base().OnHoverIn != nil {
								dispatchInvoke(rt, n.Base().OnHoverIn, nil)
								break
							}
						}
						inter.Hovered = hm
						redrawNeeded = true
					}
				}

				// Bubble up the tree until a handler is found
				dispatched := false
				for hit != nil {
					var evt *model.Invoke

					if e.Type == app.PointerPress {
						evt = hit.Base().OnPress
						if evt == nil {
							evt = hit.Base().OnTouchStart
						}
					} else if e.Type == app.PointerRelease {
						evt = hit.Base().OnTouchEnd
					} else if e.Type == app.PointerMove && e.Buttons > 0 {
						evt = hit.Base().OnTouchMove
					}

					if evt != nil {
						dispatchInvoke(rt, evt, nil)
						dispatched = true
						break
					}

					// Bubble up to parent
					if hit.Base().Parent != nil {
						hit = hit.Base().Parent
					} else {
						hit = nil
					}
				}

				// Redraw on press/release even when no handler dispatched, so
				// the visual pressed state tracks the pointer unconditionally.
				if redrawNeeded || dispatched {
					redraw(rt)
				}
			}
		}
	}()

	app.Main()
	return true
}

// sceneRoot resolves the root node of the active scene.
func sceneRoot(rt *runtime.Runtime) *model.Node {
	root := rt.App.EntryRoot()
	if sceneID := rt.CurrentScene(); sceneID != "" {
		if sc := rt.App.Scenes[sceneID]; sc != nil {
			root = sc
		}
	}
	return root
}

// dispatchInvoke evaluates the invoke's args against live state and dispatches
// the action. extraArgs seeds args (e.g. the pressed key); the author's
// explicit args win over seeds.
func dispatchInvoke(rt *runtime.Runtime, evt *model.Invoke, extraArgs map[string]any) {
	if evt == nil {
		return
	}
	args := make(map[string]any, len(evt.Args)+len(extraArgs))
	for k, v := range extraArgs {
		args[k] = v
	}
	for k, v := range evt.Args {
		args[k] = runtime.EvalBinding(v, map[string]any{"state": rt.State})
	}
	rt.Dispatch(evt.Name, args)
}

// drawFrame lays out, rasterizes and publishes one frame. Callers must hold
// the draw lock (see redraw in launchWindow).
func drawFrame(srv *server.Server, win *app.Window, ops *op.Ops, rt *runtime.Runtime, inter *canvas.Interaction, lastRoot **model.Node, ww, hh int) graph.Node {
	if rt == nil || rt.App == nil {
		return nil
	}

	// Dynamic Theme Switching
	themeName, ok := rt.State["theme"].(string)
	if ok && themeName != "" && (rt.Theme == nil || rt.Theme.Name != themeName) {
		// We don't have the directory path readily available, but for examples/counter,
		// it's running from QORM root, so we look in themes/
		t, err := theme.LoadTheme(fmt.Sprintf("themes/%s.json", themeName))
		if err == nil {
			rt.Theme = t
		}
	} else if !ok && rt.Theme == nil {
		rt.Theme = theme.GetDefault()
	}

	root := sceneRoot(rt)
	if root != *lastRoot {
		// Scene switched or app reloaded: the identity pointers held in inter
		// could alias nodes that no longer exist — reset all interaction state.
		*inter = canvas.Interaction{}
		*lastRoot = root
	}

	rootNode, needsRedraw := canvas.Layout(ops, root, image.Pt(ww, hh), rt, inter)

	// --- PHYSICS / COLLISION PASS ---
	isDirty, _ := rt.State["_physics_dirty"].(bool)
	if rootNode != nil && !isDirty {
		var colliders []graph.Node

		// Helper to collect colliders recursively
		var collect func(n graph.Node)
		collect = func(n graph.Node) {
			if n == nil {
				return
			}
			if n.Base().OnCollide != nil {
				colliders = append(colliders, n)
			}
			// Only Group has children in our current setup, but let's check generically
			if g, ok := n.(*graph.Group); ok {
				for _, child := range g.Children {
					collect(child)
				}
			}
		}
		collect(rootNode)

		// O(N^2) naive intersection check for all colliders
		hasCollision := false
		for i := 0; i < len(colliders); i++ {
			for j := i + 1; j < len(colliders); j++ {
				a := colliders[i]
				b := colliders[j]

				bboxA := a.GetBBox()
				bboxB := b.GetBBox()

				if bboxA.Intersects(bboxB) {
					hasCollision = true
					// Dispatch collision event for A
					args := make(map[string]any)
					for k, v := range a.Base().OnCollide.Args {
						args[k] = runtime.EvalBinding(v, map[string]any{"state": rt.State})
					}

					// Lock physics to prevent infinite trigger loop this frame/tick
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
		// Reset dirty flag after one frame without triggering new physics events
		rt.State["_physics_dirty"] = false
	}
	// ---------------------------------

	img := canvas.Rasterize(ops, image.Pt(ww, hh))
	win.UpdateImage(img)

	if needsRedraw {
		go func() {
			time.Sleep(16 * time.Millisecond) // ~60fps
			srv.OnStateChange(rt)             // trigger next frame
		}()
	}

	return rootNode
}

func runLogWindow(_, _ string) {}

func runMeasure(_, _ string, _ int) error {
	return fmt.Errorf("measure needs a -tags desktop build (native WebView) or full canvas implementation")
}

func runCheck(_, _, _ string, _ bool, _ int) error {
	return fmt.Errorf("check needs a -tags desktop build (native WebView)")
}

func runPreview(_ string, _ int, _, _ string) error {
	return fmt.Errorf("preview needs a -tags desktop build (native WebView)")
}

func runTray(_, _, _ string) {}
