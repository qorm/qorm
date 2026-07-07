//go:build !(darwin && desktop)

package main

import (
	"fmt"
	"os"
)

// cmdShot renders a QORM app to a PNG via the WebKit WebView; that path only
// exists in a macOS build with -tags desktop.
func cmdShot(args []string) int {
	fmt.Fprintln(os.Stderr, "qorm shot needs a macOS build with -tags desktop (WebKit render)")
	return 2
}
