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

	"github.com/qorm/qorm/internal/loader"
	"github.com/qorm/qorm/internal/runtime"
)

// Server-side tests for the intermediate-frame execution model:
//   - the `render` step's frame sink (installed by this host in initAgent),
//   - the deliberate rule that an intermediate frame never drains onEnter,
//   - the rev-scoped handler tables that keep a click on frame N resolving
//     against frame N even though the action published frames N+1, N+2 …
//
// Async waits use waitUntil (http_coverage_test.go), never a sleep.

// docsFrom parses a JSON document list into the loader's input form.
func docsFrom(t *testing.T, raw ...string) []map[string]any {
	t.Helper()
	docs := make([]map[string]any, len(raw))
	for i, s := range raw {
		if err := json.Unmarshal([]byte(s), &docs[i]); err != nil {
			t.Fatalf("fixture doc %d: %v", i, err)
		}
	}
	return docs
}

// serverFromDocs boots a live server on a fixture app and returns it with its
// base URL and page event token (the handler table is primed by the GET / the
// token extraction performs).
func serverFromDocs(t *testing.T, raw ...string) (*Server, string, string) {
	t.Helper()
	app := loader.FromDocs(docsFrom(t, raw...))
	if len(app.Diagnostics) != 0 {
		t.Fatalf("fixture must load clean: %v", app.Diagnostics)
	}
	s := New(runtime.New(app))
	ts := httptest.NewServer(s.Handler())
	t.Cleanup(ts.Close)
	return s, ts.URL, pageEventToken(t, ts.URL)
}

