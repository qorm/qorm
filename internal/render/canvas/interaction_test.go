package canvas

import (
	"image"
	"testing"

	"github.com/qorm/platform/internal/loader"
	"github.com/qorm/platform/internal/model"
	"github.com/qorm/platform/internal/render/graph"
	"github.com/qorm/platform/internal/runtime"
	"github.com/qorm/platform/internal/theme"
)

func rtWithDefaultTheme(t *testing.T) *runtime.Runtime {
	t.Helper()
	app := &model.App{Entry: "main", Scenes: map[string]*model.Node{
		"main": {Type: "column", ID: "root"},
	}}
	rt := runtime.New(app)
	rt.Theme = theme.GetDefault()
	return rt
}

func newButton(id string) *model.Node {
	return &model.Node{Type: "button", ID: id, Props: map[string]any{"label": "Hi"}}
}

func TestPressedOverlayFromTheme(t *testing.T) {
	rt := rtWithDefaultTheme(t)
	btn := newButton("b")

	ln := Measure(btn, rt, &Interaction{Pressed: btn}, 1)
	if want := parseColor("#0062CC"); ln.Style.Background != want {
		t.Errorf("pressed background = %v, want theme pressedBackgroundColor %v", ln.Style.Background, want)
	}
	if ln.Style.Opacity != 0.9 {
		t.Errorf("pressed opacity = %v, want 0.9 from theme pressedOpacity", ln.Style.Opacity)
	}
}

func TestHoveredOverlayFromTheme(t *testing.T) {
	rt := rtWithDefaultTheme(t)
	btn := newButton("b")

	ln := Measure(btn, rt, &Interaction{Hovered: btn}, 1)
	if want := parseColor("#1A86FF"); ln.Style.Background != want {
		t.Errorf("hovered background = %v, want theme hoveredBackgroundColor %v", ln.Style.Background, want)
	}
	if ln.Style.Opacity != 1 {
		t.Errorf("hover must not change opacity, got %v", ln.Style.Opacity)
	}
}

func TestPressedWinsOverHovered(t *testing.T) {
	rt := rtWithDefaultTheme(t)
	btn := newButton("b")

	ln := Measure(btn, rt, &Interaction{Pressed: btn, Hovered: btn}, 1)
	if want := parseColor("#0062CC"); ln.Style.Background != want {
		t.Errorf("pressed+hovered background = %v, want pressed to win (%v)", ln.Style.Background, want)
	}
}

func TestNoInteractionLeavesStyleAlone(t *testing.T) {
	rt := rtWithDefaultTheme(t)
	btn := newButton("b")

	plain := Measure(btn, rt, nil, 1)
	withEmpty := Measure(btn, rt, &Interaction{}, 1)
	if plain.Style.Background != withEmpty.Style.Background || plain.Style.Opacity != withEmpty.Style.Opacity {
		t.Error("empty interaction state must not alter the resolved style")
	}
	if want := parseColor("#007AFF"); plain.Style.Background != want {
		t.Errorf("button default background = %v, want theme primary %v", plain.Style.Background, want)
	}
}

func TestQSSPseudoStateCascadeAndFocusBorder(t *testing.T) {
	btn := newButton("b")
	btn.Props["class"] = "probe"
	rt := rtWithDefaultTheme(t)
	rt.App.Styles = []model.StyleRule{
		{Kind: model.StyleRuleClass, Name: "probe", Style: map[string]any{
			"hoverBackground": "#112233", "hoverColor": "#abcdef", "hoverOpacity": 0.4,
			"pressedBackground": "#334455", "pressedOpacity": 0.3,
			"focusBorderColor": "#fedcba",
		}},
	}
	btn.Style = map[string]any{"hoverOpacity": 0.6} // inline beats class

	hover := Measure(btn, rt, &Interaction{Hovered: btn}, 1)
	if hover.Style.Background != parseColor("#112233") || hover.Style.Color != parseColor("#abcdef") || hover.Style.Opacity != 0.6 {
		t.Fatalf("QSS hover cascade = bg %v color %v opacity %v", hover.Style.Background, hover.Style.Color, hover.Style.Opacity)
	}
	pressed := Measure(btn, rt, &Interaction{Pressed: btn, Hovered: btn}, 1)
	if pressed.Style.Background != parseColor("#334455") || pressed.Style.Opacity != 0.3 {
		t.Fatalf("pressed must beat hover: bg %v opacity %v", pressed.Style.Background, pressed.Style.Opacity)
	}
	inter := &Interaction{Focused: btn, FocusVisible: true}
	g := PerformLayout(Measure(btn, rt, inter, 1), image.Rect(0, 0, 200, 100), inter, rt, 1).(*graph.Group)
	var ring *graph.Rect
	for _, c := range g.Children {
		if r, ok := c.(*graph.Rect); ok && r.NoHit {
			ring = r
		}
	}
	if ring == nil || ring.Stroke != parseColor("#fedcba") {
		t.Fatalf("focus ring = %#v, want QSS focusBorderColor", ring)
	}
}

