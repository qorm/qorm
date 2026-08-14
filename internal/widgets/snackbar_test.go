package widgets

import (
	"image"
	"testing"

	"github.com/qorm/platform/internal/model"
	"github.com/qorm/platform/internal/render/canvas"
	"github.com/qorm/platform/internal/runtime"
	"github.com/qorm/platform/internal/theme"
)

// The snackbar mounts when its `open` binding is truthy; a tap on the bar
// dispatches the node's onPress.
func TestSnackbarOpenAndTap(t *testing.T) {
	sb := &model.Node{Type: "snackbar", ID: "sb",
		Props:   map[string]any{"open": "{{state.show}}", "label": "Saved"},
		OnPress: &model.Invoke{Name: "dismiss"}}
	root := &model.Node{Type: "column", ID: "root", Children: []*model.Node{sb}}
	app := &model.App{Entry: "main", Scenes: map[string]*model.Node{"main": root},
		Actions: map[string]*model.Action{
			"dismiss": {ID: "dismiss", Steps: []model.Step{{Type: "state.set", Path: "show", Value: "false"}}},
		}}
	rt := runtime.New(app)
	rt.Theme = theme.GetDefault()
	e := canvas.NewEngine(rt, canvas.SoftwareRenderer{})
	surf := canvas.NewHeadlessSurface(image.Pt(400, 400))
	rt.State["show"] = false
	e.DrawFrame(surf)
	w, _ := canvas.LookupWidget("snackbar")
	sw := w.(*Snackbar)
	if sw.OverlayOpen(sb, rt) {
		t.Fatal("the snackbar must be hidden while open=false")
	}

	rt.State["show"] = true
	e.MarkDirty()
	e.DrawFrame(surf)
	if !sw.OverlayOpen(sb, rt) {
		t.Fatal("the snackbar must mount while open=true")
	}
	// Tap the bar (bottom center): the onPress dispatch flips open back off.
	e.HandlePointer(canvas.PointerInput{Type: canvas.PointerPress, X: 200, Y: 360})
	if got := rt.State["show"]; got != "false" {
		t.Errorf("a bar tap must dispatch onPress, show = %v", got)
	}
}
