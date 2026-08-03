package widgets

import (
	"image"
	"testing"

	"github.com/qorm/qorm/internal/model"
	"github.com/qorm/qorm/internal/render/canvas"
	"github.com/qorm/qorm/internal/runtime"
	"github.com/qorm/qorm/internal/theme"
)

// Swiping a dismissible row left past half its width dismisses it (dispatch
// onPress) and leaves the row slid out; a smaller swipe snaps back.
func TestDismissibleSwipeToDismiss(t *testing.T) {
	row := &model.Node{Type: "box", ID: "row", Style: map[string]any{"width": 200.0, "height": 40.0}}
	dis := &model.Node{Type: "dismissible", ID: "dis",
		OnPress:  &model.Invoke{Name: "gone"},
		Children: []*model.Node{row}}
	root := &model.Node{Type: "column", ID: "root", Children: []*model.Node{dis}}
	app := &model.App{Entry: "main", Scenes: map[string]*model.Node{"main": root},
		Actions: map[string]*model.Action{
			"gone": {ID: "gone", Steps: []model.Step{{Type: "state.set", Path: "removed", Value: "yes"}}},
		}}
	rt := runtime.New(app)
	rt.Theme = theme.GetDefault()
	e := canvas.NewEngine(rt, canvas.SoftwareRenderer{})
	surf := canvas.NewHeadlessSurface(image.Pt(240, 160))
	e.DrawFrame(surf)
	if n := countPixels(surf.Frame(), isRed); n != 0 {
		t.Fatalf("danger background must be hidden before the swipe, got %d red px", n)
	}

	// A 30px swipe snaps back (not dismissed).
	e.HandlePointer(canvas.PointerInput{Type: canvas.PointerPress, X: 180, Y: 20, Buttons: 1})
	e.MarkDirty()
	e.HandlePointer(canvas.PointerInput{Type: canvas.PointerMove, X: 150, Y: 20, Buttons: 1})
	e.MarkDirty()
	e.HandlePointer(canvas.PointerInput{Type: canvas.PointerRelease, X: 150, Y: 20})
	e.MarkDirty()
	e.DrawFrame(surf)
	if rt.State["removed"] == "yes" {
		t.Fatal("a small swipe must snap back, not dismiss")
	}

	// A full swipe past half the row width dismisses + dispatches.
	e.HandlePointer(canvas.PointerInput{Type: canvas.PointerPress, X: 180, Y: 20, Buttons: 1})
	e.MarkDirty()
	e.HandlePointer(canvas.PointerInput{Type: canvas.PointerMove, X: 50, Y: 20, Buttons: 1})
	e.MarkDirty()
	e.HandlePointer(canvas.PointerInput{Type: canvas.PointerRelease, X: 50, Y: 20})
	e.MarkDirty()
	e.DrawFrame(surf)
	if rt.State["removed"] != "yes" {
		t.Error("a full swipe past half the row must dismiss and dispatch onPress")
	}
	if n := countPixels(surf.Frame(), isRed); n == 0 {
		t.Error("a dismissed row stays slid out (danger background visible)")
	}
}
