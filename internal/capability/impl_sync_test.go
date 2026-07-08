package capability

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// This file guards the registry's Platforms declarations against the actual
// implementations, in both directions:
//
//   - declared but not implemented (a stub or a missing switch case) → FAIL
//   - implemented but not declared → FAIL
//
// Desktop (mac/linux/windows) is checked strongly: the per-OS op switches in
// cmd/qorm/hardware_desktop.go are extracted from the AST, and ops that route
// to native* functions instead are pinned in nativeRouted below. iOS / Android
// / Web are checked weakly: the generated bridge templates must at least
// mention every declared op by name.

// nativeRouted pins, per op handled by desktopHardware's top-level switch
// (routing to native* functions rather than the per-OS switches), which
// desktop platforms actually have a non-stub implementation. When you
// implement one of these on a new platform (or add a new native-routed op),
// update this table — the test failing here is the reminder to also update
// the registry, docs, and support matrix.
var nativeRouted = map[string]map[string]bool{
	"notify":         {Mac: true, Linux: true, Windows: true}, // desktopNotify: nativeNotify / notify-send / NotifyIcon balloon
	"badge":          {Mac: true},                             // setDockBadge is a stub off darwin
	"loginItem":      {Mac: true},                             // SMAppService
	"loginItemGet":   {Mac: true},
	"screens":        {Mac: true}, // screenInfo returns "[]" off darwin
	"biometric":      {Mac: true}, // Touch ID
	"wifiInfo":       {Mac: true}, // CoreWLAN
	"getModes":       {Mac: true}, // nativeSystemModes returns "{}" off darwin
	"bluetoothScan":  {Mac: true}, // CoreBluetooth
	"bluetoothState": {Mac: true},
}

// desktopPlatforms maps registry platform keys to the per-OS switch functions
// in hardware_desktop.go.
var desktopPlatforms = map[string]string{
	Mac:     "desktopHardwareDarwin",
	Linux:   "desktopHardwareLinux",
	Windows: "desktopHardwareWindows",
}

// gracefulShims are per-OS cases that exist only to resolve the UI on hardware
// the platform doesn't have (torch reports OFF, vibrate is a no-op). They are
// NOT implementations, so the registry rightly does not declare them and the
// reverse check skips them.
var gracefulShims = map[string]map[string]bool{
	Mac:   {"torchGet": true, "torchToggle": true, "vibrate": true, "haptic": true},
	Linux: {"torchGet": true, "torchToggle": true, "vibrate": true, "haptic": true},
}

// knownTemplateGaps are op-level holes in the mobile/web bridge templates for
// capabilities that are otherwise supported on that platform (tracked for
// v0.2.x). Remove an entry when the op is implemented — a stale entry makes
// the weak check fail in reverse.
var knownTemplateGaps = map[string]map[string]bool{
	Android: {"volumeSet": true, "brightnessSet": true},
	Web:     {"clipboardGet": true, "listenStop": true, "speakStop": true, "headingStop": true},
}

// parseFile parses one cmd/qorm source file (single-file parse, so build tags
// don't matter).
func parseFile(t *testing.T, name string) *ast.File {
	t.Helper()
	path := filepath.Join("..", "..", "cmd", "qorm", name)
	f, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
	if err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	return f
}

// caseStrings extracts every string literal used in a case clause anywhere
// inside the named function.
func caseStrings(f *ast.File, fn string) map[string]bool {
	ops := map[string]bool{}
	for _, d := range f.Decls {
		fd, ok := d.(*ast.FuncDecl)
		if !ok || fd.Name.Name != fn {
			continue
		}
		ast.Inspect(fd, func(n ast.Node) bool {
			cc, ok := n.(*ast.CaseClause)
			if !ok {
				return true
			}
			for _, e := range cc.List {
				if lit, ok := e.(*ast.BasicLit); ok && lit.Kind == token.STRING {
					if s, err := strconv.Unquote(lit.Value); err == nil {
						ops[s] = true
					}
				}
			}
			return true
		})
	}
	return ops
}

