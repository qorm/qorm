package runtime

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/qorm/qorm/internal/model"
)

// Tests for the GOVERNANCE fields layered on the async step: `key` (one request
// per slot, newest wins), `timeout` (a bounded round trip), `pending` (a state
// flag that tracks the request's lifetime), the in-flight ceiling, and the
// `delay` step that shares the same background sink.
//
// Same discipline as async_http_test.go: the host sink is a test double, so
// "the request is open" and "the continuation has not run yet" are states the
// test stands still in rather than timing guesses. The one place a real clock
// appears is the timeout suite, where the elapsed deadline IS the behaviour
// under test — and even there the outcome is deterministic, because the backend
// those tests point at never replies at all.

// countingBackend replies with a fixed JSON body and records how many requests
// actually reached it — which is how a cancellation is proven: a superseded
// request's transport is torn down before it is ever sent, so the backend never
// sees it.
type countingBackend struct {
	srv     *httptest.Server
	arrived atomic.Int64
}

func newCountingBackend(t *testing.T, body string) *countingBackend {
	t.Helper()
	b := &countingBackend{}
	b.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		b.arrived.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(b.srv.Close)
	return b
}

// deadBackend accepts the connection and never answers, so a request against it
// can only ever end by timing out. No sleep is involved: the test waits on the
// deadline it declared, which is the thing being tested.
func deadBackend(t *testing.T) string {
	t.Helper()
	block := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		select {
		case <-block:
		case <-r.Context().Done():
		}
	}))
	t.Cleanup(func() { close(block); srv.Close() })
	return srv.URL
}

// ---- key: one request per slot, newest wins -----------------------------------

// TestKeyedRequestSupersedesTheOpenOne is the search-box guarantee. Two requests
// on one key overlap; the second supersedes the first, so what lands is the
// answer to the LAST keystroke — even though the first request completes first,
// which is precisely the ordering that makes an unkeyed app show stale results.
func TestKeyedRequestSupersedesTheOpenOne(t *testing.T) {
	first := newCountingBackend(t, `{"hits":"old"}`)
	second := newCountingBackend(t, `{"hits":"new"}`)
	d := &deferHost{}
	rt := asyncRT(nil, nil)
	rt.Async = d.sink
	search := func(url string) {
		rt.App.Actions["call"].Steps = []model.Step{{
			Type: "http.get", URL: url, Async: true, Key: "search", Result: "hits",
			OnSuccess: []model.Step{{Type: "state.increment", Path: "landed"}},
		}}
		rt.Dispatch("call", nil)
	}
	search(first.srv.URL)
	search(second.srv.URL)

	if rt.Inflight() != 2 {
		t.Fatalf("both requests are open until their outcomes arrive: %d", rt.Inflight())
	}
	d.runFirst() // the superseded one completes first, as on a real network
	if rt.State["hits"] != nil {
		t.Errorf("a superseded request must not write its result: %v", rt.State["hits"])
	}
	if rt.State["landed"] != nil {
		t.Errorf("nor may its onSuccess branch run: %v", rt.State["landed"])
	}
	d.run()
	got, _ := rt.State["hits"].(map[string]any)
	if got["hits"] != "new" {
		t.Errorf("the surviving request must land: %v", rt.State["hits"])
	}
	if rt.State["landed"] != float64(1) {
		t.Errorf("exactly one branch must run: %v", rt.State["landed"])
	}
	if rt.Inflight() != 0 {
		t.Errorf("a dropped continuation must still release its slot: %d", rt.Inflight())
	}
}

// TestSupersededRequestIsCancelledOnTheWire: dropping the continuation would be
// enough for correctness, but not for cost — a fast typist would leave a dozen
// full round trips running against the backend. The transport is torn down when
// the key is taken over, so the superseded request is never even sent.
func TestSupersededRequestIsCancelledOnTheWire(t *testing.T) {
	be := newCountingBackend(t, `{"hits":1}`)
	d := &deferHost{}
	rt := asyncRT([]model.Step{{
		Type: "http.get", URL: be.srv.URL, Async: true, Key: "search", Result: "hits",
	}}, nil)
	rt.Async = d.sink
	rt.Dispatch("call", nil)
	rt.Dispatch("call", nil)
	d.run()

	if n := be.arrived.Load(); n != 1 {
		t.Errorf("the superseded request must never reach the backend: %d requests arrived", n)
	}
}

