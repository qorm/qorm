package server

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/qorm/platform/internal/loader"
	"github.com/qorm/platform/internal/runtime"
)

// Server-side tests for the async `http.*` step — this host's spawn sink.
//
// The runtime-level semantics (frozen args, live state, result branches, the
// in-flight counter) are pinned deterministically with test doubles in
// internal/runtime/async_http_test.go. What is only true HERE, and only with
// real goroutines, is the concurrency: the dispatch returns while the request
// is open, s.mu is free for everything else meanwhile, the completion lands in
// one critical section, and a runtime swapped out mid-request never receives
// the reply.
//
// Every wait is either a channel handshake with the backend or waitUntil
// (http_coverage_test.go). No sleeps: the backend blocks until the test
// releases it, so "while the request is open" is a fact, not a timing guess.

// gate is a backend that blocks every request until the test releases it, and
// reports how many requests are parked inside it. It is what turns "in flight"
// from a race into a controlled state the test can stand still in.
type gate struct {
	srv     *httptest.Server
	release chan struct{}
	arrived atomic.Int64
	body    string
	status  int
}

func newGate(t *testing.T, body string) *gate {
	t.Helper()
	g := &gate{release: make(chan struct{}), body: body}
	g.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		g.arrived.Add(1)
		<-g.release
		if g.status != 0 {
			w.WriteHeader(g.status)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, g.body)
	}))
	t.Cleanup(g.srv.Close)
	return g
}

func (g *gate) open() { close(g.release) }

// inflight reads the runtime's open-request count under the host lock that
// guards it — the same discipline production code uses.
func inflight(s *Server) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.rt.Inflight()
}

// asyncDocs is the canonical async shape: flag the loading state, fire the
// request in the background, clear the flag in the result branch.
func asyncDocs(backend string) []string {
	return []string{
		`{"type":"app","id":"as","entry":"main","globalState":{
			"schema":{"loading":"bool","resp":"object","err":"string","taps":"number"},
			"initial":{"loading":false,"resp":{},"err":"","taps":0}}}`,
		`{"type":"scene","id":"main","root":{"type":"column","id":"root","children":[
			{"type":"text","id":"busy","if":"{{ state.loading }}","text":"LOADING-NOW"},
			{"type":"text","id":"got","text":"got={{ state.resp.fact }} taps={{ state.taps }}"},
			{"type":"button","id":"go","label":"Go","onPress":{"name":"go"}},
			{"type":"button","id":"tap","label":"Tap","onPress":{"name":"tap"}}]}}`,
		`{"type":"action","id":"tap","steps":[{"type":"state.increment","path":"taps"}]}`,
		`{"type":"action","id":"go","steps":[
			{"type":"state.set","path":"loading","value":"{{ true }}"},
			{"type":"http.get","url":"` + backend + `","async":true,"result":"resp","error":"err",
			 "onSuccess":[{"type":"state.set","path":"loading","value":"{{ false }}"}],
			 "onError":[{"type":"state.set","path":"loading","value":"{{ false }}"}]}]}`,
	}
}

// TestServerInstallsAsyncSinkOnEveryRuntime: the sink is what makes "async"
// mean anything on this host, and every runtime swap must re-install it. A hot
// reload that dropped it would silently turn every async request back into a
// blocking one — a performance regression with no error to notice.
func TestServerInstallsAsyncSinkOnEveryRuntime(t *testing.T) {
	docs := asyncDocs("http://127.0.0.1:1/never")
	s, _, _ := serverFromDocs(t, docs...)
	s.mu.Lock()
	installed := s.rt.Async != nil
	s.mu.Unlock()
	if !installed {
		t.Fatal("New must install the host background sink on the runtime")
	}
	s.Reload(runtime.New(loader.FromDocs(docsFrom(t, docs...))))
	s.mu.Lock()
	installed = s.rt.Async != nil
	s.mu.Unlock()
	if !installed {
		t.Error("a hot reload must re-install the background sink on the new runtime")
	}
}

