//go:build !desktop && darwin

package main

import (
	"bytes"
	"fmt"
	"net"
	"net/http"

	"github.com/qorm/qorm/internal/app"
	"github.com/qorm/qorm/internal/render/canvas"
	"github.com/qorm/qorm/internal/runtime"
	"github.com/qorm/qorm/internal/server"
)

// launchWindow hosts the app in the native canvas engine: a pure-Go AppKit
// window whose pixels come from the canvas Renderer (no WebView).
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
func launchWindow(srv *server.Server, ln net.Listener, url, title string) bool {
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

	// An external (MCP) state change flags a redraw; the main loop renders it.
	// Runs on the main thread inside a marshalled mutation (atomic store, so it
	// would be safe from any goroutine regardless).
	srv.OnStateChange = func(_ *runtime.Runtime) { eng.RequestDraw() }

	go func() { _ = http.Serve(ln, marshalToMain(eng, srv.Handler())) }()

	scale := win.Scale()
	physW, physH := ww*scale, hh*scale
	s := float64(scale)

	// app.Run's loop delivers events here AND, when idle, returns so we can
	// render. We drive the engine from this same (main) thread.
	app.Run(func(ev app.Event) {
		switch e := ev.(type) {
		case app.PointerEvent:
			// AppKit gives logical points; the engine/graph are in physical px.
			if eng.HandlePointer(canvas.PointerInput{
				Type: canvas.PointerType(e.Type), X: float64(e.Position.X) * s, Y: float64(e.Position.Y) * s, Buttons: e.Buttons,
			}) {
				// state-changing input already dirtied the engine; nothing else
			}
		case app.KeyEvent:
			eng.HandleKey(canvas.KeyInput{Key: e.Key, Shift: e.Shift, Down: e.Type == app.KeyDown})
		case app.ScrollEvent:
			eng.HandleScroll(canvas.ScrollInput{DX: e.DeltaX * s, DY: e.DeltaY * s})
		}
	}, func() {
		// Tick: render + present when there is work (input/state change, a
		// queued external mutation, or an in-flight tween). Single buffer,
		// single thread → no race, no flicker.
		if eng.Dirty() || eng.Animating() {
			if rendered, _ := eng.RenderInto(app.Pt(physW, physH), scale, win.Backbuffer()); rendered {
				win.PresentImage()
			}
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
