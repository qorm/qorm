package widgets

import (
	"image"
	"testing"

	"github.com/qorm/qorm/internal/model"
	"github.com/qorm/qorm/internal/render/canvas"
	"github.com/qorm/qorm/internal/runtime"
	"github.com/qorm/qorm/internal/theme"
)

// The modal mounts when its `open` binding is truthy; a tap on the backdrop
// (outside the centered panel) dispatches the dismiss handler.
func TestModalOpenBackdropDismiss(t *testing.T) {
	md := &model.Node{Type: "modal", ID: "md",
		Props: map[string]any{"open": "{{state.show}}", "title": "Hello"},
		OnPress: &model.Invoke{Name: "close"},
		Children: []*model.Node{
			{Type: "text", ID: "body", Props: map[string]any{"text": "body"}},
		}}
	root := &model.Node{Type: "column", ID: "root", Children: []*model.Node{md}}
	app := &model.App{Entry: "main", Scenes: map[string]*model.Node{"main": root},
		Actions: map[string]*model.Action{
			"close": {ID: "close", Steps: []model.Step{{Type: "state.set", Path: "show", Value: "false"}}},
		}}
	rt := runtime.New(app)
	rt.Theme = theme.GetDefault()
	e := canvas.NewEngine(rt, canvas.SoftwareRenderer{})
	surf := canvas.NewHeadlessSurface(image.Pt(400, 400))
	rt.State["show"] = false
	e.DrawFrame(surf)
	w, _ := canvas.LookupWidget("modal")
	mw := w.(*Modal)
	if mw.OverlayOpen(md, rt) {
		t.Fatal("the modal must be closed while open=false")
	}

	rt.State["show"] = true
	e.MarkDirty()
	e.DrawFrame(surf)
	if !mw.OverlayOpen(md, rt) {
		t.Fatal("the modal must mount while open=true")
	}
	// A backdrop tap (the corner, outside the centered panel) dispatches the
	// dismiss handler, which flips open back off (the state.set literal writes
	// the string "false", which formTruthy reads as off).
	e.HandlePointer(canvas.PointerInput{Type: canvas.PointerPress, X: 5, Y: 5})
	if got := rt.State["show"]; got != "false" {
		t.Errorf("a backdrop tap must dispatch the dismiss handler, show = %v", got)
	}
}
