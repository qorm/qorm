//go:build desktop

// Drives the platform-native WebView (WKWebView / WebView2 / WebKitGTK) — like
// Wails, built per-platform. The desktop app opens TWO native windows: the real
// app (the user's actual experience) and a separate live activity-log window.
//
//	go build -tags desktop ./cmd/qorm
package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	webview "github.com/qorm/qorm/internal/webview"

	"github.com/qorm/qorm/internal/measure"
	qrt "github.com/qorm/qorm/internal/runtime"
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

// measureRows renders appDir in a WebView, waits for the app to self-report its
// layout, and returns the raw measured rows + the runtime (for intent joining).
// The provided step callback runs once measurement is captured, then the window
// closes.
func measureRows(appDir string, width int, use func(rt *qrt.Runtime, url string, measured []byte)) error {
	rt, err := loadRuntime(appDir, "", "")
	if err != nil {
		return err
	}
	srv := server.New(rt)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return err
	}
	url := "http://" + ln.Addr().String() + "/"
	go func() { _ = http.Serve(ln, srv.Handler()) }()

	runtime.LockOSThread()
	w := webview.New(false)
	defer w.Destroy()
	w.SetTitle("qorm measure")
	w.SetSize(width, 820, webview.HintNone)
	w.Navigate(url)
	go func() {
		var measured []byte
		for i := 0; i < 120; i++ {
			time.Sleep(80 * time.Millisecond)
			resp, e := http.Get(url + "measure")
			if e != nil {
				continue
			}
			b, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			if len(b) > 2 {
				measured = b
				break
			}
		}
		use(rt, url, measured)
		w.Terminate()
	}()
	w.Run()
	return nil
}

// runMeasure prints the complete intent+result report.
func runMeasure(appDir, out string, width int) error {
	return measureRows(appDir, width, func(rt *qrt.Runtime, _ string, measured []byte) {
		report, _ := measure.Report(rt, measured)
		if out != "" {
			_ = os.WriteFile(out, report, 0o644)
			fmt.Fprintf(os.Stderr, "measured %s -> %s\n", appDir, out)
		} else {
			fmt.Println(string(report))
		}
	})
}

// runCheck measures the app and evaluates the checks against the rendered
// reality, printing a precise pass/fail report.
func runCheck(appDir, checksPath, out string, audit bool, width int) error {
	var checks []byte
	if !audit {
		var err error
		checks, err = os.ReadFile(checksPath)
		if err != nil {
			return err
		}
	}
	var rerr error
	e := measureRows(appDir, width, func(rt *qrt.Runtime, url string, measured []byte) {
		var report []byte
		if audit {
			report, rerr = measure.Audit(rt, measured)
		} else if isFlow(checks) {
			report, rerr = evalFlow(rt, url, checks)
		} else {
			report, rerr = measure.Eval(rt, measured, checks)
		}
		if rerr != nil {
			return
		}
		if out != "" {
			_ = os.WriteFile(out, report, 0o644)
			fmt.Fprintf(os.Stderr, "checked %s -> %s\n", appDir, out)
		} else {
			fmt.Println(string(report))
		}
	})
	if e != nil {
		return e
	}
	return rerr
}

// isFlow reports whether the checks JSON is a step-flow object ({"steps":[…]})
// rather than a flat array of static checks.
func isFlow(b []byte) bool {
	t := bytesTrimLeadingSpace(b)
	return len(t) > 0 && t[0] == '{'
}

func bytesTrimLeadingSpace(b []byte) []byte {
	i := 0
	for i < len(b) && (b[i] == ' ' || b[i] == '\n' || b[i] == '\t' || b[i] == '\r') {
		i++
	}
	return b[i:]
}

func postMCP(url, tool string, args map[string]any) {
	body, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 1, "method": "tools/call",
		"params": map[string]any{"name": tool, "arguments": args}})
	resp, err := http.Post(url+"mcp", "application/json", bytesReader(body))
	if err == nil {
		resp.Body.Close()
	}
}

func httpGetBytes(u string) []byte {
	resp, err := http.Get(u)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return b
}

