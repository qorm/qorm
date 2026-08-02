package canvas

import (
	"image"
	"testing"
	"time"

	"github.com/qorm/qorm/internal/model"
	"github.com/qorm/qorm/internal/runtime"
	"github.com/qorm/qorm/internal/theme"
)

// timerFixture builds a headless engine whose scene mounts one timer node
// (every 300ms, onTick "bump" incrementing state.count) inside a column.
func timerFixture(t *testing.T, timerProps map[string]any) (*Engine, *HeadlessSurface, *model.Node, *runtime.Runtime) {
	t.Helper()
	timer := &model.Node{Type: "timer", ID: "t1", Props: timerProps}
	root := &model.Node{
		Type: "column", ID: "root",
		Children: []*model.Node{
			{Type: "text", ID: "label", Props: map[string]any{"text": "hello"}},
			timer,
		},
	}
	app := &model.App{
		Entry:  "main",
		Scenes: map[string]*model.Node{"main": root},
		Actions: map[string]*model.Action{
			"bump": {ID: "bump", Steps: []model.Step{
				{Type: "state.increment", Path: "count"},
			}},
		},
	}
	rt := runtime.New(app)
	rt.Theme = theme.GetDefault()
	rt.State["count"] = 0.0
	surf := NewHeadlessSurface(image.Pt(400, 400))
	return NewEngine(rt, SoftwareRenderer{}), surf, timer, rt
}

// A mounted repeating timer keeps the engine animating and dispatches its
// onTick once the deadline passes — the frame loop is the native scheduler.
func TestEngineTimerFiresOnDeadline(t *testing.T) {
	e, surf, timer, rt := timerFixture(t, map[string]any{"every": 300.0, "onTick": "bump"})

	e.DrawFrame(surf)
	if !e.Animating() {
		t.Fatal("a pending timer must keep the engine animating")
	}
	if len(e.timers) != 1 {
		t.Fatalf("timers = %d, want 1 registered", len(e.timers))
	}
	if got := rt.State["count"]; got != 0.0 {
		t.Fatalf("count = %v before any deadline, want 0", got)
	}

	// Force the deadline into the past (the wall-clock wait is the host's,
	// not the test's) and render the next frame: the tick must dispatch.
	e.timers[timer].nextFire = time.Now().Add(-time.Millisecond)
	e.MarkDirty()
	e.DrawFrame(surf)
	if got := rt.State["count"]; got != 1.0 {
		t.Fatalf("count = %v after deadline, want 1 (onTick dispatched)", got)
	}
	if !e.Animating() {
		t.Fatal("a repeating timer stays pending after firing")
	}

	// A second forced deadline fires again — the schedule repeats.
	e.timers[timer].nextFire = time.Now().Add(-time.Millisecond)
	e.MarkDirty()
	e.DrawFrame(surf)
	if got := rt.State["count"]; got != 2.0 {
		t.Fatalf("count = %v after second deadline, want 2", got)
	}
}

// An `if` prop turning falsy unmounts the timer: the schedule cancels and the
// loop settles — a game's pause stops its gravity, exactly like an
// AnimatedWidget settling.
func TestEngineTimerHiddenCancels(t *testing.T) {
	e, surf, _, rt := timerFixture(t, map[string]any{
		"every": 300.0, "onTick": "bump", "if": "{{ state.running }}",
	})
	rt.State["running"] = true
	e.DrawFrame(surf)
	if len(e.timers) != 1 {
		t.Fatalf("timers = %d with running=true, want 1", len(e.timers))
	}

	rt.State["running"] = false
	e.MarkDirty()
	e.DrawFrame(surf)
	if len(e.timers) != 0 {
		t.Fatalf("timers = %d after if=false, want 0 (cancelled)", len(e.timers))
	}
	if e.Animating() {
		t.Fatal("engine must settle once the only timer hides")
	}

	// No frame, no fire — even well past the old deadline.
	if got := rt.State["count"]; got != 0.0 {
		t.Fatalf("count = %v after cancel, want 0", got)
	}
}

// A one-shot `after` timer fires exactly once, then settles.
func TestEngineTimerAfterOneShot(t *testing.T) {
	e, surf, timer, rt := timerFixture(t, map[string]any{"after": 300.0, "onTick": "bump"})
	e.DrawFrame(surf)
	if !e.Animating() {
		t.Fatal("a pending one-shot must keep the engine animating")
	}
	e.timers[timer].nextFire = time.Now().Add(-time.Millisecond)
	e.MarkDirty()
	e.DrawFrame(surf)
	if got := rt.State["count"]; got != 1.0 {
		t.Fatalf("count = %v after one-shot deadline, want 1", got)
	}
	if e.Animating() {
		t.Fatal("a fired one-shot must settle the loop")
	}
	e.timers[timer].nextFire = time.Now().Add(-time.Millisecond)
	e.MarkDirty()
	e.DrawFrame(surf)
	if got := rt.State["count"]; got != 1.0 {
		t.Fatalf("count = %v after refire attempt, want 1 (one-shot)", got)
	}
}

// The interval floor clamps a hostile every: 1 to the shared minimum.
func TestEngineTimerIntervalFloor(t *testing.T) {
	e, surf, timer, _ := timerFixture(t, map[string]any{"every": 1.0, "onTick": "bump"})
	e.DrawFrame(surf)
	if got := e.timers[timer].every; got != timerMinEveryMS {
		t.Fatalf("every = %v, want clamped to %v", got, timerMinEveryMS)
	}
}
