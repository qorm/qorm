package runtime

import (
	"testing"

	"github.com/qorm/qorm/internal/model"
)

// These tests complement steps_test.go: they drive every Dispatch step type
// through its edge cases (missing path, type mismatch, non-array targets) so a
// regression that panics or silently corrupts state is caught.

func TestStateSetStep(t *testing.T) {
	// A plain literal value (no bindings) is stored verbatim.
	rt := rtWith(map[string]any{}, model.Step{Type: "state.set", Path: "name", Value: "Ada"})
	rt.Dispatch("a", nil)
	if rt.State["name"] != "Ada" {
		t.Errorf("set literal: got %v", rt.State["name"])
	}

	// A whole-string binding keeps its type (float64 / bool), not a string.
	rt = rtWith(map[string]any{},
		model.Step{Type: "state.set", Path: "n", Value: "{{ 3 + 4 }}"},
		model.Step{Type: "state.set", Path: "ok", Value: "{{ true }}"})
	rt.Dispatch("a", nil)
	if rt.State["n"] != float64(7) {
		t.Errorf("set typed number: got %v (%T)", rt.State["n"], rt.State["n"])
	}
	if rt.State["ok"] != true {
		t.Errorf("set typed bool: got %v (%T)", rt.State["ok"], rt.State["ok"])
	}

	// A broken expression degrades to "" rather than panicking.
	rt = rtWith(map[string]any{}, model.Step{Type: "state.set", Path: "bad", Value: "{{ 1 + }}"})
	rt.Dispatch("a", nil)
	if rt.State["bad"] != "" {
		t.Errorf("set broken expr: got %v", rt.State["bad"])
	}

	// A nested path auto-creates intermediate maps.
	rt = rtWith(map[string]any{}, model.Step{Type: "state.set", Path: "a.b.c", Value: "deep"})
	rt.Dispatch("a", nil)
	if got := getPath(rt.State, "a.b.c"); got != "deep" {
		t.Errorf("set nested path: got %v", got)
	}

	// Type mismatch: writing through a scalar intermediate replaces it with a map.
	rt = rtWith(map[string]any{"a": "scalar"}, model.Step{Type: "state.set", Path: "a.b", Value: "x"})
	rt.Dispatch("a", nil)
	if got := getPath(rt.State, "a.b"); got != "x" {
		t.Errorf("set through scalar: got %v", got)
	}
	if _, isMap := rt.State["a"].(map[string]any); !isMap {
		t.Errorf("set through scalar should replace scalar with map, got %T", rt.State["a"])
	}
}

func TestStateAppendStep(t *testing.T) {
	// Append onto an existing array (binding evaluated to a typed value).
	rt := rtWith(map[string]any{"xs": []any{"a"}},
		model.Step{Type: "state.append", Path: "xs", Value: "{{ 'b' }}"})
	rt.Dispatch("a", nil)
	if got := rt.State["xs"].([]any); len(got) != 2 || got[1] != "b" {
		t.Errorf("append existing: got %v", got)
	}

	// Append to a missing path creates a one-element array.
	rt = rtWith(map[string]any{}, model.Step{Type: "state.append", Path: "xs", Value: "{{ 1 }}"})
	rt.Dispatch("a", nil)
	if got := rt.State["xs"].([]any); len(got) != 1 || got[0] != float64(1) {
		t.Errorf("append missing path: got %v", got)
	}

	// Append onto a non-array value replaces it with a fresh array.
	rt = rtWith(map[string]any{"xs": "not-an-array"},
		model.Step{Type: "state.append", Path: "xs", Value: "{{ 9 }}"})
	rt.Dispatch("a", nil)
	if got := rt.State["xs"].([]any); len(got) != 1 || got[0] != float64(9) {
		t.Errorf("append over scalar: got %v", got)
	}
}

