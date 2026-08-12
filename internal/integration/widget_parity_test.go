package integration

// HTML ↔ canvas widget-type parity harness (W2-F).
//
// The two render paths share one scene JSON vocabulary but wire types
// differently:
//
//   - HTML: the node() switch in internal/render/render.go (also the source of
//     api/widgets.md via TestWidgetCatalogInSync).
//   - Canvas: engine-native leaves/containers (text/button/input/image, flex,
//     scroll, board, …) plus canvas.RegisterWidget names from internal/widgets.
//
// This test does NOT pixel-compare. It fails CI when a *core* type (present in
// the HTML switch AND used by a bundled example scene/component) is missing on
// canvas without an allowlist entry, or when a canvas registration has no HTML
// counterpart without an allowlist entry.
//
// Update allowlists:
//   - Port a widget to canvas → remove it from htmlOnlyCoreAllowlist (the reverse
//     check fails if you leave a stale entry).
//   - Ship HTML-only deliberately → add the type with a one-line reason.
//   - Register a canvas-only custom name in internal/widgets → add it to
//     canvasOnlyAllowlist (app middle-layer RegisterWidget calls are out of scope).
//
// Run: go test ./internal/integration/ -run TestHTMLCanvasWidgetParity -count=1

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/qorm/qorm/internal/loader"
	"github.com/qorm/qorm/internal/model"
	"github.com/qorm/qorm/internal/render/canvas"

	// Side-effect: built-in canvas.RegisterWidget calls.
	_ "github.com/qorm/qorm/internal/widgets"
)

// canvasEngineNativeTypes are scene types the canvas engine handles without
// going through RegisterWidget (measure/layout special cases + generic flex
// containers). Keep in sync with internal/render/canvas measure/layout/scroll.
var canvasEngineNativeTypes = map[string]bool{
	// leaves
	"text": true, "button": true, "input": true, "image": true,
	// repeats / tracks
	"list": true, "gridview": true, "grid": true,
	// layers / viewports / camera
	"stack": true, "absolute": true,
	"scroll": true, "scrollview": true,
	"board": true,
	// control-flow / structure
	"when": true, "timer": true, "slot": true,
	// animated style path (measure.go special-cases both spellings)
	"animated_container": true,
	"animatedcontainer":  true,
	// flex / generic containers (HTML container() case aliases)
	"column": true, "row": true, "columns": true,
	"vstack": true, "hstack": true, "zstack": true,
	"view": true, "box": true, "div": true, "container": true,
	"group": true, "fragment": true, "wrapper": true, "panel": true,
	"body": true, "content": true, "main": true, "section": true,
	"header": true, "footer": true, "aside": true, "nav": true,
	"center": true, "flex": true, "component": true, "wrap": true,
	"start": true, "end": true, "between": true, "around": true,
	"evenly": true, "stretch": true,
}

// htmlOnlyCoreAllowlist: types that appear in the HTML switch and in bundled
// examples but intentionally lack a canvas RegisterWidget / engine-native path
// today (they still lay out as generic boxes). Reason is for humans; the key is
// what the harness checks. Remove an entry when canvas gains real support —
// a stale entry fails the reverse check once the type is canvas-known.
var htmlOnlyCoreAllowlist = map[string]string{
	"actionsheet":      "canvas sheet/bottomsheet only; dedicated action sheet pending",
	"alertdialog":      "dedicated alert dialog pending (modal/alert exist)",
	"autocomplete":     "canvas port pending",
	"carousel":         "canvas port pending",
	"descriptions":     "canvas port pending",
	"dropdownbutton":   "canvas select/dropdown cover the common case",
	"field":            "canvas port pending (input/textarea exist)",
	"materialstepper":  "canvas port pending (steps exists)",
	"monthview":        "canvas port pending",
	"motion":           "canvas port pending (entrance/transition styles cover some cases)",
	"pageview":         "canvas port pending",
	"picker":           "canvas port pending (date/time pickers exist)",
	"rangeslider":      "canvas port pending (slider exists)",
	"rating":           "HTML-only; examples/customwidget registers canvas rating in middle layer",
	"refreshindicator": "canvas port pending",
	"richtext":         "canvas port pending",
	"searchbar":        "canvas port pending",
	"selectabletext":   "canvas port pending",
	"textformfield":    "canvas port pending (input/textarea exist)",
	"transform":        "canvas port pending",
	"tree":             "canvas port pending",
	// Ported to canvas — removed from allowlist:
	// activityindicator, animatedcontainer, aspectratio, circularprogress,
	// ignorepointer, skeleton, tag, fab, switchlisttile.
}

// canvasOnlyAllowlist: RegisterWidget names with no HTML switch case. Empty
// while internal/widgets only registers dual-path types. App middle-layer
// custom widgets (examples/customwidget) are not imported here.
var canvasOnlyAllowlist = map[string]string{}

