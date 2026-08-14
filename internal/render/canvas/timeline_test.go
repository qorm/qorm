package canvas

import (
	"testing"
	"time"

	"github.com/qorm/platform/internal/model"
	"github.com/qorm/platform/internal/runtime"
)

func TestTimelineSequenceScaleThenMove(t *testing.T) {
	n := &model.Node{Type: "box", ID: "hero", Props: map[string]any{
		"timeline": []any{
			map[string]any{"scale": 2.0, "duration": 100.0, "ease": "linear"},
			map[string]any{"dx": 40.0, "duration": 100.0, "ease": "linear"},
		},
		"timelineToken": "1",
	}}
	rt := runtime.New(&model.App{Entry: "main", Scenes: map[string]*model.Node{"main": n}})
	inter := &Interaction{}
	start := time.Now()
	// Arm
	_ = timelineFor(n, 0, rt, inter, start)
	mid1 := timelineFor(n, 0, rt, inter, start.Add(50*time.Millisecond))
	if !mid1.running || mid1.scale < 1.4 || mid1.scale > 1.6 {
		t.Fatalf("step1 mid scale=%v run=%v", mid1.scale, mid1.running)
	}
	mid2 := timelineFor(n, 0, rt, inter, start.Add(150*time.Millisecond))
	if !mid2.running || mid2.scale < 1.9 {
		t.Fatalf("step2 should hold scale≈2, got %v", mid2.scale)
	}
	if mid2.dx < 15 || mid2.dx > 25 {
		t.Fatalf("step2 mid dx=%v want ~20", mid2.dx)
	}
	done := timelineFor(n, 0, rt, inter, start.Add(250*time.Millisecond))
	if done.running {
		t.Fatal("timeline should finish")
	}
	if !done.active || done.dx < 38 {
		t.Fatalf("hold end pose dx=%v active=%v", done.dx, done.active)
	}
}

func TestTimelineParallelJoin(t *testing.T) {
	n := &model.Node{Type: "box", ID: "p", Props: map[string]any{
		"timeline": []any{
			map[string]any{"scale": 2.0, "duration": 100.0, "ease": "linear"},
			map[string]any{"dx": 40.0, "duration": 100.0, "ease": "linear", "parallel": true},
		},
	}}
	rt := runtime.New(&model.App{Entry: "main", Scenes: map[string]*model.Node{"main": n}})
	inter := &Interaction{}
	start := time.Now()
	_ = timelineFor(n, 0, rt, inter, start)
	mid := timelineFor(n, 0, rt, inter, start.Add(50*time.Millisecond))
	if mid.scale < 1.4 || mid.dx < 15 {
		t.Fatalf("parallel mid scale=%v dx=%v", mid.scale, mid.dx)
	}
}

func TestTimelineTokenRestarts(t *testing.T) {
	n := &model.Node{Type: "box", ID: "r", Props: map[string]any{
		"timeline": []any{
			map[string]any{"scale": 2.0, "duration": 200.0, "ease": "linear"},
		},
		"timelineToken": "a",
	}}
	rt := runtime.New(&model.App{Entry: "main", Scenes: map[string]*model.Node{"main": n}})
	inter := &Interaction{}
	start := time.Now()
	_ = timelineFor(n, 0, rt, inter, start)
	_ = timelineFor(n, 0, rt, inter, start.Add(180*time.Millisecond))
	n.Props["timelineToken"] = "b"
	again := timelineFor(n, 0, rt, inter, start.Add(190*time.Millisecond))
	if !again.running || again.scale > 1.2 {
		t.Fatalf("token bump should restart near scale 1, got scale=%v run=%v", again.scale, again.running)
	}
}

func TestTimelineYoyoObjectForm(t *testing.T) {
	n := &model.Node{Type: "box", ID: "y", Props: map[string]any{
		"timeline": map[string]any{
			"yoyo":   true,
			"repeat": 1.0,
			"steps": []any{
				map[string]any{"scale": 2.0, "duration": 100.0, "ease": "linear"},
			},
		},
	}}
	rt := runtime.New(&model.App{Entry: "main", Scenes: map[string]*model.Node{"main": n}})
	inter := &Interaction{}
	start := time.Now()
	_ = timelineFor(n, 0, rt, inter, start)
	rev := timelineFor(n, 0, rt, inter, start.Add(150*time.Millisecond))
	if !rev.running || rev.scale < 1.2 || rev.scale > 1.8 {
		t.Fatalf("yoyo reverse scale=%v run=%v", rev.scale, rev.running)
	}
}

func TestTimelineCubicPath(t *testing.T) {
	n := &model.Node{Type: "box", ID: "c", Props: map[string]any{
		"timeline": []any{
			map[string]any{
				"path": []any{
					[]any{0.0, 0.0}, []any{0.0, 100.0},
					[]any{100.0, 100.0}, []any{100.0, 0.0},
				},
				"cubic":    true,
				"duration": 100.0,
				"ease":     "linear",
			},
		},
	}}
	rt := runtime.New(&model.App{Entry: "main", Scenes: map[string]*model.Node{"main": n}})
	inter := &Interaction{}
	start := time.Now()
	_ = timelineFor(n, 0, rt, inter, start)
	mid := timelineFor(n, 0, rt, inter, start.Add(50*time.Millisecond))
	if !mid.running {
		t.Fatal("cubic path should run")
	}
	// Symmetric cubic mid near (50, ~75)
	if mid.dx < 30 || mid.dx > 70 || mid.dy < 40 {
		t.Fatalf("cubic mid = (%v,%v)", mid.dx, mid.dy)
	}
}

