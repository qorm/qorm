//go:build !desktop && darwin

package main

import (
	"fmt"
	"net"
	"sync"

	"github.com/qorm/qorm/internal/app"
	"github.com/qorm/qorm/internal/render/canvas"
	"github.com/qorm/qorm/internal/runtime"
	"github.com/qorm/qorm/internal/server"
)

// launchWindow hosts the app in the native canvas engine: a pure-Go AppKit
// window whose pixels come from the canvas Renderer (no WebView). All engine
// logic (layout, recording, interaction, focus, physics) lives in
// internal/render/canvas — this file is only wiring: window ↔ engine ↔ server.
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
		eng := canvas.NewEngine(srv.Runtime(), canvas.SoftwareRenderer{}, win)

		// All frame work (window events, animation ticks, state changes from
		// actions) funnels through one serialized draw.
		var drawMu sync.Mutex
		redraw := func() {
			drawMu.Lock()
			eng.DrawFrame()
			drawMu.Unlock()
		}
		eng.OnRedraw = redraw // animation continuation ticks

		srv.OnStateChange = func(_ *runtime.Runtime) { redraw() }

		for e := range win.Events() {
			switch e := e.(type) {
			case app.FrameEvent:
				redraw()

			case app.KeyEvent:
				if eng.HandleKey(canvas.KeyInput{
					Key:   e.Key,
					Shift: e.Shift,
					Down:  e.Type == app.KeyDown,
				}) {
					redraw()
				}

			case app.PointerEvent:
				// app and canvas share the Press/Release/Move ordering.
				if eng.HandlePointer(canvas.PointerInput{
					Type:    canvas.PointerType(e.Type),
					X:       float64(e.Position.X),
					Y:       float64(e.Position.Y),
					Buttons: e.Buttons,
				}) {
					redraw()
				}
			}
		}
	}()

	app.Main()
	return true
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
