package runtime

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/qorm/qorm/internal/model"
)

// Tests for the async `http.*` step — the concurrency primitive. {"async":true}
// hands the round trip to a HOST-installed background sink (Runtime.Async) and
// lets the dispatch return, so the loading state written before it is painted
// and the session is not held hostage by the request.
//
// Everything here runs against TEST DOUBLES for that sink rather than real
// goroutines, which is what makes the whole file deterministic and sleep-free:
//
//   - syncHost resumes immediately on the calling goroutine, so the async code
//     path is exercised with the timing of a plain function call;
//   - deferHost parks (work, resume) in a queue the test drives by hand, which
//     is the only way to ASSERT the in-between state — request open, dispatch
//     already returned, continuation not run yet.
//
// The real goroutine-based host is the server's spawn, covered end to end in
// internal/server/async_http_test.go.

// syncHost is a Runtime.Async implementation that completes inline.
func syncHost(work func() any, resume func(any)) { resume(work()) }

// pending is one deferred unit of background work.
type pending struct {
	work   func() any
	resume func(any)
}

// deferHost collects background work instead of running it. run() drains the
// queue in FIFO order; runLast drains it in reverse, which is how a slow first
// request and a fast second one interleave on a real network.
type deferHost struct{ q []pending }

func (d *deferHost) sink(work func() any, resume func(any)) {
	d.q = append(d.q, pending{work, resume})
}

func (d *deferHost) run() {
	q := d.q
	d.q = nil
	for _, p := range q {
		p.resume(p.work())
	}
}

// runFirst drains exactly the oldest queued unit of work, leaving the rest
// parked — the only way to stand still BETWEEN two continuations and assert
// what the first one did (or, for a superseded request, did not do).
func (d *deferHost) runFirst() {
	if len(d.q) == 0 {
		return
	}
	p := d.q[0]
	d.q = d.q[1:]
	p.resume(p.work())
}

func (d *deferHost) runLast() {
	q := d.q
	d.q = nil
	for i := len(q) - 1; i >= 0; i-- {
		q[i].resume(q[i].work())
	}
}

// asyncRT builds a one-action runtime over the given steps, with the state map
// and host sink supplied by the caller.
func asyncRT(steps []model.Step, state map[string]any) *Runtime {
	if state == nil {
		state = map[string]any{}
	}
	app := &model.App{
		Entry:   "main",
		Scenes:  map[string]*model.Node{"main": {Type: "view", ID: "root"}},
		Actions: map[string]*model.Action{"call": {ID: "call", Steps: steps}},
	}
	return &Runtime{App: app, State: state, RouteParams: map[string]any{}}
}

// jsonBackend replies with a fixed JSON body and status.
func jsonBackend(t *testing.T, status int, body string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if status != 0 {
			w.WriteHeader(status)
		}
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv
}

// ---- the fallback that keeps every other host unchanged ----------------------

// TestAsyncWithoutHostRunsSynchronously is the load-bearing degradation: the
// step is marked async but nobody installed a sink, so it takes the ordinary
// blocking path and the result is readable the instant Dispatch returns. This
// is what keeps `qorm render`, the miniapp export, MCP simulate and every
// existing test in this package correct without touching a line of them.
func TestAsyncWithoutHostRunsSynchronously(t *testing.T) {
	be := jsonBackend(t, 0, `{"fact":"cats"}`)
	rt := asyncRT([]model.Step{
		{Type: "http.get", URL: be.URL, Async: true, Result: "resp"},
		{Type: "state.set", Path: "after", Value: "{{ state.resp.fact }}"},
	}, nil)
	rt.Dispatch("call", nil)
	if rt.Inflight() != 0 {
		t.Errorf("a hookless async step must not register as in flight: %d", rt.Inflight())
	}
	got, _ := rt.State["resp"].(map[string]any)
	if got["fact"] != "cats" {
		t.Fatalf("the response must be readable the moment Dispatch returns: %v", rt.State["resp"])
	}
	if rt.State["after"] != "cats" {
		t.Errorf("a sibling step must still see the result: %v", rt.State["after"])
	}
}

