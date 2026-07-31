package canvas

import (
	"image"
	"testing"

	"github.com/qorm/qorm/internal/model"
	"github.com/qorm/qorm/internal/render/graph"
	"github.com/qorm/qorm/internal/runtime"
	"github.com/qorm/qorm/internal/theme"
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

	ln := Measure(btn, rt, &Interaction{Pressed: btn})
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

	ln := Measure(btn, rt, &Interaction{Hovered: btn})
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

	ln := Measure(btn, rt, &Interaction{Pressed: btn, Hovered: btn})
	if want := parseColor("#0062CC"); ln.Style.Background != want {
		t.Errorf("pressed+hovered background = %v, want pressed to win (%v)", ln.Style.Background, want)
	}
}

func TestNoInteractionLeavesStyleAlone(t *testing.T) {
	rt := rtWithDefaultTheme(t)
	btn := newButton("b")

	plain := Measure(btn, rt, nil)
	withEmpty := Measure(btn, rt, &Interaction{})
	if plain.Style.Background != withEmpty.Style.Background || plain.Style.Opacity != withEmpty.Style.Opacity {
		t.Error("empty interaction state must not alter the resolved style")
	}
	if want := parseColor("#007AFF"); plain.Style.Background != want {
		t.Errorf("button default background = %v, want theme primary %v", plain.Style.Background, want)
	}
}

func TestPerformLayoutStampsInteractionState(t *testing.T) {
	rt := rtWithDefaultTheme(t)
	btn := newButton("b")
	inter := &Interaction{Pressed: btn, Hovered: btn, Focused: btn}

	ln := Measure(btn, rt, inter)
	g, ok := PerformLayout(ln, image.Rect(0, 0, 200, 100), inter, rt).(*graph.Group)
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
		ln := Measure(btn, rt, inter)
		g := PerformLayout(ln, bounds, inter, rt).(*graph.Group)
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
