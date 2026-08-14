package runtime

import (
	"strings"
	"testing"

	"github.com/qorm/platform/internal/model"
)

// A script action dispatches its qscript source: reads and writes state
// through the `state` handle, receives the dispatch args as `args`, and —
// unlike a steps action — runs no steps at all.
func TestDispatchScriptAction(t *testing.T) {
	app := &model.App{
		Entry: "main",
		Scenes: map[string]*model.Node{
			"main": {Type: "column", ID: "root"},
		},
		Actions: map[string]*model.Action{
			"move": {ID: "move", Script: `
fn clamp(v, lo, hi) {
  if (v < lo) { return lo }
  if (v > hi) { return hi }
  return v
}
state.piece.x = clamp(state.piece.x + args.dx, 0, 9)
state.moves = state.moves + 1
`},
		},
	}
	rt := New(app)
	rt.State["piece"] = map[string]any{"x": 8.0}
	rt.State["moves"] = 0.0
	rt.Dispatch("move", map[string]any{"dx": 5.0})
	if rt.LastScriptError != "" {
		t.Fatalf("LastScriptError = %q", rt.LastScriptError)
	}
	if x := rt.State["piece"].(map[string]any)["x"]; x != 9.0 {
		t.Fatalf("piece.x = %v, want 9 (clamped)", x)
	}
	if rt.State["moves"] != 1.0 {
		t.Fatalf("moves = %v, want 1", rt.State["moves"])
	}
	// The error surface clears on the next clean dispatch.
	rt.Dispatch("move", map[string]any{"dx": -20.0})
	if rt.LastScriptError != "" {
		t.Fatalf("LastScriptError = %q after a clean dispatch", rt.LastScriptError)
	}
	if x := rt.State["piece"].(map[string]any)["x"]; x != 0.0 {
		t.Fatalf("piece.x = %v, want 0", x)
	}
}

// A script runtime failure (here: index-assign past the array) never panics
// the dispatch; it lands on LastScriptError with the script line number.
func TestDispatchScriptErrorSurface(t *testing.T) {
	app := &model.App{
		Entry:  "main",
		Scenes: map[string]*model.Node{"main": {Type: "column", ID: "root"}},
		Actions: map[string]*model.Action{
			"bad": {ID: "bad", Script: "state.a = 1\nstate.arr[9] = 1"},
		},
	}
	rt := New(app)
	rt.State["arr"] = []any{1.0}
	rt.Dispatch("bad", nil)
	if rt.LastScriptError == "" {
		t.Fatal("LastScriptError must record the script failure")
	}
	if !strings.Contains(rt.LastScriptError, "line 2") || !strings.Contains(rt.LastScriptError, "out of range") {
		t.Fatalf("LastScriptError = %q, want line 2 + out of range", rt.LastScriptError)
	}
	if rt.State["a"] != 1.0 {
		t.Fatalf("state.a = %v — writes before the failure must stand", rt.State["a"])
	}
}

// When an action declares both script and steps, the script wins outright.
func TestDispatchScriptWinsOverSteps(t *testing.T) {
	app := &model.App{
		Entry:  "main",
		Scenes: map[string]*model.Node{"main": {Type: "column", ID: "root"}},
		Actions: map[string]*model.Action{
			"both": {
				ID:     "both",
				Script: "state.count = 1",
				Steps:  []model.Step{{Type: "state.set", Path: "count", Value: "{{ 99 }}"}},
			},
		},
	}
	rt := New(app)
	rt.Dispatch("both", nil)
	if rt.State["count"] != 1.0 {
		t.Fatalf("count = %v, want 1 (the script, not the steps)", rt.State["count"])
	}
}

// Script actions compose with the runtime's nested-dispatch governance: an
// invoke step calling a script action runs it at the nested depth.
func TestDispatchScriptViaInvoke(t *testing.T) {
	app := &model.App{
		Entry:  "main",
		Scenes: map[string]*model.Node{"main": {Type: "column", ID: "root"}},
		Actions: map[string]*model.Action{
			"outer": {ID: "outer", Steps: []model.Step{
				{Type: "invoke", Name: "inner"},
			}},
			"inner": {ID: "inner", Script: "state.hit = true"},
		},
	}
	rt := New(app)
	rt.Dispatch("outer", nil)
	if rt.State["hit"] != true {
		t.Fatalf("hit = %v, want true (script ran via invoke)", rt.State["hit"])
	}
}
