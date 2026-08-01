package canvas

// Tests for the optional widget-seam extensions (InteractiveWidget /
// AnimatedWidget) using registry entries defined by the tests themselves —
// the same path an app's custom components take.

import (
	"image"
	"image/color"
	"testing"

	"github.com/qorm/qorm/internal/model"
	"github.com/qorm/qorm/internal/render/graph"
	"github.com/qorm/qorm/internal/runtime"
)

// testToggle is an InteractiveWidget: a press flips state["on"], and while
// pressed it captures the stream (records every event it sees).
type testToggle struct{ seen *[]PointerType }

func (testToggle) Measure(n *model.Node, rt *runtime.Runtime, _ map[string]any, scale int) (int, int) {
	return 40 * scale, 40 * scale
}
func (testToggle) Record(ln *LayoutNode, rt *runtime.Runtime, scale int) graph.Node {
	r := graph.NewRect()
	r.Width, r.Height = float64(ln.Width), float64(ln.Height)
	r.Fill = color.RGBA{128, 128, 128, 255}
	return r
}
func (t testToggle) HandlePointer(n *model.Node, rt *runtime.Runtime, p PointerInput, inter *Interaction, _ image.Rectangle) bool {
	*t.seen = append(*t.seen, p.Type)
	if p.Type == PointerPress {
		inter.Pressed = n // take capture, like a drag-capable widget would
		if on, _ := rt.State["on"].(bool); !on {
			rt.State["on"] = true
		} else {
			rt.State["on"] = false
		}
		return true
	}
	return false
}

func TestInteractiveWidgetRoutesAndCaptures(t *testing.T) {
	var seen []PointerType
	RegisterWidget("testtoggle", testToggle{seen: &seen})

	root := &model.Node{Type: "column", ID: "root", Children: []*model.Node{
		{Type: "testtoggle", ID: "tt"},
	}}
	app := &model.App{Entry: "main", Scenes: map[string]*model.Node{"main": root}}
	rt := runtime.New(app)
	e := NewEngine(rt, SoftwareRenderer{})
	surf := NewHeadlessSurface(image.Pt(200, 200))
	e.DrawFrame(surf)

	e.HandlePointer(PointerInput{Type: PointerPress, X: 10, Y: 10})
	if on, _ := rt.State["on"].(bool); !on {
		t.Fatal("press on the widget did not flip state")
	}
	if e.Inter.Pressed == nil || e.Inter.Pressed.Type != "testtoggle" {
		t.Fatal("widget did not take press capture")
	}
	// A move far outside the widget still routes to it (capture), then
	// release clears capture.
	e.HandlePointer(PointerInput{Type: PointerMove, X: 199, Y: 199, Buttons: 1})
	e.HandlePointer(PointerInput{Type: PointerRelease, X: 199, Y: 199})
	if e.Inter.Pressed != nil {
		t.Error("capture not cleared on release")
	}
	if len(seen) != 3 {
		t.Errorf("widget saw %d events, want 3 (press/move/release all routed)", len(seen))
	}
}

// testSpinner is an AnimatedWidget that never settles.
type testSpinner struct{}

func (testSpinner) Measure(n *model.Node, rt *runtime.Runtime, _ map[string]any, scale int) (int, int) {
	return 10 * scale, 10 * scale
}
func (testSpinner) Record(ln *LayoutNode, rt *runtime.Runtime, scale int) graph.Node {
	r := graph.NewRect()
	r.Width, r.Height = float64(ln.Width), float64(ln.Height)
	r.Fill = color.RGBA{128, 128, 128, 255}
	return r
}
func (testSpinner) Animating() bool { return true }

func TestAnimatedWidgetKeepsFrameLoopAlive(t *testing.T) {
	RegisterWidget("testspinner", testSpinner{})

	root := &model.Node{Type: "column", ID: "root", Children: []*model.Node{
		{Type: "testspinner", ID: "ts"},
	}}
	app := &model.App{Entry: "main", Scenes: map[string]*model.Node{"main": root}}
	rt := runtime.New(app)
	e := NewEngine(rt, SoftwareRenderer{})
	surf := NewHeadlessSurface(image.Pt(100, 100))

	e.DrawFrame(surf)
	if !e.Animating() {
		t.Fatal("an AnimatedWidget on stage must keep the frame loop alive")
	}
	// No dirty flag, no input — the animation alone must drive the next frame.
	presents := surf.Presents
	e.DrawFrame(surf)
	if surf.Presents != presents+1 {
		t.Error("animating widget did not drive a second frame without a dirty flag")
	}
}
