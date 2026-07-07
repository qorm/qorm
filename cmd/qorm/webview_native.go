//go:build darwin && desktop

package main

/*
#cgo CFLAGS: -x objective-c -fobjc-arc
#cgo LDFLAGS: -framework Cocoa -framework WebKit
#include <stdlib.h>
void* qormWVOpen(const char* wid, const char* title, const char* url, int w, int h, int chromeless, int transparent);
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
*/
import "C"

import (
	"encoding/json"
	"os"
	"runtime"
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

func openWin(id, title, url string, w, h int, chromeless, transparent bool) {
	ci, ct, cu := cstr(id), cstr(title), cstr(url)
	C.qormWVOpen(ci, ct, cu, C.int(w), C.int(h), cbool(chromeless), cbool(transparent))
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

func runAppWindow(url, title string, w, h int, chromeless, transparent bool) {
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
	openWin("main", title, url, w, h, chromeless, transparent)

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
	openWin("log", title, url, 460, 640, false, false)
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
