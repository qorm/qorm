package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/qorm/qorm/internal/capability"
	"github.com/qorm/qorm/internal/model"
)

// normPlatform maps a -p value to a support-matrix key.
func normPlatform(p string) string {
	switch p {
	case "macos", "desktop":
		return "mac"
	default:
		return p
	}
}

// usedFeatures walks every scene collecting the platform-specific widget types
// the app actually uses (portable widgets are ignored).
func usedFeatures(app *model.App) []string {
	seen := map[string]bool{}
	var walk func(n *model.Node)
	walk = func(n *model.Node) {
		if n == nil {
			return
		}
		if capability.ForWidget(n.Type) != nil {
			seen[n.Type] = true
		}
		for _, c := range n.Children {
			walk(c)
		}
	}
	for _, root := range app.Scenes {
		walk(root)
	}
	var out []string
	for f := range seen {
		out = append(out, f)
	}
	sort.Strings(out)
	return out
}

// warnPlatformGaps prints a warning for every feature the app uses that the
// target platform doesn't support, so an inconsistency is surfaced at package
// time rather than silently shipping a no-op. Returns the count.
func warnPlatformGaps(app *model.App, platform string) int {
	target := normPlatform(platform)
	var gaps []string
	for _, f := range usedFeatures(app) {
		if !capability.Supported(f, target) {
			gaps = append(gaps, f)
		}
	}
	if len(gaps) == 0 {
		return 0
	}
	fmt.Fprintf(os.Stderr, "\n[warn] platform check — this app uses features not supported on %q:\n", platform)
	for _, f := range gaps {
		on := supportedOn(f)
		line := fmt.Sprintf("    • %-10s works on: %s", f, on)
		if c := capability.ForWidget(f); c != nil && c.Notes != "" {
			line += " — " + c.Notes
		}
		fmt.Fprintln(os.Stderr, line)
	}
	fmt.Fprintf(os.Stderr, "  These widgets will render but their hardware action won't run on %s.\n\n", platform)
	return len(gaps)
}

// supportedOn lists the platforms a feature works on, for the warning.
func supportedOn(feature string) string {
	order := []string{"ios", "android", "mac", "linux", "windows", "web"}
	var on []string
	for _, p := range order {
		if capability.Supported(feature, p) {
			on = append(on, p)
		}
	}
	if len(on) == 0 {
		return "(none)"
	}
	s := on[0]
	for _, p := range on[1:] {
		s += ", " + p
	}
	return s
}

// checkPlatform prints a cross-platform support table for the features this app
// uses (so an iOS-vs-Android-vs-desktop divergence is visible) and then warns
// about gaps on the specific build target.
func checkPlatform(app *model.App, platform string) {
	used := usedFeatures(app)
	if len(used) == 0 {
		return // fully portable app — nothing platform-specific to flag
	}
	cols := []string{"ios", "android", "mac", "linux", "web"}
	fmt.Fprintf(os.Stderr, "\nplatform capability matrix (features this app uses):\n")
	header := fmt.Sprintf("    %-11s", "")
	for _, c := range cols {
		header += fmt.Sprintf("%-8s", c)
	}
	fmt.Fprintln(os.Stderr, header)
	for _, f := range used {
		row := fmt.Sprintf("    %-11s", f)
		for _, c := range cols {
			mark := "·"
			if capability.Supported(f, c) {
				mark = "[ok]"
			}
			row += fmt.Sprintf("%-8s", mark)
		}
		fmt.Fprintln(os.Stderr, row)
	}
	warnPlatformGaps(app, platform)
}

// warnUserNativeGaps flags a missing native/<platform> snippet when the app has
// custom native ops for another platform (so custom ops won't run on this one).
func warnUserNativeGaps(appDir, platform string) {
	nd := filepath.Join(appDir, "native")
	files := map[string]string{"ios": "ios.swift", "android": "android.java"}
	have := map[string]bool{}
	for p, fn := range files {
		if _, err := os.Stat(filepath.Join(nd, fn)); err == nil {
			have[p] = true
		}
	}
	if len(have) == 0 {
		return
	}
	tgt := normPlatform(platform)
	if (tgt == "ios" || tgt == "android") && !have[tgt] {
		var other []string
		for p := range have {
			other = append(other, p)
		}
		fmt.Fprintf(os.Stderr, "[warn] native middle-layer: custom native ops exist for %v but not %q — your qormToNative custom ops won't run on %s (add native/%s).\n", other, tgt, platform, files[tgt])
	}
}