func TestPerformLayoutStampsInteractionState(t *testing.T) {
	rt := rtWithDefaultTheme(t)
	btn := newButton("b")
	inter := &Interaction{Pressed: btn, Hovered: btn, Focused: btn}

	ln := Measure(btn, rt, inter, 1)
	g, ok := PerformLayout(ln, image.Rect(0, 0, 200, 100), inter, rt, 1).(*graph.Group)
	if !ok {
		t.Fatal("PerformLayout must return a group for a button")
	}
	if g.Model != btn {
		t.Error("group.Model back-reference must point at the model node")
	}
	if !g.Pressed || !g.Hovered || !g.Focused {
		t.Errorf("state flags not stamped: pressed=%v hovered=%v focused=%v", g.Pressed, g.Hovered, g.Focused)
	}
}

func TestFocusRingOnlyWhenKeyboardVisible(t *testing.T) {
	rt := rtWithDefaultTheme(t)
	bounds := image.Rect(0, 0, 200, 100)

	findRing := func(btn *model.Node, inter *Interaction) *graph.Rect {
		ln := Measure(btn, rt, inter, 1)
		g := PerformLayout(ln, bounds, inter, rt, 1).(*graph.Group)
		for _, c := range g.Children {
			if c.Base().NoHit {
				r, ok := c.(*graph.Rect)
				if !ok {
					t.Fatalf("NoHit child is %T, want *graph.Rect", c)
				}
				return r
			}
		}
		return nil
	}

	btn := newButton("b")

	// Keyboard focus → ring.
	ring := findRing(btn, &Interaction{Focused: btn, FocusVisible: true})
	if ring == nil {
		t.Fatal("keyboard-visible focus must draw a ring")
	}
	if ring.StrokeWidth != 2 {
		t.Errorf("ring stroke width = %v, want 2", ring.StrokeWidth)
	}
	if want := resolveFocusColor(rt); ring.Stroke != want {
		t.Errorf("ring stroke = %v, want theme focus color %v", ring.Stroke, want)
	}

	// Pointer focus (FocusVisible=false) → no ring.
	if r := findRing(btn, &Interaction{Focused: btn}); r != nil {
		t.Error("pointer-driven focus must NOT draw a ring (focus-visible semantics)")
	}
	// No focus → no ring.
	if r := findRing(btn, &Interaction{}); r != nil {
		t.Error("no focus must not draw a ring")
	}
}

// Regression (red-team R1 P0-1): pressing a region with NO pressable ancestor
// walked VisualTarget/ModelOf past the root group — the root's Parent is a
// typed nil *graph.Group, which is non-nil once stored in the graph.Node
// interface, so the next iteration dereferenced nil (SIGSEGV in the native
// window). Both walks must stop at the root.
func TestPressWithoutPressableAncestorDoesNotPanic(t *testing.T) {
	root := &model.Node{Type: "column", ID: "root", Children: []*model.Node{
		{Type: "text", ID: "t1", Props: map[string]any{"text": "plain"}},
	}}
	app := &model.App{Entry: "main", Scenes: map[string]*model.Node{"main": root}}
	rt := runtime.New(app)
	rt.Theme = theme.GetDefault()
	e := NewEngine(rt, SoftwareRenderer{})
	surf := NewHeadlessSurface(image.Pt(400, 400))
	e.DrawFrame(surf)

	// On the text, on the column padding, in a far corner: nothing here has a
	// pressable ancestor, so every walk climbs to the root group.
	for _, pt := range [][2]float64{{20, 20}, {200, 200}, {399, 399}} {
		e.HandlePointer(PointerInput{Type: PointerPress, X: pt[0], Y: pt[1]})
		if e.Inter.Pressed != nil {
			t.Fatalf("press at %v with no pressable ancestor set Pressed=%v", pt, e.Inter.Pressed)
		}
		e.HandlePointer(PointerInput{Type: PointerRelease, X: pt[0], Y: pt[1]})
		// The hover path (ModelOf walk) had the same latent typed-nil hazard.
		e.HandlePointer(PointerInput{Type: PointerMove, X: pt[0], Y: pt[1]})
	}
}

// End-to-end regression for the exact R1 reproduction: the real
// examples/counter scene rendered headless, then clicked on blank background
// at (10,810) — previously a nil-pointer SIGSEGV at engine.HandlePointer.
func TestCounterExampleBlankBackgroundClickDoesNotPanic(t *testing.T) {
	app, err := loader.LoadDir("../../../examples/counter")
	if err != nil {
		t.Fatalf("load counter example: %v", err)
	}
	rt := runtime.New(app)
	rt.Theme = theme.GetDefault()
	e := NewEngine(rt, SoftwareRenderer{})
	surf := NewHeadlessSurface(image.Pt(400, 820))
	e.DrawFrame(surf)

	e.HandlePointer(PointerInput{Type: PointerPress, X: 10, Y: 810})
	e.HandlePointer(PointerInput{Type: PointerRelease, X: 10, Y: 810})
}