func TestTimelinePathStep(t *testing.T) {
	n := &model.Node{Type: "box", ID: "p", Props: map[string]any{
		"timeline": []any{
			map[string]any{
				"path":     []any{[]any{0.0, 0.0}, []any{80.0, 0.0}, []any{80.0, 40.0}},
				"duration": 100.0,
				"ease":     "linear",
			},
		},
	}}
	rt := runtime.New(&model.App{Entry: "main", Scenes: map[string]*model.Node{"main": n}})
	inter := &Interaction{}
	start := time.Now()
	_ = timelineFor(n, 0, rt, inter, start)
	end := timelineFor(n, 0, rt, inter, start.Add(100*time.Millisecond))
	if end.dx < 75 || end.dy < 35 {
		t.Fatalf("path end dx,dy=%v,%v", end.dx, end.dy)
	}
}

func TestTimelineOnComplete(t *testing.T) {
	n := &model.Node{Type: "box", ID: "c", Props: map[string]any{
		"timeline": []any{
			map[string]any{"scale": 1.5, "duration": 50.0, "ease": "linear"},
		},
		"timelineToken":      "1",
		"timelineOnComplete": "mark_done",
	}}
	app := &model.App{
		Entry:  "main",
		Scenes: map[string]*model.Node{"main": n},
		Actions: map[string]*model.Action{
			"mark_done": {
				ID:    "mark_done",
				Steps: []model.Step{{Type: "state.set", Path: "done", Value: "true"}},
			},
		},
	}
	rt := runtime.New(app)
	if rt.State == nil {
		rt.State = map[string]any{}
	}
	rt.State["done"] = false
	e := NewEngine(rt, SoftwareRenderer{})
	inter := &e.Inter
	start := time.Now()
	_ = timelineFor(n, 0, rt, inter, start)
	_ = timelineFor(n, 0, rt, inter, start.Add(80*time.Millisecond))
	if len(inter.MotionCompletes) != 1 {
		t.Fatalf("want 1 complete queued, got %d", len(inter.MotionCompletes))
	}
	if inter.MotionCompletes[0].inv == nil || inter.MotionCompletes[0].inv.Name != "mark_done" {
		t.Fatalf("complete invoke = %+v", inter.MotionCompletes[0].inv)
	}
	e.drainMotionCompletes()
	// Verify complete only once
	_ = timelineFor(n, 0, rt, inter, start.Add(100*time.Millisecond))
	if len(inter.MotionCompletes) != 0 {
		t.Fatal("onComplete must not re-fire")
	}
}

func TestTimelineFadeToZeroHolds(t *testing.T) {
	n := &model.Node{Type: "box", ID: "fade", Props: map[string]any{
		"timeline": []any{
			map[string]any{"opacity": 0.0, "duration": 80.0, "ease": "linear"},
		},
	}}
	rt := runtime.New(&model.App{Entry: "main", Scenes: map[string]*model.Node{"main": n}})
	inter := &Interaction{}
	start := time.Now()
	_ = timelineFor(n, 0, rt, inter, start)
	done := timelineFor(n, 0, rt, inter, start.Add(120*time.Millisecond))
	if done.running {
		t.Fatal("fade timeline should finish")
	}
	if done.opacity > 0.05 {
		t.Fatalf("finished fade must hold opacity≈0, got %v", done.opacity)
	}
}

func TestStaggerMS(t *testing.T) {
	n := &model.Node{Type: "box", Props: map[string]any{"stagger": 40.0}}
	rt := runtime.New(&model.App{Entry: "main", Scenes: map[string]*model.Node{"main": n}})
	if staggerMS(n, 0, rt) != 0 {
		t.Fatal("index 0 stagger must be 0")
	}
	if staggerMS(n, 3, rt) != 120 {
		t.Fatalf("index 3 stagger = %v want 120", staggerMS(n, 3, rt))
	}
}

func TestTransitionYoyoStyle(t *testing.T) {
	// Style tween with yoyo: opacity 1 → 0.4 → 1
	begin := NodeStyle{Opacity: 1, TransitionYoyo: true, TransitionRepeat: 1, TransitionEasing: "linear"}
	end := NodeStyle{Opacity: 0.4, TransitionYoyo: true, TransitionRepeat: 1, TransitionEasing: "linear"}
	rt := runtime.New(&model.App{Entry: "main", Scenes: map[string]*model.Node{"main": {Type: "box"}}})
	// Seed state
	delete(globalAnimStates, "yoyo1")
	got, run := UpdateAndGetAnimatedStyleD("yoyo1", begin, rt, 100*time.Millisecond)
	if run {
		t.Fatalf("first sight should not animate, run=%v op=%v", run, got.Opacity)
	}
	// Retarget to end
	got, run = UpdateAndGetAnimatedStyleD("yoyo1", end, rt, 100*time.Millisecond)
	if !run {
		// Immediately after retarget, t≈0 so still running
	}
	// Force mid by rewinding controller
	st := globalAnimStates["yoyo1"]
	if st == nil {
		t.Fatal("missing anim state")
	}
	st.Controller.StartTime = time.Now().Add(-50 * time.Millisecond)
	got, run = UpdateAndGetAnimatedStyleD("yoyo1", end, rt, 100*time.Millisecond)
	if !run || got.Opacity > 0.8 || got.Opacity < 0.5 {
		t.Fatalf("yoyo forward mid opacity=%v run=%v", got.Opacity, run)
	}
	st.Controller.StartTime = time.Now().Add(-250 * time.Millisecond)
	got, run = UpdateAndGetAnimatedStyleD("yoyo1", end, rt, 100*time.Millisecond)
	if run {
		t.Fatalf("yoyo should finish, op=%v", got.Opacity)
	}
	// Settles at begin (1.0)
	if got.Opacity < 0.95 {
		t.Fatalf("yoyo settle opacity=%v want ~1", got.Opacity)
	}
	delete(globalAnimStates, "yoyo1")
}