// builtinOps extracts the desktopBuiltins map keys from window_desktop.go —
// the set of ops the built-in desktop bridge claims to handle.
func builtinOps(t *testing.T) map[string]bool {
	t.Helper()
	f := parseFile(t, "window_desktop.go")
	ops := map[string]bool{}
	ast.Inspect(f, func(n ast.Node) bool {
		vs, ok := n.(*ast.ValueSpec)
		if !ok {
			return true
		}
		for i, name := range vs.Names {
			if name.Name != "desktopBuiltins" || i >= len(vs.Values) {
				continue
			}
			if cl, ok := vs.Values[i].(*ast.CompositeLit); ok {
				for _, elt := range cl.Elts {
					if kv, ok := elt.(*ast.KeyValueExpr); ok {
						if lit, ok := kv.Key.(*ast.BasicLit); ok && lit.Kind == token.STRING {
							if s, err := strconv.Unquote(lit.Value); err == nil {
								ops[s] = true
							}
						}
					}
				}
			}
		}
		return true
	})
	if len(ops) == 0 {
		t.Fatal("desktopBuiltins not found in window_desktop.go — did it move?")
	}
	return ops
}

// TestDesktopImplMatchesRegistry checks declared-vs-implemented both ways for
// the desktop platforms.
func TestDesktopImplMatchesRegistry(t *testing.T) {
	hw := parseFile(t, "hardware_desktop.go")
	builtins := builtinOps(t)
	impl := map[string]map[string]bool{} // platform → op → true
	for p, fn := range desktopPlatforms {
		impl[p] = caseStrings(hw, fn)
	}
	topLevel := caseStrings(hw, "desktopHardware") // native-routed + platform

	// op → owning capability, for the reverse direction.
	opOwner := map[string]*Cap{}
	for i := range All {
		for _, op := range All[i].Ops {
			opOwner[op] = &All[i]
		}
	}

	declared := func(c *Cap, p string) bool {
		for _, dp := range c.Platforms {
			if dp == p {
				return true
			}
		}
		return false
	}

	// Forward: every declared desktop op must have an implementation.
	for i := range All {
		c := &All[i]
		for p := range desktopPlatforms {
			if !declared(c, p) {
				continue
			}
			for _, op := range c.Ops {
				if !builtins[op] {
					continue // web-layer op (getUserMedia, geolocation, …), not bridged
				}
				if impl[p][op] || nativeRouted[op][p] {
					continue
				}
				t.Errorf("capability %q declares %s but op %q has no %s implementation "+
					"(no case in %s, not in nativeRouted)",
					c.Stem, p, op, p, desktopPlatforms[p])
			}
		}
	}

	// Reverse: every implemented op must be declared.
	for p := range desktopPlatforms {
		for op := range impl[p] {
			c := opOwner[op]
			if c == nil {
				continue // not a registry op (platform, winDrag*, …)
			}
			if gracefulShims[p][op] {
				continue // UI-resolving shim, not an implementation
			}
			if !declared(c, p) {
				t.Errorf("op %q is implemented in %s but capability %q does not declare %s",
					op, desktopPlatforms[p], c.Stem, p)
			}
		}
	}
	for op, plats := range nativeRouted {
		c := opOwner[op]
		if c == nil {
			continue
		}
		if !topLevel[op] {
			t.Errorf("nativeRouted op %q is not handled by desktopHardware's top-level switch — table is stale", op)
		}
		for p, ok := range plats {
			if ok && !declared(c, p) {
				t.Errorf("op %q is native-routed with a %s implementation but capability %q does not declare %s",
					op, p, c.Stem, p)
			}
		}
	}
}

// TestMobileWebTemplatesMentionDeclaredOps weakly checks the generated bridge
// templates: a declared op whose name never appears in the platform's template
// was forgotten there.
func TestMobileWebTemplatesMentionDeclaredOps(t *testing.T) {
	read := func(rel string) string {
		b, err := os.ReadFile(filepath.Join("..", "..", rel))
		if err != nil {
			t.Fatalf("read %s: %v", rel, err)
		}
		return string(b)
	}
	sources := map[string]string{
		IOS:     read("cmd/qorm/packageios.go"),
		Android: read("cmd/qorm/packageandroid.go"),
		Web:     read("internal/server/app.js"),
	}
	for i := range All {
		c := &All[i]
		for _, p := range c.Platforms {
			src, ok := sources[p]
			if !ok {
				continue
			}
			for _, op := range c.Ops {
				if knownTemplateGaps[p][op] {
					if strings.Contains(src, op) {
						t.Errorf("op %q is now in the %s template — remove it from knownTemplateGaps", op, p)
					}
					continue
				}
				if !strings.Contains(src, op) {
					t.Errorf("capability %q declares %s but op %q never appears in that platform's template",
						c.Stem, p, op)
				}
			}
		}
	}
}
