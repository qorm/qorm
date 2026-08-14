package canvas

// Regression tests for the round-3 red-team review findings F3–F6
// (planning/rendering-engine/reports/redteam/round2-review.md).

import (
	"image"
	"image/color"
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/qorm/platform/internal/loader"
	"github.com/qorm/platform/internal/model"
	"github.com/qorm/platform/internal/runtime"
	"github.com/qorm/platform/internal/theme"
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

// A skin file on disk is authoritative over the built-in default palette even
// when the default already carries the same name — otherwise the first frame
// renders the built-in apple-light while a re-toggle renders the JSON
// apple-light (which ships extra component styles), and the two visibly
// differ (user report).
func TestThemeFileWinsOverBuiltinAdopt(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "themes"), 0o755); err != nil {
		t.Fatal(err)
	}
	// A file named apple-light.json with a distinguishing component style the
	// built-in default lacks (boxShadowBlur on button).
	skin := `{"name":"apple-light","colors":{"background":"#F5F5F7"},` +
		`"components":{"button":{"boxShadowBlur":99}}}`
	if err := os.WriteFile(filepath.Join(dir, "themes", "apple-light.json"), []byte(skin), 0o644); err != nil {
		t.Fatal(err)
	}

	app := &model.App{Entry: "main", BaseDir: dir, Scenes: map[string]*model.Node{
		"main": {Type: "column", ID: "root"},
	}}
	rt := runtime.New(app)
	rt.Theme = theme.GetDefault() // name is already "apple-light"
	rt.State["theme"] = "apple-light"
	e := NewEngine(rt, SoftwareRenderer{})

	e.resolveTheme()
	btn, ok := rt.Theme.Components["button"]
	if !ok || btn.BoxShadowBlur == nil || *btn.BoxShadowBlur != 99 {
		t.Errorf("theme = builtin adopt (no button shadow), want the disk JSON to win: %+v", rt.Theme.Components["button"])
	}
	if e.themeLoaded != "apple-light" {
		t.Errorf("themeLoaded = %q, want %q", e.themeLoaded, "apple-light")
	}
}

// Built-in theme aliases mirror the HTML shell (render/theme.go
// ThemeVarsFor): "apple"/"auto" resolve as apple-light, "dark" as
// apple-dark, "material" as win11-light — through the normal probe, with no
// FAILED log noise (examples ship theme:"apple" widely; user report).
func TestThemeAliasesResolveThroughNormalProbe(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "themes"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "themes", "apple-dark.json"),
		[]byte(`{"name":"apple-dark","colors":{"background":"#000000"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	app := &model.App{Entry: "main", BaseDir: dir, Scenes: map[string]*model.Node{
		"main": {Type: "column", ID: "root"},
	}}
	rt := runtime.New(app)
	rt.Theme = theme.GetDefault()
	e := NewEngine(rt, SoftwareRenderer{})

	// "apple" resolves as apple-light: the built-in default is already active,
	// and the adopt must be silent (no themeFailed, no FAILED log).
	rt.State["theme"] = "apple"
	e.resolveTheme()
	if rt.Theme.Name != "apple-light" {
		t.Errorf(`theme:"apple" resolved to %q, want apple-light`, rt.Theme.Name)
	}
	if e.themeFailed != "" {
		t.Errorf(`theme:"apple" produced themeFailed %q — aliases must not fail`, e.themeFailed)
	}

	// "dark" resolves as apple-dark through the disk probe.
	rt.State["theme"] = "dark"
	e.resolveTheme()
	if rt.Theme.Name != "apple-dark" {
		t.Errorf(`theme:"dark" resolved to %q, want apple-dark`, rt.Theme.Name)
	}
	if e.themeFailed != "" {
		t.Errorf(`theme:"dark" produced themeFailed %q`, e.themeFailed)
	}

	// "auto" resolves as apple-light (OS tracking is an HTML-shell feature).
	rt.State["theme"] = "auto"
	e.resolveTheme()
	if rt.Theme.Name != "apple-light" {
		t.Errorf(`theme:"auto" resolved to %q, want apple-light`, rt.Theme.Name)
	}
}

// designTokens render as var(--qorm-token-<name>) and the HTML shell's theme
// variables alias onto canvas theme tokens — scenes authored for either
// source keep their colors (gallery's magenta background was the user-
// visible symptom).
func TestResolveColorDesignTokensAndAliases(t *testing.T) {
	rt := rtWithDefaultTheme(t)
	rt.App.DesignTokens = map[string]model.DesignToken{
		"color.bg":      {Type: "color", Value: "#f2f2f7"},
		"color.primary": {Type: "color", Value: "#0a84ff"},
		"spacing.md":    {Type: "number", Value: "16"},
	}

	if got := resolveColor("var(--qorm-token-color-bg)", rt); got != (color.RGBA{0xf2, 0xf2, 0xf7, 255}) {
		t.Errorf("designToken bg = %v", got)
	}
	if got := resolveColor("var(--qorm-token-color-primary)", rt); got != (color.RGBA{0x0a, 0x84, 0xff, 255}) {
		t.Errorf("designToken primary = %v", got)
	}
	// Number tokens are not colors: they must NOT resolve as one.
	if got := resolveColor("var(--qorm-token-spacing-md)", rt); got == (color.RGBA{0xf2, 0xf2, 0xf7, 255}) {
		t.Error("a number token must not resolve as a color")
	}
	// HTML theme-var aliases (apple-light default theme values).
	if got := resolveColor("var(--label)", rt); got != (color.RGBA{0x1d, 0x1d, 0x1f, 255}) {
		t.Errorf("--label alias = %v, want text %v", got, color.RGBA{0x1d, 0x1d, 0x1f, 255})
	}
	if got := resolveColor("var(--label2)", rt); got != (color.RGBA{0x86, 0x86, 0x8b, 255}) {
		t.Errorf("--label2 alias = %v, want textSecondary", got)
	}
	if got := resolveColor("var(--fill)", rt); got != (color.RGBA{0xe8, 0xe8, 0xed, 255}) {
		t.Errorf("--fill alias = %v, want inputBg", got)
	}
}
