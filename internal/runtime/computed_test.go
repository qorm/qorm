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

	"github.com/qorm/platform/internal/model"
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

// TestSetStatePathIsTheOneWriteHostsMake pins the host-facing write: a value
// written from OUTSIDE a dispatch (the MCP qorm_set_state tool is the only
// caller today) lands where an app binding reads it, and cannot forge a derived
// value. Without a shared entry point each host reimplemented the assignment —
// and the one that did wrote a literal "a.b" key past both the loader's refusal
// and applyStep's.
func TestSetStatePathIsTheOneWriteHostsMake(t *testing.T) {
	rt := New(cartApp())

	if !rt.SetStatePath("customer.name", "ada") {
		t.Fatal("a plain nested write must be accepted")
	}
	if got := rt.StatePath("customer.name"); got != "ada" {
		t.Errorf("StatePath = %v, want ada — a dotted path must NEST, like state.set", got)
	}
	if _, flat := rt.State["customer.name"]; flat {
		t.Error("the write created a literal dotted key, which no binding can read")
	}

	// The derived namespace is refused, whole or by name, and stays derived.
	for _, path := range []string{"computed", "computed.subtotal", ""} {
		if rt.SetStatePath(path, "OWNED") {
			t.Errorf("path %q was accepted", path)
		}
	}
	if got := rt.ComputedVars()["subtotal"]; got != 10.0 {
		t.Errorf("subtotal = %v, want the derived 10", got)
	}

	// An app that declares nothing derived keeps "computed" as an ordinary key,
	// exactly as applyStep does.
	plain := New(&model.App{Entry: "main", Scenes: map[string]*model.Node{"main": {Type: "scaffold"}}})
	if !plain.SetStatePath("computed.mine", 1.0) {
		t.Error("an app without declarations must keep its own 'computed' key writable")
	}
	if got := plain.StatePath("computed.mine"); got != 1.0 {
		t.Errorf("plain nested write = %v, want 1", got)
	}
}

// ---- the reserved roots ---------------------------------------------------------

// shadowApp is a minimal derived app whose values are spelled three different
// ways, so a broken evaluation context shows up whichever spelling an author
// used: `double` reads the state root, `viaBare` reads a bare state key, and
// `chained` reads another derived value.
func shadowApp() *model.App {
	return &model.App{
		Entry:       "main",
		Scenes:      map[string]*model.Node{"main": {Type: "scaffold"}},
		GlobalState: model.GlobalState{Initial: map[string]any{"n": 1.0}},
		Computed: map[string]string{
			"double":  "{{ state.n * 2 }}",
			"viaBare": "{{ n * 3 }}",
			"chained": "{{ computed.double + 1 }}",
		},
		Actions: map[string]*model.Action{
			"bump": {ID: "bump", Steps: []model.Step{
				{Type: "state.set", Path: "n", Value: "{{ state.n + 1 }}"},
			}},
		},
	}
}

// assertDerived checks the whole derived view against the one input it depends
// on, through both the published namespace and a scene binding.
func assertDerived(t *testing.T, rt *Runtime, n float64) {
	t.Helper()
	if got := rt.State["n"]; got != n {
		t.Fatalf("state.n = %v, want %v", got, n)
	}
	for name, want := range map[string]any{"double": n * 2, "viaBare": n * 3, "chained": n*2 + 1} {
		if got := computedOf(t, rt, name); got != want {
			t.Errorf("computed.%s = %v, want %v", name, got, want)
		}
	}
	if got := EvalBinding("{{ state.computed.double }}", rt.sceneCtx()); got != n*2 {
		t.Errorf("scene binding state.computed.double = %v, want %v", got, n*2)
	}
}

