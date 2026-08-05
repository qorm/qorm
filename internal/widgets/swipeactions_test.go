package widgets

import (
	"image"
	"testing"

	"github.com/qorm/qorm/internal/model"
	"github.com/qorm/qorm/internal/render/canvas"
	"github.com/qorm/qorm/internal/runtime"
	"github.com/qorm/qorm/internal/theme"
)

// Dragging a swipeactions row left reveals its trailing action strip (red),
// and release snaps the row open.
func TestSwipeActionsDragReveals(t *testing.T) {
	row := &model.Node{Type: "box", ID: "row", Style: map[string]any{"width": 200.0, "height": 40.0}}
	sw := &model.Node{Type: "swipeactions", ID: "sw",
		Props: map[string]any{"actions": []any{
			map[string]any{"name": "del", "label": "Delete", "color": "#ff0000"},
		}},
		Children: []*model.Node{row}}
	root := &model.Node{Type: "column", ID: "root", Children: []*model.Node{sw}}

	e, surf, _ := feedbackEngine(t, root)
	e.DrawFrame(surf)
	if n := countPixels(surf.Frame(), isRed); n != 0 {
		t.Fatalf("actions must be hidden behind the content before the swipe, got %d red px", n)
	}

	// Drag the row left by 100px (press at its right, move left).
	e.HandlePointer(canvas.PointerInput{Type: canvas.PointerPress, X: 180, Y: 20, Buttons: 1})
	e.MarkDirty()
	e.HandlePointer(canvas.PointerInput{Type: canvas.PointerMove, X: 80, Y: 20, Buttons: 1})
	e.MarkDirty()
	e.DrawFrame(surf)
	if n := countPixels(surf.Frame(), isRed); n == 0 {
		t.Error("dragging left must reveal the red action strip")
	}

	// Release snaps the row open (actions stay visible).
	e.HandlePointer(canvas.PointerInput{Type: canvas.PointerRelease, X: 80, Y: 20})
	e.MarkDirty()
	e.DrawFrame(surf)
	if n := countPixels(surf.Frame(), isRed); n == 0 {
		t.Error("release must snap the row open (actions visible)")
	}
}

// Tapping an open action dispatches its handler.
func TestSwipeActionsActionTapDispatches(t *testing.T) {
	row := &model.Node{Type: "box", ID: "row", Style: map[string]any{"width": 200.0, "height": 40.0}}
	sw := &model.Node{Type: "swipeactions", ID: "sw",
		Props: map[string]any{"actions": []any{
			map[string]any{"name": "del", "label": "Delete", "color": "#ff0000"},
		}},
		Children: []*model.Node{row}}
	root := &model.Node{Type: "column", ID: "root", Children: []*model.Node{sw}}
	app := &model.App{Entry: "main", Scenes: map[string]*model.Node{"main": root},
		Actions: map[string]*model.Action{
			"del": {ID: "del", Steps: []model.Step{{Type: "state.set", Path: "deleted", Value: "yes"}}},
		}}
	rt := runtime.New(app)
	rt.Theme = theme.GetDefault()
	e := canvas.NewEngine(rt, canvas.SoftwareRenderer{})
	surf := canvas.NewHeadlessSurface(image.Pt(240, 160))
	e.DrawFrame(surf)

	// Reveal the action, then tap it (in the revealed 76px strip at the right).
	e.HandlePointer(canvas.PointerInput{Type: canvas.PointerPress, X: 180, Y: 20, Buttons: 1})
	e.MarkDirty()
	e.HandlePointer(canvas.PointerInput{Type: canvas.PointerMove, X: 80, Y: 20, Buttons: 1})
	e.MarkDirty()
	e.HandlePointer(canvas.PointerInput{Type: canvas.PointerRelease, X: 80, Y: 20})
	e.MarkDirty()
	e.DrawFrame(surf)
	// Now tap the revealed action (rightmost 76px: x 164..240).
	e.HandlePointer(canvas.PointerInput{Type: canvas.PointerPress, X: 200, Y: 20, Buttons: 1})
	e.HandlePointer(canvas.PointerInput{Type: canvas.PointerRelease, X: 200, Y: 20})
	if rt.State["deleted"] != "yes" {
		t.Error("tapping the revealed action must dispatch its handler")
	}
}
