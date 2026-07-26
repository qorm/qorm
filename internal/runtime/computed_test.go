package runtime

// Runtime behaviour of DERIVED (computed) values.
//
// The contract under test:
//   - a declaration is published read-only at state.computed.<name>, so a scene
//     binding reads {{ state.computed.total }} and an action — which also sees
//     every top-level state key bare — reads {{ computed.total }};
//   - it is evaluated ONCE per frame boundary (New, the end of a top-level
//     Dispatch, a mid-action `render`, and each host render via
//     RunPendingEnter), never once per binding;
//   - a value may read another one, in declaration-independent order;
//   - a dependency cycle publishes nothing instead of recursing;
//   - the namespace is read-only: an action step that targets it is dropped;
//   - an app that declares nothing keeps "computed" as an ordinary state key.

import (
	"testing"

	"github.com/qorm/qorm/internal/model"
)

// cartApp is the canonical derived-value app: a line-item cart whose totals are
// declared once and read everywhere.
func cartApp() *model.App {
	return &model.App{
		Entry:  "main",
		Scenes: map[string]*model.Node{"main": {Type: "scaffold"}},
		GlobalState: model.GlobalState{Initial: map[string]any{
			"items": []any{
				map[string]any{"id": "a", "price": 2.5, "qty": 2.0},
				map[string]any{"id": "b", "price": 5.0, "qty": 1.0},
			},
		}},
		Computed: map[string]string{
			"subtotal": `{{ sum(map(state.items, "it.price * it.qty")) }}`,
			"isEmpty":  "{{ len(state.items) == 0 }}",
			"withTax":  "{{ computed.subtotal * 2 }}", // reads another derived value
		},
		Actions: map[string]*model.Action{
			"addItem": {ID: "addItem", Steps: []model.Step{
				{Type: "state.appendObject", Path: "items", Object: map[string]string{
					"id": "'c'", "price": "1", "qty": "3",
				}},
			}},
			"clear": {ID: "clear", Steps: []model.Step{{Type: "state.clear", Path: "items"}}},
		},
	}
}

func computedOf(t *testing.T, rt *Runtime, name string) any {
	t.Helper()
	m, ok := rt.State[model.ComputedNamespace].(map[string]any)
	if !ok {
		t.Fatalf("state.%s is not published (got %#v)", model.ComputedNamespace, rt.State[model.ComputedNamespace])
	}
	return m[name]
}

func TestComputedPublishedFromTheFirstFrame(t *testing.T) {
	rt := New(cartApp())
	if got := computedOf(t, rt, "subtotal"); got != 10.0 {
		t.Errorf("subtotal = %v, want 10 (New must evaluate before anything renders)", got)
	}
	if got := computedOf(t, rt, "isEmpty"); got != false {
		t.Errorf("isEmpty = %v, want false", got)
	}
	// A derived value that reads another one sees it already filled in,
	// whatever the declaration order.
	if got := computedOf(t, rt, "withTax"); got != 20.0 {
		t.Errorf("withTax = %v, want 20 (must read the already-evaluated subtotal)", got)
	}
}

func TestComputedReadableFromBothScopes(t *testing.T) {
	rt := New(cartApp())
	// Scene bindings: the state-rooted spelling.
	if got := EvalBinding("{{ state.computed.subtotal }}", rt.sceneCtx()); got != 10.0 {
		t.Errorf("scene binding state.computed.subtotal = %v, want 10", got)
	}
	if got := EvalBinding("Total: {{ state.computed.subtotal }}", rt.sceneCtx()); got != "Total: 10" {
		t.Errorf("interpolated scene binding = %v, want %q", got, "Total: 10")
	}
	// ComputedVars is the accessor a host adds to its own binding context to
	// offer the shorter bare spelling.
	if got := rt.ComputedVars()["subtotal"]; got != 10.0 {
		t.Errorf("ComputedVars()[subtotal] = %v, want 10", got)
	}
	// Action expressions: the bare spelling, via the top-level state flattening.
	rt.App.Actions["record"] = &model.Action{ID: "record", Steps: []model.Step{
		{Type: "state.set", Path: "seenBare", Value: "{{ computed.subtotal }}"},
		{Type: "state.set", Path: "seenRooted", Value: "{{ state.computed.withTax }}"},
	}}
	rt.Dispatch("record", nil)
	if rt.State["seenBare"] != 10.0 {
		t.Errorf("action bare computed.subtotal = %v, want 10", rt.State["seenBare"])
	}
	if rt.State["seenRooted"] != 20.0 {
		t.Errorf("action state.computed.withTax = %v, want 20", rt.State["seenRooted"])
	}
}

