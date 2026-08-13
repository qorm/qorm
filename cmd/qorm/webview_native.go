//go:build darwin && (desktop || canvaswebview)

package main

/*
#cgo CFLAGS: -x objective-c -fobjc-arc
#cgo LDFLAGS: -framework Cocoa -framework WebKit
#include <stdlib.h>
void* qormWVOpen(const char* wid, const char* title, const char* url, int w, int h, int chromeless, int transparent, int resizable);
void qormWVEval(const char* wid, const char* js);
void qormSetDockMenu(const char* json);
void qormWinDragStart(const char* wid);
void qormWinDragMove(const char* wid, int dx, int dy);
void qormWVWake(void);
void qormWVMove(const char* wid, int x, int y, int w, int h);
void qormWVOp(const char* wid, const char* op);
const char* qormWVGetFrame(const char* wid);
const char* qormWVList(void);
void qormWVRun(void);
void* qormWVEmbed(void* parent, const char* wid, const char* url, const char* html, double x, double y, double w, double h);
void qormWVFrame(void* view, double x, double y, double w, double h);
void qormWVLoad(void* view, const char* url, const char* html);
void qormWVRemove(void* view);
*/
import "C"

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
	"unsafe"
)

// A registry of self-owned WKWebView windows (replaces webview_go on macOS,
// whose WKWebView aborts on frame changes). Supports multiple windows by id.
var (
	desktopMessageHandler func(wid, msg string)
	nativeMainQueue       = make(chan func(), 256)
)

//export goDesktopMessage
func goDesktopMessage(wid, msg *C.char) {
	if desktopMessageHandler != nil {
		desktopMessageHandler(C.GoString(wid), C.GoString(msg))
	}
}

//export goDesktopDrain
func goDesktopDrain() {
	for {
		select {
		case f := <-nativeMainQueue:
			f()
		default:
			return
		}
	}
}

func dispatchMain(f func()) { nativeMainQueue <- f; C.qormWVWake() }

func cstr(s string) *C.char { return C.CString(s) }

func openWin(id, title, url string, w, h int, chromeless, transparent, resizable bool) {
	ci, ct, cu := cstr(id), cstr(title), cstr(url)
	C.qormWVOpen(ci, ct, cu, C.int(w), C.int(h), cbool(chromeless), cbool(transparent), cbool(resizable))
	C.free(unsafe.Pointer(ci))
	C.free(unsafe.Pointer(ct))
	C.free(unsafe.Pointer(cu))
}
func nativeEval(id, js string) {
	ci, cj := cstr(id), cstr(js)
	C.qormWVEval(ci, cj)
	C.free(unsafe.Pointer(ci))
	C.free(unsafe.Pointer(cj))
}
func moveWin(id string, x, y, w, h int) {
	ci := cstr(id)
	C.qormWVMove(ci, C.int(x), C.int(y), C.int(w), C.int(h))
	C.free(unsafe.Pointer(ci))
}
func opWin(id, op string) {
	ci, co := cstr(id), cstr(op)
	C.qormWVOp(ci, co)
	C.free(unsafe.Pointer(ci))
	C.free(unsafe.Pointer(co))
}
func frameFor(id string) string {
	ci := cstr(id)
	c := C.qormWVGetFrame(ci)
	C.free(unsafe.Pointer(ci))
	s := C.GoString(c)
	C.free(unsafe.Pointer(c))
	return s
}
func listWins() string { c := C.qormWVList(); s := C.GoString(c); C.free(unsafe.Pointer(c)); return s }

// ---- Embedded WKWebViews (canvaswebview build): windowless web views that a
// canvas host adds as subviews over its pixel plane, one per webview widget.
// All four MUST be called on the main thread (the host's tick loop is it);
// unlike the window ops above they act synchronously, no dispatch_async.

// embedWebView creates a WKWebView with the qormdesktop script bridge wired
// to goDesktopMessage(wid, …), adds it as a subview of parent at the given
// frame (superview coordinates, bottom-left origin) and loads url — or html
// via loadHTMLString when url is empty. Returns the WKWebView handle.
func embedWebView(parent unsafe.Pointer, wid, url, html string, x, y, w, h float64) unsafe.Pointer {
	ci, cu, ch := cstr(wid), cstr(url), cstr(html)
	v := C.qormWVEmbed(parent, ci, cu, ch, C.double(x), C.double(y), C.double(w), C.double(h))
	C.free(unsafe.Pointer(ci))
	C.free(unsafe.Pointer(cu))
	C.free(unsafe.Pointer(ch))
	return v
}

// frameWebView repositions an embedded WKWebView (same coordinate space as
// embedWebView).
func frameWebView(view unsafe.Pointer, x, y, w, h float64) {
	C.qormWVFrame(view, C.double(x), C.double(y), C.double(w), C.double(h))
}

