package runtime

import (
	"testing"

	"github.com/qorm/qorm/internal/model"
)

func rtWith(state map[string]any, steps ...model.Step) *Runtime {
	app := &model.App{Actions: map[string]*model.Action{"a": {ID: "a", Steps: steps}}}
	return &Runtime{App: app, State: state}
}

func TestActionSteps(t *testing.T) {
	// increment (default +1) and by amount
	rt := rtWith(map[string]any{"n": float64(5)},
		model.Step{Type: "state.increment", Path: "n"},
		model.Step{Type: "state.increment", Path: "n", Value: "{{ 10 }}"})
	rt.Dispatch("a", nil)
	if rt.State["n"] != float64(16) {
		t.Errorf("increment: got %v want 16", rt.State["n"])
	}

	// remove by matchKey
	rt = rtWith(map[string]any{"items": []any{
		map[string]any{"id": float64(1)}, map[string]any{"id": float64(2)}, map[string]any{"id": float64(3)}}},
		model.Step{Type: "state.remove", Path: "items", MatchKey: "id", Match: "{{ 2 }}"})
	rt.Dispatch("a", nil)
	if got := rt.State["items"].([]any); len(got) != 2 {
		t.Errorf("remove: want 2 items, got %d", len(got))
	}

	// updateWhere sets fields on matching element
	rt = rtWith(map[string]any{"items": []any{map[string]any{"id": float64(1), "done": false}}},
		model.Step{Type: "state.updateWhere", Path: "items", MatchKey: "id", Match: "{{ 1 }}",
			Object: map[string]string{"done": "{{ true }}"}})
	rt.Dispatch("a", nil)
	if rt.State["items"].([]any)[0].(map[string]any)["done"] != true {
		t.Error("updateWhere: done should be true")
	}

	// merge into object
	rt = rtWith(map[string]any{"user": map[string]any{"name": "Ada"}},
		model.Step{Type: "state.merge", Path: "user", Object: map[string]string{"role": "admin"}})
	rt.Dispatch("a", nil)
	if rt.State["user"].(map[string]any)["role"] != "admin" || rt.State["user"].(map[string]any)["name"] != "Ada" {
		t.Errorf("merge: got %v", rt.State["user"])
	}

	// sort by key, asc then desc
	rt = rtWith(map[string]any{"xs": []any{
		map[string]any{"v": float64(3)}, map[string]any{"v": float64(1)}, map[string]any{"v": float64(2)}}},
		model.Step{Type: "state.sort", Path: "xs", Field: "v", Value: "asc"})
	rt.Dispatch("a", nil)
	if rt.State["xs"].([]any)[0].(map[string]any)["v"] != float64(1) {
		t.Errorf("sort asc: first should be 1, got %v", rt.State["xs"].([]any)[0])
	}

	// clear resets by type
	rt = rtWith(map[string]any{"arr": []any{1, 2}, "s": "x"},
		model.Step{Type: "state.clear", Path: "arr"},
		model.Step{Type: "state.clear", Path: "s"})
	rt.Dispatch("a", nil)
	if len(rt.State["arr"].([]any)) != 0 || rt.State["s"] != "" {
		t.Errorf("clear: got arr=%v s=%v", rt.State["arr"], rt.State["s"])
	}
}

func TestSortByDynamicField(t *testing.T) {
	// field comes from an arg (e.g. a clicked table column)
	rt := rtWith(map[string]any{"rows": []any{
		map[string]any{"name": "C"}, map[string]any{"name": "A"}, map[string]any{"name": "B"}}},
		model.Step{Type: "state.sort", Path: "rows", Field: "{{ column }}", Value: "asc"})
	rt.Dispatch("a", map[string]any{"column": "name"})
	if rt.State["rows"].([]any)[0].(map[string]any)["name"] != "A" {
		t.Errorf("dynamic sort: first should be A, got %v", rt.State["rows"].([]any)[0])
	}
}

func TestToggleScalarMembership(t *testing.T) {
	// scalar array: toggle membership of match (append when absent)
	rt := rtWith(map[string]any{"sel": []any{"r1"}},
		model.Step{Type: "state.toggle", Path: "sel", Match: "{{ key }}"})
	rt.Dispatch("a", map[string]any{"key": "r2"})
	if got := rt.State["sel"].([]any); len(got) != 2 {
		t.Errorf("toggle append: got %v", got)
	}
	// remove when present
	rt.Dispatch("a", map[string]any{"key": "r1"})
	if got := rt.State["sel"].([]any); len(got) != 1 || got[0] != "r2" {
		t.Errorf("toggle remove: got %v", got)
	}
	// an empty match is a no-op (a handler's select-all sentinel stays out)
	rt.Dispatch("a", map[string]any{"key": ""})
	if got := rt.State["sel"].([]any); len(got) != 1 {
		t.Errorf("empty match should be a no-op: got %v", got)
	}
	// object arrays still flip the field, unchanged
	rt = rtWith(map[string]any{"items": []any{map[string]any{"id": "a", "on": false}}},
		model.Step{Type: "state.toggle", Path: "items", MatchKey: "id", Match: "{{ key }}", Field: "on"})
	rt.Dispatch("a", map[string]any{"key": "a"})
	if rt.State["items"].([]any)[0].(map[string]any)["on"] != true {
		t.Error("object toggle should still flip the field")
	}
}

func TestStateResetStep(t *testing.T) {
	initial := map[string]any{"name": "Ada", "tags": []any{"a", "b"}, "n": float64(1)}
	mk := func(steps ...model.Step) *Runtime {
		app := &model.App{GlobalState: model.GlobalState{Initial: initial},
			Actions: map[string]*model.Action{"a": {ID: "a", Steps: steps}}}
		return New(app)
	}
	// no path: every declared key returns to its initial value
	rt := mk(model.Step{Type: "state.reset"})
	rt.State["name"] = "Grace"
	rt.State["tags"] = []any{"x"}
	rt.State["n"] = float64(9)
	rt.Dispatch("a", nil)
	if rt.State["name"] != "Ada" || rt.State["n"] != float64(1) || len(rt.State["tags"].([]any)) != 2 {
		t.Errorf("reset all: got %v", rt.State)
	}
	// the restored value is a copy — mutating state must not corrupt the manifest
	rt.State["tags"].([]any)[0] = "zzz"
	if initial["tags"].([]any)[0] != "a" {
		t.Error("reset should deep-copy the initial values")
	}
	// with path: only that one key resets
	rt = mk(model.Step{Type: "state.reset", Path: "name"})
	rt.State["name"] = "Grace"
	rt.State["n"] = float64(9)
	rt.Dispatch("a", nil)
	if rt.State["name"] != "Ada" || rt.State["n"] != float64(9) {
		t.Errorf("reset path: got %v", rt.State)
	}
}