// TestNewAndCloneHaveNoAsyncSink pins the same invariant Commit has: neither a
// fresh runtime nor a simulation clone may ever open a socket on a background
// goroutine. Without this, qorm_simulate would produce real side effects and
// the pure-render determinism guard would sit on top of concurrency.
func TestNewAndCloneHaveNoAsyncSink(t *testing.T) {
	rt := New(asyncRT(nil, nil).App)
	if rt.Async != nil {
		t.Error("runtime.New must not install a background sink — hosts install it explicitly")
	}
	rt.Async = syncHost
	if c := rt.Clone(); c.Async != nil {
		t.Error("Clone must not carry the background sink into a simulation")
	}
}

// ---- the async path itself ---------------------------------------------------

// TestAsyncSuccessWritesResultAndBranch: on the async path the classic Result
// write still happens first, a stale error is still cleared, and onSuccess still
// sees `{{ response }}` — the branch contract is mode-independent.
func TestAsyncSuccessWritesResultAndBranch(t *testing.T) {
	be := jsonBackend(t, 0, `{"fact":"cats"}`)
	rt := asyncRT([]model.Step{{
		Type: "http.get", URL: be.URL, Async: true, Result: "resp", Error: "err",
		OnSuccess: []model.Step{{Type: "state.set", Path: "seen", Value: "{{ response.fact }}"}},
	}}, map[string]any{"err": "stale"})
	rt.Async = syncHost
	rt.Dispatch("call", nil)

	got, _ := rt.State["resp"].(map[string]any)
	if got["fact"] != "cats" {
		t.Errorf("result path: %v", rt.State["resp"])
	}
	if rt.State["err"] != "" {
		t.Errorf("a success must clear the stale error: %v", rt.State["err"])
	}
	if rt.State["seen"] != "cats" {
		t.Errorf("onSuccess must see {{ response }}: %v", rt.State["seen"])
	}
}

// TestAsyncFailureWritesErrorAndBranch covers both failure shapes an async step
// can reach through the background sink: a non-2xx status and a transport error.
func TestAsyncFailureWritesErrorAndBranch(t *testing.T) {
	boom := jsonBackend(t, http.StatusInternalServerError, `{"nope":true}`)
	for _, tc := range []struct{ name, url string }{
		{"non-2xx", boom.URL},
		{"transport", "http://127.0.0.1:1/gone"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rt := asyncRT([]model.Step{{
				Type: "http.get", URL: tc.url, Async: true, Result: "resp", Error: "err",
				OnError: []model.Step{{Type: "state.set", Path: "seen", Value: "{{ error }}"}},
			}}, nil)
			rt.Async = syncHost
			rt.Dispatch("call", nil)

			if rt.State["err"] == "" || rt.State["err"] == nil {
				t.Errorf("the error path must be written: %v", rt.State["err"])
			}
			if rt.State["seen"] != rt.State["err"] {
				t.Errorf("onError must see the same message as the error path: %v vs %v", rt.State["seen"], rt.State["err"])
			}
			if rt.State["resp"] != nil {
				t.Errorf("a failed request must not write a result: %v", rt.State["resp"])
			}
		})
	}
}

// TestAsyncNonJSONBodyBecomesText: the decoding rule (raw text when the body is
// not JSON) belongs to the shared outcome normaliser, so it applies identically
// on the background path.
func TestAsyncNonJSONBodyBecomesText(t *testing.T) {
	be := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("plain words"))
	}))
	defer be.Close()
	rt := asyncRT([]model.Step{{Type: "http.get", URL: be.URL, Async: true, Result: "resp"}}, nil)
	rt.Async = syncHost
	rt.Dispatch("call", nil)
	if rt.State["resp"] != "plain words" {
		t.Errorf("a non-JSON body must arrive as raw text: %v", rt.State["resp"])
	}
}