// TestSupersededFailureIsAlsoDropped: a cancelled request FAILS (that is what
// cancelling does), and writing that failure would put a spurious error on
// screen for a request the user has already replaced. The whole outcome is
// discarded, not just the success half.
func TestSupersededFailureIsAlsoDropped(t *testing.T) {
	d := &deferHost{}
	rt := asyncRT([]model.Step{{
		Type: "http.get", URL: "http://127.0.0.1:1/gone", Async: true, Key: "search",
		Error:   "err",
		OnError: []model.Step{{Type: "state.set", Path: "sawError", Value: "yes"}},
	}}, nil)
	rt.Async = d.sink
	rt.Dispatch("call", nil)
	rt.Dispatch("call", nil)
	d.runFirst()

	if rt.State["err"] != nil || rt.State["sawError"] != nil {
		t.Errorf("a superseded request must write nothing at all: err=%v sawError=%v", rt.State["err"], rt.State["sawError"])
	}
	d.run()
	if rt.State["sawError"] != "yes" {
		t.Errorf("the surviving request still reports its own failure: %v", rt.State)
	}
}

// TestUnkeyedRequestsNeverSupersedeEachOther: `key` is opt-in. Two independent
// fetches launched from one action (a dashboard filling two panels) must both
// land, which is the behaviour every app written before the field existed
// depends on.
func TestUnkeyedRequestsNeverSupersedeEachOther(t *testing.T) {
	be := newCountingBackend(t, `{"n":1}`)
	d := &deferHost{}
	rt := asyncRT([]model.Step{
		{Type: "http.get", URL: be.srv.URL, Async: true, Result: "a"},
		{Type: "http.get", URL: be.srv.URL, Async: true, Result: "b"},
	}, nil)
	rt.Async = d.sink
	rt.Dispatch("call", nil)
	d.run()
	if rt.State["a"] == nil || rt.State["b"] == nil {
		t.Errorf("both unkeyed requests must land: %v", rt.State)
	}
	if n := be.arrived.Load(); n != 2 {
		t.Errorf("neither may be cancelled: %d requests arrived", n)
	}
}

// TestDistinctKeysAreIndependentSlots: a key is a slot name, not a global lock —
// a search and a save running at once must not cancel each other.
func TestDistinctKeysAreIndependentSlots(t *testing.T) {
	be := newCountingBackend(t, `{"n":1}`)
	d := &deferHost{}
	rt := asyncRT([]model.Step{
		{Type: "http.get", URL: be.srv.URL, Async: true, Key: "search", Result: "a"},
		{Type: "http.get", URL: be.srv.URL, Async: true, Key: "save", Result: "b"},
	}, nil)
	rt.Async = d.sink
	rt.Dispatch("call", nil)
	d.run()
	if rt.State["a"] == nil || rt.State["b"] == nil {
		t.Errorf("distinct keys must not interfere: %v", rt.State)
	}
}

// TestKeySlotIsFreedOnCompletion: the slot belongs to the request only while it
// is open. A key used once a second for an hour must not accumulate entries, and
// a later request on a free slot must never be treated as superseded.
func TestKeySlotIsFreedOnCompletion(t *testing.T) {
	be := newCountingBackend(t, `{"n":1}`)
	d := &deferHost{}
	rt := asyncRT([]model.Step{{
		Type: "http.get", URL: be.srv.URL, Async: true, Key: "search", Result: "hits",
		OnSuccess: []model.Step{{Type: "state.increment", Path: "landed"}},
	}}, nil)
	rt.Async = d.sink
	for i := 0; i < 3; i++ {
		rt.Dispatch("call", nil)
		d.run()
	}
	if rt.State["landed"] != float64(3) {
		t.Errorf("sequential keyed requests each land: %v", rt.State["landed"])
	}
	if len(rt.keyed) != 0 {
		t.Errorf("a settled request must leave no slot behind: %v", rt.keyed)
	}
}