// TestAsyncHTTPRespondsBeforeTheRequestCompletes is the headline behaviour.
// POST /event returns — carrying the loading frame — while the backend is still
// holding the request open. The completion arrives later as its own live-sync
// revision. Contrast with the synchronous step, where the POST cannot return
// until the backend replies.
func TestAsyncHTTPRespondsBeforeTheRequestCompletes(t *testing.T) {
	g := newGate(t, `{"fact":"cats"}`)
	s, base, tok := serverFromDocs(t, asyncDocs(g.srv.URL)...)

	_, cancel, lines := connectSSE(t, base, nil)
	defer cancel()
	waitUntil(t, 2*time.Second, func() bool { return s.subscriberCount() == 1 }, "SSE subscriber to register")

	// The POST returns even though the backend has not replied.
	body, code := postEventAtRev(t, base, tok, handlerIndexByName(t, s, "go"), nil)
	if code != http.StatusOK {
		t.Fatalf("POST /event: %d", code)
	}
	if !strings.Contains(body, "LOADING-NOW") {
		t.Errorf("the response to the click must already be the loading frame: %s", body)
	}
	if strings.Contains(body, "got=cats") {
		t.Errorf("it cannot carry a response the backend has not sent: %s", excerpt(body, "got="))
	}
	// The counter is raised synchronously by the dispatch, so it is already
	// exact; the request itself reaches the backend on the sink's goroutine,
	// which may not have been scheduled yet.
	if n := inflight(s); n != 1 {
		t.Fatalf("the request must still be open after the dispatch returned: inflight=%d", n)
	}
	waitUntil(t, 3*time.Second, func() bool { return g.arrived.Load() == 1 }, "the request to reach the backend")

	g.open()
	waitUntil(t, 3*time.Second, func() bool { return inflight(s) == 0 }, "the async request to settle")

	s.mu.Lock()
	loading, resp := s.rt.State["loading"], s.rt.State["resp"]
	s.mu.Unlock()
	if loading != false {
		t.Errorf("the completion branch must clear the loading state: %v", loading)
	}
	if got, _ := resp.(map[string]any); got["fact"] != "cats" {
		t.Errorf("the completion must write the result: %v", resp)
	}

	// The completion reaches live subscribers as a later revision. Frames before
	// it are the loading ones; frame COUNTS are deliberately not asserted (a
	// full subscriber buffer legitimately drops frames).
	var lastRev float64
	waitUntil(t, 3*time.Second, func() bool {
		frame := nextDataFrame(t, lines, 3*time.Second)
		var d map[string]any
		if json.Unmarshal([]byte(frame), &d) != nil {
			return false
		}
		rev, _ := d["rev"].(float64)
		if rev <= lastRev && lastRev != 0 {
			t.Errorf("revisions must advance strictly: %v after %v", rev, lastRev)
		}
		lastRev = rev
		html, _ := d["html"].(string)
		return strings.Contains(html, "got=cats") && !strings.Contains(html, "LOADING-NOW")
	}, "the completion frame to reach live subscribers")
}

// TestSessionStaysUsableDuringAnAsyncRequest is the architectural win: the
// request no longer holds s.mu, so every other surface keeps answering. Under
// the synchronous step all of these would block for the full round trip — and
// a 20-second timeout would freeze the human and the agent together.
func TestSessionStaysUsableDuringAnAsyncRequest(t *testing.T) {
	g := newGate(t, `{"fact":"cats"}`)
	s, base, tok := serverFromDocs(t, asyncDocs(g.srv.URL)...)

	postEventAtRev(t, base, tok, handlerIndexByName(t, s, "go"), nil)
	if n := inflight(s); n != 1 {
		t.Fatalf("fixture: a request must be open: inflight=%d", n)
	}

	// Reads keep working...
	if page := getPage(t, base+"/"); !strings.Contains(page, "LOADING-NOW") {
		t.Error("GET / must answer during an in-flight request, showing the loading state")
	}
	if code, _ := doJSON(t, http.MethodGet, base+"/poll", "", "", ""); code != http.StatusOK {
		t.Errorf("GET /poll must answer during an in-flight request: %d", code)
	}
	// ...and so do further mutations: the user can go on using the app.
	postEventAtRev(t, base, tok, handlerIndexByName(t, s, "tap"), nil)
	s.mu.Lock()
	taps := s.rt.State["taps"]
	s.mu.Unlock()
	if taps != float64(1) {
		t.Errorf("a click during an in-flight request must land immediately: taps=%v", taps)
	}

	g.open()
	waitUntil(t, 3*time.Second, func() bool { return inflight(s) == 0 }, "the async request to settle")
	s.mu.Lock()
	taps, loading := s.rt.State["taps"], s.rt.State["loading"]
	s.mu.Unlock()
	if taps != float64(1) {
		t.Errorf("the completion must not clobber the writes made while it was open: taps=%v", taps)
	}
	if loading != false {
		t.Errorf("the completion still settles its own state: loading=%v", loading)
	}
}