// evalFlow applies each step's action to the live app, waits for the re-render
// and re-measure, then evaluates that step's checks — verifying interactions.
func evalFlow(rt *qrt.Runtime, url string, checksJSON []byte) ([]byte, error) {
	var flow struct {
		Steps []struct {
			Name string `json:"name"`
			Do   struct {
				SetState *struct {
					Path  string `json:"path"`
					Value any    `json:"value"`
				} `json:"setState"`
				Dispatch string         `json:"dispatch"`
				Args     map[string]any `json:"args"`
			} `json:"do"`
			Checks []map[string]any `json:"checks"`
		} `json:"steps"`
	}
	if err := json.Unmarshal(checksJSON, &flow); err != nil {
		return nil, fmt.Errorf("bad flow JSON: %w", err)
	}
	var steps []map[string]any
	allPass := true
	for i, st := range flow.Steps {
		action := "(none)"
		if st.Do.SetState != nil {
			postMCP(url, "qorm_set_state", map[string]any{"path": st.Do.SetState.Path, "value": st.Do.SetState.Value})
			action = fmt.Sprintf("set_state %s=%v", st.Do.SetState.Path, st.Do.SetState.Value)
		} else if st.Do.Dispatch != "" {
			postMCP(url, "qorm_dispatch", map[string]any{"action": st.Do.Dispatch, "args": st.Do.Args})
			action = "dispatch " + st.Do.Dispatch
		}
		// wait until the app re-measures after the morph (poll until it changes)
		before := httpGetBytes(url + "measure")
		measured := before
		for j := 0; j < 25; j++ {
			time.Sleep(80 * time.Millisecond)
			now := httpGetBytes(url + "measure")
			if len(now) > 2 && !bytes.Equal(now, before) {
				measured = now
				break
			}
			measured = now
		}
		cb, _ := json.Marshal(st.Checks)
		rep, err := measure.Eval(rt, measured, cb)
		if err != nil {
			return nil, err
		}
		var rd map[string]any
		json.Unmarshal(rep, &rd)
		if ok, _ := rd["ok"].(bool); !ok {
			allPass = false
		}
		steps = append(steps, map[string]any{"step": i + 1, "name": st.Name, "action": action, "result": rd})
	}
	return json.MarshalIndent(map[string]any{"app": rt.App.Name, "ok": allPass, "steps": steps}, "", "  ")
}

func bytesReader(b []byte) *bytes.Reader { return bytes.NewReader(b) }

// runPreview serves a packaged (static, offline) app directory and loads it in
// the WebView — the app boots its WASM runtime and renders client-side with no
// server. It captures the app's self-measurement (POST /measure) plus, if an
// eval is given, runs it (e.g. "qorm(0)") to exercise interactivity, then
// writes the measurement to out and closes.
func runPreview(dir string, width int, eval, out string) error {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return err
	}
	var mu sync.Mutex
	var measured []byte
	mux := http.NewServeMux()
	mux.HandleFunc("/measure", func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(io.LimitReader(r.Body, 4<<20))
		mu.Lock()
		measured = b
		mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
	})
	mux.Handle("/", http.FileServer(http.Dir(dir)))
	url := "http://" + ln.Addr().String() + "/"
	go func() { _ = http.Serve(ln, mux) }()

	runtime.LockOSThread()
	w := webview.New(false)
	defer w.Destroy()
	w.SetTitle("qorm preview")
	w.SetSize(width, 820, webview.HintNone)
	w.Navigate(url)
	go func() {
		read := func() []byte { mu.Lock(); defer mu.Unlock(); return measured }
		for i := 0; i < 100 && len(read()) <= 2; i++ {
			time.Sleep(80 * time.Millisecond)
		}
		if eval != "" {
			mu.Lock()
			measured = nil
			mu.Unlock()
			w.Dispatch(func() { w.Eval(eval) })
			for i := 0; i < 100 && len(read()) <= 2; i++ {
				time.Sleep(80 * time.Millisecond)
			}
		}
		m := read()
		if out != "" {
			_ = os.WriteFile(out, m, 0o644)
			fmt.Fprintf(os.Stderr, "preview measured -> %s (%d bytes)\n", out, len(m))
		} else {
			fmt.Println(string(m))
		}
		w.Terminate()
	}()
	w.Run()
	return nil
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

