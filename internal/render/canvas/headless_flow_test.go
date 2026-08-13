package canvas

import (
	"image"
	"testing"
	"time"

	"github.com/qorm/qorm/internal/model"
	"github.com/qorm/qorm/internal/runtime"
)

func TestAdvanceTimeFiresTimerWithoutSleeping(t *testing.T) {
	e, surf, _, rt := timerFixture(t, map[string]any{"after": 60000.0, "onTick": "bump"})
	e.DrawFrame(surf) // mount and arm the timer

	e.AdvanceTime(30 * time.Second)
	e.DrawFrame(surf)
	if got := rt.State["count"]; got != 0.0 {
		t.Fatalf("count before virtual deadline = %v, want 0", got)
	}

	e.AdvanceTime(30 * time.Second)
	e.DrawFrame(surf)
	if got := rt.State["count"]; got != 1.0 {
		t.Fatalf("count at virtual deadline = %v, want 1", got)
	}
	if e.Animating() {
		t.Fatal("one-shot timer must settle after virtual deadline")
	}
}

func TestAdvanceTimeCompletesTimelineAndDispatches(t *testing.T) {
	box := &model.Node{Type: "box", ID: "motion", Style: map[string]any{"width": 40.0, "height": 40.0}, Props: map[string]any{
		"timeline":           []any{map[string]any{"scale": 1.5, "duration": 80.0, "ease": "linear"}},
		"timelineOnComplete": "finish",
	}}
	app := &model.App{Entry: "main", Scenes: map[string]*model.Node{"main": box}, Actions: map[string]*model.Action{
		"finish": {ID: "finish", Steps: []model.Step{{Type: "state.set", Path: "done", Value: "{{ true }}"}}},
	}}
	rt := runtime.New(app)
	e := NewEngine(rt, SoftwareRenderer{})
	surf := NewHeadlessSurface(image.Pt(100, 100))
	e.DrawFrame(surf) // mount and start the timeline

	e.AdvanceTime(100 * time.Millisecond)
	e.DrawFrame(surf)
	if got := rt.State["done"]; got != true {
		t.Fatalf("timeline completion state = %v, want true", got)
	}
}

func TestHeadlessTargetHelpersFocusRenderedInput(t *testing.T) {
	e, surf, input := inputFixture(t)
	e.DrawFrame(surf)

	b, ok := e.BoundsByID(input.ID)
	if !ok || b.Empty() {
		t.Fatalf("BoundsByID(%q) = %v, %v; want rendered bounds", input.ID, b, ok)
	}
	if !e.RevealID(input.ID) {
		t.Fatal("RevealID must find a rendered input")
	}
	if !e.FocusID(input.ID, true) {
		t.Fatal("FocusID must focus an enabled input")
	}
	if e.Inter.Focused != input || !e.Inter.FocusVisible || e.Inter.Input == nil {
		t.Fatalf("focus state = focused:%p visible:%v edit:%p", e.Inter.Focused, e.Inter.FocusVisible, e.Inter.Input)
	}
	if _, ok := e.BoundsByID("missing"); ok {
		t.Fatal("missing id unexpectedly resolved")
	}
}