// Regression (red-team R1 P1-3): a button whose style marks it disabled must
// be transparent to activation — the web renderer gives it pointer-events:none
// and never wires a handler, so canvas must match: no pressed state, no
// dispatch.
func TestDisabledButtonDoesNotDispatch(t *testing.T) {
	e, surf, btn := engineFixture(t)
	btn.Style = map[string]any{"disabled": true}
	e.RT.App.Actions = map[string]*model.Action{
		"fire": {ID: "fire", Steps: []model.Step{{Type: "state.set", Path: "fired", Value: "{{ 'yes' }}"}}},
	}
	btn.OnPress = &model.Invoke{Name: "fire"}

	e.DrawFrame(surf)
	cx, cy := buttonCenter(t, e, btn)
	e.HandlePointer(PointerInput{Type: PointerPress, X: float64(cx), Y: float64(cy)})
	e.HandlePointer(PointerInput{Type: PointerRelease, X: float64(cx), Y: float64(cy)})

	if e.Inter.Pressed != nil {
		t.Error("disabled button must not take the pressed state")
	}
	if v := e.RT.State["fired"]; v != nil {
		t.Errorf("disabled button dispatched its action (fired=%v), want suppressed", v)
	}

	// Sanity: the same button WITHOUT disabled fires (the fixture works).
	btn.Style = nil
	e.HandlePointer(PointerInput{Type: PointerPress, X: float64(cx), Y: float64(cy)})
	e.HandlePointer(PointerInput{Type: PointerRelease, X: float64(cx), Y: float64(cy)})
	if v := e.RT.State["fired"]; v != "yes" {
		t.Errorf("enabled button did not dispatch (fired=%v), want \"yes\"", v)
	}
}

func TestQSSDisabledBindingBlocksInteractionAndDims(t *testing.T) {
	e, surf, btn := engineFixture(t)
	btn.Props["class"] = "disabled-probe"
	e.RT.App.Styles = []model.StyleRule{{Kind: model.StyleRuleClass, Name: "disabled-probe", Style: map[string]any{
		"disabled": "{{state.locked}}", "disabledOpacity": 0.25,
	}}}
	e.RT.State["locked"] = true
	e.RT.App.Actions = map[string]*model.Action{
		"fire": {ID: "fire", Steps: []model.Step{{Type: "state.set", Path: "fired", Value: "{{ 'yes' }}"}}},
	}
	btn.OnPress = &model.Invoke{Name: "fire"}

	ln := Measure(btn, e.RT, nil, 1)
	if ln.Style.Opacity != 0.25 {
		t.Fatalf("QSS disabledOpacity = %v, want 0.25", ln.Style.Opacity)
	}
	if canPress(btn, e.RT) {
		t.Fatal("QSS-disabled button must not be pressable")
	}
	if got := Focusables(e.RT.App.Scenes["main"], e.RT); len(got) != 0 {
		t.Fatalf("QSS-disabled button remained focusable: %v", ids(got))
	}
	e.DrawFrame(surf)
	cx, cy := buttonCenter(t, e, btn)
	e.HandlePointer(PointerInput{Type: PointerPress, X: float64(cx), Y: float64(cy)})
	e.HandlePointer(PointerInput{Type: PointerRelease, X: float64(cx), Y: float64(cy)})
	if e.RT.State["fired"] != nil {
		t.Fatal("QSS-disabled button dispatched")
	}

	e.RT.State["locked"] = false
	if !canPress(btn, e.RT) || len(Focusables(e.RT.App.Scenes["main"], e.RT)) != 1 {
		t.Fatal("button did not re-enable when the QSS binding flipped false")
	}
}

// nodeDisabled mirrors the web renderer's styleDisabled truthiness.
func TestNodeDisabledTruthiness(t *testing.T) {
	cases := []struct {
		style map[string]any
		want  bool
	}{
		{nil, false},
		{map[string]any{"disabled": true}, true},
		{map[string]any{"disabled": false}, false},
		{map[string]any{"disabled": "true"}, true},
		{map[string]any{"disabled": "1"}, true},
		{map[string]any{"disabled": float64(1)}, true},
		{map[string]any{"disabled": float64(0)}, false},
		{map[string]any{"disabled": "yes"}, false},
	}
	for _, c := range cases {
		if got := nodeDisabled(&model.Node{Type: "button", Style: c.style}, nil); got != c.want {
			t.Errorf("nodeDisabled(%v) = %v, want %v", c.style, got, c.want)
		}
	}
	if nodeDisabled(nil, nil) {
		t.Error("nodeDisabled(nil, nil) must be false")
	}
}