// desktopHardware runs one hardware op off the UI thread and calls back via cb,
// dispatching to per-OS implementations.
func desktopHardware(op string, m map[string]interface{}, cb func(string), onMain func(func())) {
	// Native (Cocoa) ops MUST run on the main thread — CoreBluetooth / NSScreen /
	// LocalAuthentication / dockTile / SMAppService / CoreWLAN abort otherwise.
	// onMain also gives them the main run loop (needed by CoreBluetooth's scan
	// timer). Subprocess ops (osascript / pmset / pactl) stay off-thread.
	switch op {
	case "notify":
		title, _ := m["title"].(string)
		body, _ := m["body"].(string)
		id, _ := m["id"].(string)
		onMain(func() { desktopNotify(title, body, id) })
		return
	case "badge":
		label := ""
		if c, ok := m["count"].(float64); ok && c > 0 {
			label = strconv.Itoa(int(c))
		}
		onMain(func() { setDockBadge(label) })
		return
	case "loginItem":
		enabled, _ := m["enabled"].(bool)
		onMain(func() {
			ok := setLoginItem(enabled)
			cb(fmt.Sprintf("qormOnLoginItem(%t,%t)", enabled && ok, ok))
		})
		return
	case "loginItemGet":
		onMain(func() { cb(fmt.Sprintf("qormOnLoginItem(%t,true)", loginItemEnabled())) })
		return
	case "screens":
		onMain(func() { cb("qormOnScreens(" + screenInfo() + ")") })
		return
	case "biometric":
		biometricHandler = func(ok bool, msg string) {
			cb(fmt.Sprintf("qormOnBiometric(%t,%s)", ok, strconv.Quote(msg)))
		}
		onMain(func() { nativeBiometric() })
		return
	case "wifiInfo":
		onMain(func() { cb("qormOnWifi(" + wifiDesktopInfo() + ")") })
		return
	case "getModes":
		onMain(func() { cb("qormOnModes(" + strconv.Quote(nativeSystemModes()) + ")") })
		return
	case "winDragStart":
		nativeWinDragStart("main")
		return
	case "winDragMove":
		dx, _ := m["dx"].(float64)
		dy, _ := m["dy"].(float64)
		nativeWinDragMove("main", int(dx), int(dy))
		return
	case "bluetoothScan":
		onMain(func() { nativeBluetoothScan() })
		return
	case "bluetoothState":
		onMain(func() { nativeBluetoothState() })
		return
	case "platform":
		p := "linux"
		if runtime.GOOS == "darwin" {
			p = "mac"
		} else if runtime.GOOS == "windows" {
			p = "windows"
		}
		cb("qormOnPlatform(" + strconv.Quote(p) + ")")
		return
	}
	switch runtime.GOOS {
	case "darwin":
		desktopHardwareDarwin(op, m, cb)
	case "linux":
		desktopHardwareLinux(op, m, cb)
	case "windows":
		desktopHardwareWindows(op, m, cb)
	}
	// A custom op the built-in bridge doesn't know → the app's own Go handler
	// (native/desktop.go, compiled INTO this binary via qormext.Register).
	if !desktopBuiltins[op] {
		if fn := qormext.Ops[op]; fn != nil {
			if js := fn(m); js != "" {
				cb(js)
			}
		}
	}
}

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

// clamp01 constrains v to [0,1].
func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

// darwinBrightness reads the main display brightness (0..1) via the optional
// `brightness` CLI. Returns ok=false when the tool is absent or unparseable.
func darwinBrightness() (float64, bool) {
	if lvl, ok := nativeBrightnessGet(); ok {
		return lvl, true
	}
	out, err := exec.Command("brightness", "-l").Output()
	if err != nil {
		return 0, false
	}
	// Lines look like: "display 0: brightness 0.500000" — take the last one.
	lvl, ok := 0.0, false
	for _, ln := range strings.Split(string(out), "\n") {
		if i := strings.LastIndex(ln, "brightness "); i >= 0 {
			if v, e := strconv.ParseFloat(strings.TrimSpace(ln[i+len("brightness "):]), 64); e == nil {
				lvl, ok = clamp01(v), true
			}
		}
	}
	return lvl, ok
}

