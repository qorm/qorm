package render_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/qorm/qorm/internal/model"
	"github.com/qorm/qorm/internal/render"
	qrt "github.com/qorm/qorm/internal/runtime"
)

// timerApp builds a minimal app whose entry scene holds the given nodes plus a
// "tick" action recording dispatches.
func timerApp(nodes ...*model.Node) *model.App {
	return &model.App{
		Entry: "main",
		Scenes: map[string]*model.Node{
			"main": {Type: "column", ID: "root", Children: nodes},
		},
		Actions: map[string]*model.Action{
			"tick": {ID: "tick", Steps: []model.Step{{Type: "state.increment", Path: "ticks"}}},
		},
	}
}

func TestTimerRendersMarker(t *testing.T) {
	app := timerApp(&model.Node{
		Type: "timer", ID: "poll",
		Props: map[string]any{"every": float64(5000), "onTick": map[string]any{"name": "tick"}},
	})
	res := render.Render(qrt.New(app))
	if !strings.Contains(res.HTML, `data-qorm-timer="poll"`) {
		t.Fatalf("timer must render a data-qorm-timer marker: %s", res.HTML)
	}
	if !strings.Contains(res.HTML, `data-every="5000"`) {
		t.Errorf("timer must carry its interval: %s", res.HTML)
	}
	if !strings.Contains(res.HTML, ` hidden `) && !strings.Contains(res.HTML, ` hidden>`) && !strings.Contains(res.HTML, ` hidden aria-hidden`) {
		t.Errorf("timer marker must be invisible: %s", res.HTML)
	}
	// The onTick handler is registered like a button press handler; data-h
	// points at it and dispatching it runs the action.
	if len(res.Handlers) != 1 || res.Handlers[0].Name != "tick" {
		t.Fatalf("timer must register its onTick handler: %+v", res.Handlers)
	}
	if !strings.Contains(res.HTML, `data-h="0"`) {
		t.Errorf("marker must reference the handler index: %s", res.HTML)
	}
	if len(res.Unknown) != 0 {
		t.Errorf("timer is a known widget, got unknowns %v", res.Unknown)
	}
}

func TestTimerStringShorthandOnTick(t *testing.T) {
	app := timerApp(&model.Node{
		Type: "timer", ID: "poll",
		Props: map[string]any{"every": float64(1000), "onTick": "tick"},
	})
	res := render.Render(qrt.New(app))
	if len(res.Handlers) != 1 || res.Handlers[0].Name != "tick" {
		t.Fatalf("string-shorthand onTick must register: %+v", res.Handlers)
	}
}

func TestTimerEveryClampedToFloor(t *testing.T) {
	app := timerApp(&model.Node{
		Type: "timer", ID: "fast",
		Props: map[string]any{"every": float64(10), "onTick": "tick"},
	})
	html := render.Render(qrt.New(app)).HTML
	want := fmt.Sprintf(`data-every="%d"`, render.TimerMinEveryMS)
	if !strings.Contains(html, want) {
		t.Errorf("a sub-floor interval must be clamped to %d: %s", render.TimerMinEveryMS, html)
	}
}

func TestTimerAfterOneShot(t *testing.T) {
	app := timerApp(&model.Node{
		Type: "timer", ID: "later",
		Props: map[string]any{"after": float64(1500), "onTick": "tick"},
	})
	html := render.Render(qrt.New(app)).HTML
	if !strings.Contains(html, `data-after="1500"`) {
		t.Errorf("after timer must carry data-after: %s", html)
	}
	if strings.Contains(html, "data-every") {
		t.Errorf("a pure one-shot must not carry data-every: %s", html)
	}
}

func TestTimerEveryWinsOverAfter(t *testing.T) {
	app := timerApp(&model.Node{
		Type: "timer", ID: "both",
		Props: map[string]any{"every": float64(1000), "after": float64(2000), "onTick": "tick"},
	})
	html := render.Render(qrt.New(app)).HTML
	if !strings.Contains(html, `data-every="1000"`) || strings.Contains(html, "data-after") {
		t.Errorf("every must win when both schedules are set: %s", html)
	}
}

func TestTimerConditionalExistence(t *testing.T) {
	// The `if` prop governs the marker's presence — the client stops a timer
	// whose marker left the rendered tree (a countdown reaching zero).
	app := timerApp(&model.Node{
		Type: "timer", ID: "cd",
		Props: map[string]any{"every": float64(1000), "onTick": "tick", "if": "{{ state.running }}"},
	})
	rt := qrt.New(app)
	rt.State["running"] = true
	if html := render.Render(rt).HTML; !strings.Contains(html, `data-qorm-timer="cd"`) {
		t.Fatalf("timer with a truthy if must render: %s", html)
	}
	rt.State["running"] = false
	if html := render.Render(rt).HTML; strings.Contains(html, "data-qorm-timer") {
		t.Errorf("timer with a falsy if must not render: %s", html)
	}
}

func TestTimerWithoutIDNotScheduled(t *testing.T) {
	app := timerApp(&model.Node{
		Type:  "timer",
		Props: map[string]any{"every": float64(1000), "onTick": "tick"},
	})
	if html := render.Render(qrt.New(app)).HTML; strings.Contains(html, "data-qorm-timer") {
		t.Errorf("a timer without an id (the idempotency key) must not emit a marker: %s", html)
	}
}

func TestTimerWithoutScheduleNotEmitted(t *testing.T) {
	app := timerApp(&model.Node{
		Type: "timer", ID: "idle",
		Props: map[string]any{"onTick": "tick"},
	})
	if html := render.Render(qrt.New(app)).HTML; strings.Contains(html, "data-qorm-timer") {
		t.Errorf("a timer with neither every nor after must not emit a marker: %s", html)
	}
}

func TestTimerDynamicInterval(t *testing.T) {
	// `every` may be a {{...}} binding — resolved against live state at render
	// time (still clamped to the floor).
	app := timerApp(&model.Node{
		Type: "timer", ID: "dyn",
		Props: map[string]any{"every": "{{ state.rate }}", "onTick": "tick"},
	})
	rt := qrt.New(app)
	rt.State["rate"] = float64(2000)
	if html := render.Render(rt).HTML; !strings.Contains(html, `data-every="2000"`) {
		t.Errorf("bound interval must resolve from state: %s", html)
	}
}

func TestTimerTickDispatchChain(t *testing.T) {
	// End-to-end at the runtime level: dispatching the registered handler (what
	// the client's qorm(h) does on a tick) runs the onTick action.
	app := timerApp(&model.Node{
		Type: "timer", ID: "poll",
		Props: map[string]any{"every": float64(1000), "onTick": "tick"},
	})
	rt := qrt.New(app)
	res := render.Render(rt)
	h := res.Handlers[0]
	rt.Dispatch(h.Name, nil)
	if rt.State["ticks"] != float64(1) {
		t.Errorf("dispatching the timer handler must run onTick: got %v", rt.State["ticks"])
	}
}