// TestAsyncWithoutResultPathStillRunsBranch: `result` is optional — a step that
// only wants the branch must still get `{{ response }}` and write no state of
// its own.
func TestAsyncWithoutResultPathStillRunsBranch(t *testing.T) {
	be := jsonBackend(t, 0, `{"fact":"cats"}`)
	rt := asyncRT([]model.Step{{
		Type: "http.get", URL: be.URL, Async: true,
		OnSuccess: []model.Step{{Type: "state.set", Path: "seen", Value: "{{ response.fact }}"}},
	}}, nil)
	rt.Async = syncHost
	rt.Dispatch("call", nil)
	if rt.State["seen"] != "cats" {
		t.Errorf("onSuccess must run without a result path: %v", rt.State["seen"])
	}
	if len(rt.State) != 1 {
		t.Errorf("a result-less step must write nothing else: %v", rt.State)
	}
}

// TestAsyncRequestBuildFailureStaysSynchronous: a request that cannot be built
// never reaches the sink. The failure is reported inline, so a typo'd URL
// behaves identically in both modes instead of vanishing into the background.
func TestAsyncRequestBuildFailureStaysSynchronous(t *testing.T) {
	d := &deferHost{}
	rt := asyncRT([]model.Step{{
		Type: "http.request", Method: "BAD METHOD", URL: "http://example.invalid",
		Async: true, Error: "err",
		OnError: []model.Step{{Type: "state.set", Path: "seen", Value: "{{ error }}"}},
	}}, nil)
	rt.Async = d.sink
	rt.Dispatch("call", nil)

	if len(d.q) != 0 {
		t.Errorf("an unbuildable request must never be handed to the background sink: %d queued", len(d.q))
	}
	if rt.Inflight() != 0 {
		t.Errorf("it must not count as in flight either: %d", rt.Inflight())
	}
	if rt.State["seen"] == nil || rt.State["seen"] == "" {
		t.Errorf("the failure must be reported inline: %v", rt.State)
	}
}

// ---- what "async" actually buys: the dispatch does not wait -------------------

// TestAsyncDispatchReturnsBeforeCompletion is the whole point of the feature.
// With the deferred host the request is provably still open when Dispatch
// returns: the loading flag is set, the sibling steps have run, and the
// continuation has not. Then the completion lands and settles the state.
func TestAsyncDispatchReturnsBeforeCompletion(t *testing.T) {
	be := jsonBackend(t, 0, `{"fact":"cats"}`)
	d := &deferHost{}
	rt := asyncRT([]model.Step{
		{Type: "state.set", Path: "loading", Value: "{{ true }}"},
		{Type: "http.get", URL: be.URL, Async: true, Result: "resp",
			OnSuccess: []model.Step{{Type: "state.set", Path: "loading", Value: "{{ false }}"}}},
		{Type: "state.set", Path: "sibling", Value: "ran"},
	}, nil)
	rt.Async = d.sink
	rt.Dispatch("call", nil)

	if rt.State["loading"] != true {
		t.Error("the dispatch must return with the loading state still showing")
	}
	if rt.State["sibling"] != "ran" {
		t.Error("the steps after an async request must run without waiting for it")
	}
	if rt.State["resp"] != nil {
		t.Errorf("the response cannot be in state yet: %v", rt.State["resp"])
	}
	if rt.Inflight() != 1 {
		t.Errorf("one open request must be counted in flight: %d", rt.Inflight())
	}

	d.run()

	if rt.Inflight() != 0 {
		t.Errorf("a completed request must be released: %d", rt.Inflight())
	}
	if rt.State["loading"] != false {
		t.Error("the completion branch must clear the loading state")
	}
	if got, _ := rt.State["resp"].(map[string]any); got["fact"] != "cats" {
		t.Errorf("the completion must write the result: %v", rt.State["resp"])
	}
}

