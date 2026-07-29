package runtime

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/qorm/qorm/internal/model"
)

// Tests for the execution-model steps: `if` (conditional branches), `invoke`
// (action calls action), the http.* onSuccess/onError result branches, and the
// scene onEnter lifecycle hook.

func execApp(actions map[string]*model.Action) *model.App {
	if actions == nil {
		actions = map[string]*model.Action{}
	}
	return &model.App{
		Entry:   "main",
		Scenes:  map[string]*model.Node{"main": {Type: "view", ID: "root"}, "detail": {Type: "view", ID: "droot"}},
		Actions: actions,
	}
}

// ---- if step ----------------------------------------------------------------

func TestIfStepBranches(t *testing.T) {
	app := execApp(map[string]*model.Action{
		"check": {ID: "check", Steps: []model.Step{{
			Type:      "if",
			Condition: "{{ state.count > 0 }}",
			Then:      []model.Step{{Type: "state.set", Path: "result", Value: "positive"}},
			Else:      []model.Step{{Type: "state.set", Path: "result", Value: "zero"}},
		}}},
	})
	rt := New(app)
	rt.State["count"] = float64(3)
	rt.Dispatch("check", nil)
	if rt.State["result"] != "positive" {
		t.Errorf("truthy condition must run then: got %v", rt.State["result"])
	}
	rt.State["count"] = float64(0)
	rt.Dispatch("check", nil)
	if rt.State["result"] != "zero" {
		t.Errorf("falsy condition must run else: got %v", rt.State["result"])
	}
}

func TestIfStepTruthiness(t *testing.T) {
	// The condition uses the expression language's truthiness: empty string,
	// empty array, 0 and false are falsy; non-empty values are truthy.
	cases := []struct {
		val  any
		want string
	}{
		{"", "no"}, {"x", "yes"},
		{float64(0), "no"}, {float64(1), "yes"},
		{false, "no"}, {true, "yes"},
		{[]any{}, "no"}, {[]any{1.0}, "yes"},
		{nil, "no"},
	}
	app := execApp(map[string]*model.Action{
		"check": {ID: "check", Steps: []model.Step{{
			Type:      "if",
			Condition: "{{ state.v }}",
			Then:      []model.Step{{Type: "state.set", Path: "out", Value: "yes"}},
			Else:      []model.Step{{Type: "state.set", Path: "out", Value: "no"}},
		}}},
	})
	for _, c := range cases {
		rt := New(app)
		rt.State["v"] = c.val
		rt.Dispatch("check", nil)
		if rt.State["out"] != c.want {
			t.Errorf("v=%v: want %q got %v", c.val, c.want, rt.State["out"])
		}
	}
}

func TestIfStepMissingConditionRunsElse(t *testing.T) {
	// A missing condition evaluates "" -> falsy, so the else branch runs (and
	// the loader flags the step at load time).
	app := execApp(map[string]*model.Action{
		"check": {ID: "check", Steps: []model.Step{{
			Type: "if",
			Then: []model.Step{{Type: "state.set", Path: "out", Value: "then"}},
			Else: []model.Step{{Type: "state.set", Path: "out", Value: "else"}},
		}}},
	})
	rt := New(app)
	rt.Dispatch("check", nil)
	if rt.State["out"] != "else" {
		t.Errorf("missing condition must run else: got %v", rt.State["out"])
	}
}

func TestIfStepNested(t *testing.T) {
	app := execApp(map[string]*model.Action{
		"grade": {ID: "grade", Steps: []model.Step{{
			Type:      "if",
			Condition: "{{ state.n >= 50 }}",
			Then: []model.Step{{
				Type:      "if",
				Condition: "{{ state.n >= 90 }}",
				Then:      []model.Step{{Type: "state.set", Path: "grade", Value: "A"}},
				Else:      []model.Step{{Type: "state.set", Path: "grade", Value: "B"}},
			}},
			Else: []model.Step{{Type: "state.set", Path: "grade", Value: "F"}},
		}}},
	})
	for _, c := range []struct {
		n    float64
		want string
	}{{95, "A"}, {60, "B"}, {10, "F"}} {
		rt := New(app)
		rt.State["n"] = c.n
		rt.Dispatch("grade", nil)
		if rt.State["grade"] != c.want {
			t.Errorf("n=%v: want %q got %v", c.n, c.want, rt.State["grade"])
		}
	}
}

func TestIfStepDepthGuard(t *testing.T) {
	// Build an if-chain nested beyond maxIfDepth; the innermost set must be
	// skipped while the guard depth still executes.
	inner := []model.Step{{Type: "state.set", Path: "deep", Value: "reached"}}
	for i := 0; i < maxIfDepth+4; i++ {
		inner = []model.Step{{Type: "if", Condition: "{{ 1 }}", Then: inner}}
	}
	app := execApp(map[string]*model.Action{"go": {ID: "go", Steps: inner}})
	rt := New(app)
	rt.Dispatch("go", nil)
	if _, ok := rt.State["deep"]; ok {
		t.Error("steps nested beyond maxIfDepth must not execute")
	}
}

