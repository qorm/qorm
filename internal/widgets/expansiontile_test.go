package widgets

import (
	"image"
	"testing"

	"github.com/qorm/qorm/internal/model"
	"github.com/qorm/qorm/internal/render/canvas"
	"github.com/qorm/qorm/internal/runtime"
	"github.com/qorm/qorm/internal/theme"
)

// Clicking the summary toggles the expansion: initiallyExpanded opens it, a
// click collapses, another re-expands.
func TestExpansionTileToggle(t *testing.T) {
	child := &model.Node{Type: "box", ID: "kid", Style: map[string]any{"width": 200.0, "height": 40.0}}
	et := &model.Node{Type: "expansiontile", ID: "et",
		Props:    map[string]any{"title": "Details", "initiallyExpanded": true},
		Children: []*model.Node{child}}
	root := &model.Node{Type: "column", ID: "root", Children: []*model.Node{et}}
	app := &model.App{Entry: "main", Scenes: map[string]*model.Node{"main": root}}
	rt := runtime.New(app)
	rt.Theme = theme.GetDefault()
	e := canvas.NewEngine(rt, canvas.SoftwareRenderer{})
	surf := canvas.NewHeadlessSurface(image.Pt(400, 400))
	e.DrawFrame(surf)
	w, _ := canvas.LookupWidget("expansiontile")
	etW := w.(*ExpansionTile)

	if !etW.isOpen(et) {
		t.Fatal("initiallyExpanded must open the tile")
	}
	// Click the summary (y 0..44): collapses.
	e.HandlePointer(canvas.PointerInput{Type: canvas.PointerPress, X: 30, Y: 22})
	if etW.isOpen(et) {
		t.Error("clicking the summary must collapse the tile")
	}
	e.MarkDirty()
	e.DrawFrame(surf)
	// Click again: re-expands.
	e.HandlePointer(canvas.PointerInput{Type: canvas.PointerPress, X: 30, Y: 22})
	if !etW.isOpen(et) {
		t.Error("clicking the collapsed summary must re-expand the tile")
	}
}

// A button inside the expanded content keeps its tap (the tile forwards the
// press to the child's handler).
func TestExpansionTileContentButtonWorks(t *testing.T) {
	btn := &model.Node{Type: "button", ID: "kid", Props: map[string]any{"label": "Go"},
		OnPress: &model.Invoke{Name: "hit"}}
	et := &model.Node{Type: "expansiontile", ID: "et",
		Props:    map[string]any{"title": "Details", "initiallyExpanded": true},
		Children: []*model.Node{btn}}
	root := &model.Node{Type: "column", ID: "root", Children: []*model.Node{et}}
	app := &model.App{Entry: "main", Scenes: map[string]*model.Node{"main": root},
		Actions: map[string]*model.Action{
			"hit": {ID: "hit", Steps: []model.Step{{Type: "state.set", Path: "pressed", Value: "yes"}}},
		}}
	rt := runtime.New(app)
	rt.Theme = theme.GetDefault()
	e := canvas.NewEngine(rt, canvas.SoftwareRenderer{})
	surf := canvas.NewHeadlessSurface(image.Pt(400, 400))
	e.DrawFrame(surf)

	// The expanded content (the button) sits below the 44px summary.
	e.HandlePointer(canvas.PointerInput{Type: canvas.PointerPress, X: 30, Y: 60})
	if rt.State["pressed"] != "yes" {
		t.Error("a button inside the expanded content must fire on press")
	}
}