// TestInflightCountsConcurrentRequests: the counter is what a host (and the
// quiescence guard) uses to know the app has settled, so it has to track every
// open request, not just the last one — and reach zero however they interleave.
func TestInflightCountsConcurrentRequests(t *testing.T) {
	be := jsonBackend(t, 0, `{"n":1}`)
	d := &deferHost{}
	rt := asyncRT([]model.Step{
		{Type: "http.get", URL: be.URL, Async: true, OnSuccess: []model.Step{{Type: "state.increment", Path: "done"}}},
		{Type: "http.get", URL: be.URL, Async: true, OnSuccess: []model.Step{{Type: "state.increment", Path: "done"}}},
		{Type: "http.get", URL: be.URL, Async: true, OnSuccess: []model.Step{{Type: "state.increment", Path: "done"}}},
	}, nil)
	rt.Async = d.sink
	rt.Dispatch("call", nil)
	if rt.Inflight() != 3 {
		t.Fatalf("three open requests: got %d", rt.Inflight())
	}
	// Out-of-order completion is the normal case on a real network.
	d.runLast()
	if rt.Inflight() != 0 {
		t.Errorf("all three must be released: %d", rt.Inflight())
	}
	if rt.State["done"] != float64(3) {
		t.Errorf("every continuation must run exactly once: %v", rt.State["done"])
	}
}

// ---- the context split: frozen args, live state ------------------------------

// TestAsyncContinuationSeesFrozenArgsAndLiveState pins the rule that makes
// async intelligible: `{{ someArg }}` still means what the click carried, while
// `{{ state.x }}` means what it means NOW. Reading args live would be
// impossible (the dispatch is gone); reading state frozen would silently
// resurrect values the user has since changed.
func TestAsyncContinuationSeesFrozenArgsAndLiveState(t *testing.T) {
	be := jsonBackend(t, 0, `{"ok":true}`)
	d := &deferHost{}
	rt := asyncRT([]model.Step{{
		Type: "http.get", URL: be.URL, Async: true,
		OnSuccess: []model.Step{
			{Type: "state.set", Path: "sawArg", Value: "{{ label }}"},
			{Type: "state.set", Path: "sawState", Value: "{{ state.draft }}"},
			{Type: "state.set", Path: "sawBare", Value: "{{ draft }}"},
		},
	}}, map[string]any{"draft": "before"})
	rt.Async = d.sink
	rt.Dispatch("call", map[string]any{"label": "at-dispatch"})

	// The world moves on while the request is open.
	rt.State["draft"] = "after"
	d.run()

	if rt.State["sawArg"] != "at-dispatch" {
		t.Errorf("the action's args must be frozen at dispatch time: %v", rt.State["sawArg"])
	}
	if rt.State["sawState"] != "after" {
		t.Errorf("state must be read live in the continuation: %v", rt.State["sawState"])
	}
	if rt.State["sawBare"] != "after" {
		t.Errorf("a bare state key must resolve live too: %v", rt.State["sawBare"])
	}
}

// TestAsyncArgShadowingAStateKeyIsReadLive pins the tie-break in the freeze
// rule, which is deliberately decided by NAME, not by origin: state owns every
// name it declares, so an arg that collides with a top-level state key is read
// live in the continuation rather than frozen. Deciding it the other way would
// need the continuation to remember which names the dispatch's args occupied,
// and would leave `{{ draft }}` meaning "live" or "dispatch-time" depending on
// an argument list written somewhere else entirely. Authors who need the
// dispatch-time value of a colliding name pass it under a distinct arg name.
func TestAsyncArgShadowingAStateKeyIsReadLive(t *testing.T) {
	be := jsonBackend(t, 0, `{"ok":true}`)
	d := &deferHost{}
	rt := asyncRT([]model.Step{{
		Type: "http.get", URL: be.URL, Async: true,
		OnSuccess: []model.Step{
			{Type: "state.set", Path: "seenShadowed", Value: "{{ draft }}"},
			{Type: "state.set", Path: "seenDistinct", Value: "{{ label }}"},
		},
	}}, map[string]any{"draft": "state-value"})
	rt.Async = d.sink
	// `draft` collides with a state key; `label` does not.
	rt.Dispatch("call", map[string]any{"draft": "arg-value", "label": "at-dispatch"})
	rt.State["draft"] = "changed"
	d.run()
	if rt.State["seenShadowed"] != "changed" {
		t.Errorf("a name the state declares is read live in the continuation: %v", rt.State["seenShadowed"])
	}
	if rt.State["seenDistinct"] != "at-dispatch" {
		t.Errorf("a name only the args declare stays frozen: %v", rt.State["seenDistinct"])
	}
}

