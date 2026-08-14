//go:build darwin && canvaswebview && !desktop

package main

import (
	"encoding/json"
	"fmt"
	"image"
	"net"
	"os"
	"unsafe"

	"github.com/qorm/platform/internal/app"
	"github.com/qorm/platform/internal/model"
	"github.com/qorm/platform/internal/render/canvas"
	"github.com/qorm/platform/internal/server"
	"github.com/qorm/platform/internal/widgets"
)

// The canvaswebview build: the shared pure-Go canvas window (canvas_host_
// darwin.go) plus REAL WKWebView subviews layered over every webview widget —
// a platform-view overlay. The canvas still renders the widget's placeholder
// underneath; each overlay simply occludes it.
//
//	go run -tags canvaswebview ./cmd/qorm run examples/webdemo
//
// The overlay sync runs on the main thread at the end of every tick
// (afterFrame hook): Engine.WidgetFrames reports where each webview widget
// was laid out, and the host creates / re-frames / reloads / destroys one
// WKWebView per widget to match. Events over an overlay never reach the
// engine (AppKit already delivered them to the subview — the pointerFilter /
// scrollFilter hooks prevent double-processing). Pages inside an overlay use
// the standard qormDesktop JS bridge (webview_native.m's QormWV handler →
// goDesktopMessage → desktopHardware), so the existing qormToNative contract
// works unchanged.

// launchWindow hosts the app in the canvas window with WKWebView overlays.
func launchWindow(srv *server.Server, ln net.Listener, _, title string) bool {
	// The JS bridge: a page in any embedded WKWebView posts
	// window.qormDesktop(json) → goDesktopMessage(wid, msg) on the main
	// thread. Dispatch the op exactly like the desktop window does — off the
	// UI thread, with Cocoa ops re-entering the main thread via dispatchMain
	// (nativeMainQueue + qormWVWake → goDesktopDrain) — and eval the callback
	// back into the SAME embed.
	desktopMessageHandler = func(wid, msg string) {
		fmt.Fprintf(os.Stderr, "[canvaswebview] js bridge %s: %s\n", wid, msg)
		var m map[string]interface{}
		_ = json.Unmarshal([]byte(msg), &m)
		op, _ := m["op"].(string)
		cb := func(js string) { dispatchMain(func() { nativeEval(wid, js) }) }
		go desktopHardware(op, m, cb, dispatchMain)
	}

	ov := &wvOverlayHost{byModel: map[*model.Node]*wvOverlay{}}
	return runCanvasWindow(srv, ln, title, &canvasWindowHooks{
		pointerFilter: ov.pointerFilter,
		scrollFilter:  ov.scrollFilter,
		afterFrame:    ov.afterFrame,
		// Scripts reach the embeds through native("webviewEval", {id, js}) —
		// the Go→page direction of the bridge (page→Go is window.qormDesktop).
		// Everything else falls through to the standard canvas bridge (which
		// runCanvasWindow wires to canvasHardwareDarwin before this runs).
		nativeHook: func(op string, data map[string]any, cb func(string, any)) {
			if op == "webviewEval" {
				id, _ := data["id"].(string)
				js, _ := data["js"].(string)
				if id != "" && js != "" {
					dispatchMain(func() { nativeEval(id, js) })
				}
				cb("qormOnEval", true)
				return
			}
			canvas.InvokeNative(op, data, cb)
		},
	})
}

// wvOverlay is one live WKWebView covering one webview widget.
type wvOverlay struct {
	view    unsafe.Pointer
	url     string // last loaded url/html, to reload only on change
	html    string
	rectPts image.Rectangle // logical points, top-left origin (event coords)
}

// wvOverlayHost tracks the live overlays and owns the event-filter state.
// Everything here is main-thread only (the tick loop and event callback both
// run on it), so no locks.
type wvOverlayHost struct {
	parent     uintptr // the window's content view (NSView), captured lazily
	byModel    map[*model.Node]*wvOverlay
	rects      []image.Rectangle // current overlay boxes, logical top-left
	lastPtr    image.Point       // last pointer position (for the wheel filter)
	engineDown bool              // a canvas-owned press stream is in flight
}