func TestComputedRefreshesAtTheDispatchBoundary(t *testing.T) {
	rt := New(cartApp())
	rt.Dispatch("addItem", nil)
	if got := computedOf(t, rt, "subtotal"); got != 13.0 {
		t.Errorf("subtotal after append = %v, want 13 (10 + 1*3)", got)
	}
	if got := computedOf(t, rt, "withTax"); got != 26.0 {
		t.Errorf("withTax after append = %v, want 26", got)
	}
	rt.Dispatch("clear", nil)
	if got := computedOf(t, rt, "isEmpty"); got != true {
		t.Errorf("isEmpty after clear = %v, want true", got)
	}
	if got := computedOf(t, rt, "subtotal"); got != 0.0 {
		t.Errorf("subtotal after clear = %v, want 0", got)
	}
}

func TestComputedRefreshedOncePerDispatchNotPerStep(t *testing.T) {
	// The memoisation property, observed from the outside: within one dispatch
	// the derived view is FROZEN at the value the last boundary published. If
	// the runtime re-evaluated per step (or per read) the two reads below would
	// disagree with each other and with the pre-dispatch value. A nested invoke
	// shares the outer boundary, so it does not create one either.
	app := &model.App{
		Entry:       "main",
		Scenes:      map[string]*model.Node{"main": {Type: "scaffold"}},
		GlobalState: model.GlobalState{Initial: map[string]any{"n": 0.0}},
		Computed:    map[string]string{"double": "{{ state.n * 2 }}"},
		Actions: map[string]*model.Action{
			"bump": {ID: "bump", Steps: []model.Step{
				{Type: "state.increment", Path: "n"},
				{Type: "state.increment", Path: "n"},
				{Type: "invoke", Name: "inner"},
				{Type: "state.set", Path: "r1", Value: "{{ computed.double }}"},
				{Type: "state.set", Path: "r2", Value: "{{ computed.double }}"},
			}},
			"inner": {ID: "inner", Steps: []model.Step{{Type: "state.increment", Path: "n"}}},
		},
	}
	rt := New(app)
	rt.Dispatch("bump", nil)

	if rt.State["n"] != 3.0 {
		t.Fatalf("n = %v, want 3", rt.State["n"])
	}
	if rt.State["r1"] != 0.0 || rt.State["r2"] != 0.0 {
		t.Errorf("mid-dispatch reads = %v / %v, want the stable pre-dispatch 0 for both", rt.State["r1"], rt.State["r2"])
	}
	// After the boundary the value is current again — once, for all of it.
	if got := computedOf(t, rt, "double"); got != 6.0 {
		t.Errorf("double after dispatch = %v, want 6", got)
	}
}

func TestComputedRefreshedByRenderStepAndPendingEnter(t *testing.T) {
	app := cartApp()
	app.Actions["stage"] = &model.Action{ID: "stage", Steps: []model.Step{
		{Type: "state.append", Path: "items", Value: "{{ state.items[0] }}"},
		{Type: "render"},
	}}
	rt := New(app)
	var atFrame any
	rt.Commit = func() { atFrame = computedOf(t, rt, "subtotal") }
	rt.Dispatch("stage", nil)
	if atFrame != 15.0 {
		t.Errorf("subtotal at the intermediate frame = %v, want 15 — a published frame must carry a consistent derived view", atFrame)
	}

	// A host that writes state directly (an agent over MCP, a viewport report)
	// gets its refresh at the render choke point.
	rt2 := New(cartApp())
	rt2.State["items"] = []any{}
	rt2.RunPendingEnter()
	if got := computedOf(t, rt2, "isEmpty"); got != true {
		t.Errorf("isEmpty after a direct host write + RunPendingEnter = %v, want true", got)
	}
}