func TestStateAppendObjectStep(t *testing.T) {
	// Object fields are each expression-evaluated and appended as one map.
	rt := rtWith(map[string]any{"items": []any{map[string]any{"id": float64(1)}}},
		model.Step{Type: "state.appendObject", Path: "items", Object: map[string]string{
			"id":   "{{ 2 }}",
			"done": "{{ false }}",
			"name": "static",
		}})
	rt.Dispatch("a", nil)
	items := rt.State["items"].([]any)
	if len(items) != 2 {
		t.Fatalf("appendObject: want 2 items, got %d", len(items))
	}
	obj := items[1].(map[string]any)
	if obj["id"] != float64(2) || obj["done"] != false || obj["name"] != "static" {
		t.Errorf("appendObject fields: got %#v", obj)
	}

	// Append to a missing path creates the array.
	rt = rtWith(map[string]any{},
		model.Step{Type: "state.appendObject", Path: "rows", Object: map[string]string{"k": "v"}})
	rt.Dispatch("a", nil)
	if got := rt.State["rows"].([]any); len(got) != 1 || got[0].(map[string]any)["k"] != "v" {
		t.Errorf("appendObject missing path: got %v", got)
	}
}

func TestStateToggleEdgeCases(t *testing.T) {
	// Toggling a path that is not an array is a no-op (no panic, no change).
	rt := rtWith(map[string]any{"sel": "oops"},
		model.Step{Type: "state.toggle", Path: "sel", Match: "{{ k }}"})
	rt.Dispatch("a", map[string]any{"k": "x"})
	if rt.State["sel"] != "oops" {
		t.Errorf("toggle non-array should be a no-op, got %v", rt.State["sel"])
	}

	// Object array with no matching element: nothing is flipped or appended.
	rt = rtWith(map[string]any{"items": []any{map[string]any{"id": "a", "on": false}}},
		model.Step{Type: "state.toggle", Path: "items", MatchKey: "id", Match: "{{ k }}", Field: "on"})
	rt.Dispatch("a", map[string]any{"k": "zzz"})
	items := rt.State["items"].([]any)
	if len(items) != 1 || items[0].(map[string]any)["on"] != false {
		t.Errorf("toggle no-match object array changed: %v", items)
	}

	// Missing path: getPath returns nil, the []any guard fails, state stays clean.
	rt = rtWith(map[string]any{}, model.Step{Type: "state.toggle", Path: "nope", Match: "{{ k }}"})
	rt.Dispatch("a", map[string]any{"k": "x"})
	if _, exists := rt.State["nope"]; exists {
		t.Errorf("toggle missing path should not create the key, got %v", rt.State["nope"])
	}
}

func TestStateRemoveEdgeCases(t *testing.T) {
	// Non-map elements and non-matching maps survive a remove.
	rt := rtWith(map[string]any{"items": []any{
		"scalar",
		map[string]any{"id": float64(1)},
		map[string]any{"id": float64(2)},
	}},
		model.Step{Type: "state.remove", Path: "items", MatchKey: "id", Match: "{{ 1 }}"})
	rt.Dispatch("a", nil)
	got := rt.State["items"].([]any)
	if len(got) != 2 || got[0] != "scalar" || got[1].(map[string]any)["id"] != float64(2) {
		t.Errorf("remove survivors wrong: %v", got)
	}

	// Removing a value nothing matches leaves the array intact.
	rt = rtWith(map[string]any{"items": []any{map[string]any{"id": float64(1)}}},
		model.Step{Type: "state.remove", Path: "items", MatchKey: "id", Match: "{{ 99 }}"})
	rt.Dispatch("a", nil)
	if got := rt.State["items"].([]any); len(got) != 1 {
		t.Errorf("remove no-match: got %v", got)
	}

	// Removing at a missing/non-array path yields an empty array, never a panic.
	rt = rtWith(map[string]any{}, model.Step{Type: "state.remove", Path: "gone", MatchKey: "id", Match: "{{ 1 }}"})
	rt.Dispatch("a", nil)
	if got := rt.State["gone"].([]any); len(got) != 0 {
		t.Errorf("remove missing path: got %v", got)
	}
}