// linuxBrightness reads backlight brightness (0..1) via brightnessctl's
// machine-readable output ("device,class,current,NN%,max"). ok=false when the
// tool/backlight is unavailable.
func linuxBrightness() (float64, bool) {
	out, err := exec.Command("brightnessctl", "-m").Output()
	if err != nil {
		return 0, false
	}
	fields := strings.Split(strings.TrimSpace(string(out)), ",")
	if len(fields) < 4 {
		return 0, false
	}
	pct := strings.TrimSuffix(fields[3], "%")
	n, err := strconv.Atoi(strings.TrimSpace(pct))
	if err != nil {
		return 0, false
	}
	return clamp01(float64(n) / 100), true
}

// desktopNotify shows a native OS notification.
func desktopNotify(title, body, id string) {
	switch runtime.GOOS {
	case "darwin":
		nativeNotify(title, body, id) // in-process → click routes back to the app
	case "linux":
		exec.Command("notify-send", title, body).Run()
	}
}

// desktopHardwareLinux implements the ops with common Linux tools: volume via
// pactl (PulseAudio/PipeWire), battery via /sys/class/power_supply.
func desktopHardwareLinux(op string, m map[string]interface{}, cb func(string)) {
	num := func(s string) int { n, _ := strconv.Atoi(strings.TrimSpace(s)); return n }
	switch op {
	case "volumeGet", "volumeUp", "volumeDown":
		if op == "volumeUp" {
			exec.Command("pactl", "set-sink-volume", "@DEFAULT_SINK@", "+6%").Run()
		} else if op == "volumeDown" {
			exec.Command("pactl", "set-sink-volume", "@DEFAULT_SINK@", "-6%").Run()
		}
		out, _ := exec.Command("pactl", "get-sink-volume", "@DEFAULT_SINK@").Output()
		if i := strings.Index(string(out), "%"); i > 0 {
			j := i
			for j > 0 && string(out)[j-1] >= '0' && string(out)[j-1] <= '9' {
				j--
			}
			cb(fmt.Sprintf("qormOnVolume(%g)", float64(num(string(out)[j:i]))/100))
		}
	case "battery":
		base := "/sys/class/power_supply/BAT0/"
		cap, err := os.ReadFile(base + "capacity")
		if err != nil {
			base = "/sys/class/power_supply/BAT1/"
			cap, _ = os.ReadFile(base + "capacity")
		}
		st, _ := os.ReadFile(base + "status")
		charging := strings.TrimSpace(string(st)) == "Charging" || strings.TrimSpace(string(st)) == "Full"
		cb(fmt.Sprintf("qormOnBattery(%g,%t)", float64(num(string(cap)))/100, charging))
	case "brightnessGet", "brightnessUp", "brightnessDown":
		// Best-effort via brightnessctl (backlight). Falls back to a clear
		// "n/a on desktop" when there's no backlight/tool, so the auto-fired
		// brightnessGet never leaves the UI stuck.
		if op == "brightnessUp" {
			exec.Command("brightnessctl", "set", "+10%").Run()
		} else if op == "brightnessDown" {
			exec.Command("brightnessctl", "set", "10%-").Run()
		}
		if lvl, ok := linuxBrightness(); ok {
			cb(fmt.Sprintf("qormOnBrightness(%g)", lvl))
		} else {
			cb(brightnessUnsupportedJS)
		}
	case "torchGet", "torchToggle":
		// No camera flash/torch on desktop; report OFF so the UI resolves.
		cb("qormOnTorch(false)")
	case "vibrate":
		// No vibration motor; the web layer already updates its label.
	case "clipboardSet":
		t, _ := m["text"].(string)
		if !linuxClip(t) {
		}
		cb("qormOnClipboard(" + strconv.Quote(t) + ")")
	case "clipboardGet":
		out, _ := exec.Command("sh", "-c", "wl-paste 2>/dev/null || xclip -o -selection clipboard 2>/dev/null || xsel -b 2>/dev/null").Output()
		cb("qormOnClipboard(" + strconv.Quote(strings.TrimSpace(string(out))) + ")")
	case "speak":
		if t, _ := m["text"].(string); t != "" {
			exec.Command("spd-say", t).Start()
		}
	case "openURL":
		if u, _ := m["url"].(string); u != "" {
			exec.Command("xdg-open", u).Run()
			cb("qormOnOpenUrl(true)")
		}
	case "share":
		t, _ := m["text"].(string)
		linuxClip(t)
		cb("qormOnShare(true)")
	case "deviceInfo":
		host, _ := os.Hostname()
		kern, _ := exec.Command("uname", "-sr").Output()
		info := fmt.Sprintf(`{"model":"Linux","name":%q,"os":%q}`, host, strings.TrimSpace(string(kern)))
		cb("qormOnDeviceInfo(" + strconv.Quote(info) + ")")
	case "networkStatus":
		online := exec.Command("sh", "-c", "ip route get 1.1.1.1 >/dev/null 2>&1").Run() == nil
		cb("qormOnNetwork(" + strconv.Quote(fmt.Sprintf(`{"online":%t,"type":"desktop"}`, online)) + ")")
	case "keepAwake":
		on, _ := m["on"].(bool)
		if on {
			if inhibitCmd == nil {
				inhibitCmd = exec.Command("systemd-inhibit", "--what=idle:sleep", "--why=qorm", "sleep", "infinity")
				inhibitCmd.Start()
			}
		} else if inhibitCmd != nil {
			inhibitCmd.Process.Kill()
			inhibitCmd = nil
		}
	case "haptic":
	case "storageSet":
		os.WriteFile(filepath.Join(os.TempDir(), "qorm-store-"+strFromMap(m, "key")), []byte(strFromMap(m, "value")), 0o644)
	case "storageGet":
		k := strFromMap(m, "key")
		d, _ := os.ReadFile(filepath.Join(os.TempDir(), "qorm-store-"+k))
		cb("qormOnStorage(" + strconv.Quote(k) + ", " + strconv.Quote(string(d)) + ")")
	case "screenshot":
		fp := filepath.Join(os.TempDir(), "qorm-shot.png")
		exec.Command("sh", "-c", "grim "+fp+" 2>/dev/null || scrot -o "+fp+" 2>/dev/null || import -window root "+fp+" 2>/dev/null").Run()
		if data, err := os.ReadFile(fp); err == nil {
			cb("qormOnScreenshot(" + strconv.Quote("data:image/png;base64,"+base64.StdEncoding.EncodeToString(data)) + ")")
		}
	case "notify":
		exec.Command("notify-send", strFromMap(m, "title"), strFromMap(m, "body")).Run()
	}
}