// TestKeyOnASynchronousRequestIsInert: a blocking request cannot be superseded —
// its own dispatch is still running, so there is no second request to supersede
// it. The field is accepted and simply does nothing, which keeps one JSON file
// portable between a threaded host and a single-threaded one (where AsyncAll
// makes the very same step async, and the key then matters).
func TestKeyOnASynchronousRequestIsInert(t *testing.T) {
	be := newCountingBackend(t, `{"n":1}`)
	rt := asyncRT([]model.Step{{
		Type: "http.get", URL: be.srv.URL, Key: "search", Result: "hits",
	}}, nil)
	rt.Dispatch("call", nil)
	rt.Dispatch("call", nil)
	if rt.State["hits"] == nil {
		t.Errorf("a synchronous keyed request must still land: %v", rt.State)
	}
	if len(rt.keyed) != 0 {
		t.Errorf("it must not occupy a slot: %v", rt.keyed)
	}
	if n := be.arrived.Load(); n != 2 {
		t.Errorf("neither request may be cancelled: %d arrived", n)
	}
}

// ---- timeout ------------------------------------------------------------------

// TestTimeoutTakesTheErrorPath: an expiry is an ordinary failure, so everything
// an app already wrote for failures applies — the `error` path is written and
// onError runs. The message names what the author declared (a timeout of N ms)
// rather than Go's transport wording, so it is safe to show and stable to match.
func TestTimeoutTakesTheErrorPath(t *testing.T) {
	d := &deferHost{}
	rt := asyncRT([]model.Step{{
		Type: "http.get", URL: deadBackend(t), Async: true, TimeoutMS: 30,
		Result: "resp", Error: "err",
		OnError: []model.Step{{Type: "state.set", Path: "seen", Value: "{{ error }}"}},
	}}, nil)
	rt.Async = d.sink
	rt.Dispatch("call", nil)
	d.run()

	if want := "request timed out after 30ms"; rt.State["err"] != want {
		t.Errorf("error path: got %v, want %q", rt.State["err"], want)
	}
	if rt.State["seen"] != rt.State["err"] {
		t.Errorf("onError must see the same message: %v", rt.State["seen"])
	}
	if rt.State["resp"] != nil {
		t.Errorf("a timed-out request must write no result: %v", rt.State["resp"])
	}
}

// TestTimeoutAppliesToSynchronousRequestsToo: the field bounds the round trip,
// not the async machinery. On the blocking path it is what stops one slow
// backend from holding the host's dispatch lock for the client's full 20s.
func TestTimeoutAppliesToSynchronousRequestsToo(t *testing.T) {
	rt := asyncRT([]model.Step{{
		Type: "http.get", URL: deadBackend(t), TimeoutMS: 30, Error: "err",
	}}, nil)
	rt.Dispatch("call", nil)
	if want := "request timed out after 30ms"; rt.State["err"] != want {
		t.Errorf("a synchronous request must honour its timeout: got %v, want %q", rt.State["err"], want)
	}
}

// TestTimeoutDoesNotRewriteOtherFailures: only a deadline gets the declared
// wording. A refused connection must keep the transport's own message, which is
// the only thing that says WHY it failed.
func TestTimeoutDoesNotRewriteOtherFailures(t *testing.T) {
	rt := asyncRT([]model.Step{{
		Type: "http.get", URL: "http://127.0.0.1:1/gone", TimeoutMS: 5000, Error: "err",
	}}, nil)
	rt.Dispatch("call", nil)
	msg, _ := rt.State["err"].(string)
	if msg == "" || strings.Contains(msg, "timed out") {
		t.Errorf("a transport failure must keep its own message: %q", msg)
	}
}

// TestNoTimeoutLeavesTheRequestOnTheClientCeiling: the default is unchanged
// behaviour, so an app that never heard of the field keeps the shared client's
// 20s ceiling and a normal reply still lands.
func TestNoTimeoutLeavesTheRequestOnTheClientCeiling(t *testing.T) {
	be := newCountingBackend(t, `{"ok":true}`)
	rt := asyncRT([]model.Step{{Type: "http.get", URL: be.srv.URL, Result: "resp"}}, nil)
	rt.Dispatch("call", nil)
	if got, _ := rt.State["resp"].(map[string]any); got["ok"] != true {
		t.Errorf("a request without a timeout must still succeed: %v", rt.State["resp"])
	}
}

// ---- the in-flight ceiling ----------------------------------------------------

// httpStep is a keyless async request written to a distinct result path.
func httpStep(url, result string) model.Step {
	return model.Step{Type: "http.get", URL: url, Async: true, Result: result}
}

