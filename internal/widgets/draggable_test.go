package widgets

import (
	"image"
	"testing"

	"github.com/qorm/platform/internal/model"
	"github.com/qorm/platform/internal/render/canvas"
	"github.com/qorm/platform/internal/runtime"
	"github.com/qorm/platform/internal/theme"
)

// Dragging a draggable onto a dragtarget dispatches the target's onDrop with
// the draggable's evaluated `data` payload; the drag clears after the drop.
func TestDraggableDropOnTarget(t *testing.T) {
	drag := &model.Node{Type: "draggable", ID: "dr", Props: map[string]any{"data": "item-1"},
		Children: []*model.Node{{Type: "box", ID: "dh", Style: map[string]any{"width": 80.0, "height": 40.0}}}}
	tgt := &model.Node{Type: "dragtarget", ID: "tg",
		Props:    map[string]any{"onDrop": map[string]any{"name": "drop"}},
		Children: []*model.Node{{Type: "box", ID: "tb", Style: map[string]any{"width": 120.0, "height": 60.0}}}}
	row := &model.Node{Type: "row", ID: "row", Children: []*model.Node{drag, tgt}}
	root := &model.Node{Type: "column", ID: "root", Children: []*model.Node{row}}
	app := &model.App{Entry: "main", Scenes: map[string]*model.Node{"main": root},
		Actions: map[string]*model.Action{
			"drop": {ID: "drop", Steps: []model.Step{{Type: "state.set", Path: "dropped", Value: "{{data}}"}}},
		}}
	rt := runtime.New(app)
	rt.Theme = theme.GetDefault()
	e := canvas.NewEngine(rt, canvas.SoftwareRenderer{})
	surf := canvas.NewHeadlessSurface(image.Pt(400, 400))
	e.DrawFrame(surf)

	// The draggable sits at (0,0)-(80,40); the target next to it at (80,0)+.
	e.HandlePointer(canvas.PointerInput{Type: canvas.PointerPress, X: 40, Y: 20, Buttons: 1})
	e.HandlePointer(canvas.PointerInput{Type: canvas.PointerMove, X: 100, Y: 30, Buttons: 1}) // past the slop → drag published
	e.HandlePointer(canvas.PointerInput{Type: canvas.PointerMove, X: 160, Y: 30, Buttons: 1}) // over the target
	e.HandlePointer(canvas.PointerInput{Type: canvas.PointerRelease, X: 160, Y: 30})
	if rt.State["dropped"] != "item-1" {
		t.Errorf("dropping on the target must dispatch onDrop with the payload, dropped = %v", rt.State["dropped"])
	}
	if e.Inter.Drag.Active {
		t.Error("the drop must clear the in-flight drag")
	}
}

// A draggable inside a list evaluates its `data` binding against the ITEM
// scope: dragging item 2 carries "{{item.id}}" → "b", not the literal template.
func TestDraggableListScopedData(t *testing.T) {
	dragTpl := &model.Node{Type: "draggable", ID: "dr", Props: map[string]any{"data": "{{item.id}}"},
		Children: []*model.Node{{Type: "box", Style: map[string]any{"width": 60.0, "height": 30.0}}}}
	list := &model.Node{Type: "list", ID: "l", Data: "{{state.items}}", Template: dragTpl}
	tgt := &model.Node{Type: "dragtarget", ID: "tg",
		Props:    map[string]any{"onDrop": map[string]any{"name": "drop"}},
		Children: []*model.Node{{Type: "box", Style: map[string]any{"width": 120.0, "height": 60.0}}}}
	row := &model.Node{Type: "row", ID: "row", Children: []*model.Node{list, tgt}}
	root := &model.Node{Type: "column", ID: "root", Children: []*model.Node{row}}
	app := &model.App{Entry: "main", Scenes: map[string]*model.Node{"main": root},
		Actions: map[string]*model.Action{
			"drop": {ID: "drop", Steps: []model.Step{{Type: "state.set", Path: "dropped", Value: "{{data}}"}}},
		}}
	rt := runtime.New(app)
	rt.Theme = theme.GetDefault()
	rt.State["items"] = []any{map[string]any{"id": "a"}, map[string]any{"id": "b"}}
	e := canvas.NewEngine(rt, canvas.SoftwareRenderer{})
	surf := canvas.NewHeadlessSurface(image.Pt(400, 400))
	e.DrawFrame(surf)

	// Item 2 (the second instance) is at y 30..60; drag it onto the target.
	e.HandlePointer(canvas.PointerInput{Type: canvas.PointerPress, X: 40, Y: 45, Buttons: 1})
	e.HandlePointer(canvas.PointerInput{Type: canvas.PointerMove, X: 100, Y: 50, Buttons: 1})
	e.HandlePointer(canvas.PointerInput{Type: canvas.PointerMove, X: 180, Y: 50, Buttons: 1})
	e.HandlePointer(canvas.PointerInput{Type: canvas.PointerRelease, X: 180, Y: 50})
	if rt.State["dropped"] != "b" {
		t.Errorf("the dropped payload must carry the item's id, dropped = %v", rt.State["dropped"])
	}
}

