//go:build desktop

package main

import (
	"encoding/base64"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/qorm/qorm/pkg/qormext"
)

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
			nativeSpeak(t)
		}
		return
	case "secureSet":
		k, _ := m["key"].(string)
		v, _ := m["value"].(string)
		if nativeSecureSet(k, v) {
			cb("qormOnSecure(" + strconv.Quote(k) + ", " + strconv.Quote("saved") + ")")
		} else {
			cb("qormOnSecure(" + strconv.Quote(k) + ", " + strconv.Quote("error") + ")")
		}
		return
	case "secureGet":
		k, _ := m["key"].(string)
		val := nativeSecureGet(k)
		cb("qormOnSecure(" + strconv.Quote(k) + ", " + strconv.Quote(val) + ")")
		return
	case "speakStop":
		nativeSpeakStop()
		return
	case "openURL":
		if u, _ := m["url"].(string); u != "" {
			nativeOpenURL(u)
			cb("qormOnOpenUrl(true)")
		}
		return
	case "clipboardSet":
		t, _ := m["text"].(string)
		nativeClipboardSet(t)
		cb("qormOnClipboard(" + strconv.Quote(t) + ")")
		return
	case "clipboardGet":
		val := nativeClipboardGet()
		cb("qormOnClipboard(" + strconv.Quote(val) + ")")
		return
	case "share":
		t, _ := m["text"].(string)
		nativeShare(t) // already dispatches to the main queue internally
		cb("qormOnShare(true)")
		return
	case "deviceInfo":
		host, _ := os.Hostname()
		ver := nativeOSVersion()
		info := fmt.Sprintf(`{"model":"Mac","name":%q,"os":"macOS %s"}`, host, ver)
		cb("qormOnDeviceInfo(" + strconv.Quote(info) + ")")
		return
	case "networkStatus":
		// Fast ping check or system API would be better, but the path routing is okay best-effort.
		online := exec.Command("sh", "-c", "route -n get default >/dev/null 2>&1").Run() == nil
		cb("qormOnNetwork(" + strconv.Quote(fmt.Sprintf(`{"online":%t,"type":"desktop"}`, online)) + ")")
		return
	case "keepAwake":
		on, _ := m["on"].(bool)
		nativeSetKeepAwake(on)
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
		b64 := nativeScreenshot()
		if b64 != "" {
			cb("qormOnScreenshot(" + strconv.Quote("data:image/jpeg;base64,"+b64) + ")")
		} else {
			f := filepath.Join(os.TempDir(), "qorm-shot.jpg")
			if exec.Command("screencapture", "-x", "-t", "jpg", f).Run() == nil {
				if data, err := os.ReadFile(f); err == nil {
					cb("qormOnScreenshot(" + strconv.Quote("data:image/jpeg;base64,"+base64.StdEncoding.EncodeToString(data)) + ")")
				}
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
		if v, ok := nativeVolumeGet(); ok {
			cb(fmt.Sprintf("qormOnVolume(%g)", v))
		} else {
			if v, err := strconv.Atoi(sh("output volume of (get volume settings)")); err == nil {
				cb(fmt.Sprintf("qormOnVolume(%g)", float64(v)/100))
			}
		}
		if mu := nativeReadMute(); mu >= 0 {
			cb(fmt.Sprintf("qormEmit(%q,%t)", "mute", mu == 1))
		}
	case "volumeSet":
		if v, ok := m["value"].(float64); ok {
			if !nativeVolumeSet(v) {
				sh(fmt.Sprintf("set volume output volume %d", int(v*100)))
			}
			cb(fmt.Sprintf("qormOnVolume(%g)", v))
		}
	case "volumeUp", "volumeDown":
		var v float64
		var ok bool
		if v, ok = nativeVolumeGet(); !ok {
			if vol, err := strconv.Atoi(sh("output volume of (get volume settings)")); err == nil {
				v = float64(vol) / 100
			}
		}
		if op == "volumeUp" {
			v += 0.06
		} else {
			v -= 0.06
		}
		if v < 0 {
			v = 0
		} else if v > 1 {
			v = 1
		}
		if !nativeVolumeSet(v) {
			sh(fmt.Sprintf("set volume output volume %d", int(v*100)))
		}
		cb(fmt.Sprintf("qormOnVolume(%g)", v))
	case "brightnessGet", "brightnessUp", "brightnessDown":
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
		cb("qormOnTorch(false)")
		return
	case "vibrate":
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
			// Windows Speech Synthesizer via PowerShell COM object (non-blocking in background)
			go ps("Add-Type -AssemblyName System.Speech; $s=New-Object System.Speech.Synthesis.SpeechSynthesizer; $s.Speak(" + psQuote(t) + ")")
		}
		return
	case "secureSet":
		k, _ := m["key"].(string)
		v, _ := m["value"].(string)
		if nativeSecureSet(k, v) {
			cb("qormOnSecure(" + strconv.Quote(k) + ", " + strconv.Quote("saved") + ")")
		} else {
			cb("qormOnSecure(" + strconv.Quote(k) + ", " + strconv.Quote("error") + ")")
		}
		return
	case "secureGet":
		k, _ := m["key"].(string)
		val := nativeSecureGet(k)
		cb("qormOnSecure(" + strconv.Quote(k) + ", " + strconv.Quote(val) + ")")
		return
	case "speakStop":
		// Stop any speaking powershell processes if they are running
		exec.Command("taskkill", "/F", "/IM", "powershell.exe").Run()
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