// TestInflightCapRefusesThroughTheErrorPath: past the ceiling a request is not
// queued and not silently dropped — it fails, immediately and visibly, on the
// path the app already handles. A 250ms timer firing against a backend that
// takes seconds is the shape this exists for.
func TestInflightCapRefusesThroughTheErrorPath(t *testing.T) {
	be := newCountingBackend(t, `{"n":1}`)
	d := &deferHost{}
	steps := make([]model.Step, 0, maxInflight+1)
	for i := 0; i < maxInflight; i++ {
		steps = append(steps, httpStep(be.srv.URL, fmt.Sprintf("r%d", i)))
	}
	steps = append(steps, model.Step{
		Type: "http.get", URL: be.srv.URL, Async: true, Result: "overflow", Error: "err",
		OnError: []model.Step{{Type: "state.set", Path: "seen", Value: "{{ error }}"}},
	})
	rt := asyncRT(steps, nil)
	rt.Async = d.sink
	rt.Dispatch("call", nil)

	if rt.Inflight() != maxInflight {
		t.Fatalf("the ceiling must hold at %d: %d", maxInflight, rt.Inflight())
	}
	if len(d.q) != maxInflight {
		t.Errorf("a refused request must never reach the sink: %d queued", len(d.q))
	}
	if rt.State["err"] != errTooManyInflight {
		t.Errorf("the refusal must be reported on the error path: %v", rt.State["err"])
	}
	if rt.State["seen"] != errTooManyInflight {
		t.Errorf("onError must run for a refusal: %v", rt.State["seen"])
	}
	if rt.State["overflow"] != nil {
		t.Errorf("a refused request writes no result: %v", rt.State["overflow"])
	}
}

// TestInflightCapReleasesAsRequestsSettle: the ceiling is a concurrency limit,
// not a quota. Once the open requests finish, the next one goes through.
func TestInflightCapReleasesAsRequestsSettle(t *testing.T) {
	be := newCountingBackend(t, `{"n":1}`)
	d := &deferHost{}
	steps := make([]model.Step, 0, maxInflight)
	for i := 0; i < maxInflight; i++ {
		steps = append(steps, httpStep(be.srv.URL, fmt.Sprintf("r%d", i)))
	}
	rt := asyncRT(steps, nil)
	rt.Async = d.sink
	rt.Dispatch("call", nil)
	d.run()
	if rt.Inflight() != 0 {
		t.Fatalf("every request must be released: %d", rt.Inflight())
	}

	rt.App.Actions["call"].Steps = []model.Step{{
		Type: "http.get", URL: be.srv.URL, Async: true, Result: "after", Error: "err",
	}}
	rt.Dispatch("call", nil)
	d.run()
	if rt.State["after"] == nil || rt.State["err"] == errTooManyInflight {
		t.Errorf("a request after the burst drained must go through: %v", rt.State)
	}
}

// TestSynchronousRequestsAreNotCapped: the ceiling counts BACKGROUND work,
// which is the only kind that can accumulate. A blocking request holds the
// dispatch, so there can never be more than one per host thread — capping it
// would refuse requests that are, by construction, already serialised.
func TestSynchronousRequestsAreNotCapped(t *testing.T) {
	be := newCountingBackend(t, `{"n":1}`)
	d := &deferHost{}
	steps := make([]model.Step, 0, maxInflight+1)
	for i := 0; i < maxInflight; i++ {
		steps = append(steps, httpStep(be.srv.URL, fmt.Sprintf("r%d", i)))
	}
	steps = append(steps, model.Step{Type: "http.get", URL: be.srv.URL, Result: "sync", Error: "err"})
	rt := asyncRT(steps, nil)
	rt.Async = d.sink
	rt.Dispatch("call", nil)

	if rt.State["sync"] == nil {
		t.Errorf("a synchronous request must not be refused by the async ceiling: %v", rt.State["err"])
	}
}

// ---- pending ------------------------------------------------------------------

