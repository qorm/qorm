package canvas

// Regression tests for the round-3 red-team review findings F3–F6
// (planning/rendering-engine/reports/redteam/round2-review.md).

import (
	"image"
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/qorm/qorm/internal/loader"
	"github.com/qorm/qorm/internal/model"
	"github.com/qorm/qorm/internal/runtime"
	"github.com/qorm/qorm/internal/theme"
)

// F3: a huge float `columns` (1e19) converts to int platform-dependently
// (arm64 saturates to maxInt64) and overflowed the row-count math
// (make([]int, -1) panic). gridColumns now clamps to [1, maxGridColumns].
func TestGridColumnsClampsExtremeValues(t *testing.T) {
	mk := func(c any) *model.Node { return &model.Node{Type: "grid", Props: map[string]any{"columns": c}} }
	if got := gridColumns(mk(1e19)); got != maxGridColumns {
		t.Errorf("columns=1e19 -> %d, want clamped to %d", got, maxGridColumns)
	}
	if got := gridColumns(mk(-5)); got != 1 {
		t.Errorf("columns=-5 -> %d, want 1", got)
	}
	if got := gridColumns(mk(math.NaN())); got < 1 || got > maxGridColumns {
		t.Errorf("columns=NaN -> %d, want within [1,%d]", got, maxGridColumns)
	}

	// The full layout path must not panic on any platform.
	rt := rtWithDefaultTheme(t)
	g := mk(1e19)
	g.Children = []*model.Node{newButton("a"), newButton("b")}
	ln := Measure(g, rt, &Interaction{}, 1)
	if root := PerformLayout(ln, image.Rect(0, 0, 800, 600), &Interaction{}, rt, 1); root == nil {
		t.Error("layout of a huge-columns grid returned nil root")
	}
}

// F4: a BOUND `disabled` ("{{state.x}}") must evaluate against live state,
// matching the web path where style bindings resolve before styleDisabled
// reads them. Previously canvas read the raw string (never "true"/"1"), so a
// bound-disabled button kept dispatching — a cross-platform semantic fork.
func TestBoundDisabledEvaluatesAgainstState(t *testing.T) {
	e, surf, btn := engineFixture(t)
	btn.Style = map[string]any{"disabled": "{{state.dis}}"}
	btn.OnPress = &model.Invoke{Name: "fire"}
	e.RT.App.Actions = map[string]*model.Action{
		"fire": {ID: "fire", Steps: []model.Step{{Type: "state.set", Path: "fired", Value: "{{ 'yes' }}"}}},
	}
	e.RT.State["dis"] = true

	e.DrawFrame(surf)
	cx, cy := buttonCenter(t, e, btn)
	e.HandlePointer(PointerInput{Type: PointerPress, X: float64(cx), Y: float64(cy)})
	e.HandlePointer(PointerInput{Type: PointerRelease, X: float64(cx), Y: float64(cy)})
	if v := e.RT.State["fired"]; v != nil {
		t.Errorf("bound-disabled button dispatched (fired=%v), want suppressed", v)
	}

	// Flipping the bound state re-enables the same button (per-event evaluation).
	e.RT.State["dis"] = false
	e.HandlePointer(PointerInput{Type: PointerPress, X: float64(cx), Y: float64(cy)})
	e.HandlePointer(PointerInput{Type: PointerRelease, X: float64(cx), Y: float64(cy)})
	if v := e.RT.State["fired"]; v != "yes" {
		t.Errorf("button with disabled bound to false did not dispatch (fired=%v)", v)
	}
}

// F5: a focused node that becomes disabled before activation must not
// dispatch on Enter/Space — disabled is re-checked at dispatch time.
func TestFocusedEnterRechecksDisabled(t *testing.T) {
	e, surf, btn := engineFixture(t)
	btn.OnPress = &model.Invoke{Name: "fire"}
	e.RT.App.Actions = map[string]*model.Action{
		"fire": {ID: "fire", Steps: []model.Step{{Type: "state.set", Path: "fired", Value: "{{ 'yes' }}"}}},
	}

	e.DrawFrame(surf)
	e.HandleKey(KeyInput{Down: true, Key: "tab"})
	if e.Inter.Focused != btn {
		t.Fatalf("tab did not focus the button (focused=%v)", e.Inter.Focused)
	}

	btn.Style = map[string]any{"disabled": true} // disabled while holding focus
	e.HandleKey(KeyInput{Down: true, Key: "return"})
	if v := e.RT.State["fired"]; v != nil {
		t.Errorf("Enter dispatched on a newly-disabled node (fired=%v)", v)
	}

	btn.Style = nil
	e.HandleKey(KeyInput{Down: true, Key: "return"})
	if v := e.RT.State["fired"]; v != "yes" {
		t.Errorf("Enter on re-enabled node did not dispatch (fired=%v)", v)
	}
}

