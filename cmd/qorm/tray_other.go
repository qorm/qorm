//go:build desktop && !darwin

package main

import (
	"unsafe"

	webview "github.com/qorm/qorm/internal/webview"
)

// nativeTray: native trays for Linux (AppIndicator/DBus) and Windows
// (Shell_NotifyIcon) are not wired yet, so the tray process simply exits.
func nativeTray(png []byte, items []string, tip string, onClick func(int)) {}

func setDockIcon(png []byte) {}

func setAppMenu(appName, menuJSON string) {}

func setDockBadge(label string) {}

func setLoginItem(enabled bool) bool { return false }
func loginItemEnabled() bool         { return false }

func nativeNotify(title, body, ident string) {}

func centerWindow()      {}
func screenInfo() string { return "[]" }

func windowFrame() string           { return "" }
func setWindowFrame(x, y, w, h int) {}

func grantMedia(window unsafe.Pointer) {}

func nativeBiometric() {}

func wifiDesktopInfo() string { return "{\"error\":\"not supported\"}" }

func nativeBluetoothScan()  {}
func nativeBluetoothState() {}

func disableRestore() {}

func fixWindow()                    {}
func moveMainWindow(x, y, w, h int) {}

func nativeBrightnessGet() (float64, bool) { return 0, false }
func nativeBrightnessSet(v float64) int    { return -1 }

func nativeShare(text string) {}

var volumeWatchHandler func(float64)

func nativeWatchVolume() {}

var muteWatchHandler func(bool)

func nativeReadMute() int { return -1 }

func nativeSystemModes() string { return "{}" }

var brightnessWatchHandler func(float64)

func nativeWatchBrightness() {}

var shortcutHandler func(string)

func nativeSetDockMenu(json string) {}

func nativeWinDragStart(id string) {
	var wv webview.WebView
	if id == "main" || id == "" {
		wv = appWebView
	} else {
		winMu.Lock()
		wv = activeWindows[id]
		winMu.Unlock()
	}
	if wv == nil {
		return
	}
	if hwnd := wv.Window(); hwnd != nil {
		startWindowDrag(hwnd)
	}
}

func nativeWinDragMove(id string, dx, dy int) {}

func nativeTrayJSON(png []byte, menuJSON, tip string) {}

func nativeClipboardSet(text string) {}
func nativeClipboardGet() string     { return "" }
func nativeOpenURL(url string)       {}
func nativeOSVersion() string        { return "" }
func nativeSetKeepAwake(on bool)     {}
func nativeSpeak(text string)        {}
func nativeSpeakStop()               {}
func nativeScreenshot() string       { return "" }