// postEventAtRev POSTs /event naming the frame the click came from, the way the
// browser does. A nil rev is the driver that reports no frame at all (curl,
// ci-smoke, the offline WASM driver) — the compatibility path.
func postEventAtRev(t *testing.T, base, tok string, h int, rev *int64) (string, int) {
	t.Helper()
	m := map[string]any{"h": h}
	if rev != nil {
		m["rev"] = *rev
	}
	body, _ := json.Marshal(m)
	req, _ := http.NewRequest(http.MethodPost, base+"/event", strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Qorm-Token", tok)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("post /event: %v", err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return string(b), resp.StatusCode
}

// ---- M0: the frame sink ------------------------------------------------------

// TestServerInstallsFrameSinkOnEveryRuntime: the sink is what makes `render`
// do anything at all on this host. Every path that swaps s.rt funnels through
// initAgent, so a runtime can never be left without it — a hot reload that
// dropped it would silently turn every loading state back into dead code.
func TestServerInstallsFrameSinkOnEveryRuntime(t *testing.T) {
	s, _, _ := serverFromDocs(t,
		`{"type":"app","id":"fs","entry":"main","globalState":{"schema":{"n":"number"},"initial":{"n":0}}}`,
		`{"type":"scene","id":"main","root":{"type":"text","id":"t","text":"{{ state.n }}"}}`,
	)
	s.mu.Lock()
	installed := s.rt.Commit != nil
	s.mu.Unlock()
	if !installed {
		t.Fatal("New must install the host frame sink on the runtime")
	}
	next := loader.FromDocs(docsFrom(t,
		`{"type":"app","id":"fs","entry":"main","globalState":{"schema":{"n":"number"},"initial":{"n":0}}}`,
		`{"type":"scene","id":"main","root":{"type":"text","id":"t","text":"reloaded {{ state.n }}"}}`,
	))
	s.Reload(runtime.New(next))
	s.mu.Lock()
	installed = s.rt.Commit != nil
	hist := len(s.handlerHist)
	s.mu.Unlock()
	if !installed {
		t.Error("a hot reload must re-install the frame sink on the new runtime")
	}
	if hist > 1 {
		t.Errorf("a reload must drop the previous app's handler tables: %d retained", hist)
	}
}

// TestIntermediateFrameReachesSSEBeforeTheResponse is the headline behaviour:
// the loading state written before a slow step is broadcast to live subscribers
// WHILE the dispatch is still running — the POST that started it has not
// returned yet. Determinism comes from the backend, which blocks until the test
// releases it, so "before" is a fact and not a timing race.
func TestIntermediateFrameReachesSSEBeforeTheResponse(t *testing.T) {
	release := make(chan struct{})
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		<-release
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"ok":true}`)
	}))
	defer backend.Close()

	s, base, tok := serverFromDocs(t,
		`{"type":"app","id":"rs","entry":"main","globalState":{
			"schema":{"saving":"bool","done":"bool"},"initial":{"saving":false,"done":false}}}`,
		`{"type":"scene","id":"main","root":{"type":"column","id":"root","children":[
			{"type":"text","id":"busy","if":"{{ state.saving }}","text":"SAVING-NOW"},
			{"type":"text","id":"ok","if":"{{ state.done }}","text":"SAVED-OK"},
			{"type":"button","id":"save","label":"Save","onPress":{"name":"save"}}]}}`,
		`{"type":"action","id":"save","steps":[
			{"type":"state.set","path":"saving","value":"{{ true }}"},
			{"type":"render"},
			{"type":"http.get","url":"`+backend.URL+`","result":"resp","error":"err"},
			{"type":"state.set","path":"saving","value":"{{ false }}"},
			{"type":"state.set","path":"done","value":"{{ true }}"}]}`,
	)

	_, cancel, lines := connectSSE(t, base, nil)
	defer cancel()
	waitUntil(t, 2*time.Second, func() bool { return s.subscriberCount() == 1 }, "SSE subscriber to register")

	h := handlerIndexByName(t, s, "save")
	var responded atomic.Bool
	var final string
	done := make(chan struct{})
	go func() {
		defer close(done)
		final, _ = postEventAtRev(t, base, tok, h, nil)
		responded.Store(true)
	}()

	// The intermediate frame must arrive while the request is still in flight.
	frame := nextDataFrame(t, lines, 3*time.Second)
	if !strings.Contains(frame, "SAVING-NOW") {
		t.Fatalf("first live frame must carry the loading state: %s", frame)
	}
	if strings.Contains(frame, "SAVED-OK") {
		t.Errorf("the loading frame must not already show the finished state: %s", frame)
	}
	if responded.Load() {
		t.Error("the loading frame must reach subscribers BEFORE POST /event returns")
	}

	var mid map[string]any
	if err := json.Unmarshal([]byte(frame), &mid); err != nil {
		t.Fatalf("intermediate frame is not a live-sync payload: %v", err)
	}
	midRev, _ := mid["rev"].(float64)

	close(release)
	<-done

	if strings.Contains(final, "SAVING-NOW") {
		t.Errorf("the completion frame must not still show the loading state: %s", excerpt(final, "SAVING-NOW"))
	}
	if !strings.Contains(final, "SAVED-OK") {
		t.Errorf("the completion frame must show the finished state: %s", final)
	}
	last := nextDataFrame(t, lines, 3*time.Second)
	var end map[string]any
	if err := json.Unmarshal([]byte(last), &end); err != nil {
		t.Fatalf("completion frame is not a live-sync payload: %v", err)
	}
	endRev, _ := end["rev"].(float64)
	if endRev <= midRev {
		t.Errorf("revisions must advance strictly: intermediate %v, completion %v", midRev, endRev)
	}
	if !strings.Contains(last, "SAVED-OK") || strings.Contains(last, "SAVING-NOW") {
		t.Errorf("the last broadcast frame must be the settled one: %s", last)
	}
}

// TestRenderStepDoesNotDrainOnEnter pins the deliberate rule: an intermediate
// frame renders the scene the action navigated to, but the target's onEnter
// still fires exactly once, at the dispatch boundary. Draining it inside the
// frame sink would re-enter Dispatch from inside Dispatch.
func TestRenderStepDoesNotDrainOnEnter(t *testing.T) {
	s, base, tok := serverFromDocs(t,
		`{"type":"app","id":"en","entry":"main","globalState":{
			"schema":{"entered":"number"},"initial":{"entered":0}}}`,
		`{"type":"scene","id":"main","root":{"type":"column","id":"root","children":[
			{"type":"button","id":"go","label":"Go","onPress":{"name":"go"}}]}}`,
		`{"type":"scene","id":"detail","onEnter":"enter","root":{"type":"text","id":"d","text":"DETAIL entered={{ state.entered }}"}}`,
		`{"type":"action","id":"enter","steps":[{"type":"state.increment","path":"entered"}]}`,
		`{"type":"action","id":"go","steps":[
			{"type":"navigate","to":"detail"},
			{"type":"render"}]}`,
	)
	_, cancel, lines := connectSSE(t, base, nil)
	defer cancel()
	waitUntil(t, 2*time.Second, func() bool { return s.subscriberCount() == 1 }, "SSE subscriber to register")

	final, _ := postEventAtRev(t, base, tok, handlerIndexByName(t, s, "go"), nil)

	mid := nextDataFrame(t, lines, 3*time.Second)
	if !strings.Contains(mid, "DETAIL") {
		t.Errorf("the intermediate frame must already show the navigated-to scene: %s", mid)
	}
	if !strings.Contains(mid, "entered=0") {
		t.Errorf("an intermediate frame must NOT drain the scene-entry hook: %s", mid)
	}
	if !strings.Contains(final, "entered=1") {
		t.Errorf("onEnter must fire once, at the dispatch boundary: %s", final)
	}
	s.mu.Lock()
	entered := s.rt.State["entered"]
	s.mu.Unlock()
	if entered != float64(1) {
		t.Errorf("onEnter must run exactly once across the whole dispatch: entered=%v", entered)
	}
}

// TestConcurrentDispatchesWithIntermediateFrames: intermediate frames add
// re-entrant render+broadcast calls inside the dispatch critical section. Run
// under -race with concurrent readers, every mutation must still land exactly
// once and every frame must be published under s.mu.
func TestConcurrentDispatchesWithIntermediateFrames(t *testing.T) {
	s, base, tok := serverFromDocs(t,
		`{"type":"app","id":"cc","entry":"main","globalState":{
			"schema":{"n":"number","busy":"bool"},"initial":{"n":0,"busy":false}}}`,
		`{"type":"scene","id":"main","root":{"type":"column","id":"root","children":[
			{"type":"text","id":"t","text":"n={{ state.n }}"},
			{"type":"button","id":"b","label":"Go","onPress":{"name":"go"}}]}}`,
		`{"type":"action","id":"go","steps":[
			{"type":"state.set","path":"busy","value":"{{ true }}"},
			{"type":"render"},
			{"type":"state.increment","path":"n"},
			{"type":"render"},
			{"type":"state.set","path":"busy","value":"{{ false }}"}]}`,
	)
	_, cancel, _ := connectSSE(t, base, nil)
	defer cancel()
	waitUntil(t, 2*time.Second, func() bool { return s.subscriberCount() == 1 }, "SSE subscriber to register")

	h := handlerIndexByName(t, s, "go")
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(2)
		go func() { defer wg.Done(); postEventAtRev(t, base, tok, h, nil) }()
		go func() { defer wg.Done(); getPage(t, base+"/") }()
	}
	wg.Wait()
	s.mu.Lock()
	n, busy := s.rt.State["n"], s.rt.State["busy"]
	hist := len(s.handlerHist)
	s.mu.Unlock()
	if n != float64(8) {
		t.Errorf("every concurrent dispatch must land exactly once: n=%v", n)
	}
	if busy != false {
		t.Errorf("the loading flag must be settled after the last dispatch: busy=%v", busy)
	}
	if hist > handlerHistory {
		t.Errorf("the handler ring must stay bounded at %d: got %d", handlerHistory, hist)
	}
}

// ---- M1: rev-scoped handler tables -------------------------------------------

// staleHandlerDocs renders a button list whose FIRST entry only exists while
// state.extra is true, so flipping that flag renumbers every handler index —
// the exact shape of the staleness bug.
func staleHandlerDocs() []string {
	return []string{
		`{"type":"app","id":"st","entry":"main","globalState":{
			"schema":{"extra":"bool","a":"number","b":"number"},"initial":{"extra":false,"a":0,"b":0}}}`,
		`{"type":"scene","id":"main","root":{"type":"column","id":"root","children":[
			{"type":"button","id":"ba","if":"{{ state.extra }}","label":"A","onPress":{"name":"doA"}},
			{"type":"button","id":"bb","label":"B","onPress":{"name":"doB"}}]}}`,
		`{"type":"action","id":"doA","steps":[{"type":"state.increment","path":"a"}]}`,
		`{"type":"action","id":"doB","steps":[{"type":"state.increment","path":"b"}]}`,
	}
}

// TestStaleHandlerIndexResolvesAgainstItsFrame: the browser painted frame N,
// where index 0 was "doB". A newer frame renumbered index 0 to "doA". A click
// carrying rev=N must still dispatch doB — resolving it against the newest
// table would silently run a DIFFERENT action than the one the human pressed.
func TestStaleHandlerIndexResolvesAgainstItsFrame(t *testing.T) {
	s, base, tok := serverFromDocs(t, staleHandlerDocs()...)

	revN := s.rev.Load()
	if got := handlerIndexByName(t, s, "doB"); got != 0 {
		t.Fatalf("fixture: on frame N index 0 must be doB, got index %d", got)
	}

	// A newer frame lands (an agent edit, or an intermediate `render` frame)
	// and renumbers the table.
	s.mu.Lock()
	s.rt.State["extra"] = true
	s.bump()
	s.mu.Unlock()
	if got := handlerIndexByName(t, s, "doA"); got != 0 {
		t.Fatalf("fixture: on the newer frame index 0 must be doA, got index %d", got)
	}

	postEventAtRev(t, base, tok, 0, &revN)
	s.mu.Lock()
	a, b := s.rt.State["a"], s.rt.State["b"]
	s.mu.Unlock()
	if b != float64(1) || a != float64(0) {
		t.Errorf("a click from frame %d must dispatch that frame's handler (doB): a=%v b=%v", revN, a, b)
	}
}

// TestEventWithoutRevFallsBackToNewestTable is the compatibility half: a driver
// that names no revision (an older page, the offline WASM driver, ci-smoke's
// raw curl) keeps the pre-M1 behaviour exactly — newest table, positional index.
func TestEventWithoutRevFallsBackToNewestTable(t *testing.T) {
	s, base, tok := serverFromDocs(t, staleHandlerDocs()...)
	s.mu.Lock()
	s.rt.State["extra"] = true
	s.bump()
	s.mu.Unlock()

	postEventAtRev(t, base, tok, 0, nil)
	s.mu.Lock()
	a, b := s.rt.State["a"], s.rt.State["b"]
	s.mu.Unlock()
	if a != float64(1) || b != float64(0) {
		t.Errorf("a rev-less event must resolve against the newest table (doA): a=%v b=%v", a, b)
	}
}

// TestEventWithEvictedRevFallsBackToNewestTable: the ring keeps a bounded
// number of frames. A revision older than the ring is unknown, so it degrades
// to the newest table rather than dropping the human's click on the floor.
func TestEventWithEvictedRevFallsBackToNewestTable(t *testing.T) {
	s, base, tok := serverFromDocs(t, staleHandlerDocs()...)
	revN := s.rev.Load()
	s.mu.Lock()
	s.rt.State["extra"] = true
	for i := 0; i < handlerHistory+2; i++ {
		s.bump()
	}
	s.mu.Unlock()

	s.mu.Lock()
	_, known := func() (int, bool) {
		for i := range s.handlerHist {
			if s.handlerHist[i].rev == revN {
				return i, true
			}
		}
		return 0, false
	}()
	s.mu.Unlock()
	if known {
		t.Fatalf("fixture: rev %d should have been evicted from the ring", revN)
	}

	postEventAtRev(t, base, tok, 0, &revN)
	s.mu.Lock()
	a, b := s.rt.State["a"], s.rt.State["b"]
	s.mu.Unlock()
	if a != float64(1) || b != float64(0) {
		t.Errorf("an evicted revision must fall back to the newest table: a=%v b=%v", a, b)
	}
}

// TestSetHandlersOverwritesSameRevision: serveIndex, /poll and the SSE catch-up
// all re-render the CURRENT revision without advancing it. Those must refresh
// that revision's entry, never stack duplicates that push live frames out of
// the ring.
func TestSetHandlersOverwritesSameRevision(t *testing.T) {
	s, base, _ := serverFromDocs(t, staleHandlerDocs()...)
	s.mu.Lock()
	before := len(s.handlerHist)
	s.mu.Unlock()
	for i := 0; i < 4; i++ {
		getPage(t, base+"/")
	}
	s.mu.Lock()
	after := len(s.handlerHist)
	s.mu.Unlock()
	if after != before {
		t.Errorf("re-rendering the same revision must not grow the ring: %d -> %d", before, after)
	}
}