// TestConcurrentAsyncDispatchesAllLand runs many overlapping requests through
// the sink at once. Every continuation must run exactly once, under the lock,
// interleaved with unrelated clicks — this is the test that earns the -race run.
func TestConcurrentAsyncDispatchesAllLand(t *testing.T) {
	const n = 20
	g := newGate(t, `{"fact":"cats"}`)
	s, base, tok := serverFromDocs(t, append(asyncDocs(g.srv.URL),
		`{"type":"action","id":"count","steps":[{"type":"state.increment","path":"landed"}]}`)...)
	// Count completions rather than trusting a last-writer-wins flag.
	s.mu.Lock()
	s.rt.App.Actions["go"].Steps[1].OnSuccess = append(s.rt.App.Actions["go"].Steps[1].OnSuccess,
		s.rt.App.Actions["count"].Steps[0])
	s.mu.Unlock()

	goH, tapH := handlerIndexByName(t, s, "go"), handlerIndexByName(t, s, "tap")
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(2)
		go func() { defer wg.Done(); postEventAtRev(t, base, tok, goH, nil) }()
		go func() { defer wg.Done(); postEventAtRev(t, base, tok, tapH, nil) }()
	}
	wg.Wait()

	// All n dispatches returned; all n requests are parked in the backend.
	waitUntil(t, 3*time.Second, func() bool { return g.arrived.Load() == n }, "every async request to reach the backend")
	if got := inflight(s); got != n {
		t.Fatalf("every dispatch must have returned with its request still open: inflight=%d", got)
	}

	g.open()
	waitUntil(t, 5*time.Second, func() bool { return inflight(s) == 0 }, "all async requests to settle")

	s.mu.Lock()
	landed, taps := s.rt.State["landed"], s.rt.State["taps"]
	s.mu.Unlock()
	if landed != float64(n) {
		t.Errorf("every continuation must run exactly once: landed=%v, want %d", landed, n)
	}
	if taps != float64(n) {
		t.Errorf("clicks interleaved with the completions must all land: taps=%v, want %d", taps, n)
	}
}

// TestAsyncResumeDroppedAfterReload pins the generation check. A hot reload
// swaps the runtime while a request is open; the reply belongs to an app that is
// no longer running, so it is dropped rather than written into its successor —
// which would resurrect a retired app's values (and could publish a frame
// against a scene the edit deleted).
func TestAsyncResumeDroppedAfterReload(t *testing.T) {
	g := newGate(t, `{"fact":"cats"}`)
	docs := asyncDocs(g.srv.URL)
	s, base, tok := serverFromDocs(t, docs...)

	postEventAtRev(t, base, tok, handlerIndexByName(t, s, "go"), nil)
	if n := inflight(s); n != 1 {
		t.Fatalf("fixture: a request must be open: inflight=%d", n)
	}

	// The edit lands while the request is in flight. The reload carries state
	// across, so `loading` is still true on the new runtime — proof that what
	// follows is the dropped continuation and not a fresh initial value.
	s.Reload(runtime.New(loader.FromDocs(docsFrom(t, docs...))))
	s.mu.Lock()
	carried := s.rt.State["loading"]
	s.mu.Unlock()
	if carried != true {
		t.Fatalf("fixture: the reload must carry the in-progress loading state: %v", carried)
	}

	g.open()
	// The reply arrives at the retired runtime. Wait for the drop to be logged,
	// which is the only observable the drop leaves behind.
	waitUntil(t, 3*time.Second, func() bool {
		_, body := doJSON(t, http.MethodGet, base+"/log?since=0", "", "", "")
		return strings.Contains(body, "dropped an async response")
	}, "the stale completion to be dropped")

	s.mu.Lock()
	loading, resp, live := s.rt.State["loading"], s.rt.State["resp"], inflightLocked(s)
	s.mu.Unlock()
	if resp != nil && len(resp.(map[string]any)) != 0 {
		t.Errorf("a reply to the previous app must never be written into its successor: %v", resp)
	}
	if loading != true {
		t.Errorf("nor may its continuation run: loading=%v", loading)
	}
	if live != 0 {
		t.Errorf("the fresh runtime starts with nothing in flight: %d", live)
	}
}

