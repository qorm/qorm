//go:build desktop && !darwin

package main

import (
	"encoding/json"
	"os"
	"runtime"
	"time"

	webview "github.com/webview/webview_go"
)

// appWebView holds the running app window so nativeEval can reach it from any
// goroutine (async hardware callbacks, the user Go middle-layer, window-control
// eval). webview_go is single-window here (openWin is a no-op), so one handle
// suffices. Eval MUST be dispatched onto the WebView's main thread.
var appWebView webview.WebView

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

// moveAppWindow: webview_go has no window-move API; no-op for now.
func moveAppWindow(x, y, w, h int)                         {}
func moveWin(id string, x, y, w, h int)                    {}
func opWin(id, op string)                                  {}
func openWin(id, title, url string, w, h int, cl, tr bool) {}

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
