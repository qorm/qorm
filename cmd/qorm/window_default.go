//go:build !desktop

package main

import (
	"fmt"
	"net"

	"github.com/qorm/qorm/internal/server"
)

// launchWindow is a no-op in the default (pure-Go) build: there is no native
// WebView, so the caller falls back to opening a browser. This is what keeps a
// single `go build` cross-compilable to every platform with no C toolchain.
//
// Build with `-tags desktop` for the Wails-style native-WebView window.
func launchWindow(_ *server.Server, _ net.Listener, _, _ string) bool {
	return false
}

// runLogWindow is a no-op without the native WebView (default build); the log
// window only exists in a `-tags desktop` binary.
func runLogWindow(_, _ string) {}

// runMeasure needs the native WebView; only the -tags desktop build can render
// and self-measure the app.
func runMeasure(_, _ string, _ int) error {
	return fmt.Errorf("measure needs a -tags desktop build (native WebView)")
}

func runCheck(_, _, _ string, _ bool, _ int) error {
	return fmt.Errorf("check needs a -tags desktop build (native WebView)")
}

func runPreview(_ string, _ int, _, _ string) error {
	return fmt.Errorf("preview needs a -tags desktop build (native WebView)")
}

func runTray(_, _, _ string) {}