// inflightLocked reads the counter with s.mu already held.
func inflightLocked(s *Server) int { return s.rt.Inflight() }

// TestAsyncCompletionAfterASceneSwitchStillLands pins the navigation semantics,
// which the runtime swap deliberately does NOT share. State is global and
// cross-scene, so a reply that arrives after the user has moved on is still the
// answer to a question this same session asked: it is written, and the frame it
// publishes renders whatever scene is current. Dropping it instead would lose
// data the app asked for, and would make "did my fetch land?" depend on how fast
// the user tapped. (A runtime SWAP is the opposite case — there the app itself is
// gone, so the reply belongs to nothing; see TestAsyncResumeDroppedAfterReload.)
func TestAsyncCompletionAfterASceneSwitchStillLands(t *testing.T) {
	g := newGate(t, `{"fact":"cats"}`)
	docs := append(asyncDocs(g.srv.URL),
		`{"type":"scene","id":"detail","root":{"type":"text","id":"d","text":"DETAIL got={{ state.resp.fact }}"}}`,
		`{"type":"action","id":"away","steps":[{"type":"navigate","to":"detail"}]}`)
	// The main scene needs a way to reach the new one.
	docs[1] = `{"type":"scene","id":"main","root":{"type":"column","id":"root","children":[
		{"type":"text","id":"busy","if":"{{ state.loading }}","text":"LOADING-NOW"},
		{"type":"text","id":"got","text":"got={{ state.resp.fact }} taps={{ state.taps }}"},
		{"type":"button","id":"go","label":"Go","onPress":{"name":"go"}},
		{"type":"button","id":"tap","label":"Tap","onPress":{"name":"tap"}},
		{"type":"button","id":"away","label":"Away","onPress":{"name":"away"}}]}}`
	s, base, tok := serverFromDocs(t, docs...)

	postEventAtRev(t, base, tok, handlerIndexByName(t, s, "go"), nil)
	if n := inflight(s); n != 1 {
		t.Fatalf("fixture: a request must be open: inflight=%d", n)
	}
	// The user navigates away while the request is still open.
	away, _ := postEventAtRev(t, base, tok, handlerIndexByName(t, s, "away"), nil)
	if !strings.Contains(away, "DETAIL") {
		t.Fatalf("fixture: the navigation must have happened: %s", away)
	}

	g.open()
	waitUntil(t, 3*time.Second, func() bool { return inflight(s) == 0 }, "the async request to settle")

	s.mu.Lock()
	resp, scene, loading := s.rt.State["resp"], s.rt.CurrentScene(), s.rt.State["loading"]
	s.mu.Unlock()
	if got, _ := resp.(map[string]any); got["fact"] != "cats" {
		t.Errorf("the reply must still be written after a scene switch: %v", resp)
	}
	if loading != false {
		t.Errorf("its branch must still run: loading=%v", loading)
	}
	if scene != "detail" {
		t.Errorf("the completion must not navigate anywhere itself: scene=%q", scene)
	}
	if page := getPage(t, base+"/"); !strings.Contains(page, "DETAIL got=cats") {
		t.Errorf("the completion frame renders the CURRENT scene: %s", page)
	}
}

// TestAsyncErrorBranchRunsInTheBackground: the failure path settles through the
// same sink — the error state path is written and onError runs, so a spinner
// started before the request is always cleared, however it ends.
func TestAsyncErrorBranchRunsInTheBackground(t *testing.T) {
	g := newGate(t, `{"nope":true}`)
	g.status = http.StatusInternalServerError
	s, base, tok := serverFromDocs(t, asyncDocs(g.srv.URL)...)

	postEventAtRev(t, base, tok, handlerIndexByName(t, s, "go"), nil)
	g.open()
	waitUntil(t, 3*time.Second, func() bool { return inflight(s) == 0 }, "the failing request to settle")

	s.mu.Lock()
	loading, errMsg := s.rt.State["loading"], s.rt.State["err"]
	s.mu.Unlock()
	if loading != false {
		t.Errorf("the error branch must clear the loading state too: %v", loading)
	}
	if errMsg == "" || errMsg == nil {
		t.Errorf("the error state path must be written: %v", errMsg)
	}
}