func TestStateUpdateWhereEdgeCases(t *testing.T) {
	// Only elements whose matchKey matches are patched; the rest are untouched,
	// and non-map elements are skipped without panicking.
	rt := rtWith(map[string]any{"items": []any{
		"scalar",
		map[string]any{"id": float64(1), "done": false},
		map[string]any{"id": float64(2), "done": false},
	}},
		model.Step{Type: "state.updateWhere", Path: "items", MatchKey: "id", Match: "{{ 2 }}",
			Object: map[string]string{"done": "{{ true }}", "tag": "{{ 'x' }}"}})
	rt.Dispatch("a", nil)
	items := rt.State["items"].([]any)
	if items[1].(map[string]any)["done"] != false {
		t.Errorf("updateWhere touched a non-matching element: %v", items[1])
	}
	matched := items[2].(map[string]any)
	if matched["done"] != true || matched["tag"] != "x" {
		t.Errorf("updateWhere did not patch the matching element: %v", matched)
	}

	// Missing path is a silent no-op.
	rt = rtWith(map[string]any{},
		model.Step{Type: "state.updateWhere", Path: "nope", MatchKey: "id", Match: "{{ 1 }}",
			Object: map[string]string{"a": "b"}})
	rt.Dispatch("a", nil)
	if _, exists := rt.State["nope"]; exists {
		t.Errorf("updateWhere missing path should not create the key")
	}
}

func TestStateMergeEdgeCases(t *testing.T) {
	// Merging into a missing path creates the object.
	rt := rtWith(map[string]any{},
		model.Step{Type: "state.merge", Path: "user", Object: map[string]string{"role": "admin"}})
	rt.Dispatch("a", nil)
	if got := rt.State["user"].(map[string]any)["role"]; got != "admin" {
		t.Errorf("merge into missing path: got %v", rt.State["user"])
	}

	// Type mismatch: merging over a scalar replaces it with the new object.
	rt = rtWith(map[string]any{"user": "scalar"},
		model.Step{Type: "state.merge", Path: "user", Object: map[string]string{"role": "admin"}})
	rt.Dispatch("a", nil)
	if got := rt.State["user"].(map[string]any)["role"]; got != "admin" {
		t.Errorf("merge over scalar: got %v", rt.State["user"])
	}
}

