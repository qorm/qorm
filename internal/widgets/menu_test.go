package widgets

import (
	"image"
	"testing"

	"github.com/qorm/qorm/internal/model"
	"github.com/qorm/qorm/internal/render/canvas"
	"github.com/qorm/qorm/internal/runtime"
	"github.com/qorm/qorm/internal/theme"
)

// Clicking the trigger opens the dropdown; clicking an item dispatches its
// onPress and closes; clicking outside closes without dispatching.
func TestMenuToggleAndItemDispatch(t *testing.T) {
	mn := &model.Node{Type: "menu", ID: "mn",
		Props: map[string]any{"label": "Actions", "items": []any{
			map[string]any{"label": "Copy", "onPress": map[string]any{"name": "copy"}},
			map[string]any{"label": "Delete", "disabled": true, "onPress": map[string]any{"name": "del"}},
		}}}
	root := &model.Node{Type: "column", ID: "root", Children: []*model.Node{mn}}
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
	w, _ := canvas.LookupWidget("menu")
	mnW := w.(*Menu)

	// Click the trigger (top-left corner).
	e.HandlePointer(canvas.PointerInput{Type: canvas.PointerPress, X: 30, Y: 20})
	if !mnW.OverlayOpen(mn, rt) {
		t.Fatal("clicking the trigger must open the dropdown")
	}
	e.MarkDirty()
	e.DrawFrame(surf)
	if rt.State["picked"] != nil {
		t.Fatal("opening must not dispatch")
	}

	// The panel spans below the trigger (y ~36..): item 0 (Copy) is at the top.
	e.HandlePointer(canvas.PointerInput{Type: canvas.PointerPress, X: 30, Y: 45})
	if rt.State["picked"] != "copy" {
		t.Errorf("clicking an item must dispatch its onPress, picked = %v", rt.State["picked"])
	}
	if mnW.OverlayOpen(mn, rt) {
		t.Error("selecting an item must close the menu")
	}

	// Reopen and click the DISABLED item: nothing dispatches, but it closes.
	e.HandlePointer(canvas.PointerInput{Type: canvas.PointerPress, X: 30, Y: 20})
	e.MarkDirty()
	e.DrawFrame(surf)
	rt.State["picked"] = nil
	e.HandlePointer(canvas.PointerInput{Type: canvas.PointerPress, X: 30, Y: 45 + 32})
	if rt.State["picked"] != nil {
		t.Error("a disabled item must not dispatch")
	}
	if mnW.OverlayOpen(mn, rt) {
		t.Error("clicking a disabled item must still close the menu")
	}

	// Reopen and click outside: closes without dispatching.
	e.HandlePointer(canvas.PointerInput{Type: canvas.PointerPress, X: 30, Y: 20})
	e.MarkDirty()
	e.DrawFrame(surf)
	rt.State["picked"] = nil
	e.HandlePointer(canvas.PointerInput{Type: canvas.PointerPress, X: 300, Y: 300})
	if rt.State["picked"] != nil {
		t.Error("a press outside must not dispatch")
	}
	if mnW.OverlayOpen(mn, rt) {
		t.Error("a press outside must close the menu")
	}
}
