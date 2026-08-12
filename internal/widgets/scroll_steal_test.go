package widgets

import (
	"image"
	"testing"

	"github.com/qorm/qorm/internal/model"
	"github.com/qorm/qorm/internal/render/canvas"
	"github.com/qorm/qorm/internal/runtime"
	"github.com/qorm/qorm/internal/theme"
)

// Vertical drag on an InteractiveWidget inside a scroll viewport steals for
// scrolling after slop (list-row scroll gesture).
func TestScrollStealsFromCheckboxChild(t *testing.T) {
	row := &model.Node{Type: "checkbox", ID: "row", Label: "Row",
		Style: map[string]any{"height": 40.0, "width": 200.0}}
	filler := make([]*model.Node, 10)
	for i := range filler {
		filler[i] = &model.Node{Type: "column", Style: map[string]any{"height": 50.0}}
	}
	sv := &model.Node{
		Type: "scroll", ID: "sv",
		Style:    map[string]any{"width": 200.0, "height": 100.0},
		Children: append([]*model.Node{row}, filler...),
	}
	root := &model.Node{Type: "column", ID: "root", Children: []*model.Node{sv}}
	rt := runtime.New(&model.App{Entry: "main", Scenes: map[string]*model.Node{"main": root}})
	rt.Theme = theme.GetDefault()
	e := canvas.NewEngine(rt, canvas.SoftwareRenderer{})
	surf := canvas.NewHeadlessSurface(image.Pt(400, 400))
	e.DrawFrame(surf)

	e.HandlePointer(canvas.PointerInput{Type: canvas.PointerPress, X: 20, Y: 15, Buttons: 1})
	if e.Inter.ScrollDrag.Node != sv || !e.Inter.ScrollDrag.Pending {
		t.Fatalf("press on checkbox must arm parent scroll, got pending=%v node=%v", e.Inter.ScrollDrag.Pending, e.Inter.ScrollDrag.Node)
	}
	e.HandlePointer(canvas.PointerInput{Type: canvas.PointerMove, X: 20, Y: 5, Buttons: 1})
	e.HandlePointer(canvas.PointerInput{Type: canvas.PointerMove, X: 20, Y: -50, Buttons: 1})
	if !e.Inter.ScrollDrag.Active {
		t.Fatal("vertical drag past slop must activate scroll steal")
	}
	if e.Inter.Pressed != nil {
		t.Fatal("scroll steal must clear Pressed capture")
	}
	if off := e.Inter.ScrollOffsets[sv]; off.Y <= 0 {
		t.Fatalf("stolen drag must scroll, offset=%v", off)
	}
}
