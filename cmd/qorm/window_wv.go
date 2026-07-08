//go:build desktop && !darwin

package main

import (
	"encoding/json"
	"os"
	"runtime"
	"sync"
	"time"

	webview "github.com/qorm/qorm/internal/webview"
)

// appWebView holds the running app window so nativeEval can reach it from any
// goroutine (async hardware callbacks, the user Go middle-layer, window-control
// eval). webview_go is single-window here (openWin is a no-op), so one handle
// suffices. Eval MUST be dispatched onto the WebView's main thread.
var (
	appWebView    webview.WebView
	activeWindows = make(map[string]webview.WebView)
	winMu         sync.Mutex
)

// runAppWindow (non-macOS): the app window via webview_go (WebKitGTK/WebView2).
func runAppWindow(url, title string, ww, hh int, chromeless, transparent bool) {
	w := webview.New(false)
	appWebView = w
	defer func() { appWebView = nil }()
	defer w.Destroy()
	w.SetTitle(title)
	if ww == 0 {
		ww = 400
	}
	if hh == 0 {
		hh = 820
	}
	w.SetSize(ww, hh, webview.HintNone)
	bindDesktopHardware(w)
	go func() {
		time.Sleep(600 * time.Millisecond)
		w.Dispatch(func() {
			setAppMenu(title, gMenuJSON)
			grantMedia(w.Window())
			disableRestore()
		})
	}()
	w.Navigate(url)
	w.Run()
}

func bindDesktopHardware(w webview.WebView) {
	cb := func(js string) { w.Dispatch(func() { w.Eval(js) }) }
	notifyClickHandler = func(id string) { cb("qormOnNotifyClick(" + jsQuote(id) + ")") }
	biometricHandler = func(ok bool, msg string) { cb("qormOnBiometric(" + boolJS(ok) + "," + jsQuote(msg) + ")") }
	btStateHandler = func(on bool) { cb("qormOnBluetoothState(" + boolJS(on) + ")") }
	btScanHandler = func(j string) { cb("qormOnBluetooth(" + jsQuote(j) + ")") }
	w.Bind("qormDesktop", func(msg string) string {
		var m map[string]interface{}
		json.Unmarshal([]byte(msg), &m)
		op, _ := m["op"].(string)
		go desktopHardware(op, m, cb, func(fn func()) { w.Dispatch(fn) })
		return ""
	})
}

func boolJS(b bool) string {
	if b {
		return "true"
	}
	return "false"
}
func jsQuote(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

// runLogWindow (non-macOS): activity-log window via webview_go.
func runLogWindow(url, title string) {
	runtime.LockOSThread()
	w := webview.New(false)
	defer w.Destroy()
	w.SetTitle(title)
	w.SetSize(460, 640, webview.HintNone)
	w.Navigate(url)
	parent := os.Getppid()
	go func() {
		for {
			time.Sleep(400 * time.Millisecond)
			if os.Getppid() != parent {
				w.Terminate()
				return
			}
		}
	}()
	w.Run()
}

// moveAppWindow sets the position and size of the native window (Windows via
// user32 SetWindowPos; no GTK path yet, see winapi_other.go).
func moveAppWindow(x, y, w, h int) {
	wv := appWebView
	if wv == nil {
		return
	}
	if hwnd := wv.Window(); hwnd != nil {
		setWindowPos(hwnd, x, y, w, h)
	}
}

func openWin(id, title, url string, w, h int, cl, tr bool) {
	winMu.Lock()
	if old, ok := activeWindows[id]; ok {
		winMu.Unlock()
		old.Dispatch(func() { old.Navigate(url) })
		return
	}
	winMu.Unlock()

	go func() {
		runtime.LockOSThread()
		wv := webview.New(false)

		winMu.Lock()
		activeWindows[id] = wv
		winMu.Unlock()

		defer func() {
			winMu.Lock()
			delete(activeWindows, id)
			winMu.Unlock()
			wv.Destroy()
		}()

		wv.SetTitle(title)
		if w == 0 {
			w = 400
		}
		if h == 0 {
			h = 600
		}
		wv.SetSize(w, h, webview.HintNone)
		wv.Navigate(url)
		wv.Run()
	}()
}

func moveWin(id string, x, y, w, h int) {
	if id == "main" || id == "" {
		moveAppWindow(x, y, w, h)
		return
	}
	winMu.Lock()
	wv, ok := activeWindows[id]
	winMu.Unlock()

	if ok && wv != nil {
		if hwnd := wv.Window(); hwnd != nil {
			setWindowPos(hwnd, x, y, w, h)
		}
	}
}

func opWin(id, op string) {
	winMu.Lock()
	wv, ok := activeWindows[id]
	winMu.Unlock()

	if ok && wv != nil {
		if op == "close" {
			wv.Dispatch(func() { wv.Terminate() })
		}
	}
}

// windowOp: no-op on non-macOS for now.
func windowOp(op string) {}

func dispatchMain(f func()) { f() }

// nativeEval evaluates js in the app window. On webview_go there is a single
// window (openWin is a no-op), so id is ignored. Dispatch marshals the Eval
// onto the WebView's main thread, which is required by WebKitGTK/WebView2.
func nativeEval(id, js string) {
	w := appWebView
	if w == nil {
		return
	}
	w.Dispatch(func() { w.Eval(js) })
}