// TestPendingHeldForTheRequestLifetime: the flag is true exactly while the
// request is open — set before the round trip starts, cleared when the outcome
// settles. This is the hand-written state.set pair, minus the chance to forget
// half of it.
func TestPendingHeldForTheRequestLifetime(t *testing.T) {
	be := newCountingBackend(t, `{"n":1}`)
	d := &deferHost{}
	rt := asyncRT([]model.Step{{
		Type: "http.get", URL: be.srv.URL, Async: true, Pending: "searching", Result: "hits",
	}}, nil)
	rt.Async = d.sink
	rt.Dispatch("call", nil)
	if rt.State["searching"] != true {
		t.Fatalf("the flag must be up while the request is open: %v", rt.State["searching"])
	}
	d.run()
	if rt.State["searching"] != false {
		t.Errorf("and down once it settles: %v", rt.State["searching"])
	}
}

// TestPendingIsClearedOnEveryFailurePath is the half a hand-written pair
// forgets: a spinner left spinning forever after an error is the classic bug
// this field exists to make impossible.
func TestPendingIsClearedOnEveryFailurePath(t *testing.T) {
	for _, tc := range []struct {
		name string
		step model.Step
	}{
		{"transport", model.Step{Type: "http.get", URL: "http://127.0.0.1:1/gone", Async: true, Pending: "busy"}},
		{"timeout", model.Step{Type: "http.get", URL: deadBackend(t), Async: true, TimeoutMS: 30, Pending: "busy"}},
		{"unbuildable", model.Step{Type: "http.request", Method: "BAD METHOD", URL: "http://x.invalid", Async: true, Pending: "busy"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := &deferHost{}
			rt := asyncRT([]model.Step{tc.step}, nil)
			rt.Async = d.sink
			rt.Dispatch("call", nil)
			d.run()
			if busy := rt.State["busy"]; busy != false && busy != nil {
				t.Errorf("the pending flag must come down on a %s failure: %v", tc.name, busy)
			}
		})
	}
}

// TestPendingIsReferenceCounted: two requests sharing one flag must hold it up
// until BOTH are done. A plain boolean would let the first completion switch off
// a spinner the second request is still earning.
func TestPendingIsReferenceCounted(t *testing.T) {
	be := newCountingBackend(t, `{"n":1}`)
	d := &deferHost{}
	rt := asyncRT([]model.Step{
		{Type: "http.get", URL: be.srv.URL, Async: true, Pending: "busy", Result: "a"},
		{Type: "http.get", URL: be.srv.URL, Async: true, Pending: "busy", Result: "b"},
	}, nil)
	rt.Async = d.sink
	rt.Dispatch("call", nil)
	d.runFirst()
	if rt.State["busy"] != true {
		t.Errorf("one open request still holds the flag: %v", rt.State["busy"])
	}
	d.run()
	if rt.State["busy"] != false {
		t.Errorf("the last completion releases it: %v", rt.State["busy"])
	}
}

// TestSupersededRequestDoesNotClearItsSuccessorsPending combines the two
// features an app uses together: a keyed search with a spinner. The superseded
// request writes nothing — but it must still release its OWN reference, and
// must not take the spinner down while its replacement is still fetching.
func TestSupersededRequestDoesNotClearItsSuccessorsPending(t *testing.T) {
	be := newCountingBackend(t, `{"hits":1}`)
	d := &deferHost{}
	rt := asyncRT([]model.Step{{
		Type: "http.get", URL: be.srv.URL, Async: true, Key: "search", Pending: "searching",
		Result: "hits",
	}}, nil)
	rt.Async = d.sink
	rt.Dispatch("call", nil)
	rt.Dispatch("call", nil)

	d.runFirst() // the superseded request's outcome arrives
	if rt.State["searching"] != true {
		t.Errorf("the spinner must stay up while the surviving request runs: %v", rt.State["searching"])
	}
	d.run()
	if rt.State["searching"] != false {
		t.Errorf("the surviving request takes it down: %v", rt.State["searching"])
	}
	if rt.State["hits"] == nil {
		t.Errorf("and lands its result: %v", rt.State)
	}
}

