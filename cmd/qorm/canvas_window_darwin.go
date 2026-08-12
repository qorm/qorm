//go:build !desktop && darwin && !canvaswebview

package main

import (
	"fmt"
	"net"

	"github.com/qorm/qorm/internal/server"
)

// launchWindow hosts the app in the plain native canvas window: the shared
// loop in canvas_host_darwin.go with no platform-view overlays (the webview
// widget draws its placeholder here; -tags canvaswebview layers real
// WKWebViews instead).
func launchWindow(srv *server.Server, ln net.Listener, _, title string) bool {
	return runCanvasWindow(srv, ln, title, nil)
}

func runLogWindow(_, _ string) {}

// runMeasure / runCheck: measure_pure.go (pure-Go canvas headless).

func runPreview(_ string, _ int, _, _ string) error {
	return fmt.Errorf("preview needs a -tags desktop build (native WebView)")
}

func runTray(_, _, _ string) {}
