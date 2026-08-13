//go:build !(darwin && desktop)

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
)

// cmdShot renders an app with the pure-Go canvas engine. WebKit page/URL and
// operating-system window capture remain exclusive to darwin+desktop builds.
func cmdShot(args []string) int {
	in, out, width, height := "", "", 440, 720
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "-o", "--out", "--width", "--height":
			flag := args[i]
			if i+1 >= len(args) {
				fmt.Fprintf(os.Stderr, "error: %s requires a value\n", flag)
				return 2
			}
			i++
			if flag == "-o" || flag == "--out" {
				out = args[i]
				continue
			}
			n, err := strconv.Atoi(args[i])
			if err != nil {
				fmt.Fprintf(os.Stderr, "error: %s must be an integer\n", flag)
				return 2
			}
			if flag == "--width" {
				width = n
			} else {
				height = n
			}
		case "--html", "--url", "--live":
			fmt.Fprintf(os.Stderr, "error: %s capture requires macOS with -tags desktop; app-directory capture uses pure Canvas in this build\n", args[i])
			return 2
		default:
			if len(args[i]) > 0 && args[i][0] == '-' {
				fmt.Fprintf(os.Stderr, "error: unknown qorm shot option %q\n", args[i])
				return 2
			}
			if in != "" {
				fmt.Fprintln(os.Stderr, "error: qorm shot accepts one app directory")
				return 2
			}
			in = args[i]
		}
	}
	if in == "" {
		fmt.Fprintln(os.Stderr, "usage: qorm shot <app-dir> [-o out.png] [--width W --height H]")
		return 2
	}
	if out == "" {
		base := filepath.Base(filepath.Clean(in))
		if base == "." || base == string(filepath.Separator) || base == "" {
			base = "shot"
		}
		out = base + ".png"
	}
	if err := renderCanvasPNG(in, width, height, out); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}
	fmt.Printf("wrote %s (%dx%d, pure Canvas)\n", out, width, height)
	return 0
}
