package canvas

import (
	"image"
	"image/color"
	"testing"

	"github.com/qorm/qorm/internal/model"
	"github.com/qorm/qorm/internal/runtime"
	"github.com/qorm/qorm/internal/theme"
)

// A readonly input stays focusable (click reaches it) but opens no edit
// session, so typing falls through and never mutates the bound value.
func TestInputReadonlyFocusesWithoutEditing(t *testing.T) {
	in := &model.Node{Type: "input", ID: "in1", Value: "{{state.name}}",
		Props: map[string]any{"readonly": true}}
	root := &model.Node{Type: "column", ID: "root",
		Layout:   map[string]any{"align": "center", "justify": "center"},
		Children: []*model.Node{in}}
	app := &model.App{Entry: "main", Scenes: map[string]*model.Node{"main": root}}
	rt := runtime.New(app)
	rt.Theme = theme.GetDefault()
	rt.State["name"] = "fixed"
	e := NewEngine(rt, SoftwareRenderer{})
	surf := NewHeadlessSurface(image.Pt(400, 400))
	e.DrawFrame(surf)

	clickNode(t, e, in)
	if e.Inter.Focused != in {
		t.Fatal("readonly input must still focus")
	}
	if e.Inter.Input != nil {
		t.Fatal("readonly input must not open an edit session")
	}
	typeRunes(e, "x")
	if got := rt.State["name"]; got != "fixed" {
		t.Errorf("typing must not edit a readonly input: state.name = %v", got)
	}
}

// A bound readonly evaluates the binding and flips live: locked → no session;
// unlocked → editing resumes on re-press; and a lock flipped ON mid-session is
// re-validated by activeEdit so the next keystroke cannot edit.
func TestInputReadonlyBoundAndLive(t *testing.T) {
	in := &model.Node{Type: "input", ID: "in1", Value: "{{state.name}}",
		Props: map[string]any{"readonly": "{{state.lock}}"}}
	root := &model.Node{Type: "column", ID: "root",
		Layout:   map[string]any{"align": "center", "justify": "center"},
		Children: []*model.Node{in}}
	app := &model.App{Entry: "main", Scenes: map[string]*model.Node{"main": root}}
	rt := runtime.New(app)
	rt.Theme = theme.GetDefault()
	rt.State["name"] = "hello"
	rt.State["lock"] = true
	e := NewEngine(rt, SoftwareRenderer{})
	surf := NewHeadlessSurface(image.Pt(400, 400))
	e.DrawFrame(surf)

	clickNode(t, e, in)
	if e.Inter.Input != nil {
		t.Fatal("bound readonly=true must not open an edit session")
	}
	rt.State["lock"] = false
	clickNode(t, e, in)
	if e.Inter.Input == nil {
		t.Fatal("after releasing the lock the field must edit again")
	}
	typeRunes(e, "!")
	if got := rt.State["name"]; got != "hello!" {
		t.Fatalf("editing after unlock: state.name = %v", got)
	}
	rt.State["lock"] = true
	typeRunes(e, "x")
	if got := rt.State["name"]; got != "hello!" {
		t.Fatalf("readonly mid-session must block edits: state.name = %v", got)
	}
}

// A per-node style hoverBackground wins over the theme component's hovered
// color while the node is hovered (and applies even with no theme component).
func TestPerNodeHoverBackground(t *testing.T) {
	rt := testRuntime(nil)
	rt.Theme = theme.GetDefault()
	n := &model.Node{Type: "button", ID: "b", Style: map[string]any{"hoverBackground": "#112233"}}
	s := NodeStyle{Background: color.RGBA{255, 0, 0, 255}}
	applyInteractiveOverlay(&s, n, rt, &Interaction{Hovered: n})
	if s.Background != (color.RGBA{0x11, 0x22, 0x33, 255}) {
		t.Errorf("hover background = %v, want the per-node #112233", s.Background)
	}
}