func TestStateSortEdgeCases(t *testing.T) {
	// desc reverses numeric order.
	rt := rtWith(map[string]any{"xs": []any{
		map[string]any{"v": float64(1)}, map[string]any{"v": float64(3)}, map[string]any{"v": float64(2)}}},
		model.Step{Type: "state.sort", Path: "xs", Field: "v", Value: "desc"})
	rt.Dispatch("a", nil)
	xs := rt.State["xs"].([]any)
	if xs[0].(map[string]any)["v"] != float64(3) || xs[2].(map[string]any)["v"] != float64(1) {
		t.Errorf("sort desc: got %v", xs)
	}

	// String fields sort lexicographically.
	rt = rtWith(map[string]any{"xs": []any{
		map[string]any{"n": "banana"}, map[string]any{"n": "apple"}, map[string]any{"n": "cherry"}}},
		model.Step{Type: "state.sort", Path: "xs", Field: "n", Value: "asc"})
	rt.Dispatch("a", nil)
	if got := rt.State["xs"].([]any)[0].(map[string]any)["n"]; got != "apple" {
		t.Errorf("sort strings: first = %v", got)
	}

	// An empty sort key (e.g. a binding that resolved to "") is a no-op.
	rt = rtWith(map[string]any{"xs": []any{
		map[string]any{"v": float64(2)}, map[string]any{"v": float64(1)}}},
		model.Step{Type: "state.sort", Path: "xs", Field: "", Value: "asc"})
	rt.Dispatch("a", nil)
	if got := rt.State["xs"].([]any)[0].(map[string]any)["v"]; got != float64(2) {
		t.Errorf("empty-key sort should not reorder: got %v", got)
	}

	// A dynamic field that resolves to "" is likewise a no-op.
	rt = rtWith(map[string]any{"xs": []any{
		map[string]any{"v": float64(2)}, map[string]any{"v": float64(1)}}},
		model.Step{Type: "state.sort", Path: "xs", Field: "{{ missingCol }}", Value: "asc"})
	rt.Dispatch("a", nil)
	if got := rt.State["xs"].([]any)[0].(map[string]any)["v"]; got != float64(2) {
		t.Errorf("dynamic empty-key sort should not reorder: got %v", got)
	}

	// Sorting a missing/non-array path is a no-op.
	rt = rtWith(map[string]any{}, model.Step{Type: "state.sort", Path: "nope", Field: "v", Value: "asc"})
	rt.Dispatch("a", nil)
	if _, exists := rt.State["nope"]; exists {
		t.Errorf("sort missing path should not create the key")
	}

	// Elements missing the sort key (nil) sort by their stringified form.
	rt = rtWith(map[string]any{"xs": []any{
		map[string]any{"v": float64(2)}, map[string]any{"other": 1}}},
		model.Step{Type: "state.sort", Path: "xs", Field: "v", Value: "asc"})
	rt.Dispatch("a", nil) // must not panic; the nil-key row stringifies below "2"
	if len(rt.State["xs"].([]any)) != 2 {
		t.Errorf("sort lost an element: %v", rt.State["xs"])
	}

	// Sorting an array of non-map elements must not panic (fieldOf yields nil
	// for scalars; the stable sort leaves them in place).
	rt = rtWith(map[string]any{"xs": []any{"b", "a"}},
		model.Step{Type: "state.sort", Path: "xs", Field: "v", Value: "asc"})
	rt.Dispatch("a", nil)
	if got := rt.State["xs"].([]any); len(got) != 2 || got[0] != "b" {
		t.Errorf("scalar-element sort: got %v", got)
	}
}

func TestStateMoveStep(t *testing.T) {
	mk := func() *Runtime {
		return rtWith(map[string]any{"xs": []any{"a", "b", "c", "d"}},
			model.Step{Type: "state.move", Path: "xs", From: "{{ from }}", To: "{{ to }}"})
	}
	strs := func(v any) []string {
		var out []string
		for _, e := range v.([]any) {
			out = append(out, e.(string))
		}
		return out
	}
	eq := func(a, b []string) bool {
		if len(a) != len(b) {
			return false
		}
		for i := range a {
			if a[i] != b[i] {
				return false
			}
		}
		return true
	}

	cases := []struct {
		name     string
		from, to int
		want     []string
	}{
		{"forward", 0, 2, []string{"b", "c", "a", "d"}},
		{"backward", 2, 0, []string{"c", "a", "b", "d"}},
		{"noop-same", 1, 1, []string{"a", "b", "c", "d"}},
		{"from-out-of-range", 9, 0, []string{"a", "b", "c", "d"}},
		{"from-negative", -1, 0, []string{"a", "b", "c", "d"}},
		{"to-negative-clamps-start", 3, -5, []string{"d", "a", "b", "c"}},
		{"to-overflow-clamps-end", 0, 99, []string{"b", "c", "d", "a"}},
	}
	for _, c := range cases {
		rt := mk()
		rt.Dispatch("a", map[string]any{"from": float64(c.from), "to": float64(c.to)})
		if got := strs(rt.State["xs"]); !eq(got, c.want) {
			t.Errorf("move %s (%d->%d): got %v, want %v", c.name, c.from, c.to, got, c.want)
		}
	}

	// Moving on a non-array path is a no-op.
	rt := rtWith(map[string]any{"xs": "scalar"},
		model.Step{Type: "state.move", Path: "xs", From: "0", To: "1"})
	rt.Dispatch("a", nil)
	if rt.State["xs"] != "scalar" {
		t.Errorf("move non-array should be a no-op, got %v", rt.State["xs"])
	}
}