// TestPendingOnTheSynchronousPathSettlesFalse: the field is portable. On a host
// with no background sink nobody can observe the flag going up (the dispatch
// never yields), but the state it leaves behind must be identical, so the same
// app renders the same way after the action either way.
func TestPendingOnTheSynchronousPathSettlesFalse(t *testing.T) {
	be := newCountingBackend(t, `{"n":1}`)
	rt := asyncRT([]model.Step{{
		Type: "http.get", URL: be.srv.URL, Pending: "busy", Result: "hits",
	}}, nil)
	rt.Dispatch("call", nil)
	if rt.State["busy"] != false {
		t.Errorf("a settled synchronous request leaves the flag down: %v", rt.State["busy"])
	}
	if len(rt.pendingRefs) != 0 {
		t.Errorf("and holds no reference: %v", rt.pendingRefs)
	}
}

// TestPendingIntoTheComputedNamespaceIsDropped: `pending` is a state write like
// any other, so it obeys the same rule — the derived namespace is republished
// every frame and nothing may write into it.
func TestPendingIntoTheComputedNamespaceIsDropped(t *testing.T) {
	be := newCountingBackend(t, `{"n":1}`)
	rt := asyncRT([]model.Step{{
		Type: "http.get", URL: be.srv.URL, Pending: "computed.busy", Result: "hits",
	}}, nil)
	rt.App.Computed = map[string]string{"busy": "{{ false }}"}
	rt.Dispatch("call", nil)
	ns, _ := rt.State[model.ComputedNamespace].(map[string]any)
	if ns["busy"] != false {
		t.Errorf("the derived value must survive the step: %v", ns)
	}
	if rt.State["hits"] != nil {
		t.Errorf("the whole step is dropped, not just the write: %v", rt.State["hits"])
	}
}

// ---- delay --------------------------------------------------------------------

// TestDelaySuspendsTheRestOfItsList is the whole semantics in one test: the
// steps before it have run, the steps after it have not, and the wait is
// counted as open background work.
func TestDelaySuspendsTheRestOfItsList(t *testing.T) {
	d := &deferHost{}
	rt := asyncRT([]model.Step{
		{Type: "state.set", Path: "phase", Value: "one"},
		{Type: "delay", DelayMS: 5},
		{Type: "state.set", Path: "phase", Value: "two"},
	}, nil)
	rt.Async = d.sink
	rt.Dispatch("call", nil)

	if rt.State["phase"] != "one" {
		t.Fatalf("the steps before the delay run immediately: %v", rt.State["phase"])
	}
	if rt.Inflight() != 1 {
		t.Errorf("a waiting delay is open background work: %d", rt.Inflight())
	}
	d.run()
	if rt.State["phase"] != "two" {
		t.Errorf("the rest of the list runs when the wait expires: %v", rt.State["phase"])
	}
	if rt.Inflight() != 0 {
		t.Errorf("and the work is released: %d", rt.Inflight())
	}
}

// TestDelayWithoutAHostSinkRunsEverythingImmediately is the portability rule:
// with no background sink the pause degrades to nothing rather than blocking.
// A blocking implementation would hold the server's mutex for the duration and
// freeze a single-threaded host outright — and would make `qorm render` and an
// MCP simulation sit through every animation an app declares.
func TestDelayWithoutAHostSinkRunsEverythingImmediately(t *testing.T) {
	rt := asyncRT([]model.Step{
		{Type: "delay", DelayMS: 60000},
		{Type: "state.set", Path: "phase", Value: "done"},
	}, nil)
	rt.Dispatch("call", nil)
	if rt.State["phase"] != "done" {
		t.Errorf("the remaining steps must run without waiting: %v", rt.State["phase"])
	}
	if rt.Inflight() != 0 {
		t.Errorf("nothing may be left open: %d", rt.Inflight())
	}
}

// TestDelayWithoutAPositiveMsIsANoOp: a delay that declares no wait is not a
// shorter wait, it is none — the following steps run in the same dispatch. The
// loader reports the JSON as an error; the runtime simply does not stall.
func TestDelayWithoutAPositiveMsIsANoOp(t *testing.T) {
	for _, ms := range []int{0, -1} {
		d := &deferHost{}
		rt := asyncRT([]model.Step{
			{Type: "delay", DelayMS: ms},
			{Type: "state.set", Path: "phase", Value: "done"},
		}, nil)
		rt.Async = d.sink
		rt.Dispatch("call", nil)
		if rt.State["phase"] != "done" || len(d.q) != 0 {
			t.Errorf("ms=%d must not suspend anything: %v (%d queued)", ms, rt.State["phase"], len(d.q))
		}
	}
}

