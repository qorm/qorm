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
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

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
			dispatchMain(func() { openWin(id, id, url, w, h, false, false) })
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
	runAppWindow(url, title, ww, hh, aw.Chromeless, aw.Transparent)
	return true
}

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

// bindDesktopHardware exposes a native hardware bridge to the web UI on desktop,
// mirroring the mobile QORM Dev bridge. JS calls window.qormDesktop({op}); the
// Go host runs the op (OS commands) and calls back qormOn<X>. Camera/mic/geo
// already work via Web APIs (the desktop WebView loads localhost — a secure
// context), so this covers the OS-level bits: volume, brightness, battery.
var notifyClickHandler func(string)

var biometricHandler func(bool, string)

var btStateHandler func(bool)

var btScanHandler func(string)

// desktopBuiltins are the ops handled by the built-in bridge (so unknown ops
// fall through to the user's plugin).
var (
	screenRecCmd  *exec.Cmd
	screenRecFile string
	caffeinateCmd *exec.Cmd
)

var desktopBuiltins = map[string]bool{
	"notify": true, "badge": true, "loginItem": true, "loginItemGet": true,
	"screens": true, "biometric": true, "wifiInfo": true, "bluetoothScan": true,
	"bluetoothState": true, "platform": true, "getModes": true, "winDragStart": true, "winDragMove": true, "screenshot": true, "screenRecordStart": true, "screenRecordStop": true, "clipboardSet": true, "clipboardGet": true, "share": true, "deviceInfo": true, "networkStatus": true, "keepAwake": true, "haptic": true, "storageSet": true, "storageGet": true, "openURL": true, "speak": true, "speakStop": true, "volumeSet": true, "brightnessSet": true, "secureSet": true, "secureGet": true, "volumeGet": true, "volumeUp": true,
	"volumeDown": true, "brightnessGet": true, "battery": true, "torchGet": true,
	"brightnessUp": true, "brightnessDown": true, "torchToggle": true, "vibrate": true,
}

// brightnessUnsupportedJS updates the brightness readout to a clear
// desktop-unsupported state (used when no backlight/tool is available) so the
// auto-fired brightnessGet never leaves the widget stuck.
const brightnessUnsupportedJS = `(function(){document.querySelectorAll('.qorm-brightness-out').forEach(function(o){o.textContent='Brightness: n/a on desktop';});})()`

// runTray shows a system-tray icon + menu for the running app (a desktop
// staple). It runs in its own process (systray needs the main run loop, which
// the WebView also owns), spawned by launchWindow. Quit terminates the app.
var gTrayURL string

var inhibitCmd *exec.Cmd