// TestStateKeyNamedStateCannotShadowTheStateRoot is the general case behind the
// three step paths below: whatever put it there, a top-level state key called
// `state` must not repoint `{{ state.x }}` at itself.
//
// The regression it pins was total and silent — every derived value in the app
// evaluated to nothing, on every frame, with no diagnostic anywhere — because
// the derived context laid the bare state keys down ON TOP of the roots.
func TestStateKeyNamedStateCannotShadowTheStateRoot(t *testing.T) {
	app := shadowApp()
	app.GlobalState.Initial["state"] = map[string]any{"n": 99.0}
	app.GlobalState.Initial["t"] = "not the catalog"
	app.GlobalState.Initial["viewport"] = "not the viewport"
	rt := New(app)
	assertDerived(t, rt, 1)

	// And it keeps working across a dispatch: the action context is built the
	// same way, so `{{ state.n + 1 }}` inside `bump` must read the real root.
	rt.Dispatch("bump", nil)
	assertDerived(t, rt, 2)

	// The colliding keys are still readable — they are shadowed, not deleted.
	if got := EvalBinding("{{ state.state.n }}", rt.sceneCtx()); got != 99.0 {
		t.Errorf("state.state.n = %v, want 99 — the key is shadowed in the roots, not dropped", got)
	}
}

// TestStateRootedStepPathsCannotBreakDerivedValues walks the three ways an
// author can write a state-rooted path into an action step. Each one used to
// have a different (and in two cases catastrophic) outcome; all three must now
// leave the derived view intact, dispatch after dispatch.
func TestStateRootedStepPathsCannotBreakDerivedValues(t *testing.T) {
	cases := []struct {
		name string
		path string
		// wrote is the top-level key the step is expected to leave behind:
		// "state" when the write is merely mis-rooted, "" when the whole step
		// is dropped as a write into the read-only namespace.
		wrote string
	}{
		// A plain mis-rooted write. It really does create a `state` key (that
		// is what the path says), and that key must stay harmless.
		{"mis-rooted plain path", "state.hello", "state"},
		// The spelling the docs teach for READING a derived value, copied into
		// a step. Refused as a write into the namespace.
		{"binding spelling", "state.computed.double", ""},
		// The literal namespace path. Refused as it always was.
		{"literal namespace", "computed.double", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			app := shadowApp()
			app.Actions["poison"] = &model.Action{ID: "poison", Steps: []model.Step{
				{Type: "state.set", Path: tc.path, Value: "{{ 'x' }}"},
				{Type: "state.set", Path: "n", Value: "{{ state.n + 1 }}"},
				{Type: "state.set", Path: "seenBare", Value: "{{ n }}"},
				{Type: "state.set", Path: "seenRooted", Value: "{{ state.computed.double }}"},
			}}
			rt := New(app)
			assertDerived(t, rt, 1)

			rt.Dispatch("poison", nil)
			// The rest of the action ran against an intact context: the state
			// root, the bare spelling and the derived namespace all resolved.
			assertDerived(t, rt, 2)
			// The bare spelling is the dispatch-entry SNAPSHOT of the state key
			// (the context is built once per dispatch), so it reads the value
			// from before step 2 — 1 on the first dispatch. What matters here is
			// that it resolves at all rather than reading nothing.
			if rt.State["seenBare"] != 1.0 {
				t.Errorf("bare `n` inside the action = %v, want the dispatch-entry 1", rt.State["seenBare"])
			}
			if rt.State["seenRooted"] != 2.0 {
				t.Errorf("state.computed.double inside the action = %v, want the pre-dispatch 2", rt.State["seenRooted"])
			}
			// Not a one-frame recovery: the poisoned context used to be rebuilt
			// from the poisoned state every single frame.
			rt.Dispatch("poison", nil)
			assertDerived(t, rt, 3)

			got, present := rt.State["state"]
			if tc.wrote == "" {
				if present {
					t.Errorf("the step must be dropped whole, but it wrote state.state = %#v", got)
				}
			} else if !present {
				t.Errorf("the mis-rooted write must still happen (the path says so), but state.state is absent")
			}
		})
	}
}