// afterFrame syncs the overlays to the just-rendered graph: create a WKWebView
// for every new webview widget frame, re-frame / reload the ones that moved
// or changed source, destroy the ones whose widget left the scene.
func (h *wvOverlayHost) afterFrame(eng *canvas.Engine, win *app.Window, scale, physW, physH int) {
	if scale < 1 {
		scale = 1
	}
	if h.parent == 0 {
		h.parent = win.ContentView()
	}
	s := float64(scale)
	contentH := float64(physH) / s // content height in points (for the y-flip)

	frames := eng.WidgetFrames("webview")
	for m, r := range frames {
		ov := h.byModel[m]
		if r.Empty() {
			if ov != nil {
				h.remove(m, ov)
			}
			continue
		}
		// physical px → logical points; the graph is top-left origin, AppKit
		// subview frames bottom-left.
		lx, ly := float64(r.Min.X)/s, float64(r.Min.Y)/s
		lw, lh := float64(r.Dx())/s, float64(r.Dy())/s
		pts := image.Rect(int(lx), int(ly), int(lx+lw), int(ly+lh))
		url, markup := widgets.WebViewSource(m, eng.RT)

		if ov == nil {
			wid := m.ID
			if wid == "" {
				wid = fmt.Sprintf("wv-%p", m)
			}
			ov = &wvOverlay{
				view: embedWebView(unsafe.Pointer(h.parent), wid, url, markup, lx, contentH-ly-lh, lw, lh),
			}
			h.byModel[m] = ov
			fmt.Fprintf(os.Stderr, "[canvaswebview] embedded WKWebView %q at %.0f,%.0f %.0fx%.0f (url=%q, html=%d bytes)\n",
				wid, lx, ly, lw, lh, url, len(markup))
		} else {
			if ov.rectPts != pts {
				frameWebView(ov.view, lx, contentH-ly-lh, lw, lh)
			}
			if ov.url != url || ov.html != markup {
				loadWebView(ov.view, url, markup)
			}
		}
		ov.url, ov.html, ov.rectPts = url, markup, pts
	}
	// Widgets that left the graph (scene switch, if-gate, hot reload) lose
	// their overlay.
	for m, ov := range h.byModel {
		if _, ok := frames[m]; !ok {
			h.remove(m, ov)
		}
	}
	// Republish the event-filter boxes.
	h.rects = h.rects[:0]
	for _, ov := range h.byModel {
		h.rects = append(h.rects, ov.rectPts)
	}
}

func (h *wvOverlayHost) remove(m *model.Node, ov *wvOverlay) {
	removeWebView(ov.view)
	delete(h.byModel, m)
}

// pointerFilter keeps events that belong to an overlay away from the engine —
// AppKit already delivered them to the WKWebView subview (it sits above the
// canvas's NSImageView), so forwarding would double-process. A press STARTED
// on canvas pixels owns its whole stream (drag/release keep flowing to the
// engine even when the pointer crosses an overlay), and vice versa.
func (h *wvOverlayHost) pointerFilter(e app.PointerEvent) bool {
	h.lastPtr = e.Position
	inside := h.inside(e.Position)
	switch e.Type {
	case app.PointerPress:
		h.engineDown = !inside
	case app.PointerRelease:
		skip := !h.engineDown && inside
		h.engineDown = false
		return skip
	}
	return !h.engineDown && inside
}

// scrollFilter gates the wheel by where the pointer rests (ScrollEvent
// carries no position), the same rule the engine itself applies.
func (h *wvOverlayHost) scrollFilter() bool { return h.inside(h.lastPtr) }

func (h *wvOverlayHost) inside(p image.Point) bool {
	for _, r := range h.rects {
		if p.In(r) {
			return true
		}
	}
	return false
}

// gMenuJSON is referenced by webview_native.go's shared runAppWindow; the
// canvaswebview host never spawns the desktop menu/tray/log children, so it
// stays empty here.
var gMenuJSON string

// traySelected satisfies the shared tray_darwin.go callback; this host spawns
// no tray process (runTray below is a no-op), so a selection can never arrive.
func traySelected(string) {}

func runTray(_, _, _ string) {}

// runMeasure / runCheck: measure_pure.go (pure-Go canvas headless).

func runPreview(_ string, _ int, _, _ string) error {
	return fmt.Errorf("preview needs a -tags desktop build (native WebView)")
}