// TestHTMLCanvasWidgetParity compares HTML-known type names against canvas
// (engine-native + RegisterWidget) and fails on material drift outside the
// documented allowlists.
func TestHTMLCanvasWidgetParity(t *testing.T) {
	htmlTypes := htmlSwitchTypes(t)
	if len(htmlTypes) < 100 {
		t.Fatalf("HTML switch parse returned only %d types — extractor broken?", len(htmlTypes))
	}

	canvasTypes := map[string]bool{}
	for name := range canvasEngineNativeTypes {
		canvasTypes[name] = true
	}
	registered := canvas.RegisteredWidgetNames()
	if len(registered) < 50 {
		t.Fatalf("canvas registry has only %d widgets — was internal/widgets imported?", len(registered))
	}
	for _, name := range registered {
		canvasTypes[name] = true
	}

	exampleTypes := exampleSceneTypes(t)
	if len(exampleTypes) < 50 {
		t.Fatalf("examples contributed only %d scene types — walker broken?", len(exampleTypes))
	}

	// Core = HTML catalog ∩ example scene/component trees.
	var coreMissing []string
	for typ := range exampleTypes {
		if !htmlTypes[typ] {
			continue
		}
		if canvasTypes[typ] {
			continue
		}
		if _, ok := htmlOnlyCoreAllowlist[typ]; ok {
			continue
		}
		coreMissing = append(coreMissing, typ)
	}
	sort.Strings(coreMissing)
	if len(coreMissing) > 0 {
		t.Errorf("core widget types (HTML switch + examples) missing on canvas — implement or add to htmlOnlyCoreAllowlist:\n  %s",
			strings.Join(coreMissing, "\n  "))
	}

	// Canvas registrations must appear in the HTML switch (or allowlist).
	var canvasOnly []string
	for _, name := range registered {
		if htmlTypes[name] {
			continue
		}
		if _, ok := canvasOnlyAllowlist[name]; ok {
			continue
		}
		canvasOnly = append(canvasOnly, name)
	}
	if len(canvasOnly) > 0 {
		t.Errorf("canvas RegisterWidget names with no HTML switch case — add HTML support or canvasOnlyAllowlist:\n  %s",
			strings.Join(canvasOnly, "\n  "))
	}

	// Stale allowlist hygiene: reverse checks.
	for typ, reason := range htmlOnlyCoreAllowlist {
		if reason == "" {
			t.Errorf("htmlOnlyCoreAllowlist[%q] needs a non-empty reason", typ)
		}
		if !htmlTypes[typ] {
			t.Errorf("htmlOnlyCoreAllowlist[%q] is not in the HTML switch — remove stale entry", typ)
		}
		if canvasTypes[typ] {
			t.Errorf("htmlOnlyCoreAllowlist[%q] is now canvas-known — remove from allowlist", typ)
		}
	}
	for typ, reason := range canvasOnlyAllowlist {
		if reason == "" {
			t.Errorf("canvasOnlyAllowlist[%q] needs a non-empty reason", typ)
		}
		if htmlTypes[typ] {
			t.Errorf("canvasOnlyAllowlist[%q] is in the HTML switch — remove from allowlist", typ)
		}
		if !mapHas(registered, typ) {
			t.Errorf("canvasOnlyAllowlist[%q] is not registered — remove stale entry", typ)
		}
	}

	if t.Failed() {
		return
	}

	// Deterministic summary for -v / CI logs.
	var gaps []string
	for typ := range htmlOnlyCoreAllowlist {
		gaps = append(gaps, typ)
	}
	sort.Strings(gaps)
	t.Logf("HTML types=%d canvas(registered=%d + engine-native) example-scene types=%d intentional HTML-only core gaps=%d: %s",
		len(htmlTypes), len(registered), len(exampleTypes), len(gaps), strings.Join(gaps, ", "))
}

func mapHas(names []string, want string) bool {
	for _, n := range names {
		if n == want {
			return true
		}
	}
	return false
}

// htmlSwitchTypes extracts every string label from the HTML renderer node()
// switch (including multi-line case clauses the catalog table builder may miss).
func htmlSwitchTypes(t *testing.T) map[string]bool {
	t.Helper()
	src, err := os.ReadFile(filepath.Join("..", "render", "render.go"))
	if err != nil {
		t.Fatal(err)
	}
	s := string(src)
	fn := strings.Index(s, "func (r *renderer) node(n *model.Node)")
	if fn < 0 {
		t.Fatal("could not locate renderer.node")
	}
	sw := strings.Index(s[fn:], "switch n.Type {")
	if sw < 0 {
		t.Fatal("could not locate switch n.Type")
	}
	start := fn + sw
	endRel := strings.Index(s[start:], "\n\tdefault:\n\t\tr.unknown(n)")
	if endRel < 0 {
		t.Fatal("could not locate default: r.unknown(n)")
	}
	body := s[start : start+endRel]
	labelRe := regexp.MustCompile(`"([a-z0-9_]+)"`)
	out := map[string]bool{}
	for _, m := range labelRe.FindAllStringSubmatch(body, -1) {
		out[m[1]] = true
	}
	return out
}

// exampleSceneTypes walks every bundled example's loaded scenes and components
// and returns the set of node.Type values (action steps are excluded).
func exampleSceneTypes(t *testing.T) map[string]bool {
	t.Helper()
	root := examplesDir(t, "")
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]bool{}
	var walk func(*model.Node)
	walk = func(n *model.Node) {
		if n == nil {
			return
		}
		if n.Type != "" {
			out[n.Type] = true
		}
		for _, c := range n.Children {
			walk(c)
		}
		walk(n.Template)
		walk(n.Then)
		walk(n.Else)
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		dir := filepath.Join(root, e.Name())
		if _, err := os.Stat(filepath.Join(dir, "qorm.json")); err != nil {
			continue
		}
		app, err := loader.LoadDir(dir)
		if err != nil {
			// A broken example is someone else's gate; skip so this harness
			// stays about type parity, not load errors.
			t.Logf("skip example %s: load: %v", e.Name(), err)
			continue
		}
		for _, sc := range app.Scenes {
			walk(sc)
		}
		for _, comp := range app.Components {
			walk(comp)
		}
	}
	return out
}