func TestComputedCycleIsInertNotRecursive(t *testing.T) {
	app := &model.App{
		Entry:  "main",
		Scenes: map[string]*model.Node{"main": {Type: "scaffold"}},
		Computed: map[string]string{
			"a":    "{{ computed.b + 1 }}",
			"b":    "{{ computed.a + 1 }}",
			"safe": "{{ 1 + 1 }}",
		},
	}
	rt := New(app) // must return: a cycle can never recurse
	if got := computedOf(t, rt, "safe"); got != 2.0 {
		t.Errorf("safe = %v, want 2 — a cycle elsewhere must not stop the rest", got)
	}
	m := rt.State[model.ComputedNamespace].(map[string]any)
	if _, ok := m["a"]; ok {
		t.Errorf("cyclic value 'a' was published as %v, want absent (reads as nil)", m["a"])
	}
	if _, ok := m["b"]; ok {
		t.Errorf("cyclic value 'b' was published as %v, want absent", m["b"])
	}
	if got := EvalBinding("{{ state.computed.a }}", rt.sceneCtx()); got != nil {
		t.Errorf("binding a cyclic value = %v, want nil", got)
	}
}

func TestComputedIsReadOnly(t *testing.T) {
	app := cartApp()
	app.Actions["cheat"] = &model.Action{ID: "cheat", Steps: []model.Step{
		{Type: "state.set", Path: "computed.subtotal", Value: "{{ 999 }}"},
		{Type: "state.set", Path: "computed", Value: "{{ 'clobbered' }}"},
		{Type: "state.set", Path: "ok", Value: "{{ 'ran' }}"},
	}}
	rt := New(app)
	rt.Dispatch("cheat", nil)
	if got := computedOf(t, rt, "subtotal"); got != 10.0 {
		t.Errorf("subtotal after a write attempt = %v, want the derived 10", got)
	}
	if rt.State["ok"] != "ran" {
		t.Errorf("dropping the read-only writes must not abort the rest of the action: %#v", rt.State["ok"])
	}
	// The http result/error paths are guarded too (no request is made: the
	// step is dropped before it runs).
	app.Actions["cheatHTTP"] = &model.Action{ID: "cheatHTTP", Steps: []model.Step{
		{Type: "http.get", URL: "http://127.0.0.1:1/never", Result: "computed.subtotal"},
		{Type: "http.get", URL: "http://127.0.0.1:1/never", Path: "resp", Error: "computed.err"},
	}}
	rt.Dispatch("cheatHTTP", nil)
	if got := computedOf(t, rt, "subtotal"); got != 10.0 {
		t.Errorf("subtotal after an http result write = %v, want 10", got)
	}
	if _, ok := rt.State["resp"]; ok {
		t.Error("the second http step should have been dropped by the error-path guard")
	}
}

func TestNoComputedDeclarationsLeavesStateUntouched(t *testing.T) {
	// Backward compatibility: without declarations, "computed" is an ordinary
	// state key an old app may already own — writable and never republished.
	app := &model.App{
		Entry:       "main",
		Scenes:      map[string]*model.Node{"main": {Type: "scaffold"}},
		GlobalState: model.GlobalState{Initial: map[string]any{"computed": "mine"}},
		Actions: map[string]*model.Action{
			"w": {ID: "w", Steps: []model.Step{{Type: "state.set", Path: "computed.deep", Value: "{{ 'still mine' }}"}}},
		},
	}
	rt := New(app)
	if rt.State["computed"] != "mine" {
		t.Fatalf("computed = %#v, want the app's own value", rt.State["computed"])
	}
	if len(rt.ComputedVars()) != 0 {
		t.Errorf("ComputedVars = %#v, want empty when the key is not a derived namespace", rt.ComputedVars())
	}
	rt.Dispatch("w", nil)
	m, ok := rt.State["computed"].(map[string]any)
	if !ok || m["deep"] != "still mine" {
		t.Errorf("an app without declarations must keep writing its own 'computed' key: %#v", rt.State["computed"])
	}
}

func TestComputedSurvivesClone(t *testing.T) {
	rt := New(cartApp())
	sim := rt.Clone()
	if got := computedOf(t, sim, "subtotal"); got != 10.0 {
		t.Errorf("clone subtotal = %v, want 10", got)
	}
	sim.Dispatch("clear", nil)
	if got := computedOf(t, sim, "isEmpty"); got != true {
		t.Errorf("clone isEmpty after clear = %v, want true", got)
	}
	if got := computedOf(t, rt, "isEmpty"); got != false {
		t.Errorf("the live runtime's derived view changed from a clone dispatch: %v", got)
	}
}
