package widgets

import (
	"image"
	"testing"

	"github.com/qorm/qorm/internal/model"
	"github.com/qorm/qorm/internal/render/canvas"
	"github.com/qorm/qorm/internal/runtime"
	"github.com/qorm/qorm/internal/theme"
)

// Hovering the wrapped child opens the tooltip bubble; no hover, no bubble.
func TestTooltipShowsOnHover(t *testing.T) {
	child := &model.Node{Type: "box", ID: "ch", Style: map[string]any{"width": 60.0, "height": 30.0}}
	tp := &model.Node{Type: "tooltip", ID: "tp",
		Props:    map[string]any{"tooltip": "Hint"},
		Children: []*model.Node{child}}
	root := &model.Node{Type: "column", ID: "root", Children: []*model.Node{tp}}
	app := &model.App{Entry: "main", Scenes: map[string]*model.Node{"main": root}}
	rt := runtime.New(app)
	rt.Theme = theme.GetDefault()
	e := canvas.NewEngine(rt, canvas.SoftwareRenderer{})
	surf := canvas.NewHeadlessSurface(image.Pt(240, 160))
	e.DrawFrame(surf)
	w, _ := canvas.LookupWidget("tooltip")
	tw := w.(*Tooltip)
	if tw.OverlayOpen(tp, rt) {
		t.Fatal("no hover must not open the tooltip")
	}

	e.HandlePointer(canvas.PointerInput{Type: canvas.PointerMove, X: 30, Y: 15})
	if !tw.OverlayOpen(tp, rt) {
		t.Error("hovering the wrapped child must open the tooltip")
	}
}