// desktopHardwareDarwin implements the ops with macOS tools.
func desktopHardwareDarwin(op string, m map[string]interface{}, cb func(string)) {
	sh := func(s string) string {
		out, _ := exec.Command("osascript", "-e", s).Output()
		return strings.TrimSpace(string(out))
	}
	switch op {
	case "speak":
		if t, _ := m["text"].(string); t != "" {
			exec.Command("say", t).Start()
		}
		return
	case "secureSet":
		k, _ := m["key"].(string)
		v, _ := m["value"].(string)
		exec.Command("security", "delete-generic-password", "-a", k, "-s", "qorm").Run()
		exec.Command("security", "add-generic-password", "-a", k, "-s", "qorm", "-w", v).Run()
		cb("qormOnSecure(" + strconv.Quote(k) + ", " + strconv.Quote("saved") + ")")
		return
	case "secureGet":
		k, _ := m["key"].(string)
		out, _ := exec.Command("security", "find-generic-password", "-a", k, "-s", "qorm", "-w").Output()
		cb("qormOnSecure(" + strconv.Quote(k) + ", " + strconv.Quote(strings.TrimSpace(string(out))) + ")")
		return
	case "speakStop":
		exec.Command("killall", "say").Run()
		return
	case "openURL":
		if u, _ := m["url"].(string); u != "" {
			exec.Command("open", u).Run()
			cb("qormOnOpenUrl(true)")
		}
		return
	case "clipboardSet":
		t, _ := m["text"].(string)
		c := exec.Command("pbcopy")
		c.Stdin = strings.NewReader(t)
		c.Run()
		cb("qormOnClipboard(" + strconv.Quote(t) + ")")
		return
	case "clipboardGet":
		out, _ := exec.Command("pbpaste").Output()
		cb("qormOnClipboard(" + strconv.Quote(string(out)) + ")")
		return
	case "share":
		t, _ := m["text"].(string)
		nativeShare(t) // already dispatches to the main queue internally
		cb("qormOnShare(true)")
		return
	case "deviceInfo":
		host, _ := os.Hostname()
		ver, _ := exec.Command("sw_vers", "-productVersion").Output()
		info := fmt.Sprintf(`{"model":"Mac","name":%q,"os":"macOS %s"}`, host, strings.TrimSpace(string(ver)))
		cb("qormOnDeviceInfo(" + strconv.Quote(info) + ")")
		return
	case "networkStatus":
		online := exec.Command("sh", "-c", "route -n get default >/dev/null 2>&1").Run() == nil
		cb("qormOnNetwork(" + strconv.Quote(fmt.Sprintf(`{"online":%t,"type":"desktop"}`, online)) + ")")
		return
	case "keepAwake":
		on, _ := m["on"].(bool)
		if on {
			if caffeinateCmd == nil {
				caffeinateCmd = exec.Command("caffeinate", "-d")
				caffeinateCmd.Start()
			}
		} else if caffeinateCmd != nil {
			caffeinateCmd.Process.Kill()
			caffeinateCmd = nil
		}
		return
	case "haptic":
		return
	case "storageSet":
		k, _ := m["key"].(string)
		v, _ := m["value"].(string)
		os.WriteFile(filepath.Join(os.TempDir(), "qorm-store-"+k), []byte(v), 0o644)
		return
	case "storageGet":
		k, _ := m["key"].(string)
		d, _ := os.ReadFile(filepath.Join(os.TempDir(), "qorm-store-"+k))
		cb("qormOnStorage(" + strconv.Quote(k) + ", " + strconv.Quote(string(d)) + ")")
		return
	case "screenshot":
		f := filepath.Join(os.TempDir(), "qorm-shot.jpg")
		if exec.Command("screencapture", "-x", "-t", "jpg", f).Run() == nil {
			if data, err := os.ReadFile(f); err == nil {
				cb("qormOnScreenshot(" + strconv.Quote("data:image/jpeg;base64,"+base64.StdEncoding.EncodeToString(data)) + ")")
			}
		}
		return
	case "screenRecordStart":
		screenRecFile = filepath.Join(os.TempDir(), "qorm-rec.mov")
		os.Remove(screenRecFile)
		screenRecCmd = exec.Command("screencapture", "-v", screenRecFile)
		if screenRecCmd.Start() == nil {
			cb("qormOnScreenRecord(" + strconv.Quote("● recording… (grant Screen Recording if prompted)") + ")")
		}
		return
	case "screenRecordStop":
		if screenRecCmd != nil && screenRecCmd.Process != nil {
			screenRecCmd.Process.Signal(os.Interrupt)
			screenRecCmd.Wait()
			screenRecCmd = nil
			cb("qormOnScreenRecord(" + strconv.Quote("saved: "+screenRecFile) + ")")
		}
		return
	case "volumeGet":
		if v, err := strconv.Atoi(sh("output volume of (get volume settings)")); err == nil {
			cb(fmt.Sprintf("qormOnVolume(%g)", float64(v)/100))
		}
		if mu := nativeReadMute(); mu >= 0 {
			cb(fmt.Sprintf("qormEmit(%q,%t)", "mute", mu == 1))
		}
	case "volumeSet":
		if v, ok := m["value"].(float64); ok {
			sh(fmt.Sprintf("set volume output volume %d", int(v*100)))
			cb(fmt.Sprintf("qormOnVolume(%g)", v))
		}
	case "volumeUp", "volumeDown":
		v, _ := strconv.Atoi(sh("output volume of (get volume settings)"))
		if op == "volumeUp" {
			v += 6
		} else {
			v -= 6
		}
		if v < 0 {
			v = 0
		} else if v > 100 {
			v = 100
		}
		sh(fmt.Sprintf("set volume output volume %d", v))
		cb(fmt.Sprintf("qormOnVolume(%g)", float64(v)/100))
	case "brightnessGet", "brightnessUp", "brightnessDown":
		// Reading/writing display brightness has no built-in macOS CLI; the
		// popular `brightness` tool (brew install brightness) provides it. Use
		// it best-effort and fall back to a clear "n/a on desktop" so the UI is
		// never stuck spinning on the auto-fired brightnessGet.
		if lvl, ok := darwinBrightness(); ok {
			if op == "brightnessUp" || op == "brightnessDown" {
				if op == "brightnessUp" {
					lvl += 0.1
				} else {
					lvl -= 0.1
				}
				lvl = clamp01(lvl)
				nativeBrightnessSet(lvl)
				if nl, ok2 := darwinBrightness(); ok2 {
					lvl = nl
				}
			}
			cb(fmt.Sprintf("qormOnBrightness(%g)", lvl))
		} else {
			cb(brightnessUnsupportedJS)
		}
		return
	case "torchGet", "torchToggle":
		// Desktops have no camera flash/torch; report OFF so the UI resolves
		// (auto-fired torchGet) instead of hanging.
		cb("qormOnTorch(false)")
		return
	case "vibrate":
		// No vibration motor on desktop; the web layer already updates its own
		// label, so this is intentionally a no-op.
		return
	case "battery":
		out, _ := exec.Command("pmset", "-g", "batt").Output()
		s := string(out)
		lvl := 1.0
		if i := strings.Index(s, "%"); i > 0 {
			j := i
			for j > 0 && s[j-1] >= '0' && s[j-1] <= '9' {
				j--
			}
			if n, err := strconv.Atoi(s[j:i]); err == nil {
				lvl = float64(n) / 100
			}
		}
		charging := strings.Contains(s, "AC Power") || strings.Contains(s, "charging")
		cb(fmt.Sprintf("qormOnBattery(%g,%t)", lvl, charging))
	}
}