func TestStateClearEdgeCases(t *testing.T) {
	// A numeric path clears to 0.0, a missing path clears to "" (the default arm).
	rt := rtWith(map[string]any{"n": float64(42)},
		model.Step{Type: "state.clear", Path: "n"},
		model.Step{Type: "state.clear", Path: "missing"},
		model.Step{Type: "state.clear", Path: "obj"})
	rt.State["obj"] = map[string]any{"k": "v"}
	rt.Dispatch("a", nil)
	if rt.State["n"] != 0.0 {
		t.Errorf("clear number: got %v", rt.State["n"])
	}
	if rt.State["missing"] != "" {
		t.Errorf("clear missing path: got %v", rt.State["missing"])
	}
	if rt.State["obj"] != "" {
		t.Errorf("clear map falls back to empty string: got %v", rt.State["obj"])
	}
}

func TestStateResetEdgeCases(t *testing.T) {
	// Resetting a path with no initial value is a no-op (state keeps its value).
	app := &model.App{
		GlobalState: model.GlobalState{Initial: map[string]any{"known": "start"}},
		Actions:     map[string]*model.Action{"a": {ID: "a", Steps: []model.Step{{Type: "state.reset", Path: "unknown"}}}},
	}
	rt := New(app)
	rt.State["unknown"] = "dirty"
	rt.Dispatch("a", nil)
	if rt.State["unknown"] != "dirty" {
		t.Errorf("reset of path absent from initial should not change it, got %v", rt.State["unknown"])
	}

	// New seeds nil initial state as an empty (non-nil) map.
	rt = New(&model.App{})
	if rt.State == nil {
		t.Fatal("New with nil initial should produce an empty map")
	}
	rt.Dispatch("missing-action", nil) // must not panic on an app with no actions map
}

func TestStateIncrementEdgeCases(t *testing.T) {
	// Incrementing a missing path treats it as 0.
	rt := rtWith(map[string]any{}, model.Step{Type: "state.increment", Path: "n"})
	rt.Dispatch("a", nil)
	if rt.State["n"] != float64(1) {
		t.Errorf("increment missing: got %v", rt.State["n"])
	}

	// A string holding a number is coerced; the step value may be an expression.
	rt = rtWith(map[string]any{"n": "3"}, model.Step{Type: "state.increment", Path: "n", Value: "{{ 2 * 5 }}"})
	rt.Dispatch("a", nil)
	if rt.State["n"] != float64(13) {
		t.Errorf("increment string + expr: got %v", rt.State["n"])
	}

	// A bool counts as 1/0.
	rt = rtWith(map[string]any{"flag": true}, model.Step{Type: "state.increment", Path: "flag"})
	rt.Dispatch("a", nil)
	if rt.State["flag"] != float64(2) {
		t.Errorf("increment bool: got %v", rt.State["flag"])
	}
}

func TestNavigateStepEdgeCases(t *testing.T) {
	app := &model.App{
		Entry:  "home",
		Scenes: map[string]*model.Node{"home": {}, "profile": {}},
		Actions: map[string]*model.Action{
			"go":    {ID: "go", Steps: []model.Step{{Type: "navigate", To: "{{ target }}"}}},
			"noop":  {ID: "noop", Steps: []model.Step{{Type: "navigate", To: "home"}}},
			"bad":   {ID: "bad", Steps: []model.Step{{Type: "navigate", To: "ghost"}}},
			"empty": {ID: "empty", Steps: []model.Step{{Type: "navigate", To: ""}}},
		},
	}
	rt := New(app)
	rt.Scene = "home"

	// The target scene can come from a binding.
	rt.Dispatch("go", map[string]any{"target": "profile"})
	if rt.Scene != "profile" {
		t.Fatalf("binding navigate: scene = %q", rt.Scene)
	}
	rt.NavigateBack()

	// Navigating to the current scene, an unknown scene, or "" is ignored.
	for _, act := range []string{"noop", "bad", "empty"} {
		rt.Dispatch(act, nil)
		if rt.Scene != "home" {
			t.Errorf("action %q should not navigate away from home, got %q", act, rt.Scene)
		}
		if len(rt.NavStack) != 0 {
			t.Errorf("action %q should not push the stack, got %v", act, rt.NavStack)
		}
	}
}