// TestDelaySuspendsOnlyItsOwnList pins the scoping rule, which is deliberately
// local: a delay owns the steps that FOLLOW IT IN ITS OWN LIST, nothing more.
// A delay inside a branch therefore paces that branch, and the action carries on
// past the `if`. The alternative — suspending every enclosing list — would make
// a step's effect depend on how deeply someone nested it.
func TestDelaySuspendsOnlyItsOwnList(t *testing.T) {
	d := &deferHost{}
	rt := asyncRT([]model.Step{
		{Type: "if", Condition: "{{ true }}", Then: []model.Step{
			{Type: "delay", DelayMS: 5},
			{Type: "state.set", Path: "inBranch", Value: "done"},
		}},
		{Type: "state.set", Path: "afterIf", Value: "done"},
	}, nil)
	rt.Async = d.sink
	rt.Dispatch("call", nil)

	if rt.State["afterIf"] != "done" {
		t.Errorf("the step after the branch is not the delay's to suspend: %v", rt.State["afterIf"])
	}
	if rt.State["inBranch"] != nil {
		t.Errorf("the rest of the branch is: %v", rt.State["inBranch"])
	}
	d.run()
	if rt.State["inBranch"] != "done" {
		t.Errorf("and runs when the wait expires: %v", rt.State["inBranch"])
	}
}

// TestDelayPacesIntermediateFrames is what the step is for: render, wait,
// render. Each resumed tail is a fresh top-level unit of work, so it gets its
// own frame budget instead of inheriting an exhausted one.
func TestDelayPacesIntermediateFrames(t *testing.T) {
	d := &deferHost{}
	rt := asyncRT([]model.Step{
		{Type: "state.set", Path: "phase", Value: "one"},
		{Type: "render"},
		{Type: "delay", DelayMS: 5},
		{Type: "state.set", Path: "phase", Value: "two"},
		{Type: "render"},
		{Type: "delay", DelayMS: 5},
		{Type: "state.set", Path: "phase", Value: "three"},
		{Type: "render"},
	}, nil)
	var frames []any
	rt.Async, rt.Commit = d.sink, func() { frames = append(frames, rt.State["phase"]) }
	rt.Dispatch("call", nil)
	d.run()
	d.run()

	want := []any{"one", "two", "three"}
	if len(frames) != len(want) {
		t.Fatalf("one frame per stage: %v", frames)
	}
	for i := range want {
		if frames[i] != want[i] {
			t.Errorf("frame %d = %v, want %v (all: %v)", i, frames[i], want[i], frames)
		}
	}
}

// TestDelayContinuationSeesFrozenArgsAndLiveState: a resumed tail follows the
// same context rule as an http continuation, because it is the same kind of
// thing — work that lands after its dispatch is over.
func TestDelayContinuationSeesFrozenArgsAndLiveState(t *testing.T) {
	d := &deferHost{}
	rt := asyncRT([]model.Step{
		{Type: "delay", DelayMS: 5},
		{Type: "state.set", Path: "sawArg", Value: "{{ label }}"},
		{Type: "state.set", Path: "sawState", Value: "{{ state.draft }}"},
	}, map[string]any{"draft": "before"})
	rt.Async = d.sink
	rt.Dispatch("call", map[string]any{"label": "at-dispatch"})
	rt.State["draft"] = "after"
	d.run()

	if rt.State["sawArg"] != "at-dispatch" {
		t.Errorf("args stay frozen at dispatch time: %v", rt.State["sawArg"])
	}
	if rt.State["sawState"] != "after" {
		t.Errorf("state is read when the wait expires: %v", rt.State["sawState"])
	}
}

// TestDelayContinuationRepublishesDerivedValues: the tail lands outside any
// Dispatch, so nothing else would refresh the derived namespace and a value
// bound to a spinner would freeze at its pre-delay reading.
func TestDelayContinuationRepublishesDerivedValues(t *testing.T) {
	d := &deferHost{}
	rt := asyncRT([]model.Step{
		{Type: "delay", DelayMS: 5},
		{Type: "state.set", Path: "n", Value: "{{ 7 }}"},
	}, map[string]any{"n": 0.0})
	rt.App.Computed = map[string]string{"double": "{{ state.n * 2 }}"}
	rt.Async = d.sink
	rt.Dispatch("call", nil)
	d.run()
	ns, _ := rt.State[model.ComputedNamespace].(map[string]any)
	if ns["double"] != float64(14) {
		t.Errorf("the resumed tail must republish derived values: %v", ns)
	}
}

