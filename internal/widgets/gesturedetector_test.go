package widgets

import (
	"image"
	"testing"
	"time"

	"github.com/qorm/qorm/internal/model"
	"github.com/qorm/qorm/internal/render/canvas"
	"github.com/qorm/qorm/internal/runtime"
	"github.com/qorm/qorm/internal/theme"
)

// gestureEngine builds an engine around one gestureDetector wrapping a box,
// with the given handlers wired to actions that record state.
func gestureEngine(t *testing.T, handlers map[string]any) (*canvas.Engine, *canvas.HeadlessSurface, *runtime.Runtime) {
	t.Helper()
	child := &model.Node{Type: "box", ID: "kid", Style: map[string]any{"width": 120.0, "height": 40.0}}
	gd := &model.Node{Type: "gesturedetector", ID: "gd", Props: handlers, Children: []*model.Node{child}}
	// The loader maps a scene JSON `onPress` into the model's OnPress FIELD
	// (tap); onDoubleTap/onLongPress stay props. Mirror that here.
	if raw, ok := handlers["onPress"].(map[string]any); ok {
		if name, ok := raw["name"].(string); ok {
			gd.OnPress = &model.Invoke{Name: name}
		}
	}
	root := &model.Node{Type: "column", ID: "root", Children: []*model.Node{gd}}
	app := &model.App{Entry: "main", Scenes: map[string]*model.Node{"main": root},
		Actions: map[string]*model.Action{
			"tap":   {ID: "tap", Steps: []model.Step{{Type: "state.set", Path: "seen", Value: "tap"}}},
			"dbl":   {ID: "dbl", Steps: []model.Step{{Type: "state.set", Path: "seen", Value: "dbl"}}},
			"long":  {ID: "long", Steps: []model.Step{{Type: "state.set", Path: "seen", Value: "long"}}},
			"press": {ID: "press", Steps: []model.Step{{Type: "state.set", Path: "seen", Value: "press"}}},
		}}
	rt := runtime.New(app)
	rt.Theme = theme.GetDefault()
	e := canvas.NewEngine(rt, canvas.SoftwareRenderer{})
	surf := canvas.NewHeadlessSurface(image.Pt(400, 400))
	e.DrawFrame(surf) // the first frame builds the graph the presses hit-test against
	return e, surf, rt
}

func tap(e *canvas.Engine, x, y float64) {
	e.HandlePointer(canvas.PointerInput{Type: canvas.PointerPress, X: x, Y: y})
	e.HandlePointer(canvas.PointerInput{Type: canvas.PointerRelease, X: x, Y: y})
}

// A quick press+release fires onPress; a second quick tap within the window
// also fires onDoubleTap (the browser's onclick + ondblclick both fire).
func TestGestureDetectorTapAndDoubleTap(t *testing.T) {
	e, _, rt := gestureEngine(t, map[string]any{
		"onPress":     map[string]any{"name": "tap"},
		"onDoubleTap": map[string]any{"name": "dbl"},
	})
	tap(e, 60, 20)
	if rt.State["seen"] != "tap" {
		t.Fatalf("a single tap must fire onPress, seen = %v", rt.State["seen"])
	}
	tap(e, 60, 20)
	if rt.State["seen"] != "dbl" {
		t.Fatalf("a second quick tap must fire onDoubleTap, seen = %v", rt.State["seen"])
	}
}

// A hold past the long-press duration fires onLongPress on release.
func TestGestureDetectorLongPress(t *testing.T) {
	e, _, rt := gestureEngine(t, map[string]any{
		"onLongPress": map[string]any{"name": "long"},
	})
	e.HandlePointer(canvas.PointerInput{Type: canvas.PointerPress, X: 60, Y: 20})
	time.Sleep(gestureLongPressMs + 20*time.Millisecond)
	e.HandlePointer(canvas.PointerInput{Type: canvas.PointerRelease, X: 60, Y: 20})
	if rt.State["seen"] != "long" {
		t.Fatalf("a held press must fire onLongPress, seen = %v", rt.State["seen"])
	}
}

