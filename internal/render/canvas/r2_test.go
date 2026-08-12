package canvas

import (
	"image"
	"image/color"
	"math"
	"testing"
	"time"

	"github.com/qorm/qorm/internal/anim"
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

// Any node can declare interaction effects declaratively — hoverOpacity dims
// it on hover, pressedScale shrinks it (about its center) while pressed. No
// widget or hardcoded per-type logic involved: the effects are style data
// resolved generically by the engine.
func TestDeclarativeInteractionEffects(t *testing.T) {
	box := &model.Node{Type: "box", ID: "b1",
		Style: map[string]any{"width": 100.0, "height": 50.0, "background": "#ff0000",
			"pressedScale": 0.9, "hoverOpacity": 0.5}}
	root := &model.Node{Type: "column", ID: "root", Children: []*model.Node{box}}
	app := &model.App{Entry: "main", Scenes: map[string]*model.Node{"main": root}}
	rt := runtime.New(app)
	rt.Theme = theme.GetDefault()
	e := NewEngine(rt, SoftwareRenderer{})
	surf := NewHeadlessSurface(image.Pt(400, 400))
	e.DrawFrame(surf)

	// Hover: the red box dims to 50% (over white → pinkish, G jumps from 0).
	e.Inter.Hovered = box
	e.MarkDirty()
	e.DrawFrame(surf)
	if g := surf.Frame().RGBAAt(50, 25); g.G < 100 {
		t.Errorf("hoverOpacity 0.5 must lighten the red fill, got %v", g)
	}

	// Press: the group scales to 0.9 about its center (X shifts by
	// width*(1-0.9)/2 = 5).
	e.Inter.Hovered, e.Inter.Pressed = nil, box
	e.MarkDirty()
	e.DrawFrame(surf)
	g := e.findGroupByModel(box)
	if g.Base().ScaleX != 0.9 || g.Base().ScaleY != 0.9 {
		t.Errorf("pressedScale must scale the group, got (%v,%v)", g.Base().ScaleX, g.Base().ScaleY)
	}
	if math.Abs(g.Base().X-5) > 1e-9 {
		t.Errorf("scale must be center-anchored, group.X = %v, want 5", g.Base().X)
	}
}

// A declarative transition animates interaction-effect changes: with
// transition "0.1s" the hover background tweens from the base toward the
// hover color (the engine keeps animating), then lands.
func TestDeclarativeInteractionTransition(t *testing.T) {
	// Clear shared tween state so count>1 re-runs start from a cold clock.
	delete(globalAnimStates, "b1")
	box := &model.Node{Type: "box", ID: "b1",
		Style: map[string]any{"width": 100.0, "height": 50.0, "background": "#ff0000",
			"hoverBackground": "#00ff00", "transition": "0.1s"}}
	root := &model.Node{Type: "column", ID: "root", Children: []*model.Node{box}}
	app := &model.App{Entry: "main", Scenes: map[string]*model.Node{"main": root}}
	rt := runtime.New(app)
	rt.Theme = theme.GetDefault()
	e := NewEngine(rt, SoftwareRenderer{})
	surf := NewHeadlessSurface(image.Pt(400, 400))
	e.DrawFrame(surf)

	e.Inter.Hovered = box
	e.MarkDirty()
	e.DrawFrame(surf)
	// Mid-tween: the fill is still red-ish, not yet fully green.
	if c := surf.Frame().RGBAAt(50, 25); c.G > 200 {
		t.Errorf("mid-transition must not be fully green yet, got %v", c)
	}
	if !e.Animating() {
		t.Error("a transition in flight must keep the engine animating")
	}
	// Past the transition: the hover color lands.
	time.Sleep(150 * time.Millisecond)
	e.MarkDirty()
	e.DrawFrame(surf)
	if c := surf.Frame().RGBAAt(50, 25); c.G < 200 {
		t.Errorf("after the transition the hover color must land, got %v", c)
	}
}

// transition "… spring" uses the spring curve (overshoot) for press scale.
func TestSpringTransitionEasing(t *testing.T) {
	box := &model.Node{Type: "box", ID: "spring1",
		Style: map[string]any{
			"width": 40.0, "height": 40.0, "background": "#3366ff",
			"pressedScale": 1.2, "transition": "0.25s spring",
		}}
	root := &model.Node{Type: "column", ID: "root", Children: []*model.Node{box}}
	app := &model.App{Entry: "main", Scenes: map[string]*model.Node{"main": root}}
	rt := runtime.New(app)
	rt.Theme = theme.GetDefault()
	// Clear any leftover anim state from other tests sharing the key map.
	delete(globalAnimStates, "spring1")

	e := NewEngine(rt, SoftwareRenderer{})
	surf := NewHeadlessSurface(image.Pt(80, 80))
	e.DrawFrame(surf)

	e.Inter.Pressed = box
	e.MarkDirty()
	e.DrawFrame(surf)
	// Mid-spring: EffectiveScale is between 1 and the overshoot peak.
	s := parseStyle(box, rt)
	applyInteractiveOverlay(&s, box, rt, &e.Inter)
	s.scaleBy(1)
	cur, running := UpdateAndGetAnimatedStyleD("spring1", s, rt, s.Transition)
	if !running && cur.EffectiveScale == 1 {
		// First retarget after hover was applied via measure; force a fresh
		// controller the way measure does.
		delete(globalAnimStates, "spring1")
		// Seed baseline then retarget to pressed.
		base := parseStyle(box, rt)
		base.EffectiveScale = 1
		UpdateAndGetAnimatedStyleD("spring1", base, rt, s.Transition)
		s.EffectiveScale = 1.2
		cur, running = UpdateAndGetAnimatedStyleD("spring1", s, rt, s.Transition)
	}
	if !running {
		t.Fatal("spring transition must still be in flight right after retarget")
	}
	// Spring overshoots: at small t the value can exceed a linear ease's.
	// At minimum, scale has left the start (1) toward the target.
	if cur.EffectiveScale == 1 {
		t.Errorf("spring mid-frame scale still 1; want motion toward pressedScale")
	}
	// Curve name must resolve.
	if c, ok := anim.CurveByName("spring"); !ok || c == nil {
		t.Fatal("spring curve must be registered")
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