// TestActionArgNamedStateCannotShadowTheStateRoot: args win over state keys,
// but not over the roots. An arg is author-supplied too (a widget handler's
// args, an `invoke`'s args), so the same collision reaches the same context.
func TestActionArgNamedStateCannotShadowTheStateRoot(t *testing.T) {
	app := shadowApp()
	app.Actions["bump"].Steps = append(app.Actions["bump"].Steps,
		model.Step{Type: "state.set", Path: "seen", Value: "{{ state.n }}"})
	rt := New(app)
	rt.Dispatch("bump", map[string]any{"state": "hijacked", "t": 1.0, "viewport": 2.0})
	if rt.State["seen"] != 2.0 {
		t.Errorf("state.n read through a hijacking arg = %v, want 2", rt.State["seen"])
	}
	assertDerived(t, rt, 2)
}

// ---- the read-only namespace, enforced at DISPATCH time -------------------------
//
// The loader reports a step that writes into the namespace, but the loader is
// not the enforcement: an app can be dispatched without ever being loaded (a
// host that builds a model.App directly, a bundle whose diagnostics were only
// warnings, an agent driving qorm_dispatch). applyStep's drop is what actually
// holds, and these tests are what pin it — the assertions below are chosen so
// they FAIL if that guard is removed, which the ones asserting only the
// post-dispatch state cannot do (the frame-boundary refresh republishes the
// namespace and hides the write).

// TestComputedWriteIsDroppedMidDispatchNotJustOverwritten reads the namespace
// back INSIDE the same action. That is the only window in which a forged value
// is observable — and the window a real app looks through, since a later step
// branching on `{{ state.computed.x }}` is exactly how an author would use one.
func TestComputedWriteIsDroppedMidDispatchNotJustOverwritten(t *testing.T) {
	app := cartApp()
	app.Actions["cheat"] = &model.Action{ID: "cheat", Steps: []model.Step{
		{Type: "state.set", Path: "computed.subtotal", Value: "{{ 999 }}"},
		{Type: "state.set", Path: "midRooted", Value: "{{ state.computed.subtotal }}"},
		{Type: "state.set", Path: "midBranch", Value: "{{ state.computed.subtotal > 100 ? 'forged' : 'derived' }}"},
	}}
	rt := New(app)
	rt.Dispatch("cheat", nil)
	if rt.State["midRooted"] != 10.0 {
		t.Errorf("a later step read the namespace as %v, want the derived 10 — the write was not dropped", rt.State["midRooted"])
	}
	if rt.State["midBranch"] != "derived" {
		t.Errorf("a later step branched on a forged derived value: %v", rt.State["midBranch"])
	}
	if got := computedOf(t, rt, "subtotal"); got != 10.0 {
		t.Errorf("subtotal after the dispatch = %v, want 10", got)
	}
}

