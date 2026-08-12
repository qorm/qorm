//go:build !desktop && darwin

package main

import (
	"bytes"
	"net"
	"net/http"
	"time"

	"github.com/qorm/qorm/internal/app"
	"github.com/qorm/qorm/internal/qscript"
	"github.com/qorm/qorm/internal/render/canvas"
	"github.com/qorm/qorm/internal/runtime"
	"github.com/qorm/qorm/internal/server"
)

// This file hosts the SHARED canvas-window machinery used by both darwin
// canvas flavours: the plain pure-Go build (canvas_window_darwin.go) and the
// canvaswebview build (canvaswebview_darwin.go), which layers real WKWebView
// subviews over webview widgets through the hooks below. Keeping one loop
// means the threading invariants documented here hold for both.

// canvasWindowHooks customises runCanvasWindow for a host that layers native
// views over the canvas. A nil *canvasWindowHooks (or nil fields) is the
// plain canvas behaviour: every event reaches the engine, ticks just render.
type canvasWindowHooks struct {
	// pointerFilter runs BEFORE the engine sees a PointerEvent; returning
	// true consumes it, so presses/hover over a platform view never reach
	// the engine (AppKit already delivered them to the overlay subview —
	// forwarding would double-process).
	pointerFilter func(app.PointerEvent) bool
	// scrollFilter likewise gates ScrollEvent (the wheel carries no
	// position, so the host filters by its own last-pointer bookkeeping).
	scrollFilter func() bool
	// afterFrame runs at the end of every tick (main thread), after any
	// render+present, with the current physical size and scale — the overlay
	// host syncs platform views to Engine.WidgetFrames there.
	afterFrame func(eng *canvas.Engine, win *app.Window, scale, physW, physH int)
	// nativeHook, when set, REPLACES canvas.InvokeNative as qscript's native()
	// bridge — the overlay host intercepts its own ops (webviewEval into the
	// WKWebView embeds) and delegates the rest to canvas.InvokeNative. It must
	// be installed by runCanvasWindow (not by the caller beforehand): anything
	// set earlier would be overwritten by the plain InvokeNative install below.
	nativeHook func(op string, data map[string]any, cb func(string, any))
}