func TestDispatchMissingAction(t *testing.T) {
	// A partially-authored app dispatches actions that do not exist yet: the
	// runtime ignores them and leaves state untouched.
	rt := rtWith(map[string]any{"n": float64(1)},
		model.Step{Type: "state.set", Path: "n", Value: "99"})
	rt.Dispatch("does-not-exist", nil)
	if rt.State["n"] != float64(1) {
		t.Errorf("missing action changed state: %v", rt.State["n"])
	}
}

func TestDispatchStateAndArgsPrecedence(t *testing.T) {
	// Top-level state keys are exposed bare (so `count + 1` works), but explicit
	// args win over state of the same name.
	rt := rtWith(map[string]any{"count": float64(1), "who": "state"},
		model.Step{Type: "state.set", Path: "count", Value: "{{ count + 1 }}"},
		model.Step{Type: "state.set", Path: "whoWon", Value: "{{ who }}"})
	rt.Dispatch("a", map[string]any{"who": "arg"})
	if rt.State["count"] != float64(2) {
		t.Errorf("bare state key should resolve: count = %v", rt.State["count"])
	}
	if rt.State["whoWon"] != "arg" {
		t.Errorf("args must win over state: whoWon = %v", rt.State["whoWon"])
	}
}

func TestBuiltinDismiss(t *testing.T) {
	rt := rtWith(map[string]any{"overlay": map[string]any{"open": true}, "flag": true})

	// Sets the addressed path to false (works through nested paths).
	rt.Dispatch(BuiltinDismiss, map[string]any{"path": "overlay.open"})
	if rt.State["overlay"].(map[string]any)["open"] != false {
		t.Errorf("__dismiss should close overlay.open, got %v", rt.State["overlay"])
	}

	rt.Dispatch(BuiltinDismiss, map[string]any{"path": "flag"})
	if rt.State["flag"] != false {
		t.Errorf("__dismiss should clear flag, got %v", rt.State["flag"])
	}

	// Missing, empty, or non-string path arguments are all no-ops.
	rt.State["flag"] = true
	for _, args := range []map[string]any{nil, {"path": ""}, {"path": float64(7)}} {
		rt.Dispatch(BuiltinDismiss, args)
		if rt.State["flag"] != true {
			t.Errorf("__dismiss with args %v should be a no-op, flag = %v", args, rt.State["flag"])
		}
	}
}