// A dragtarget wrapping an INTERACTIVE child (a gestureDetector) still gets
// the drop: the engine routes a drag release to the nearest drop target.
func TestDraggableDropThroughNestedInteractive(t *testing.T) {
	drag := &model.Node{Type: "draggable", ID: "dr", Props: map[string]any{"data": "item-1"},
		Children: []*model.Node{{Type: "box", Style: map[string]any{"width": 80.0, "height": 40.0}}}}
	inner := &model.Node{Type: "gesturedetector", ID: "in",
		Children: []*model.Node{{Type: "box", Style: map[string]any{"width": 120.0, "height": 60.0}}}}
	tgt := &model.Node{Type: "dragtarget", ID: "tg",
		Props:    map[string]any{"onDrop": map[string]any{"name": "drop"}},
		Children: []*model.Node{inner}}
	row := &model.Node{Type: "row", ID: "row", Children: []*model.Node{drag, tgt}}
	root := &model.Node{Type: "column", ID: "root", Children: []*model.Node{row}}
	app := &model.App{Entry: "main", Scenes: map[string]*model.Node{"main": root},
		Actions: map[string]*model.Action{
			"drop": {ID: "drop", Steps: []model.Step{{Type: "state.set", Path: "dropped", Value: "{{data}}"}}},
		}}
	rt := runtime.New(app)
	rt.Theme = theme.GetDefault()
	e := canvas.NewEngine(rt, canvas.SoftwareRenderer{})
	surf := canvas.NewHeadlessSurface(image.Pt(400, 400))
	e.DrawFrame(surf)

	e.HandlePointer(canvas.PointerInput{Type: canvas.PointerPress, X: 40, Y: 20, Buttons: 1})
	e.HandlePointer(canvas.PointerInput{Type: canvas.PointerMove, X: 100, Y: 30, Buttons: 1})
	e.HandlePointer(canvas.PointerInput{Type: canvas.PointerMove, X: 160, Y: 30, Buttons: 1})
	e.HandlePointer(canvas.PointerInput{Type: canvas.PointerRelease, X: 160, Y: 30})
	if rt.State["dropped"] != "item-1" {
		t.Errorf("the release must drop on the target despite the inner interactive widget, dropped = %v", rt.State["dropped"])
	}
}

// A release exactly on the target's right edge still drops (the graph's
// inclusive hit-test would otherwise consume the drag in a 1px dead band).
func TestDraggableDropOnEdge(t *testing.T) {
	drag := &model.Node{Type: "draggable", ID: "dr", Props: map[string]any{"data": "x"},
		Children: []*model.Node{{Type: "box", Style: map[string]any{"width": 80.0, "height": 40.0}}}}
	tgt := &model.Node{Type: "dragtarget", ID: "tg",
		Props:    map[string]any{"onDrop": map[string]any{"name": "drop"}},
		Children: []*model.Node{{Type: "box", Style: map[string]any{"width": 120.0, "height": 60.0}}}}
	row := &model.Node{Type: "row", ID: "row", Children: []*model.Node{drag, tgt}}
	root := &model.Node{Type: "column", ID: "root", Children: []*model.Node{row}}
	app := &model.App{Entry: "main", Scenes: map[string]*model.Node{"main": root},
		Actions: map[string]*model.Action{
			"drop": {ID: "drop", Steps: []model.Step{{Type: "state.set", Path: "dropped", Value: "{{data}}"}}},
		}}
	rt := runtime.New(app)
	rt.Theme = theme.GetDefault()
	e := canvas.NewEngine(rt, canvas.SoftwareRenderer{})
	surf := canvas.NewHeadlessSurface(image.Pt(400, 400))
	e.DrawFrame(surf)

	e.HandlePointer(canvas.PointerInput{Type: canvas.PointerPress, X: 40, Y: 20, Buttons: 1})
	e.HandlePointer(canvas.PointerInput{Type: canvas.PointerMove, X: 100, Y: 30, Buttons: 1})
	e.HandlePointer(canvas.PointerInput{Type: canvas.PointerMove, X: 200, Y: 30, Buttons: 1}) // exactly the target's right edge
	e.HandlePointer(canvas.PointerInput{Type: canvas.PointerRelease, X: 200, Y: 30})
	if rt.State["dropped"] != "x" {
		t.Errorf("a release on the target's edge must drop, dropped = %v", rt.State["dropped"])
	}
}

// A drag released over nothing dispatches nothing; the stale drag is cleared
// by the next press rather than sticking.
func TestDraggableAbandonedDragClears(t *testing.T) {
	drag := &model.Node{Type: "draggable", ID: "dr", Props: map[string]any{"data": "item-1"},
		Children: []*model.Node{{Type: "box", ID: "dh", Style: map[string]any{"width": 80.0, "height": 40.0}}}}
	root := &model.Node{Type: "column", ID: "root", Children: []*model.Node{drag}}
	app := &model.App{Entry: "main", Scenes: map[string]*model.Node{"main": root}}
	rt := runtime.New(app)
	rt.Theme = theme.GetDefault()
	e := canvas.NewEngine(rt, canvas.SoftwareRenderer{})
	surf := canvas.NewHeadlessSurface(image.Pt(400, 400))
	e.DrawFrame(surf)

	e.HandlePointer(canvas.PointerInput{Type: canvas.PointerPress, X: 40, Y: 20, Buttons: 1})
	e.HandlePointer(canvas.PointerInput{Type: canvas.PointerMove, X: 200, Y: 100, Buttons: 1}) // publish the drag
	e.HandlePointer(canvas.PointerInput{Type: canvas.PointerRelease, X: 200, Y: 100})          // released over nothing
	if !e.Inter.Drag.Active {
		t.Fatal("precondition: the drag must still be in flight after a missed release")
	}
	// The next press clears it.
	e.HandlePointer(canvas.PointerInput{Type: canvas.PointerPress, X: 40, Y: 20})
	if e.Inter.Drag.Active {
		t.Error("a new press must clear an abandoned drag")
	}
}
