package widgets

import (
	"image"
	"testing"

	"github.com/qorm/qorm/internal/model"
	"github.com/qorm/qorm/internal/render/canvas"
	"github.com/qorm/qorm/internal/runtime"
	"github.com/qorm/qorm/internal/theme"
)

// The first panel is open by default; clicking a header expands that panel
// (writing the bound active index) and clicking the open header collapses it.
func TestAccordionToggle(t *testing.T) {
	p1 := &model.Node{Type: "box", ID: "p1", Style: map[string]any{"width": 200.0, "height": 40.0},
		Props: map[string]any{"title": "One"}}
	p2 := &model.Node{Type: "box", ID: "p2", Style: map[string]any{"width": 200.0, "height": 40.0},
		Props: map[string]any{"title": "Two"}}
	ac := &model.Node{Type: "accordion", ID: "ac",
		Props:    map[string]any{"active": "{{state.open}}"},
		Children: []*model.Node{p1, p2}}
	root := &model.Node{Type: "column", ID: "root", Children: []*model.Node{ac}}
	app := &model.App{Entry: "main", Scenes: map[string]*model.Node{"main": root}}
	rt := runtime.New(app)
	rt.Theme = theme.GetDefault()
	rt.State["open"] = 0 // first panel open (the default)
	e := canvas.NewEngine(rt, canvas.SoftwareRenderer{})
	surf := canvas.NewHeadlessSurface(image.Pt(400, 400))
	e.DrawFrame(surf)

	// Panel 1 is open by default (its content occupies y 44..100); the second
	// header (index 1) sits at y 100..144. Click it: panel 2 expands.
	e.HandlePointer(canvas.PointerInput{Type: canvas.PointerPress, X: 30, Y: 122})
	if rt.State["open"] != 1 {
		t.Errorf("clicking the second header must set active=1, open = %v", rt.State["open"])
	}

	// Click the now-open header 2 again: it collapses (active=-1).
	e.HandlePointer(canvas.PointerInput{Type: canvas.PointerPress, X: 30, Y: 122})
	if rt.State["open"] != -1 {
		t.Errorf("clicking the open header must collapse it, open = %v", rt.State["open"])
	}
}
