package canvas

import (
	"image"
	"testing"

	"github.com/qorm/qorm/internal/model"
	"github.com/qorm/qorm/internal/runtime"
	"github.com/qorm/qorm/internal/theme"
)

// A right-button press dispatches the nearest onContextMenu handler up the hit
// chain — the canvas counterpart of the browser's contextmenu event. The
// handler's args flow to the action.
func TestRightClickDispatchesContextMenu(t *testing.T) {
	btn := &model.Node{Type: "button", ID: "b1", Props: map[string]any{
		"label":         "Hi",
		"onContextMenu": map[string]any{"name": "menu", "args": map[string]any{"k": "v"}},
	}}
	root := &model.Node{Type: "column", ID: "root",
		Layout:   map[string]any{"align": "center", "justify": "center"},
		Children: []*model.Node{btn}}
	app := &model.App{Entry: "main", Scenes: map[string]*model.Node{"main": root},
		Actions: map[string]*model.Action{
			"menu": {ID: "menu", Steps: []model.Step{{Type: "state.set", Path: "got", Value: "{{k}}"}}},
		}}
	rt := runtime.New(app)
	rt.Theme = theme.GetDefault()
	e := NewEngine(rt, SoftwareRenderer{})
	surf := NewHeadlessSurface(image.Pt(400, 400))
	e.DrawFrame(surf)

	cx, cy := buttonCenter(t, e, btn)
	e.HandlePointer(PointerInput{Type: PointerPress, X: float64(cx), Y: float64(cy), Right: true})
	if rt.State["got"] != "v" {
		t.Errorf("right-click must dispatch onContextMenu with its args, state.got = %v", rt.State["got"])
	}
}

// A left press on the same node does NOT fire the context menu.
func TestLeftPressDoesNotFireContextMenu(t *testing.T) {
	btn := &model.Node{Type: "button", ID: "b1", Props: map[string]any{
		"label":         "Hi",
		"onContextMenu": map[string]any{"name": "menu"},
	}}
	root := &model.Node{Type: "column", ID: "root",
		Layout:   map[string]any{"align": "center", "justify": "center"},
		Children: []*model.Node{btn}}
	app := &model.App{Entry: "main", Scenes: map[string]*model.Node{"main": root},
		Actions: map[string]*model.Action{
			"menu": {ID: "menu", Steps: []model.Step{{Type: "state.set", Path: "got", Value: "yes"}}},
		}}
	rt := runtime.New(app)
	rt.Theme = theme.GetDefault()
	e := NewEngine(rt, SoftwareRenderer{})
	surf := NewHeadlessSurface(image.Pt(400, 400))
	e.DrawFrame(surf)

	cx, cy := buttonCenter(t, e, btn)
	e.HandlePointer(PointerInput{Type: PointerPress, X: float64(cx), Y: float64(cy)})
	if rt.State["got"] == "yes" {
		t.Error("a left press must not dispatch the context menu")
	}
}
