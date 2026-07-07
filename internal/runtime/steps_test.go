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
