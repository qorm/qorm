package runtime

// Runtime behaviour of the `forEach` (bulk) step.
//
// `state.updateWhere` can only edit the elements a single match selects;
// `forEach` is the general form — run an arbitrary step list once per element,
// with the element in scope. The scope shape is deliberately the same one a
// list's renderItem template gets (alias + index/first/last, `as` to rename),
// so there is one alias rule in the format rather than two.
//
// The guards under test: a non-array collection iterates zero times, the
// iteration count is capped, the body nests under the `if` depth budget, and a
// nested loop shadows the outer one exactly like nested renderItem scopes.

import (
	"testing"

	"github.com/qorm/platform/internal/model"
)

// inboxApp is the canonical bulk-update app: mark every message read.
func inboxApp() *model.App {
	return &model.App{
		Entry:  "main",
		Scenes: map[string]*model.Node{"main": {Type: "scaffold"}},
		GlobalState: model.GlobalState{Initial: map[string]any{
			"messages": []any{
				map[string]any{"id": "m1", "read": false},
				map[string]any{"id": "m2", "read": false},
				map[string]any{"id": "m3", "read": true},
			},
			"log": []any{},
		}},
		Actions: map[string]*model.Action{
			"markAllRead": {ID: "markAllRead", Steps: []model.Step{{
				Type: "forEach", In: "{{ state.messages }}", As: "msg",
				Steps: []model.Step{
					{Type: "state.updateWhere", Path: "messages", MatchKey: "id",
						Match: "{{ msg.id }}", Object: map[string]string{"read": "{{ true }}"}},
					{Type: "state.append", Path: "log", Value: "{{ msg.id + ':' + msgIndex }}"},
				},
			}}},
		},
	}
}

func TestForEachRunsTheBodyPerElement(t *testing.T) {
	rt := New(inboxApp())
	rt.Dispatch("markAllRead", nil)

	msgs, _ := rt.State["messages"].([]any)
	if len(msgs) != 3 {
		t.Fatalf("messages = %#v", rt.State["messages"])
	}
	for i, m := range msgs {
		if m.(map[string]any)["read"] != true {
			t.Errorf("message %d not marked read: %#v", i, m)
		}
	}
	// The alias and its derived index key are both bound.
	log, _ := rt.State["log"].([]any)
	want := []string{"m1:0", "m2:1", "m3:2"}
	if len(log) != len(want) {
		t.Fatalf("log = %#v, want %v", log, want)
	}
	for i, w := range want {
		if log[i] != w {
			t.Errorf("log[%d] = %v, want %v", i, log[i], w)
		}
	}
}

func TestForEachDefaultAliasAndFirstLast(t *testing.T) {
	app := inboxApp()
	app.Actions["tag"] = &model.Action{ID: "tag", Steps: []model.Step{{
		Type: "forEach", In: "{{ state.messages }}", // no `as`: the default names
		Steps: []model.Step{
			{Type: "state.append", Path: "log", Value: "{{ item.id + '/' + index + '/' + first + '/' + last }}"},
		},
	}}}
	rt := New(app)
	rt.Dispatch("tag", nil)
	log, _ := rt.State["log"].([]any)
	want := []string{"m1/0/true/false", "m2/1/false/false", "m3/2/false/true"}
	if len(log) != 3 {
		t.Fatalf("log = %#v", log)
	}
	for i, w := range want {
		if log[i] != w {
			t.Errorf("log[%d] = %v, want %v", i, log[i], w)
		}
	}
}

func TestForEachEmptyAndNonArrayCollections(t *testing.T) {
	// Every shape that is not a populated array iterates zero times rather than
	// guessing an iteration for it — including an `in` that fails to evaluate.
	for _, in := range []string{
		"{{ state.missing }}", // nil
		"{{ state.count }}",   // number
		"{{ state.obj }}",     // object
		"{{ state.empty }}",   // empty array
		"{{ 'text' }}",        // string
		"{{ }}",               // unevaluatable
		"",                    // absent
	} {
		app := inboxApp()
		app.GlobalState.Initial["count"] = 3.0
		app.GlobalState.Initial["obj"] = map[string]any{"a": 1.0}
		app.GlobalState.Initial["empty"] = []any{}
		app.Actions["run"] = &model.Action{ID: "run", Steps: []model.Step{{
			Type: "forEach", In: in,
			Steps: []model.Step{{Type: "state.increment", Path: "runs"}},
		}}}
		rt := New(app)
		rt.Dispatch("run", nil)
		if got := rt.State["runs"]; got != nil {
			t.Errorf("in=%q ran the body %v time(s), want zero", in, got)
		}
	}
}