// TestDelayIsRefusedAtTheInflightCeiling: a `delay` holds a goroutine like a
// request does, so it answers to the same ceiling — and being refused means the
// rest of the list runs immediately rather than being lost.
func TestDelayIsRefusedAtTheInflightCeiling(t *testing.T) {
	be := newCountingBackend(t, `{"n":1}`)
	d := &deferHost{}
	steps := make([]model.Step, 0, maxInflight+2)
	for i := 0; i < maxInflight; i++ {
		steps = append(steps, httpStep(be.srv.URL, fmt.Sprintf("r%d", i)))
	}
	steps = append(steps,
		model.Step{Type: "delay", DelayMS: 60000},
		model.Step{Type: "state.set", Path: "phase", Value: "done"})
	rt := asyncRT(steps, nil)
	rt.Async = d.sink
	rt.Dispatch("call", nil)

	if rt.State["phase"] != "done" {
		t.Errorf("a refused delay must not swallow the rest of the list: %v", rt.State["phase"])
	}
	if rt.Inflight() != maxInflight {
		t.Errorf("and must not exceed the ceiling: %d", rt.Inflight())
	}
}

// TestDelayIsInTheDispatchVocabulary guards the documentation contract: the API
// reference extracts the step vocabulary from applyStep's top-level switch, so a
// step resolved only in applySteps would ship undocumented. This asserts the
// case is reachable there (an unsuspendable delay falls through to it) rather
// than only in the list walker.
func TestDelayIsInTheDispatchVocabulary(t *testing.T) {
	rt := asyncRT(nil, nil)
	// No sink, so applySteps declines and applyStep's own case handles it.
	rt.applyStep(model.Step{Type: "delay", DelayMS: 5}, map[string]any{}, 0)
	if rt.Inflight() != 0 {
		t.Errorf("applyStep's delay case must be inert, not scheduling: %d", rt.Inflight())
	}
}

// ---- abandoning a retired runtime ----------------------------------------------

// TestAbandonInflightZeroesADiscardedRuntime pins the honest answer to "is this
// runtime still working on something?" for a runtime the host has thrown away.
//
// The host contract lets a host drop a continuation outright (a hot reload, an
// OTA activate or a rollback swapped this runtime out), and the count that
// continuation would have released then stays raised forever. Inflight() is a
// PUBLISHED quiescence signal, so a caller polling it for "the app has settled"
// waits for a reply that is never coming. AbandonInflight is the host saying so.
func TestAbandonInflightZeroesADiscardedRuntime(t *testing.T) {
	be := newCountingBackend(t, `{"hits":"stale"}`)
	d := &deferHost{}
	rt := asyncRT([]model.Step{
		{Type: "http.get", URL: be.srv.URL, Async: true, Key: "search", Result: "hits"},
		{Type: "delay", DelayMS: 1},
		{Type: "state.set", Path: "phase", Value: "resumed"},
	}, nil)
	rt.Async = d.sink
	rt.Dispatch("call", nil)
	if rt.Inflight() != 2 {
		t.Fatalf("setup: the request and the delay are both open: %d", rt.Inflight())
	}

	rt.AbandonInflight() // the host retires this runtime and drops both continuations
	if rt.Inflight() != 0 {
		t.Errorf("a retired runtime must report quiescent, got %d — a poller would wait forever", rt.Inflight())
	}

	// Belt and braces: should a continuation reach the retired runtime after
	// all, its keyed slot is already marked dropped, so it writes nothing.
	d.run()
	if got := rt.State["hits"]; got != nil {
		t.Errorf("a retired runtime's continuation wrote state: %v", got)
	}
	if rt.Inflight() != 0 {
		t.Errorf("and the count must stay at zero, got %d", rt.Inflight())
	}

	// Idempotent: calling it again (or on a runtime that never opened anything)
	// is a no-op rather than a negative count.
	rt.AbandonInflight()
	asyncRT(nil, nil).AbandonInflight()
	if rt.Inflight() != 0 {
		t.Errorf("AbandonInflight must be idempotent, got %d", rt.Inflight())
	}
}