// loadWebView swaps an embedded WKWebView's content (url, or html when url is
// empty) — the host calls it when the widget's url/src/html prop changed.
func loadWebView(view unsafe.Pointer, url, html string) {
	cu, ch := cstr(url), cstr(html)
	C.qormWVLoad(view, cu, ch)
	C.free(unsafe.Pointer(cu))
	C.free(unsafe.Pointer(ch))
}

// removeWebView detaches an embedded WKWebView and drops its bridge entry.
func removeWebView(view unsafe.Pointer) { C.qormWVRemove(view) }

// windowStateFile is where a desktop app remembers its window position/size.
func windowStateFile(title string) string {
	dir, err := os.UserConfigDir()
	if err != nil {
		dir = os.TempDir()
	}
	d := filepath.Join(dir, "qorm", pkgID(title))
	os.MkdirAll(d, 0o755)
	return filepath.Join(d, "window.txt")
}

// readWindowState parses a saved "x,y,w,h" frame; nil if absent/malformed.
func readWindowState(path string) []int {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	parts := strings.Split(strings.TrimSpace(string(b)), ",")
	if len(parts) != 4 {
		return nil
	}
	out := make([]int, 0, 4)
	for _, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil {
			return nil
		}
		out = append(out, n)
	}
	return out
}

// control-engine entrypoints default to the "main" window
func moveAppWindow(x, y, w, h int) { moveWin("main", x, y, w, h) }
func windowOp(op string)           { opWin("main", op) }
func getFrame() string             { return frameFor("main") }

func cbool(b bool) C.int {
	if b {
		return 1
	}
	return 0
}

func runAppWindow(url, title string, w, h int, chromeless, transparent, resizable bool) {
	notifyClickHandler = func(id string) { nativeEval("main", "qormOnNotifyClick("+jsQuote(id)+")") }
	biometricHandler = func(ok bool, m string) { nativeEval("main", "qormOnBiometric("+boolJS(ok)+","+jsQuote(m)+")") }
	btStateHandler = func(on bool) { nativeEval("main", "qormOnBluetoothState("+boolJS(on)+")") }
	btScanHandler = func(j string) { nativeEval("main", "qormOnBluetooth("+jsQuote(j)+")") }
	desktopMessageHandler = func(wid, msg string) {
		var m map[string]interface{}
		json.Unmarshal([]byte(msg), &m)
		op, _ := m["op"].(string)
		target := wid
		cb := func(js string) { nativeEval(target, js) }
		go desktopHardware(op, m, cb, dispatchMain)
	}
	openWin("main", title, url, w, h, chromeless, transparent, resizable)

	stateFile := windowStateFile(title)
	go func() {
		dispatchMain(func() { setAppMenu(title, gMenuJSON); disableRestore() })
		if fr := readWindowState(stateFile); len(fr) == 4 {
			dispatchMain(func() { moveWin("main", fr[0], fr[1], fr[2], fr[3]) })
		}
		var last string
		for {
			time.Sleep(2 * time.Second)
			ch := make(chan string, 1)
			dispatchMain(func() { ch <- frameFor("main") })
			if f := <-ch; f != "" && f != last {
				last = f
				os.WriteFile(stateFile, []byte(f), 0o644)
			}
		}
	}()
	C.qormWVRun()
}

func runLogWindow(url, title string) {
	runtime.LockOSThread()
	openWin("log", title, url, 460, 640, false, false, true)
	parent := os.Getppid()
	go func() {
		for {
			time.Sleep(400 * time.Millisecond)
			if os.Getppid() != parent {
				os.Exit(0)
			}
		}
	}()
	C.qormWVRun()
}

func boolJS(b bool) string {
	if b {
		return "true"
	}
	return "false"
}
func jsQuote(s string) string { b, _ := json.Marshal(s); return string(b) }

var shortcutHandler func(string)

//export goShortcutSelected
func goShortcutSelected(id *C.char) {
	if shortcutHandler != nil {
		shortcutHandler(C.GoString(id))
	}
}

// nativeSetDockMenu installs the app-icon Dock quick-actions menu from a JSON array.
func nativeSetDockMenu(json string) {
	c := C.CString(json)
	defer C.free(unsafe.Pointer(c))
	C.qormSetDockMenu(c)
}

func nativeWinDragStart(id string) {
	c := C.CString(id)
	defer C.free(unsafe.Pointer(c))
	C.qormWinDragStart(c)
}
func nativeWinDragMove(id string, dx, dy int) {
	c := C.CString(id)
	defer C.free(unsafe.Pointer(c))
	C.qormWinDragMove(c, C.int(dx), C.int(dy))
}
