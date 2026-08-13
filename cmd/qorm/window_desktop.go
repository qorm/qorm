//go:build desktop

// Drives the platform-native WebView (WKWebView / WebView2 / WebKitGTK) — like
// Wails, built per-platform. The desktop app opens TWO native windows: the real
// app (the user's actual experience) and a separate live activity-log window.
//
//	go build -tags desktop ./cmd/qorm
package main

import (
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strconv"

	"github.com/qorm/qorm/internal/server"
	"github.com/qorm/qorm/pkg/qormext"
)

// launchWindow serves the app and opens the real app in a native window, while
// spawning a separate child process for the activity-log window. Returns true
// after the app window closes.
// gMenuJSON / gTrayJSON hold the desktop menu-bar + tray config (JSON), set at launch.
var gMenuJSON, gTrayJSON string

func launchWindow(srv *server.Server, ln net.Listener, url, title string) bool {
	runtime.LockOSThread()
	go func() { _ = http.Serve(ln, srv.Handler()) }()

	// Chromeless HUDs stay clean — no Activity-log window, no tray — unless the
	// app opts in; other apps can hide either with hideLog / hideTray.
	awc := srv.AppWindow()
	gMenuJSON = srv.AppMenuJSON()
	gTrayJSON = srv.AppTrayJSON()
	if exe, err := os.Executable(); err == nil {
		if !awc.Chromeless && !awc.HideLog {
			logCmd := exec.Command(exe, "__logwin", url+"logwindow", title+" — Activity log")
			if logCmd.Start() == nil && logCmd.Process != nil {
				defer logCmd.Process.Kill()
			}
		}
		if !awc.Chromeless && !awc.HideTray {
			trayCmd := exec.Command(exe, "__tray", url, title, gTrayJSON)
			if trayCmd.Start() == nil && trayCmd.Process != nil {
				defer trayCmd.Process.Kill()
			}
		}
	}

	// The real app window. On macOS this is a self-owned WKWebView (webview_go's
	// WKWebView aborts on window frame changes); elsewhere it's webview_go.
	setDockIcon(appIcon(512))
	qormext.SetEvaluator(func(js string) { nativeEval("main", js) })
	// real-time volume sync: OS listener pushes the new level the instant the
	// hardware keys (or anything else) change it — no polling lag.
	volumeWatchHandler = func(v float64) { nativeEval("main", fmt.Sprintf("qormEmit(%q, %g)", "volume", v)) }
	muteWatchHandler = func(m bool) { nativeEval("main", fmt.Sprintf("qormEmit(%q, %t)", "mute", m)) }
	brightnessWatchHandler = func(v float64) { nativeEval("main", fmt.Sprintf("qormEmit(%q, %g)", "brightness", v)) }
	shortcutHandler = func(id string) { nativeEval("main", fmt.Sprintf("qormEmit(%q,%s)", "shortcut", strconv.Quote(id))) }
	nativeWatchVolume()
	nativeWatchBrightness()
	nativeSetDockMenu(srv.AppShortcutsJSON())
	srv.SetWindowControl(
		func(id string, x, y, w, h int) { moveWin(id, x, y, w, h) },
		func(id, op string) { opWin(id, op) },
		func(id, url string, w, h int) {
			if w == 0 {
				w = 400
			}
			if h == 0 {
				h = 600
			}
			dispatchMain(func() { openWin(id, id, url, w, h, false, false, true) })
		},
		func(id, js string) { nativeEval(id, js) },
	)
	aw := srv.AppWindow()
	ww, hh := aw.Width, aw.Height
	if ww == 0 {
		ww = 400
	}
	if hh == 0 {
		hh = 820
	}
	runAppWindow(url, title, ww, hh, aw.Chromeless, aw.Transparent, !aw.Fixed)
	return true
}

// The desktop hardware bridge's shared state (notifyClickHandler & friends,
// desktopBuiltins, screenRecCmd, inhibitCmd, brightnessUnsupportedJS) lives in
// hardware_desktop.go, which also compiles into the canvaswebview build; the
// window-state helpers (windowStateFile/readWindowState) live next to their
// only caller in webview_native.go.

// runTray shows a system-tray icon + menu for the running app (a desktop
// staple). It runs in its own process (systray needs the main run loop, which
// the WebView also owns), spawned by launchWindow. Quit terminates the app.
var gTrayURL string

var caffeinateCmd *exec.Cmd