func TestBuiltinSort(t *testing.T) {
	newRT := func() *Runtime {
		return rtWith(map[string]any{
			"rows": []any{
				map[string]any{"name": "B", "age": float64(30)},
				map[string]any{"name": "A", "age": float64(10)},
				map[string]any{"name": "C", "age": float64(20)},
			},
		})
	}
	names := func(rt *Runtime) []string {
		var out []string
		for _, e := range rt.State["rows"].([]any) {
			out = append(out, e.(map[string]any)["name"].(string))
		}
		return out
	}

	args := func(col string) map[string]any {
		return map[string]any{
			"column": col, "data": "rows", "field": "sortField", "dir": "sortDir",
		}
	}

	// First click on a column: records the field and sorts ascending.
	rt := newRT()
	rt.Dispatch(BuiltinSort, args("name"))
	if got := names(rt); got[0] != "A" || got[2] != "C" {
		t.Errorf("first click asc: got %v", got)
	}
	if rt.State["sortField"] != "name" || rt.State["sortDir"] != "asc" {
		t.Errorf("first click state: field=%v dir=%v", rt.State["sortField"], rt.State["sortDir"])
	}

	// Second click on the same column flips to descending.
	rt.Dispatch(BuiltinSort, args("name"))
	if got := names(rt); got[0] != "C" || got[2] != "A" {
		t.Errorf("second click desc: got %v", got)
	}
	if rt.State["sortDir"] != "desc" {
		t.Errorf("second click dir = %v, want desc", rt.State["sortDir"])
	}

	// Third click flips back to ascending.
	rt.Dispatch(BuiltinSort, args("name"))
	if rt.State["sortDir"] != "asc" || names(rt)[0] != "A" {
		t.Errorf("third click should be asc again: dir=%v rows=%v", rt.State["sortDir"], names(rt))
	}

	// Clicking a different column resets to ascending and re-records the field.
	rt.Dispatch(BuiltinSort, args("age"))
	if rt.State["sortField"] != "age" || rt.State["sortDir"] != "asc" {
		t.Errorf("new column: field=%v dir=%v", rt.State["sortField"], rt.State["sortDir"])
	}
	if got := rt.State["rows"].([]any)[0].(map[string]any)["age"]; got != float64(10) {
		t.Errorf("new column sort: first age = %v", got)
	}

	// Without a column, nothing is sorted. NOTE: the runtime still writes the
	// empty column to the field state (the else-if only checks fieldPath), so a
	// column-less invocation clobbers a previously-recorded sort field — see the
	// bug note on Dispatch/__sort. This test pins the current behavior.
	rt = newRT()
	rt.State["sortField"] = "name"
	rt.Dispatch(BuiltinSort, map[string]any{"data": "rows", "field": "sortField", "dir": "sortDir"})
	if rt.State["sortField"] != "" {
		t.Errorf("column-less __sort currently clears the field, got %v", rt.State["sortField"])
	}
	if got := names(rt); got[0] != "B" {
		t.Errorf("no-column __sort should not reorder: %v", got)
	}

	// Without a dir path the sort still happens but no direction is stored, and
	// repeat clicks stay ascending (there is no stored "asc" to flip).
	rt = newRT()
	rt.Dispatch(BuiltinSort, map[string]any{"column": "name", "data": "rows", "field": "sortField"})
	if names(rt)[0] != "A" || rt.State["sortField"] != "name" {
		t.Errorf("dirless __sort should sort and record the field: %v", names(rt))
	}
	rt.Dispatch(BuiltinSort, map[string]any{"column": "name", "data": "rows", "field": "sortField"})
	if got := names(rt); got[0] != "A" {
		t.Errorf("dirless repeat click stays asc: %v", got)
	}
}

func TestToNumAndPathHelpers(t *testing.T) {
	// toNum covers every coercion arm.
	if got := toNum(int(7)); got != 7 {
		t.Errorf("toNum(int) = %v", got)
	}
	if got := toNum("2.5"); got != 2.5 {
		t.Errorf("toNum(string) = %v", got)
	}
	if got := toNum(true); got != 1 {
		t.Errorf("toNum(true) = %v", got)
	}
	if got := toNum(false); got != 0 {
		t.Errorf("toNum(false) = %v", got)
	}
	if got := toNum([]any{}); got != 0 {
		t.Errorf("toNum(other) = %v", got)
	}
	if got := toNum("garbage"); got != 0 {
		t.Errorf("toNum(unparseable) = %v", got)
	}

	// getPath stops at a non-map intermediate and returns nil.
	root := map[string]any{"a": "scalar"}
	if got := getPath(root, "a.b"); got != nil {
		t.Errorf("getPath through scalar = %v, want nil", got)
	}
	// deepCopyMap(nil) is nil (New relies on this before substituting an empty map).
	if got := deepCopyMap(nil); got != nil {
		t.Errorf("deepCopyMap(nil) = %v, want nil", got)
	}
}

func TestMoveElemUnit(t *testing.T) {
	// moveElem is pure — verify the boundary clamps directly.
	arr := []any{"a", "b", "c"}
	if got := moveElem(arr, 0, 0); &got[0] != &arr[0] {
		t.Errorf("no-op move should return the original slice")
	}
	if got := moveElem(arr, 0, -1); got[0] != "a" {
		t.Errorf("negative to clamps to 0: %v", got)
	}
	if got := moveElem(arr, 0, 100); got[2] != "a" {
		t.Errorf("overflow to clamps to end: %v", got)
	}
}
