package widgets

import (
	"image"
	"testing"

	"github.com/qorm/qorm/internal/model"
	"github.com/qorm/qorm/internal/render/canvas"
	"github.com/qorm/qorm/internal/runtime"
	"github.com/qorm/qorm/internal/theme"
)

// Right-clicking a contextmenu's trigger opens the floating menu; clicking an
// item dispatches its action and closes it; opening alone dispatches nothing.
func TestContextMenuOpensAndDispatches(t *testing.T) {
	child := &model.Node{Type: "box", ID: "trig", Style: map[string]any{"width": 120.0, "height": 40.0}}
	cm := &model.Node{Type: "contextmenu", ID: "cm",
		Props: map[string]any{"items": []any{
			map[string]any{"title": "Copy", "name": "copy"},
			map[string]any{"title": "Delete", "name": "del"},
		}},
		Children: []*model.Node{child}}
	root := &model.Node{Type: "column", ID: "root", Children: []*model.Node{cm}}
	app := &model.App{Entry: "main", Scenes: map[string]*model.Node{"main": root},
		Actions: map[string]*model.Action{
			"copy": {ID: "copy", Steps: []model.Step{{Type: "state.set", Path: "picked", Value: "copy"}}},
			"del":  {ID: "del", Steps: []model.Step{{Type: "state.set", Path: "picked", Value: "del"}}},
		}}
	rt := runtime.New(app)
	rt.Theme = theme.GetDefault()
	e := canvas.NewEngine(rt, canvas.SoftwareRenderer{})
	surf := canvas.NewHeadlessSurface(image.Pt(400, 400))
	e.DrawFrame(surf)
	w, _ := canvas.LookupWidget("contextmenu")
	cmW := w.(*ContextMenu)

	// Right-click the trigger (top-left, so the panel is not edge-clamped).
	e.HandlePointer(canvas.PointerInput{Type: canvas.PointerPress, X: 20, Y: 20, Right: true})
	e.MarkDirty()
	e.DrawFrame(surf)
	if !cmW.OverlayOpen(cm, rt) {
		t.Fatal("right-click must open the context menu")
	}
	if rt.State["picked"] != nil {
		t.Fatal("opening the menu must not dispatch anything")
	}

	// The panel spans (20,20)-(220,134); the first row (Copy) is y 26..60.
	e.HandlePointer(canvas.PointerInput{Type: canvas.PointerPress, X: 30, Y: 40})
	e.HandlePointer(canvas.PointerInput{Type: canvas.PointerRelease, X: 30, Y: 40})
	if rt.State["picked"] != "copy" {
		t.Errorf("clicking the first item must dispatch it, picked = %v", rt.State["picked"])
	}
	if cmW.OverlayOpen(cm, rt) {
		t.Error("selecting an item must close the menu")
	}
}

// A press outside the panel closes the menu without dispatching.
func TestContextMenuClickOutsideCloses(t *testing.T) {
	child := &model.Node{Type: "box", ID: "trig", Style: map[string]any{"width": 120.0, "height": 40.0}}
	cm := &model.Node{Type: "contextmenu", ID: "cm",
		Props: map[string]any{"items": []any{
			map[string]any{"title": "Copy", "name": "copy"},
		}},
		Children: []*model.Node{child}}
	root := &model.Node{Type: "column", ID: "root", Children: []*model.Node{cm}}
	app := &model.App{Entry: "main", Scenes: map[string]*model.Node{"main": root},
		Actions: map[string]*model.Action{
			"copy": {ID: "copy", Steps: []model.Step{{Type: "state.set", Path: "picked", Value: "copy"}}},
		}}
	rt := runtime.New(app)
	rt.Theme = theme.GetDefault()
	e := canvas.NewEngine(rt, canvas.SoftwareRenderer{})
	surf := canvas.NewHeadlessSurface(image.Pt(400, 400))
	e.DrawFrame(surf)
	w, _ := canvas.LookupWidget("contextmenu")
	cmW := w.(*ContextMenu)

	e.HandlePointer(canvas.PointerInput{Type: canvas.PointerPress, X: 20, Y: 20, Right: true})
	e.MarkDirty()
	e.DrawFrame(surf)
	if !cmW.OverlayOpen(cm, rt) {
		t.Fatal("precondition: menu must be open")
	}
	// Press far from the panel (its top-left is at the click).
	e.HandlePointer(canvas.PointerInput{Type: canvas.PointerPress, X: 300, Y: 300})
	if cmW.OverlayOpen(cm, rt) {
		t.Error("a press outside the panel must close the menu")
	}
	if rt.State["picked"] != nil {
		t.Error("a press outside the panel must not dispatch an item")
	}
}