func TestForEachIterationCap(t *testing.T) {
	// A collection longer than the cap is truncated, so an unexpectedly large
	// state array cannot turn one dispatch into unbounded work.
	big := make([]any, maxForEachIterations+250)
	for i := range big {
		big[i] = float64(i)
	}
	app := &model.App{
		Entry:       "main",
		Scenes:      map[string]*model.Node{"main": {Type: "scaffold"}},
		GlobalState: model.GlobalState{Initial: map[string]any{"big": big, "runs": 0.0}},
		Actions: map[string]*model.Action{
			"run": {ID: "run", Steps: []model.Step{{
				Type: "forEach", In: "{{ state.big }}",
				Steps: []model.Step{{Type: "state.increment", Path: "runs"}},
			}}},
		},
	}
	rt := New(app)
	rt.Dispatch("run", nil)
	if rt.State["runs"] != float64(maxForEachIterations) {
		t.Errorf("runs = %v, want the cap %d", rt.State["runs"], maxForEachIterations)
	}
}

func TestForEachNestsAndShadowsLikeRenderItemScopes(t *testing.T) {
	// A loop inside a loop: the inner alias is namespaced by `as`, so both
	// levels stay readable — the property that makes nested lists work.
	app := &model.App{
		Entry:  "main",
		Scenes: map[string]*model.Node{"main": {Type: "scaffold"}},
		GlobalState: model.GlobalState{Initial: map[string]any{
			"sections": []any{
				map[string]any{"name": "a", "rows": []any{"1", "2"}},
				map[string]any{"name": "b", "rows": []any{"3"}},
			},
			"log": []any{},
		}},
		Actions: map[string]*model.Action{
			"flatten": {ID: "flatten", Steps: []model.Step{{
				Type: "forEach", In: "{{ state.sections }}", As: "section",
				Steps: []model.Step{{
					Type: "forEach", In: "{{ section.rows }}", As: "row",
					Steps: []model.Step{{
						Type: "state.append", Path: "log",
						Value: "{{ section.name + rowIndex + '=' + row }}",
					}},
				}},
			}}},
		},
	}
	rt := New(app)
	rt.Dispatch("flatten", nil)
	log, _ := rt.State["log"].([]any)
	want := []string{"a0=1", "a1=2", "b0=3"}
	if len(log) != len(want) {
		t.Fatalf("log = %#v, want %v", log, want)
	}
	for i, w := range want {
		if log[i] != w {
			t.Errorf("log[%d] = %v, want %v", i, log[i], w)
		}
	}
}

func TestForEachBodyIsDepthCapped(t *testing.T) {
	// The loop body shares the `if` depth budget, so loops nested inside
	// branches inside loops terminate. Build maxIfDepth+2 nested forEach steps
	// over a one-element array; the innermost must be dropped.
	inner := []model.Step{{Type: "state.increment", Path: "deep"}}
	for i := 0; i < maxIfDepth+2; i++ {
		inner = []model.Step{{Type: "forEach", In: "{{ state.one }}", Steps: inner}}
	}
	app := &model.App{
		Entry:       "main",
		Scenes:      map[string]*model.Node{"main": {Type: "scaffold"}},
		GlobalState: model.GlobalState{Initial: map[string]any{"one": []any{"x"}}},
		Actions:     map[string]*model.Action{"deep": {ID: "deep", Steps: inner}},
	}
	rt := New(app)
	rt.Dispatch("deep", nil) // must terminate
	if got := rt.State["deep"]; got != nil {
		t.Errorf("the over-deep body ran (deep = %v), want it dropped by the depth guard", got)
	}
}