// TestComputedTargetDropsTheWholeStepIncludingTheRequest is the promise the
// docs and the changelog both make in so many words: the WHOLE step is dropped,
// "so a gated http.get never even issues its request". A real backend counts
// arrivals, so the assertion is about the wire and not about state.
func TestComputedTargetDropsTheWholeStepIncludingTheRequest(t *testing.T) {
	for _, tc := range []struct {
		name string
		step model.Step
	}{
		{"result", model.Step{Result: "computed.subtotal"}},
		{"path", model.Step{Path: "computed.subtotal"}},
		{"error", model.Step{Path: "resp", Error: "computed.err"}},
		{"pending", model.Step{Path: "resp", Pending: "computed.busy"}},
		// The binding spelling reaches the same guard: an author who reads
		// `{{ state.computed.x }}` in a scene writes it into a step's `result`
		// without a second thought, and that must not fire a request either.
		{"binding spelling", model.Step{Result: "state.computed.subtotal"}},
	} {
		for _, async := range []bool{false, true} {
			mode := "sync"
			if async {
				mode = "async"
			}
			t.Run(tc.name+"/"+mode, func(t *testing.T) {
				gated := newCountingBackend(t, `{"n":1}`)
				control := newCountingBackend(t, `{"n":1}`)
				step := tc.step
				step.Type, step.URL, step.Async = "http.get", gated.srv.URL, async
				app := cartApp()
				app.Actions["fetch"] = &model.Action{ID: "fetch", Steps: []model.Step{
					step,
					// The control proves the backend really does count: same
					// request, an ordinary target, and it must arrive.
					{Type: "http.get", URL: control.srv.URL, Result: "ok", Async: async},
				}}
				rt := New(app)
				if async {
					rt.Async = syncHost
				}
				rt.Dispatch("fetch", nil)

				if n := gated.arrived.Load(); n != 0 {
					t.Errorf("the gated step issued %d request(s): the drop must happen BEFORE the round trip", n)
				}
				if n := control.arrived.Load(); n != 1 {
					t.Errorf("the control step issued %d request(s), want 1 — the backend is not counting", n)
				}
				if _, ok := rt.State["resp"]; ok {
					t.Errorf("the gated step wrote its response anyway: %#v", rt.State["resp"])
				}
				if got := computedOf(t, rt, "subtotal"); got != 10.0 {
					t.Errorf("subtotal = %v, want the derived 10", got)
				}
				if rt.State["ok"] == nil {
					t.Error("dropping the gated step must not abort the rest of the action")
				}
			})
		}
	}
}

// TestBareSpellingIsADispatchEntrySnapshot pins what separates the two ways of
// reading a derived value INSIDE an action, which the two spellings' otherwise
// identical readings hide.
//
// An action context is built once, at dispatch entry: `state` is the live map,
// while every top-level key laid out bare is a copy of what that key held when
// the action started. A `render` step republishes the derived namespace by
// REPLACING the map at state.computed, so after one:
//
//	{{ state.computed.x }}  -> the refreshed value (read through the live root)
//	{{ computed.x }}        -> the dispatch-entry value
//
// That is not a quirk of `computed`: it is how every bare spelling works, as
// the plain `n` below shows. The docs say so rather than promising the two
// spellings are interchangeable, which they are not once a frame lands
// mid-action.
func TestBareSpellingIsADispatchEntrySnapshot(t *testing.T) {
	app := &model.App{
		Entry:       "main",
		Scenes:      map[string]*model.Node{"main": {Type: "scaffold"}},
		GlobalState: model.GlobalState{Initial: map[string]any{"n": 1.0}},
		Computed:    map[string]string{"double": "{{ state.n * 2 }}"},
		Actions: map[string]*model.Action{"stage": {ID: "stage", Steps: []model.Step{
			{Type: "state.set", Path: "n", Value: "{{ 5 }}"},
			{Type: "render"}, // a frame boundary mid-action: derived values refresh
			{Type: "state.set", Path: "bare", Value: "{{ computed.double }}"},
			{Type: "state.set", Path: "rooted", Value: "{{ state.computed.double }}"},
			{Type: "state.set", Path: "bareN", Value: "{{ n }}"},
			{Type: "state.set", Path: "rootedN", Value: "{{ state.n }}"},
		}}},
	}
	rt := New(app)
	rt.Commit = func() {} // a frame sink, or `render` is a no-op
	rt.Dispatch("stage", nil)

	for _, tc := range []struct {
		key  string
		want float64
		why  string
	}{
		{"rooted", 10.0, "the state root is live, so the refreshed derived value is visible"},
		{"bare", 2.0, "the bare spelling is the dispatch-entry snapshot"},
		{"rootedN", 5.0, "the same split for an ordinary state key: rooted is live"},
		{"bareN", 1.0, "…and bare is the dispatch-entry snapshot"},
	} {
		if got := rt.State[tc.key]; got != tc.want {
			t.Errorf("%s = %v, want %v — %s", tc.key, got, tc.want, tc.why)
		}
	}
}
