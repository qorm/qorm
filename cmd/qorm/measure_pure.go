//go:build !desktop

package main

import (
	"fmt"
	"os"

	"github.com/qorm/qorm/internal/measure"
	"github.com/qorm/qorm/internal/render/canvas"
)

// measureRowsCanvas lays out the app with the pure-Go canvas engine (no
// WebView) and returns HTML-compatible measurement rows from the graph.
// This is the default-build path for `qorm measure` / `qorm check` so agents
// can verify pure-Go canvas reality, not only the desktop WebView path.
func measureRowsCanvas(appDir string, width int) (measured []byte, err error) {
	rt, err := loadRuntime(appDir, "", "")
	if err != nil {
		return nil, err
	}
	h := 820
	if rt.App != nil {
		if rt.App.Window.Height > 0 {
			h = rt.App.Window.Height
		}
		if width <= 0 && rt.App.Window.Width > 0 {
			width = rt.App.Window.Width
		}
	}
	if width <= 0 {
		width = 400
	}
	return canvas.MeasureScene(rt, width, h, 1), nil
}

// runMeasure prints the intent+result report from the pure-Go canvas layout.
func runMeasure(appDir, out string, width int) error {
	measured, err := measureRowsCanvas(appDir, width)
	if err != nil {
		return err
	}
	rt, err := loadRuntime(appDir, "", "")
	if err != nil {
		return err
	}
	report, err := measure.Report(rt, measured)
	if err != nil {
		return err
	}
	if out != "" {
		if err := os.WriteFile(out, report, 0o644); err != nil {
			return err
		}
		fmt.Fprintf(os.Stderr, "measured (canvas) %s -> %s\n", appDir, out)
		return nil
	}
	fmt.Println(string(report))
	return nil
}

// runCheck evaluates checks (or audit) against pure-Go canvas measurements.
func runCheck(appDir, checksPath, out string, audit bool, width int) error {
	measured, err := measureRowsCanvas(appDir, width)
	if err != nil {
		return err
	}
	rt, err := loadRuntime(appDir, "", "")
	if err != nil {
		return err
	}
	var report []byte
	if audit {
		report, err = measure.Audit(rt, measured)
	} else {
		checks, rerr := os.ReadFile(checksPath)
		if rerr != nil {
			return rerr
		}
		// Multi-step interaction flows still need a live window; pure measure
		// only supports static check lists (same as a single-shot eval).
		if isFlow(checks) {
			return fmt.Errorf("interactive check flows need -tags desktop (WebView); static checks work on pure-Go canvas")
		}
		report, err = measure.Eval(rt, measured, checks)
	}
	if err != nil {
		return err
	}
	if out != "" {
		if err := os.WriteFile(out, report, 0o644); err != nil {
			return err
		}
		fmt.Fprintf(os.Stderr, "checked (canvas) %s -> %s\n", appDir, out)
		return nil
	}
	fmt.Println(string(report))
	return nil
}
