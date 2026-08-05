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

// Pointer capture keeps the drag stream with the widget even when the finger
// leaves the row (a vertical drift mid-swipe must not strand the gesture).
func TestDismissibleCaptureSurvivesDrift(t *testing.T) {
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

	// Press in the row, drag far left, then drift ABOVE the row before release.
	e.HandlePointer(canvas.PointerInput{Type: canvas.PointerPress, X: 180, Y: 20, Buttons: 1})
	e.HandlePointer(canvas.PointerInput{Type: canvas.PointerMove, X: 50, Y: 20, Buttons: 1})
	e.HandlePointer(canvas.PointerInput{Type: canvas.PointerMove, X: 50, Y: -10, Buttons: 1})
	e.HandlePointer(canvas.PointerInput{Type: canvas.PointerRelease, X: 50, Y: -10})
	if rt.State["removed"] != "yes" {
		t.Error("a drag that drifts off the row must still dismiss (pointer capture)")
	}
}

// A dismissed row can be dragged back (a parent that did not remove the node
// leaves the row recoverable instead of permanently locked out).
func TestDismissibleDragBackRecovers(t *testing.T) {
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

	// Dismiss it.
	e.HandlePointer(canvas.PointerInput{Type: canvas.PointerPress, X: 180, Y: 20, Buttons: 1})
	e.HandlePointer(canvas.PointerInput{Type: canvas.PointerMove, X: 50, Y: 20, Buttons: 1})
	e.HandlePointer(canvas.PointerInput{Type: canvas.PointerRelease, X: 50, Y: 20})
	e.MarkDirty()
	e.DrawFrame(surf)
	if n := countPixels(surf.Frame(), isRed); n == 0 {
		t.Fatal("precondition: the row must be dismissed (red visible)")
	}

	// Drag it back to the right: from the fully-slid offset (-200) a 120px
	// rightward drag lands at -80, past the dismiss threshold, and release
	// snaps the row closed.
	e.HandlePointer(canvas.PointerInput{Type: canvas.PointerPress, X: 100, Y: 20, Buttons: 1})
	e.HandlePointer(canvas.PointerInput{Type: canvas.PointerMove, X: 220, Y: 20, Buttons: 1})
	e.HandlePointer(canvas.PointerInput{Type: canvas.PointerRelease, X: 220, Y: 20})
	e.MarkDirty()
	e.DrawFrame(surf)
	if n := countPixels(surf.Frame(), isRed); n != 0 {
		t.Errorf("dragging a dismissed row back must recover it (red still visible: %d px)", n)
	}
}