// runTray shows a system-tray icon + menu for the running app (a desktop
// staple). It runs in its own process (systray needs the main run loop, which
// the WebView also owns), spawned by launchWindow. Quit terminates the app.
var gTrayURL string

// traySelected routes a tray click: a "quit" item terminates the app, anything
// else fires qormEmit('tray', {id}) in the app window via the local server.
func traySelected(id string) {
	if id == "quit" {
		if p := os.Getppid(); p > 1 {
			syscall.Kill(p, syscall.SIGTERM)
		}
		os.Exit(0)
	}
	js := "qormEmit('tray',{id:" + strconv.Quote(id) + "})"
	body, _ := json.Marshal(map[string]string{"id": "main", "op": "eval", "js": js})
	http.Post(gTrayURL+"window", "application/json", bytes.NewReader(body))
}

func runTray(url, title, trayJSON string) {
	gTrayURL = url
	// exit with the app (the WebView process is our parent)
	parent := os.Getppid()
	go func() {
		for {
			time.Sleep(400 * time.Millisecond)
			if os.Getppid() != parent {
				os.Exit(0)
			}
		}
	}()
	icon, _ := iconFS.ReadFile("icons/tray.png")
	if trayJSON != "" {
		nativeTrayJSON(icon, trayJSON, title)
		return
	}
	items := []string{"Activity Log", "Open in Browser", "Quit QORM"}
	nativeTray(icon, items, title, func(i int) {
		switch i {
		case 0:
			openBrowser(url + "logwindow")
		case 1:
			openBrowser(url)
		case 2:
			if p := os.Getppid(); p > 1 {
				syscall.Kill(p, syscall.SIGTERM) // quit the app window too
			}
			os.Exit(0)
		}
	})
}

