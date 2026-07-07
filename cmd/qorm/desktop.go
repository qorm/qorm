package main

import (
	"os"
	"os/exec"
	"runtime"
)

func fileExists(p string) bool {
	info, err := os.Stat(p)
	return err == nil && !info.IsDir()
}

// openAppWindow launches the app in a chromeless "app mode" window using a
// Chromium-family browser (Chrome/Edge/Chromium) when available, giving a
// native-feeling standalone window. It falls back to a normal browser tab.
//
// This deliberately avoids a cgo WebView binding: keeping the runtime pure Go
// is what lets a single `go build` cross-compile to every platform. A true
// native WebView remains a possible opt-in (build-tagged) path, but it must not
// become the default.
func openAppWindow(url string) {
	if bin := findChromium(); bin != "" {
		// #nosec G204 -- url is a localhost address we just bound.
		if err := exec.Command(bin, "--app="+url, "--new-window").Start(); err == nil {
			return
		}
	}
	openBrowser(url)
}

// findChromium returns the path/command of a Chromium-family browser, or "".
func findChromium() string {
	var candidates []string
	switch runtime.GOOS {
	case "darwin":
		candidates = []string{
			"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
			"/Applications/Microsoft Edge.app/Contents/MacOS/Microsoft Edge",
			"/Applications/Chromium.app/Contents/MacOS/Chromium",
			"/Applications/Brave Browser.app/Contents/MacOS/Brave Browser",
		}
	case "windows":
		candidates = []string{"chrome", "msedge"}
	default:
		candidates = []string{"google-chrome", "chromium", "chromium-browser", "microsoft-edge", "brave-browser"}
	}
	for _, c := range candidates {
		if p, err := exec.LookPath(c); err == nil {
			return p
		}
		if fileExists(c) {
			return c
		}
	}
	return ""
}
