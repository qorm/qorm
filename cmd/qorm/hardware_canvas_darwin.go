//go:build darwin && !desktop

package main

// canvasHardwareDarwin is the pure-Go (exec-based) native bridge for the
// canvas build, whose defining constraint is no cgo — the desktop webview
// bridge (hardware_desktop.go, desktop-tagged cgo chain) cannot be reused.
// It mirrors that bridge's fast paths only: anything needing AppKit/cgo
// (wifi info, bluetooth, biometric, tray, share sheet, screenshots) is out
// and reported as unsupported by the caller. Ops run synchronously on the
// render thread, so they are all short (a route check, an osascript query,
// a pbpaste) — scans/streams stay with the widget's slow-path guard.
//
// The callback strings match the JS bridge contract exactly
// (qormOn<Stem>(<arg>)), so canvas.ParseNativeCallback reads them back.

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

func canvasHardwareDarwin(op string, m map[string]interface{}, cb func(string)) {
	sh := func(name string, args ...string) string {
		out, _ := exec.Command(name, args...).Output()
		return strings.TrimSpace(string(out))
	}
	osa := func(s string) string { return sh("osascript", "-e", s) }
	q := strconv.Quote

	switch op {
	case "networkStatus":
		online := exec.Command("sh", "-c", "route -n get default >/dev/null 2>&1").Run() == nil
		cb("qormOnNetwork(" + q(fmt.Sprintf(`{"online":%t,"type":"desktop"}`, online)) + ")")
	case "deviceInfo":
		host, _ := os.Hostname()
		ver := sh("sw_vers", "-productVersion")
		cb("qormOnDeviceInfo(" + q(fmt.Sprintf(`{"model":"Mac","name":%q,"os":"macOS %s"}`, host, ver)) + ")")
	case "battery":
		// pmset -g batt → "97%; discharging; 5:12 remaining" — level% + state.
		// AC-powered desktops report only "Now drawing from 'AC Power'".
		out := sh("pmset", "-g", "batt")
		level, state := "", "unknown"
		if i := strings.Index(out, "%"); i > 0 {
			j := i
			for j > 0 && out[j-1] >= '0' && out[j-1] <= '9' {
				j--
			}
			level = out[j:i]
		}
		switch {
		case strings.Contains(out, "discharging"):
			state = "discharging"
		case strings.Contains(out, "charged") && !strings.Contains(out, "discharging"):
			state = "charged"
		case strings.Contains(out, "charging"):
			state = "charging"
		case strings.Contains(out, "AC Power"):
			state = "ac"
		}
		cb("qormOnBattery(" + q(fmt.Sprintf(`{"level":%q,"state":%q}`, level, state)) + ")")
	case "volumeGet":
		cb("qormOnVolume(" + q(osa("output volume of (get volume settings)")) + ")")
	case "volumeSet":
		if v, ok := m["value"]; ok {
			osa(fmt.Sprintf("set volume output volume %v", v))
		}
		cb("qormOnVolume(" + q(osa("output volume of (get volume settings)")) + ")")
	case "volumeUp":
		osa("set volume output volume (output volume of (get volume settings) + 10)")
		cb("qormOnVolume(" + q(osa("output volume of (get volume settings)")) + ")")
	case "volumeDown":
		osa("set volume output volume (output volume of (get volume settings) - 10)")
		cb("qormOnVolume(" + q(osa("output volume of (get volume settings)")) + ")")
	case "brightnessGet":
		cb("qormOnBrightness(" + q(brightnessLevel(sh)) + ")")
	case "brightnessSet":
		if v, ok := m["value"]; ok {
			sh("brightness", "-v", fmt.Sprint(v))
		}
		cb("qormOnBrightness(" + q(brightnessLevel(sh)) + ")")
	case "brightnessUp":
		// No public pure-Go read API on macOS (the `brightness` CLI is a
		// Homebrew extra): adjust through the system brightness key codes,
		// then read back if the CLI happens to exist ("—" otherwise).
		osa("tell application \"System Events\" to key code 144")
		cb("qormOnBrightness(" + q(brightnessLevel(sh)) + ")")
	case "brightnessDown":
		osa("tell application \"System Events\" to key code 145")
		cb("qormOnBrightness(" + q(brightnessLevel(sh)) + ")")
	case "clipboardGet":
		cb("qormOnClipboard(" + q(sh("pbpaste")) + ")")
	case "clipboardSet":
		t, _ := m["text"].(string)
		cmd := exec.Command("pbcopy")
		cmd.Stdin = strings.NewReader(t)
		_ = cmd.Run()
		cb("qormOnClipboard(" + q(t) + ")")
	case "openURL":
		if u, _ := m["url"].(string); u != "" {
			_ = exec.Command("open", u).Start()
			cb("qormOnOpenUrl(true)")
		}
	case "notify":
		title, _ := m["title"].(string)
		body, _ := m["body"].(string)
		if title == "" {
			title = "QORM"
		}
		osa(fmt.Sprintf("display notification %q with title %q", body, title))
		cb("qormOnNotify(true)")
	case "speak":
		if t, _ := m["text"].(string); t != "" {
			_ = exec.Command("say", t).Start()
		}
	case "speakStop":
		_ = exec.Command("killall", "say").Run()
	case "keepAwake":
		on, _ := m["on"].(bool)
		if on {
			_ = exec.Command("caffeinate", "-dims").Start()
		} else {
			_ = exec.Command("killall", "caffeinate").Run()
		}
	case "secureSet":
		k, _ := m["key"].(string)
		v, _ := m["value"].(string)
		okc := exec.Command("security", "add-generic-password", "-U", "-s", "qorm", "-a", k, "-w", v).Run() == nil
		if okc {
			cb("qormOnSecure(" + q(k) + ", " + q("saved") + ")")
		} else {
			cb("qormOnSecure(" + q(k) + ", " + q("error") + ")")
		}
	case "secureGet":
		k, _ := m["key"].(string)
		val := sh("security", "find-generic-password", "-s", "qorm", "-a", k, "-w")
		cb("qormOnSecure(" + q(k) + ", " + q(val) + ")")
	default:
		// Not implementable in pure Go (cgo bridge territory): wifi info,
		// bluetooth, biometric, tray/dock, share sheet, screenshot, screens.
		// Silence beats a fake: no callback, the widget shows its degradation.
	}
}

// brightnessLevel reads the current level when the optional `brightness` CLI
// (Homebrew) exists, else returns "—" — macOS has no pure-Go read API, and
// faking a number would be worse than an honest dash.
func brightnessLevel(sh func(string, ...string) string) string {
	if v := sh("brightness", "-l"); v != "" {
		return v
	}
	return "—"
}