// linuxClip copies text to the clipboard via whatever tool is present.
func linuxClip(text string) bool {
	for _, c := range [][]string{{"wl-copy"}, {"xclip", "-selection", "clipboard"}, {"xsel", "-b"}} {
		cmd := exec.Command(c[0], c[1:]...)
		cmd.Stdin = strings.NewReader(text)
		if cmd.Run() == nil {
			return true
		}
	}
	return false
}

func strFromMap(m map[string]interface{}, k string) string { s, _ := m[k].(string); return s }

var inhibitCmd *exec.Cmd

// desktopHardwareWindows implements the hardware ops on Windows via PowerShell.
// Runtime-unverified on this build host (needs a Windows machine); mirrors the
// mac/linux handlers.
func desktopHardwareWindows(op string, m map[string]interface{}, cb func(string)) {
	ps := func(script string) string {
		out, _ := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", script).Output()
		return strings.TrimSpace(string(out))
	}
	switch op {
	case "speak":
		if t, _ := m["text"].(string); t != "" {
			exec.Command("say", t).Start()
		}
		return
	case "secureSet":
		k, _ := m["key"].(string)
		v, _ := m["value"].(string)
		exec.Command("security", "delete-generic-password", "-a", k, "-s", "qorm").Run()
		exec.Command("security", "add-generic-password", "-a", k, "-s", "qorm", "-w", v).Run()
		cb("qormOnSecure(" + strconv.Quote(k) + ", " + strconv.Quote("saved") + ")")
		return
	case "secureGet":
		k, _ := m["key"].(string)
		out, _ := exec.Command("security", "find-generic-password", "-a", k, "-s", "qorm", "-w").Output()
		cb("qormOnSecure(" + strconv.Quote(k) + ", " + strconv.Quote(strings.TrimSpace(string(out))) + ")")
		return
	case "speakStop":
		exec.Command("killall", "say").Run()
		return
	case "openURL":
		if u := strFromMap(m, "url"); u != "" {
			ps("Start-Process " + psQuote(u))
			cb("qormOnOpenUrl(true)")
		}
	case "clipboardSet", "share":
		t := strFromMap(m, "text")
		ps("Set-Clipboard -Value " + psQuote(t))
		if op == "share" {
			cb("qormOnShare(true)")
		} else {
			cb("qormOnClipboard(" + strconv.Quote(t) + ")")
		}
	case "clipboardGet":
		cb("qormOnClipboard(" + strconv.Quote(ps("Get-Clipboard")) + ")")
	case "deviceInfo":
		name := ps("$env:COMPUTERNAME")
		ver := ps("(Get-CimInstance Win32_OperatingSystem).Caption")
		info := fmt.Sprintf(`{"model":"Windows","name":%q,"os":%q}`, name, ver)
		cb("qormOnDeviceInfo(" + strconv.Quote(info) + ")")
	case "networkStatus":
		online := ps("[bool](Test-Connection -Count 1 -Quiet 1.1.1.1)") == "True"
		cb("qormOnNetwork(" + strconv.Quote(fmt.Sprintf(`{"online":%t,"type":"desktop"}`, online)) + ")")
	case "storageSet":
		os.WriteFile(filepath.Join(os.TempDir(), "qorm-store-"+strFromMap(m, "key")), []byte(strFromMap(m, "value")), 0o644)
	case "storageGet":
		k := strFromMap(m, "key")
		d, _ := os.ReadFile(filepath.Join(os.TempDir(), "qorm-store-"+k))
		cb("qormOnStorage(" + strconv.Quote(k) + ", " + strconv.Quote(string(d)) + ")")
	case "notify":
		ps("Add-Type -AssemblyName System.Windows.Forms; $n=New-Object System.Windows.Forms.NotifyIcon; $n.Icon=[System.Drawing.SystemIcons]::Information; $n.Visible=$true; $n.ShowBalloonTip(4000," + psQuote(strFromMap(m, "title")) + "," + psQuote(strFromMap(m, "body")) + ",'Info')")
	}
}

// psQuote single-quotes a string for PowerShell (doubling embedded quotes).
func psQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", "''") + "'" }