// F6: a skin file whose inner "name" differs from its file name must load
// ONCE per requested name — keying "already active" off rt.Theme.Name made
// every frame re-load the file and re-log (engine.resolveTheme).
func TestThemeInnerNameMismatchLoadsOnce(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "themes"), 0o755); err != nil {
		t.Fatal(err)
	}
	// File foo.json, inner name "bar".
	if err := os.WriteFile(filepath.Join(dir, "themes", "foo.json"),
		[]byte(`{"name":"bar","colors":{"background":"#000000"}}`), 0o644); err != nil {
		t.Fatal(err)
	}

	app := &model.App{Entry: "main", BaseDir: dir, Scenes: map[string]*model.Node{
		"main": {Type: "column", ID: "root"},
	}}
	rt := runtime.New(app)
	rt.Theme = theme.GetDefault()
	rt.State["theme"] = "foo"
	e := NewEngine(rt, SoftwareRenderer{})

	e.resolveTheme()
	first := rt.Theme
	if first == nil || first.Name != "bar" {
		t.Fatalf("theme not loaded from foo.json (theme=%v)", rt.Theme)
	}
	e.resolveTheme()
	if rt.Theme != first {
		t.Error("resolveTheme re-loaded the same requested theme (inner-name mismatch)")
	}

	// A name CHANGE still reloads (the positive cache is keyed by name).
	rt.State["theme"] = "missing"
	e.resolveTheme()
	if rt.Theme != first {
		t.Error("a failed switch must keep the previously loaded theme")
	}
	if e.themeFailed != "missing" {
		t.Errorf("themeFailed = %q, want %q", e.themeFailed, "missing")
	}
}

// N1 (R4): evalStyleProp must recurse into nested style objects — a bound
// margin like margin:{top:"{{…}}"} silently collapsed to 0 in canvas while
// the web path resolves it. This is the binding that moves physics.json's
// moving_block.
func TestNestedMarginBindingEvaluates(t *testing.T) {
	rt := rtWithDefaultTheme(t)
	rt.State["m"] = float64(20)
	n := &model.Node{Type: "button", ID: "b", Props: map[string]any{"label": "Hi"},
		Style: map[string]any{"margin": map[string]any{"top": "{{state.m}}", "left": float64(7)}}}
	ln := Measure(n, rt, &Interaction{}, 1)
	if ln.Style.MarginTop != 20 {
		t.Errorf("bound margin top = %d, want 20", ln.Style.MarginTop)
	}
	if ln.Style.MarginLeft != 7 {
		t.Errorf("static margin left = %d, want 7", ln.Style.MarginLeft)
	}
	// The shared model must not be mutated by evaluation.
	if _, ok := n.Style["margin"].(map[string]any)["top"].(string); !ok {
		t.Error("evalStyleProp mutated the model's margin map")
	}
}

// N2 (R4): a focused node whose `if` condition flips to hidden must not
// dispatch on Enter — focus is re-validated against the visible tree.
func TestFocusedEnterRechecksMounted(t *testing.T) {
	e, surf, btn := engineFixture(t)
	btn.OnPress = &model.Invoke{Name: "fire"}
	btn.Props["if"] = "{{state.show}}"
	e.RT.App.Actions = map[string]*model.Action{
		"fire": {ID: "fire", Steps: []model.Step{{Type: "state.set", Path: "fired", Value: "{{ 'yes' }}"}}},
	}
	e.RT.State["show"] = true

	e.DrawFrame(surf)
	e.HandleKey(KeyInput{Down: true, Key: "tab"})
	if e.Inter.Focused != btn {
		t.Fatalf("tab did not focus the button (focused=%v)", e.Inter.Focused)
	}

	e.RT.State["show"] = false // hidden while holding focus
	e.HandleKey(KeyInput{Down: true, Key: "return"})
	if v := e.RT.State["fired"]; v != nil {
		t.Errorf("Enter dispatched on a hidden node (fired=%v)", v)
	}

	e.RT.State["show"] = true
	e.HandleKey(KeyInput{Down: true, Key: "return"})
	if v := e.RT.State["fired"]; v != "yes" {
		t.Errorf("Enter on re-shown node did not dispatch (fired=%v)", v)
	}
}

// F7 + R5-N1 (scene level): the counter physics demo must converge end to
// end — the moving block's bound margin animates (margin-only target changes
// re-target the tween), the AABB pass sees the overlap, and onCollide writes
// the status. Previously dead at THREE stacked layers: the empty collider
// pair set (only one node carried onCollide), the un-evaluated nested margin
// binding (N1), and the 6-field animation comparison pinning margin (R5-N1).
func TestCounterPhysicsCollisionEndToEnd(t *testing.T) {
	app, err := loader.LoadDir(filepath.Join("..", "..", "..", "examples", "counter"))
	if err != nil {
		t.Fatalf("load counter: %v", err)
	}
	rt := runtime.New(app)
	rt.Theme = theme.GetDefault()
	rt.Navigate("physics", nil)

	e := NewEngine(rt, SoftwareRenderer{})
	surf := NewHeadlessSurface(image.Pt(400, 820))

	// Each increment lowers the moving block onto the target; frames let the
	// tween and the physics pass run. (status starts as "Ready" from the
	// manifest's initial state, so gate on the collision message itself.)
	for i := 0; i < 40 && rt.State["status"] != "COLLISION DETECTED!"; i++ {
		rt.Dispatch("increment", nil)
		e.DrawFrame(surf)
	}
	// Let the final tween settle, then run the physics pass again.
	time.Sleep(300 * time.Millisecond)
	for i := 0; i < 3 && rt.State["status"] != "COLLISION DETECTED!"; i++ {
		e.DrawFrame(surf)
	}

	if got := rt.State["status"]; got != "COLLISION DETECTED!" {
		t.Errorf("status = %v, want %q (physics demo must converge end to end)", got, "COLLISION DETECTED!")
	}
}