// A drag past the tap slop cancels the gesture (no tap, no long-press).
func TestGestureDetectorDragCancels(t *testing.T) {
	e, _, rt := gestureEngine(t, map[string]any{
		"onPress":     map[string]any{"name": "tap"},
		"onLongPress": map[string]any{"name": "long"},
	})
	e.HandlePointer(canvas.PointerInput{Type: canvas.PointerPress, X: 60, Y: 20, Buttons: 1})
	e.HandlePointer(canvas.PointerInput{Type: canvas.PointerMove, X: 100, Y: 20, Buttons: 1})
	e.HandlePointer(canvas.PointerInput{Type: canvas.PointerRelease, X: 100, Y: 20})
	if rt.State["seen"] != nil {
		t.Fatalf("a drag must cancel the gesture, seen = %v", rt.State["seen"])
	}
}

// Pointer capture keeps the stream with the detector even when the finger
// leaves its bounds: a drag that exits and returns is still a drag (the slop
// is not bypassable by stepping outside).
func TestGestureDetectorCaptureSurvivesExit(t *testing.T) {
	e, _, rt := gestureEngine(t, map[string]any{
		"onPress": map[string]any{"name": "tap"},
	})
	e.HandlePointer(canvas.PointerInput{Type: canvas.PointerPress, X: 60, Y: 20, Buttons: 1})
	e.HandlePointer(canvas.PointerInput{Type: canvas.PointerMove, X: 300, Y: 20, Buttons: 1}) // far outside
	e.HandlePointer(canvas.PointerInput{Type: canvas.PointerMove, X: 60, Y: 20, Buttons: 1}) // back inside
	e.HandlePointer(canvas.PointerInput{Type: canvas.PointerRelease, X: 60, Y: 20})
	if rt.State["seen"] != nil {
		t.Fatalf("an out-of-bounds excursion must still count as a drag, seen = %v", rt.State["seen"])
	}
}

// A wrapped child with its own OnPress wins the tap (Flutter's innermost
// recognizer wins the arena) — a button inside a gestureDetector stays
// clickable.
func TestGestureDetectorChildOnPressWins(t *testing.T) {
	btn := &model.Node{Type: "button", ID: "kid",
		Props:   map[string]any{"label": "Go"},
		OnPress: &model.Invoke{Name: "press"}}
	gd := &model.Node{Type: "gesturedetector", ID: "gd",
		OnPress:  &model.Invoke{Name: "tap"},
		Children: []*model.Node{btn}}
	root := &model.Node{Type: "column", ID: "root", Children: []*model.Node{gd}}
	app := &model.App{Entry: "main", Scenes: map[string]*model.Node{"main": root},
		Actions: map[string]*model.Action{
			"tap":   {ID: "tap", Steps: []model.Step{{Type: "state.set", Path: "seen", Value: "tap"}}},
			"press": {ID: "press", Steps: []model.Step{{Type: "state.set", Path: "seen", Value: "press"}}},
		}}
	rt := runtime.New(app)
	rt.Theme = theme.GetDefault()
	e := canvas.NewEngine(rt, canvas.SoftwareRenderer{})
	surf := canvas.NewHeadlessSurface(image.Pt(400, 400))
	e.DrawFrame(surf)

	tap(e, 60, 20)
	if rt.State["seen"] != "press" {
		t.Fatalf("the wrapped child's onPress must win the tap, seen = %v", rt.State["seen"])
	}
}

// A drag between two taps ends the double-tap sequence: the tap after a
// cancelled drag is a fresh single tap, not a double.
func TestGestureDetectorDragResetsDoubleTap(t *testing.T) {
	e, _, rt := gestureEngine(t, map[string]any{
		"onPress":     map[string]any{"name": "tap"},
		"onDoubleTap": map[string]any{"name": "dbl"},
	})
	tap(e, 60, 20)
	if rt.State["seen"] != "tap" {
		t.Fatalf("first tap must fire onPress, seen = %v", rt.State["seen"])
	}
	e.HandlePointer(canvas.PointerInput{Type: canvas.PointerPress, X: 60, Y: 20, Buttons: 1})
	e.HandlePointer(canvas.PointerInput{Type: canvas.PointerMove, X: 100, Y: 20, Buttons: 1})
	e.HandlePointer(canvas.PointerInput{Type: canvas.PointerRelease, X: 100, Y: 20})
	tap(e, 60, 20) // a fresh tap, not a double
	if rt.State["seen"] != "tap" {
		t.Fatalf("the tap after a cancelled drag must NOT fire onDoubleTap, seen = %v", rt.State["seen"])
	}
}