// ---- invoke step ------------------------------------------------------------

func TestInvokeStepCallsActionWithArgs(t *testing.T) {
	app := execApp(map[string]*model.Action{
		"outer": {ID: "outer", Steps: []model.Step{
			{Type: "state.set", Path: "trace", Value: "outer"},
			{Type: "invoke", Name: "inner", Args: map[string]string{"delta": "{{ state.base + 1 }}"}},
		}},
		"inner": {ID: "inner", Steps: []model.Step{
			// The invoke's evaluated args merge into the callee's scope like
			// an event invoke's args: bare `delta` resolves here.
			{Type: "state.set", Path: "sum", Value: "{{ delta * 2 }}"},
		}},
	})
	rt := New(app)
	rt.State["base"] = float64(4)
	rt.Dispatch("outer", nil)
	if rt.State["sum"] != float64(10) {
		t.Errorf("invoke args must evaluate in the caller's context and bind in the callee: got %v", rt.State["sum"])
	}
}

func TestInvokeStepUnknownActionIsNoOp(t *testing.T) {
	app := execApp(map[string]*model.Action{
		"outer": {ID: "outer", Steps: []model.Step{
			{Type: "invoke", Name: "missing"},
			{Type: "state.set", Path: "after", Value: "ran"},
		}},
	})
	rt := New(app)
	rt.Dispatch("outer", nil)
	if rt.State["after"] != "ran" {
		t.Error("an invoke of a missing action must be ignored and later steps still run")
	}
}

func TestInvokeStepRecursionGuard(t *testing.T) {
	// A self-recursive action must terminate via the call-depth guard.
	app := execApp(map[string]*model.Action{
		"loop": {ID: "loop", Steps: []model.Step{
			{Type: "state.increment", Path: "n"},
			{Type: "invoke", Name: "loop"},
		}},
	})
	rt := New(app)
	rt.Dispatch("loop", nil) // must return, not hang
	if got := rt.State["n"]; got != float64(maxInvokeDepth) {
		t.Errorf("recursion must be capped at maxInvokeDepth (%d): got %v", maxInvokeDepth, got)
	}
}

func TestInvokeStepMutualRecursionGuard(t *testing.T) {
	app := execApp(map[string]*model.Action{
		"a": {ID: "a", Steps: []model.Step{{Type: "state.increment", Path: "n"}, {Type: "invoke", Name: "b"}}},
		"b": {ID: "b", Steps: []model.Step{{Type: "invoke", Name: "a"}}},
	})
	rt := New(app)
	rt.Dispatch("a", nil) // must terminate
	if n, _ := rt.State["n"].(float64); n <= 0 || n > float64(maxInvokeDepth) {
		t.Errorf("mutual recursion must be depth-capped: n=%v", rt.State["n"])
	}
}

func TestInvokeInsideIfBranch(t *testing.T) {
	app := execApp(map[string]*model.Action{
		"submit": {ID: "submit", Steps: []model.Step{{
			Type:      "if",
			Condition: "{{ state.ok }}",
			Then:      []model.Step{{Type: "invoke", Name: "celebrate"}},
		}}},
		"celebrate": {ID: "celebrate", Steps: []model.Step{{Type: "state.set", Path: "msg", Value: "done"}}},
	})
	rt := New(app)
	rt.State["ok"] = true
	rt.Dispatch("submit", nil)
	if rt.State["msg"] != "done" {
		t.Errorf("invoke inside an if branch must dispatch: got %v", rt.State["msg"])
	}
}

// ---- http result branches ---------------------------------------------------

func TestHTTPOnSuccessSeesResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"user":{"name":"ada"},"n":7}`))
	}))
	defer srv.Close()
	step := model.Step{
		Type: "http.get", URL: srv.URL, Result: "resp", Error: "err",
		OnSuccess: []model.Step{
			// The decoded response is bound as {{ response }} for the branch;
			// the Result path was written first, so state.resp works too.
			{Type: "state.set", Path: "who", Value: "{{ response.user.name }}"},
			{Type: "state.set", Path: "fromState", Value: "{{ state.resp.n }}"},
			{Type: "state.set", Path: "phase", Value: "loaded"},
		},
		OnError: []model.Step{{Type: "state.set", Path: "phase", Value: "failed"}},
	}
	rt := dispatchHTTP(t, map[string]any{"err": "stale"}, step, nil)
	if rt.State["who"] != "ada" {
		t.Errorf("onSuccess must see {{ response }}: got %v", rt.State["who"])
	}
	if rt.State["fromState"] != float64(7) {
		t.Errorf("onSuccess must run after Result is written: got %v", rt.State["fromState"])
	}
	if rt.State["phase"] != "loaded" {
		t.Errorf("onSuccess branch must run on 2xx: got %v", rt.State["phase"])
	}
	if rt.State["err"] != "" {
		t.Errorf("stale error must still be cleared: got %v", rt.State["err"])
	}
}

func TestHTTPOnErrorSeesError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer srv.Close()
	step := model.Step{
		Type: "http.get", URL: srv.URL, Result: "resp", Error: "err",
		OnSuccess: []model.Step{{Type: "state.set", Path: "phase", Value: "loaded"}},
		OnError: []model.Step{
			{Type: "state.set", Path: "phase", Value: "failed"},
			{Type: "state.set", Path: "detail", Value: "{{ error }}"},
		},
	}
	rt := dispatchHTTP(t, map[string]any{}, step, nil)
	if rt.State["phase"] != "failed" {
		t.Errorf("onError branch must run on a non-2xx status: got %v", rt.State["phase"])
	}
	if rt.State["detail"] != "500 Internal Server Error" {
		t.Errorf("onError must see {{ error }}: got %v", rt.State["detail"])
	}
	// The classic Error path write is preserved (and written before the branch).
	if rt.State["err"] != "500 Internal Server Error" {
		t.Errorf("Error state path must still be written: got %v", rt.State["err"])
	}
	if _, ok := rt.State["resp"]; ok {
		t.Error("a failed request must not write Result")
	}
}

func TestHTTPOnErrorOnTransportFailure(t *testing.T) {
	// A request that cannot even be built (bad URL) takes the same fail path.
	step := model.Step{
		Type: "http.get", URL: "http://[::1]:namedport", Error: "err",
		OnError: []model.Step{{Type: "state.set", Path: "phase", Value: "failed"}},
	}
	rt := dispatchHTTP(t, map[string]any{}, step, nil)
	if rt.State["phase"] != "failed" {
		t.Errorf("onError must run on a transport/build failure: got %v", rt.State["phase"])
	}
}

func TestHTTPOnSuccessWithoutResultPath(t *testing.T) {
	// onSuccess gets {{ response }} even when no Result path is configured.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`[1,2,3]`))
	}))
	defer srv.Close()
	step := model.Step{
		Type: "http.get", URL: srv.URL,
		OnSuccess: []model.Step{{Type: "state.set", Path: "len", Value: "{{ count(response) }}"}},
	}
	rt := dispatchHTTP(t, map[string]any{}, step, nil)
	if rt.State["len"] != float64(3) {
		t.Errorf("onSuccess must see the response without a Result path: got %v", rt.State["len"])
	}
}

// ---- scene onEnter ----------------------------------------------------------

func enterApp() *model.App {
	app := execApp(map[string]*model.Action{
		"loadMain":   {ID: "loadMain", Steps: []model.Step{{Type: "state.increment", Path: "mainEnters"}}},
		"loadDetail": {ID: "loadDetail", Steps: []model.Step{{Type: "state.increment", Path: "detailEnters"}}},
	})
	app.SceneEnter = map[string]*model.Invoke{
		"main":   {Name: "loadMain", Args: map[string]string{}},
		"detail": {Name: "loadDetail", Args: map[string]string{}},
	}
	return app
}

func TestOnEnterFiresOnceOnInit(t *testing.T) {
	rt := New(enterApp())
	rt.RunPendingEnter()
	if rt.State["mainEnters"] != float64(1) {
		t.Fatalf("entry scene onEnter must fire on the first drain: got %v", rt.State["mainEnters"])
	}
	rt.RunPendingEnter() // e.g. an SSE reconnect or a refresh — no replay
	rt.RunPendingEnter()
	if rt.State["mainEnters"] != float64(1) {
		t.Errorf("onEnter must not replay on later drains: got %v", rt.State["mainEnters"])
	}
}

func TestOnEnterFiresOnNavigateAndBack(t *testing.T) {
	rt := New(enterApp())
	rt.RunPendingEnter()
	rt.Navigate("detail", nil)
	rt.RunPendingEnter()
	if rt.State["detailEnters"] != float64(1) {
		t.Fatalf("navigating into a scene must fire its onEnter: got %v", rt.State["detailEnters"])
	}
	rt.NavigateBack()
	rt.RunPendingEnter()
	if rt.State["mainEnters"] != float64(2) {
		t.Errorf("navigating back re-enters the previous scene, firing its onEnter again: got %v", rt.State["mainEnters"])
	}
}

func TestOnEnterArgsEvaluate(t *testing.T) {
	app := execApp(map[string]*model.Action{
		"seed": {ID: "seed", Steps: []model.Step{{Type: "state.set", Path: "got", Value: "{{ v }}"}}},
	})
	app.SceneEnter = map[string]*model.Invoke{"main": {Name: "seed", Args: map[string]string{"v": "{{ state.src }}"}}}
	rt := New(app)
	rt.State["src"] = "hello"
	rt.RunPendingEnter()
	if rt.State["got"] != "hello" {
		t.Errorf("onEnter args must evaluate in scene context: got %v", rt.State["got"])
	}
}

func TestOnEnterSelfNavigateNoLoop(t *testing.T) {
	// An onEnter that navigates to its own scene is a runtime no-op and must
	// not loop or re-fire.
	app := execApp(map[string]*model.Action{
		"again": {ID: "again", Steps: []model.Step{
			{Type: "state.increment", Path: "n"},
			{Type: "navigate", To: "main"},
		}},
	})
	app.SceneEnter = map[string]*model.Invoke{"main": {Name: "again", Args: map[string]string{}}}
	rt := New(app)
	rt.RunPendingEnter()
	if rt.State["n"] != float64(1) {
		t.Errorf("self-navigating onEnter must fire exactly once: got %v", rt.State["n"])
	}
}

func TestOnEnterPingPongCapped(t *testing.T) {
	// Two scenes whose onEnter actions navigate to each other must drain in a
	// bounded number of hops, not hang.
	app := execApp(map[string]*model.Action{
		"toDetail": {ID: "toDetail", Steps: []model.Step{
			{Type: "state.increment", Path: "hops"},
			{Type: "navigate", To: "detail"},
		}},
		"toMain": {ID: "toMain", Steps: []model.Step{
			{Type: "state.increment", Path: "hops"},
			{Type: "navigate", Back: true},
		}},
	})
	app.SceneEnter = map[string]*model.Invoke{
		"main":   {Name: "toDetail", Args: map[string]string{}},
		"detail": {Name: "toMain", Args: map[string]string{}},
	}
	rt := New(app)
	rt.RunPendingEnter() // must terminate
	if hops, _ := rt.State["hops"].(float64); hops == 0 || hops > float64(maxEnterChain) {
		t.Errorf("onEnter ping-pong must be capped at maxEnterChain (%d): hops=%v", maxEnterChain, rt.State["hops"])
	}
}

func TestClearPendingEnter(t *testing.T) {
	rt := New(enterApp())
	rt.ClearPendingEnter() // hot-reload semantics: session continues, no replay
	rt.RunPendingEnter()
	if _, ok := rt.State["mainEnters"]; ok {
		t.Error("ClearPendingEnter must drop the undispatched entry mark")
	}
}

func TestClonePreservesPendingEnter(t *testing.T) {
	rt := New(enterApp())
	c := rt.Clone()
	c.RunPendingEnter()
	if c.State["mainEnters"] != float64(1) {
		t.Errorf("a clone must carry the pending entry mark: got %v", c.State["mainEnters"])
	}
	// ... without the original losing (or sharing) it.
	rt.RunPendingEnter()
	if rt.State["mainEnters"] != float64(1) {
		t.Errorf("the original still drains its own mark once: got %v", rt.State["mainEnters"])
	}
}

func TestOnEnterNoHookScene(t *testing.T) {
	// A scene without a hook drains quietly; a scene navigated to afterwards
	// with a hook still fires.
	app := enterApp()
	delete(app.SceneEnter, "main")
	rt := New(app)
	rt.RunPendingEnter()
	rt.Navigate("detail", nil)
	rt.RunPendingEnter()
	if rt.State["detailEnters"] != float64(1) {
		t.Errorf("hookless entry must not block later hooks: got %v", rt.State["detailEnters"])
	}
}

func TestSwapAppPreservingState(t *testing.T) {
	app1 := &model.App{
		Entry:  "main",
		Scenes: map[string]*model.Node{"main": {Type: "view", ID: "root"}},
		GlobalState: model.GlobalState{
			Initial: map[string]any{"count": float64(10), "user": "Alice"},
		},
	}
	rt := New(app1)
	rt.State["count"] = float64(42) // User modified state

	app2 := &model.App{
		Entry:  "main",
		Scenes: map[string]*model.Node{"main": {Type: "view", ID: "root"}},
		GlobalState: model.GlobalState{
			Initial: map[string]any{"count": float64(10), "theme": "dark"},
		},
	}

	rt.SwapAppPreservingState(app2)

	if rt.State["count"] != float64(42) {
		t.Errorf("modified state 'count' should be preserved: got %v", rt.State["count"])
	}
	if rt.State["user"] != "Alice" {
		t.Errorf("state 'user' should be preserved: got %v", rt.State["user"])
	}
	if rt.State["theme"] != "dark" {
		t.Errorf("new state 'theme' should be seeded: got %v", rt.State["theme"])
	}
}