// runCanvasWindow hosts the app in the native canvas engine: a pure-Go AppKit
// window whose pixels come from the canvas Renderer (no WebView of its own).
//
// Rendering runs ON THE MAIN THREAD, inside app.Run's event loop (below). This
// is the only design that avoids the render/display data race that made the old
// background-render window flicker and fail to repaint on theme changes: the
// engine writes the single shared pixel plane and the NSImageView reads it,
// both from the main thread, so they never overlap. Input is also handled on
// the main thread (single-threaded engine state). The HTTP/MCP server — the
// agent's collaboration channel — runs on its own goroutines, but every
// request that touches the runtime is marshalled onto the main thread through
// the engine's mutation queue (marshalToMain), so rt.State is only ever read
// or written on the main thread; the only cross-thread signals left are the
// queue itself and engine.RequestDraw (an atomic store).
func runCanvasWindow(srv *server.Server, ln net.Listener, title string, hooks *canvasWindowHooks) bool {
	aw := srv.AppWindow()
	ww, hh := aw.Width, aw.Height
	if ww == 0 {
		ww = 400
	}
	if hh == 0 {
		hh = 820
	}

	// Canvas mode has no browser clients, so skip HTML rendering — but DO serve
	// the HTTP API (state, MCP, dev endpoints): that is the collaboration
	// channel the agent inspects while a human watches the native window. Every
	// request is marshalled onto the main thread (see marshalToMain).
	srv.SetCanvasHost(true)

	win := app.NewWindow(title, ww, hh)
	eng := canvas.NewEngine(srv.Runtime(), canvas.SoftwareRenderer{})

	// The HTTP middleware (marshalToMain) cannot reach the two state-touching
	// paths below — the async-http completion goroutine (server.spawn) and the
	// SSE catch-up render — so those go through the same main-thread queue.
	srv.SetMarshal(eng.EnqueueMutation)

	// Native capability ops for hardware widgets (network, volume, …): the
	// canvas build's pure-Go exec bridge (hardware_canvas_darwin.go — the
	// webview cgo bridge can't be linked here), driving the engine's
	// NativeInvoker seam on the render thread (fast ops only).
	canvas.SetNativeInvoker(func(op string, data map[string]any, cb func(string, any)) {
		canvasHardwareDarwin(op, data, func(js string) {
			if name, arg, ok := canvas.ParseNativeCallback(js); ok {
				cb(name, arg)
			}
		})
	})
	// Scripts get the same bridge through native() (qscript's hook — no canvas
	// import in the interpreter, so it stays a package var the host sets). A
	// host with platform views overrides it to intercept its own ops first.
	if hooks != nil && hooks.nativeHook != nil {
		qscript.SetNativeHook(hooks.nativeHook)
	} else {
		qscript.SetNativeHook(canvas.InvokeNative)
	}

	// An external (MCP) state change flags a redraw; the main loop renders it.
	// Runs on the main thread inside a marshalled mutation (atomic store, so it
	// would be safe from any goroutine regardless).
	srv.OnStateChange = func(_ *runtime.Runtime) { eng.RequestDraw() }

	go func() { _ = http.Serve(ln, marshalToMain(eng, srv.Handler())) }()

	scale := win.Scale()
	physW, physH := ww*scale, hh*scale
	s := float64(scale)
	lastResize := time.Now()
	cursorHint := canvas.CursorArrow

	// app.Run's loop delivers events here AND, when idle, returns so we can
	// render. We drive the engine from this same (main) thread.
	app.Run(func(ev app.Event) {
		switch e := ev.(type) {
		case app.PointerEvent:
			// A platform view covering this point owns the event (AppKit
			// delivered it to the overlay subview); forwarding it to the
			// engine too would double-process the press/hover.
			if hooks != nil && hooks.pointerFilter != nil && hooks.pointerFilter(e) {
				break
			}
			// AppKit gives logical points; the engine/graph are in physical px.
			if eng.HandlePointer(canvas.PointerInput{
				Type: canvas.PointerType(e.Type), X: float64(e.Position.X) * s, Y: float64(e.Position.Y) * s, Buttons: e.Buttons, Right: e.Right,
			}) {
				// state-changing input already dirtied the engine; nothing else
			}
			// Keep the OS cursor honest with the hovered widget (I-beam over
			// text fields, pointing hand over pressables) — the browser does
			// this for free on the HTML path; the native window needs it set.
			if h := eng.CursorHint(); h != cursorHint {
				cursorHint = h
				win.SetCursor(int(h))
			}
		case app.KeyEvent:
			eng.HandleKey(canvas.KeyInput{Key: e.Key, Shift: e.Shift, Ctrl: e.Ctrl, Alt: e.Alt, Meta: e.Meta, Down: e.Type == app.KeyDown, Rune: e.Rune})
		case app.ScrollEvent:
			if hooks != nil && hooks.scrollFilter != nil && hooks.scrollFilter() {
				break
			}
			eng.HandleScroll(canvas.ScrollInput{DX: e.DeltaX * s, DY: e.DeltaY * s, Ctrl: e.Ctrl})
		}
	}, func() {
		// Keep the plane in lockstep with the real window size and current
		// backing scale factor. A size change is throttled to ~10 Hz so a drag
		// does not churn plane allocs; an unchanged size still re-syncs the
		// backing scale so moving between displays stays crisp.
		if sz := win.LiveSize(); sz.X >= 1 && sz.Y >= 1 {
			if sz.X != win.Size().X || sz.Y != win.Size().Y {
				if time.Since(lastResize) > 100*time.Millisecond {
					lastResize = time.Now()
					win.Resize(sz.X, sz.Y)
					scale = win.Scale()
					s = float64(scale)
					physW, physH = sz.X*scale, sz.Y*scale
					eng.MarkDirty()
				}
			} else {
				win.Resize(sz.X, sz.Y)
				if newScale := win.Scale(); newScale != scale {
					scale = newScale
					s = float64(scale)
					physW, physH = sz.X*scale, sz.Y*scale
					eng.MarkDirty()
				}
			}
		}
		// Tick: render + present when there is work (input/state change, a
		// queued external mutation, or an in-flight tween). Single buffer,
		// single thread → no race, no flicker.
		if eng.Dirty() || eng.Animating() {
			if rendered, _ := eng.RenderInto(app.Pt(physW, physH), scale, win.Backbuffer()); rendered {
				win.PresentImage()
			}
			// Feed agent measurement from the live graph (HTML path POSTs the
			// same shape from the browser; canvas has no DOM). Logical CSS px
			// so MCP checks match design tokens / scale-independent geometry.
			srv.SetMeasure(eng.CollectMeasureOpts(canvas.MeasureOpts{Logical: true}))
		}
		// The overlay host syncs its platform views against the graph the
		// frame above (re)built — also on idle ticks, so an unchanged scene
		// keeps its overlays after a pure window resize.
		if hooks != nil && hooks.afterFrame != nil {
			hooks.afterFrame(eng, win, scale, physW, physH)
		}
	})
	// app.Run returned → the window was closed; returning true lets cmdRun
	// exit the process (the HTTP goroutine dies with it — no idle spinning).
	return true
}

// marshalToMain routes every HTTP request through the engine's mutation
// queue, so the whole handler — dispatch, state reads, MCP tools, dev
// endpoints — executes on the main thread where rt.State lives, and replays
// the recorded response on the request's own goroutine (request/response
// semantics are preserved; EnqueueMutation blocks until the closure ran).
//
// /events is the one exception: SSE is a long-lived stream, and parking the
// main thread on it would freeze the window. In canvas mode frame() never
// broadcasts, so the stream simply stays silent (there are no browser
// clients); its one-time catch-up render only triggers for a client that
// sends ?rev=/Last-Event-Id, which canvas mode has none of.
func marshalToMain(eng *canvas.Engine, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/events" {
			next.ServeHTTP(w, r)
			return
		}
		rec := &recordedResponse{header: http.Header{}, status: http.StatusOK}
		var pan any
		eng.EnqueueMutation(func() {
			// Capture a handler panic so it re-panics HERE, on the request's
			// goroutine, where http.Server's per-connection recovery logs it —
			// exactly as if the handler had run here directly.
			defer func() { pan = recover() }()
			next.ServeHTTP(rec, r)
		})
		if pan != nil {
			panic(pan)
		}
		for k, vs := range rec.header {
			w.Header()[k] = vs
		}
		w.WriteHeader(rec.status)
		_, _ = w.Write(rec.body.Bytes())
	})
}

// recordedResponse is an http.ResponseWriter that buffers a handler's
// response so it can be replayed on the request's own goroutine after the
// handler ran on the main thread (see marshalToMain). It deliberately does
// not implement http.Flusher/Hijacker — the one streaming endpoint (/events)
// bypasses marshalling.
type recordedResponse struct {
	header http.Header
	status int
	body   bytes.Buffer
}

func (r *recordedResponse) Header() http.Header { return r.header }
func (r *recordedResponse) WriteHeader(status int) {
	r.status = status
}
func (r *recordedResponse) Write(b []byte) (int, error) { return r.body.Write(b) }