// TestAsyncContinuationSeesLiveViewport: the continuation rebuilds `viewport`
// and `t` from the runtime, so a resize (or a locale switch) during the request
// is reflected rather than replayed from a stale snapshot.
func TestAsyncContinuationSeesLiveViewport(t *testing.T) {
	be := jsonBackend(t, 0, `{"ok":true}`)
	d := &deferHost{}
	rt := asyncRT([]model.Step{{
		Type: "http.get", URL: be.URL, Async: true,
		OnSuccess: []model.Step{{Type: "state.set", Path: "w", Value: "{{ viewport.width }}"}},
	}}, nil)
	rt.Async = d.sink
	rt.Dispatch("call", nil)
	rt.Viewport = Viewport{W: 800, H: 600}
	d.run()
	if rt.State["w"] != float64(800) {
		t.Errorf("the continuation must read the live viewport: %v", rt.State["w"])
	}
}

// ---- interaction with the frame primitive ------------------------------------

// TestAsyncContinuationCanPublishFrames: `render` inside a result branch works
// on the async path too — that is how a completion that only touches part of the
// screen still reaches the user before the branch finishes.
func TestAsyncContinuationCanPublishFrames(t *testing.T) {
	be := jsonBackend(t, 0, `{"ok":true}`)
	d := &deferHost{}
	rt := asyncRT([]model.Step{{
		Type: "http.get", URL: be.URL, Async: true,
		OnSuccess: []model.Step{{Type: "state.set", Path: "phase", Value: "one"}, {Type: "render"}},
	}}, nil)
	frames := []any{}
	rt.Async, rt.Commit = d.sink, func() { frames = append(frames, rt.State["phase"]) }
	rt.Dispatch("call", nil)
	if len(frames) != 0 {
		t.Fatalf("nothing to publish before the request completes: %v", frames)
	}
	d.run()
	if len(frames) != 1 || frames[0] != "one" {
		t.Errorf("a render inside the completion branch must publish: %v", frames)
	}
}

// TestAsyncContinuationSharesTheDispatchFrameBudget: a continuation EXTENDS the
// interaction that launched the request — it draws from the same frame budget
// rather than starting over. One click can therefore never publish more than
// MaxFrames intermediate frames however many round trips it spans; before this,
// a continuation reset the budget, so dispatch + completion alone could publish
// 2*MaxFrames and push the client's revision past the live-sync buffer. The
// continuation's STEPS still run to completion past the cap — only publishing
// stops.
func TestAsyncContinuationSharesTheDispatchFrameBudget(t *testing.T) {
	be := jsonBackend(t, 0, `{"ok":true}`)
	d := &deferHost{}
	steps := make([]model.Step, 0, MaxFrames+1)
	for i := 0; i < MaxFrames; i++ {
		steps = append(steps, model.Step{Type: "render"})
	}
	steps = append(steps, model.Step{
		Type: "http.get", URL: be.URL, Async: true,
		OnSuccess: []model.Step{{Type: "state.set", Path: "landed", Value: "{{ true }}"}, {Type: "render"}},
	})
	rt := asyncRT(steps, nil)
	n := 0
	rt.Async, rt.Commit = d.sink, func() { n++ }
	rt.Dispatch("call", nil)
	if n != MaxFrames {
		t.Fatalf("the dispatch must exhaust the interaction's budget: %d", n)
	}
	d.run()
	if n != MaxFrames {
		t.Errorf("the continuation must draw from the same (exhausted) budget: %d", n)
	}
	if rt.State["landed"] != true {
		t.Error("the continuation's steps must still run past the frame cap")
	}
}
