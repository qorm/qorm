//go:build !desktop

package main

import (
	"fmt"
	"os"

	"github.com/qorm/platform/internal/measure"
	"github.com/qorm/platform/internal/render/canvas"
)

// measureRowsCanvas lays out the app with the pure-Go canvas engine (no
// WebView) and returns HTML-compatible measurement rows from the graph.
// This is the default-build path for `qorm measure` / `qorm check` so agents
// can verify pure-Go canvas reality, not only the desktop WebView path.
// logical=true reports CSS px (default); false keeps physical device px.
func measureRowsCanvas(appDir string, width int, logical bool) (measured []byte, err error) {
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
	return canvas.MeasureSceneOpts(rt, width, h, 1, canvas.MeasureOpts{Logical: logical}), nil
}

// runMeasure prints the intent+result report from the pure-Go canvas layout.
// physical=true keeps device pixels; default is logical CSS px.
func runMeasure(appDir, out string, width int, physical bool) error {
	measured, err := measureRowsCanvas(appDir, width, !physical)
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
func runCheck(appDir, checksPath, out string, audit bool, width int, physical bool) error {
	var checks []byte
	if !audit {
		var err error
		checks, err = os.ReadFile(checksPath)
		if err != nil {
			return err
		}
		if isFlow(checks) {
			rt, err := loadRuntime(appDir, "", "")
			if err != nil {
				return err
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
			report, err := evalCanvasFlow(rt, width, h, !physical, checks)
			if err != nil {
				return err
			}
			return writeCanvasCheckReport(appDir, out, report)
		}
	}
	measured, err := measureRowsCanvas(appDir, width, !physical)
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
		report, err = measure.Eval(rt, measured, checks)
	}
	if err != nil {
		return err
	}
	return writeCanvasCheckReport(appDir, out, report)
}

func writeCanvasCheckReport(appDir, out string, report []byte) error {
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