func TestForEachWithInvokeRespectsCallDepth(t *testing.T) {
	// An `invoke` in the body still goes through Dispatch's own call-depth
	// guard, so a loop that calls an action that loops cannot hang.
	app := &model.App{
		Entry:       "main",
		Scenes:      map[string]*model.Node{"main": {Type: "scaffold"}},
		GlobalState: model.GlobalState{Initial: map[string]any{"one": []any{"x"}, "n": 0.0}},
		Actions: map[string]*model.Action{
			"loop": {ID: "loop", Steps: []model.Step{{
				Type: "forEach", In: "{{ state.one }}",
				Steps: []model.Step{
					{Type: "state.increment", Path: "n"},
					{Type: "invoke", Name: "loop"},
				},
			}}},
		},
	}
	rt := New(app)
	rt.Dispatch("loop", nil) // must terminate
	if n, _ := rt.State["n"].(float64); n <= 0 || int(n) > maxInvokeDepth {
		t.Errorf("n = %v, want between 1 and the call-depth cap %d", rt.State["n"], maxInvokeDepth)
	}
}

func TestForEachAliasFallsBackForUnusableNames(t *testing.T) {
	// A reserved root or a non-identifier would break every {{state.x}} in the
	// body, so it degrades to the default alias (the loader warns about it).
	for _, as := range []string{"state", "t", "viewport", "route", "prop", "my alias", "2rows", ""} {
		alias, idx, first, last := listAliasNames(as)
		if as == "" || reservedScopeAliases[as] || !isIdent(as) {
			if alias != "item" || idx != "index" || first != "first" || last != "last" {
				t.Errorf("as=%q resolved to %q/%q/%q/%q, want the defaults", as, alias, idx, first, last)
			}
		}
	}
	// A usable alias namespaces its three derived keys.
	alias, idx, first, last := listAliasNames("row")
	if alias != "row" || idx != "rowIndex" || first != "rowFirst" || last != "rowLast" {
		t.Errorf("as=row resolved to %q/%q/%q/%q", alias, idx, first, last)
	}
}

func TestForEachMutatesTheArrayItIterates(t *testing.T) {
	// The bound element is the live value from state, so a body step that
	// rewrites the same path sees its own writes — the ordinary aliasing every
	// other step already has. Growing the array mid-loop must NOT extend the
	// iteration: the collection is snapshotted when the step starts.
	app := &model.App{
		Entry:  "main",
		Scenes: map[string]*model.Node{"main": {Type: "scaffold"}},
		GlobalState: model.GlobalState{Initial: map[string]any{
			"items": []any{"a", "b"},
			"seen":  []any{},
		}},
		Actions: map[string]*model.Action{
			"grow": {ID: "grow", Steps: []model.Step{{
				Type: "forEach", In: "{{ state.items }}",
				Steps: []model.Step{
					{Type: "state.append", Path: "seen", Value: "{{ item }}"},
					{Type: "state.append", Path: "items", Value: "{{ item + '!' }}"},
				},
			}}},
		},
	}
	rt := New(app)
	rt.Dispatch("grow", nil) // must terminate
	seen, _ := rt.State["seen"].([]any)
	if len(seen) != 2 {
		t.Errorf("seen = %#v, want exactly the two elements present when the loop started "+
			"(iterating a snapshot is what makes an appending body terminate)", seen)
	}
	items, _ := rt.State["items"].([]any)
	if len(items) != 4 {
		t.Errorf("items = %#v, want 4 after two appends", items)
	}
}

func TestForEachOverComputedCollection(t *testing.T) {
	// The collection may itself be a derived value.
	app := inboxApp()
	app.Computed = map[string]string{"unread": `{{ filter(state.messages, "!it.read") }}`}
	app.Actions["logUnread"] = &model.Action{ID: "logUnread", Steps: []model.Step{{
		Type: "forEach", In: "{{ computed.unread }}",
		Steps: []model.Step{{Type: "state.append", Path: "log", Value: "{{ item.id }}"}},
	}}}
	rt := New(app)
	rt.Dispatch("logUnread", nil)
	log, _ := rt.State["log"].([]any)
	if len(log) != 2 || log[0] != "m1" || log[1] != "m2" {
		t.Errorf("log = %#v, want the two unread ids", log)
	}
}
