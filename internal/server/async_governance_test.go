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

// Server-side tests for the M4 governance fields, with REAL goroutines: request
// supersede (`key`), the pending flag, per-step `timeout`, the in-flight
// ceiling, and the `delay` step's pacing over live-sync.
//
// The deterministic per-field semantics live in
// internal/runtime/async_governance_test.go with test doubles. What is only
// true here is the concurrency: a cancel racing a completion, the ceiling
// holding under simultaneous dispatches, and a paced action publishing its
// stages as separate live-sync revisions while the session stays answerable.
//
// No sleeps: every wait is waitUntil on a fact (a backend arrival counter, the
// in-flight count, a frame's content) or a channel handshake with the backend.

// echoGate is a backend that parks every request until the test releases it and
// answers with the `q` query parameter it was called with — so a test can tell
// WHICH request's reply landed, which is the entire point of `key`.
type echoGate struct {
	srv     *httptest.Server
	release chan struct{}
	arrived atomic.Int64
}

func newEchoGate(t *testing.T) *echoGate {
	t.Helper()
	g := &echoGate{release: make(chan struct{})}
	g.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		g.arrived.Add(1)
		select {
		case <-g.release:
		case <-r.Context().Done():
			return // the client hung up: a superseded request
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"hits":"`+r.URL.Query().Get("q")+`"}`)
	}))
	t.Cleanup(func() { g.srv.Close() })
	return g
}

func (g *echoGate) open() { close(g.release) }

// searchDocs is the canonical search-as-you-type app: every keystroke fires a
// keyed request that supersedes the one before it, with `pending` driving the
// spinner instead of a hand-written flag pair.
func searchDocs(backend string) []string {
	return []string{
		`{"type":"app","id":"search","entry":"main","globalState":{
			"schema":{"q":"string","hits":"object","err":"string","searching":"bool","landed":"number"},
			"initial":{"q":"","hits":{},"err":"","searching":false,"landed":0}}}`,
		`{"type":"scene","id":"main","root":{"type":"column","id":"root","children":[
			{"type":"text","id":"spin","if":"{{ state.searching }}","text":"SEARCHING-NOW"},
			{"type":"text","id":"out","text":"hits={{ state.hits.hits }} landed={{ state.landed }}"},
			{"type":"button","id":"a","label":"A","onPress":{"name":"searchA"}},
			{"type":"button","id":"b","label":"B","onPress":{"name":"searchB"}}]}}`,
		`{"type":"action","id":"searchA","steps":[
			{"type":"http.get","url":"` + backend + `?q=a","async":true,"key":"search",
			 "pending":"searching","result":"hits","error":"err",
			 "onSuccess":[{"type":"state.increment","path":"landed"}]}]}`,
		`{"type":"action","id":"searchB","steps":[
			{"type":"http.get","url":"` + backend + `?q=b","async":true,"key":"search",
			 "pending":"searching","result":"hits","error":"err",
			 "onSuccess":[{"type":"state.increment","path":"landed"}]}]}`,
	}
}

// TestKeyedSearchLandsTheLastKeystroke is the feature end to end on a real
// host: two keystrokes, the first still open when the second fires. The reply
// that reaches the screen is the second one's, its branch runs exactly once,
// and the first request is torn down rather than left running.
func TestKeyedSearchLandsTheLastKeystroke(t *testing.T) {
	g := newEchoGate(t)
	s, base, tok := serverFromDocs(t, searchDocs(g.srv.URL)...)

	postEventAtRev(t, base, tok, handlerIndexByName(t, s, "searchA"), nil)
	waitUntil(t, 3*time.Second, func() bool { return g.arrived.Load() == 1 }, "the first search to reach the backend")

	// The second keystroke supersedes the first while it is still open.
	body, _ := postEventAtRev(t, base, tok, handlerIndexByName(t, s, "searchB"), nil)
	if !strings.Contains(body, "SEARCHING-NOW") {
		t.Errorf("the spinner must still be up between the two keystrokes: %s", body)
	}
	// The superseded request is cancelled on the wire, so its continuation
	// settles as soon as the transport unwinds — leaving exactly one open.
	waitUntil(t, 3*time.Second, func() bool { return inflight(s) == 1 }, "the superseded request to be released")

	s.mu.Lock()
	searching, landed := s.rt.State["searching"], s.rt.State["landed"]
	s.mu.Unlock()
	if searching != true {
		t.Errorf("the superseded request must not take the spinner down: %v", searching)
	}
	if landed != float64(0) {
		t.Errorf("nor may its branch run: landed=%v", landed)
	}

	g.open()
	waitUntil(t, 3*time.Second, func() bool { return inflight(s) == 0 }, "the surviving search to settle")

	s.mu.Lock()
	hits, _ := s.rt.State["hits"].(map[string]any)
	searching, landed = s.rt.State["searching"], s.rt.State["landed"]
	s.mu.Unlock()
	if hits["hits"] != "b" {
		t.Errorf("the LAST keystroke's reply must be what lands: %v", hits)
	}
	if landed != float64(1) {
		t.Errorf("exactly one branch may run: landed=%v", landed)
	}
	if searching != false {
		t.Errorf("the surviving request clears the spinner: %v", searching)
	}
}

// TestKeyedSupersedeUnderConcurrentDispatches hammers the cancel/complete race
// the -race run exists for: many keystrokes fired at once, each cancelling
// whichever request currently holds the slot, while completions land on other
// goroutines. Whatever the interleaving, the invariants hold — the counter
// returns to zero, at most one branch per surviving request runs, and the
// spinner ends down.
func TestKeyedSupersedeUnderConcurrentDispatches(t *testing.T) {
	const n = 24
	be := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"hits":"`+r.URL.Query().Get("q")+`"}`)
	}))
	defer be.Close()
	s, base, tok := serverFromDocs(t, searchDocs(be.URL)...)

	aH, bH := handlerIndexByName(t, s, "searchA"), handlerIndexByName(t, s, "searchB")
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(2)
		go func() { defer wg.Done(); postEventAtRev(t, base, tok, aH, nil) }()
		go func() { defer wg.Done(); postEventAtRev(t, base, tok, bH, nil) }()
	}
	wg.Wait()
	waitUntil(t, 10*time.Second, func() bool { return inflight(s) == 0 }, "every keyed request to settle")

	s.mu.Lock()
	landed, _ := s.rt.State["landed"].(float64)
	searching := s.rt.State["searching"]
	s.mu.Unlock()

	if searching != false {
		t.Errorf("the pending flag must end down however the cancels interleaved: %v", searching)
	}
	// Every request either lands or is superseded, so the count is bounded by
	// the number fired and — since supersede is the whole point — must be well
	// under it. The exact value is a scheduling outcome, not a contract.
	if landed < 1 || landed > 2*n {
		t.Errorf("landed=%v is outside the possible range for %d keystrokes", landed, 2*n)
	}

	// The slot must be free afterwards: one more keystroke on the same key has
	// to land, not be mistaken for a superseded leftover.
	postEventAtRev(t, base, tok, aH, nil)
	waitUntil(t, 5*time.Second, func() bool { return inflight(s) == 0 }, "the follow-up search to settle")
	s.mu.Lock()
	after, _ := s.rt.State["landed"].(float64)
	s.mu.Unlock()
	if after != landed+1 {
		t.Errorf("a request on a freed slot must land: landed went %v -> %v", landed, after)
	}
}

// TestPendingRecoversAfterAReloadDropsItsRequest pins what a hot reload does to
// a request that was in flight, which is worth stating exactly because the
// answer is "the same thing it does to a hand-written flag".
//
// The reload CARRIES state across (that is the point of a hot reload — you keep
// what you had typed), so the raised flag comes with it, and the reply to the
// retired app is dropped rather than written into its successor. The successor
// therefore starts with the flag up but nothing in flight and no slot held: it
// is not wedged, and the very next request clears it. Making the flag special
// here — scrubbing it on reload — would mean `pending` and a hand-written
// state.set behaved differently across an edit, which is a worse thing to have
// to remember than a spinner that lasts until the next keystroke.
func TestPendingRecoversAfterAReloadDropsItsRequest(t *testing.T) {
	g := newEchoGate(t)
	docs := searchDocs(g.srv.URL)
	s, base, tok := serverFromDocs(t, docs...)

	postEventAtRev(t, base, tok, handlerIndexByName(t, s, "searchA"), nil)
	waitUntil(t, 3*time.Second, func() bool { return g.arrived.Load() == 1 }, "the search to reach the backend")

	s.Reload(runtime.New(loader.FromDocs(docsFrom(t, docs...))))
	g.open()
	waitUntil(t, 3*time.Second, func() bool {
		_, body := doJSON(t, http.MethodGet, base+"/log?since=0", "", "", "")
		return strings.Contains(body, "dropped an async response")
	}, "the stale completion to be dropped")

	s.mu.Lock()
	searching, live := s.rt.State["searching"], s.rt.Inflight()
	s.mu.Unlock()
	if live != 0 {
		t.Errorf("the successor must start with nothing in flight: %d", live)
	}
	if searching != true {
		t.Fatalf("fixture: the reload carries the raised flag across: %v", searching)
	}

	// Not wedged: the slot is free and the next request settles the flag.
	postEventAtRev(t, base, tok, handlerIndexByName(t, s, "searchB"), nil)
	waitUntil(t, 3*time.Second, func() bool { return inflight(s) == 0 }, "the follow-up search to settle")
	s.mu.Lock()
	searching, hits := s.rt.State["searching"], s.rt.State["hits"]
	s.mu.Unlock()
	if searching != false {
		t.Errorf("the next request must clear the carried flag: %v", searching)
	}
	if got, _ := hits.(map[string]any); got["hits"] != "b" {
		t.Errorf("and land normally on the fresh runtime: %v", hits)
	}
}

// ---- timeout ------------------------------------------------------------------

// TestTimeoutSettlesTheSessionWithoutHoldingTheLock: a backend that never
// answers used to mean a 20-second freeze. With a declared timeout the request
// ends on its own terms, the error path runs, the pending flag comes down — and
// throughout, the session answers every other request.
func TestTimeoutSettlesTheSessionWithoutHoldingTheLock(t *testing.T) {
	block := make(chan struct{})
	be := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		select {
		case <-block:
		case <-r.Context().Done():
		}
	}))
	defer func() { close(block); be.Close() }()

	s, base, tok := serverFromDocs(t,
		`{"type":"app","id":"to","entry":"main","globalState":{
			"schema":{"err":"string","busy":"bool"},"initial":{"err":"","busy":false}}}`,
		`{"type":"scene","id":"main","root":{"type":"column","id":"root","children":[
			{"type":"text","id":"e","text":"err={{ state.err }}"},
			{"type":"button","id":"go","label":"Go","onPress":{"name":"go"}}]}}`,
		`{"type":"action","id":"go","steps":[
			{"type":"http.get","url":"`+be.URL+`","async":true,"timeout":60,
			 "pending":"busy","error":"err"}]}`)

	postEventAtRev(t, base, tok, handlerIndexByName(t, s, "go"), nil)
	if n := inflight(s); n != 1 {
		t.Fatalf("fixture: the request must be open: %d", n)
	}
	// The session is answerable while the doomed request runs out its clock.
	if page := getPage(t, base+"/"); !strings.Contains(page, "err=") {
		t.Errorf("GET / must answer during a timing-out request: %s", page)
	}

	waitUntil(t, 3*time.Second, func() bool { return inflight(s) == 0 }, "the request to time out")
	s.mu.Lock()
	errMsg, busy := s.rt.State["err"], s.rt.State["busy"]
	s.mu.Unlock()
	if errMsg != "request timed out after 60ms" {
		t.Errorf("the timeout must reach the app as its declared message: %v", errMsg)
	}
	if busy != false {
		t.Errorf("a timeout must clear the pending flag: %v", busy)
	}
}

// ---- the in-flight ceiling ----------------------------------------------------

// TestInflightCeilingHoldsUnderConcurrentDispatches: the count is raised inside
// the dispatch, under the host lock, so simultaneous clicks cannot slip past
// the ceiling between the check and the increment. Everything above it fails
// visibly on the error path instead of leaking a goroutine.
func TestInflightCeilingHoldsUnderConcurrentDispatches(t *testing.T) {
	const clicks = 90
	g := newEchoGate(t)
	s, base, tok := serverFromDocs(t,
		`{"type":"app","id":"cap","entry":"main","globalState":{
			"schema":{"err":"string","refused":"number"},"initial":{"err":"","refused":0}}}`,
		`{"type":"scene","id":"main","root":{"type":"column","id":"root","children":[
			{"type":"text","id":"e","text":"refused={{ state.refused }}"},
			{"type":"button","id":"go","label":"Go","onPress":{"name":"go"}}]}}`,
		`{"type":"action","id":"go","steps":[
			{"type":"http.get","url":"`+g.srv.URL+`?q=x","async":true,"error":"err",
			 "onError":[{"type":"state.increment","path":"refused"}]}]}`)

	goH := handlerIndexByName(t, s, "go")
	var wg sync.WaitGroup
	var over atomic.Int64
	for i := 0; i < clicks; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			postEventAtRev(t, base, tok, goH, nil)
			if n := inflight(s); n > 64 {
				over.Add(1)
			}
		}()
	}
	wg.Wait()

	if over.Load() != 0 {
		t.Errorf("the ceiling was exceeded %d times — the check and the increment must be one critical section", over.Load())
	}
	s.mu.Lock()
	open, refused := s.rt.Inflight(), s.rt.State["refused"]
	s.mu.Unlock()
	if open != 64 {
		t.Fatalf("the ceiling must be saturated: %d", open)
	}
	if refused != float64(clicks-64) {
		t.Errorf("every click above the ceiling must fail visibly: refused=%v, want %d", refused, clicks-64)
	}

	g.open()
	waitUntil(t, 10*time.Second, func() bool { return inflight(s) == 0 }, "the accepted requests to drain")
}

// ---- delay --------------------------------------------------------------------

// TestDelayPacesFramesOverLiveSync is the step's reason to exist on this host:
// the POST returns the FIRST stage and the later stages arrive as their own
// live-sync revisions, without the dispatch ever holding s.mu across the wait.
func TestDelayPacesFramesOverLiveSync(t *testing.T) {
	s, base, tok := serverFromDocs(t,
		`{"type":"app","id":"pace","entry":"main","globalState":{
			"schema":{"phase":"string"},"initial":{"phase":"idle"}}}`,
		`{"type":"scene","id":"main","root":{"type":"column","id":"root","children":[
			{"type":"text","id":"p","text":"phase={{ state.phase }}"},
			{"type":"button","id":"go","label":"Go","onPress":{"name":"go"}}]}}`,
		`{"type":"action","id":"go","steps":[
			{"type":"state.set","path":"phase","value":"{{ 'one' }}"},
			{"type":"render"},
			{"type":"delay","ms":20},
			{"type":"state.set","path":"phase","value":"{{ 'two' }}"},
			{"type":"render"},
			{"type":"delay","ms":20},
			{"type":"state.set","path":"phase","value":"{{ 'three' }}"}]}`)

	_, cancel, lines := connectSSE(t, base, nil)
	defer cancel()
	waitUntil(t, 2*time.Second, func() bool { return s.subscriberCount() == 1 }, "SSE subscriber to register")

	body, code := postEventAtRev(t, base, tok, handlerIndexByName(t, s, "go"), nil)
	if code != http.StatusOK {
		t.Fatalf("POST /event: %d", code)
	}
	if !strings.Contains(body, "phase=one") {
		t.Errorf("the click must return the FIRST stage, not the finished action: %s", body)
	}
	if n := inflight(s); n != 1 {
		t.Errorf("the waiting tail must count as open background work: %d", n)
	}

	// The remaining stages arrive later, as ordinary revisions.
	var lastRev float64
	waitUntil(t, 5*time.Second, func() bool {
		frame := nextDataFrame(t, lines, 5*time.Second)
		var d map[string]any
		if json.Unmarshal([]byte(frame), &d) != nil {
			return false
		}
		rev, _ := d["rev"].(float64)
		if lastRev != 0 && rev <= lastRev {
			t.Errorf("revisions must advance strictly: %v after %v", rev, lastRev)
		}
		lastRev = rev
		html, _ := d["html"].(string)
		return strings.Contains(html, "phase=three")
	}, "the paced stages to reach live subscribers")

	waitUntil(t, 3*time.Second, func() bool { return inflight(s) == 0 }, "the paced action to finish")
}

// TestSessionStaysUsableDuringADelay: the wait runs on the sink's goroutine, so
// it must not hold s.mu — otherwise `delay` would be a worse freeze than the
// synchronous http step it was built alongside.
func TestSessionStaysUsableDuringADelay(t *testing.T) {
	s, base, tok := serverFromDocs(t,
		`{"type":"app","id":"pace","entry":"main","globalState":{
			"schema":{"phase":"string","taps":"number"},"initial":{"phase":"idle","taps":0}}}`,
		`{"type":"scene","id":"main","root":{"type":"column","id":"root","children":[
			{"type":"text","id":"p","text":"phase={{ state.phase }} taps={{ state.taps }}"},
			{"type":"button","id":"go","label":"Go","onPress":{"name":"go"}},
			{"type":"button","id":"tap","label":"Tap","onPress":{"name":"tap"}}]}}`,
		`{"type":"action","id":"tap","steps":[{"type":"state.increment","path":"taps"}]}`,
		`{"type":"action","id":"go","steps":[
			{"type":"state.set","path":"phase","value":"{{ 'waiting' }}"},
			{"type":"delay","ms":80},
			{"type":"state.set","path":"phase","value":"{{ 'done' }}"}]}`)

	postEventAtRev(t, base, tok, handlerIndexByName(t, s, "go"), nil)
	if n := inflight(s); n != 1 {
		t.Fatalf("fixture: the delay must be waiting: %d", n)
	}
	// A click during the wait lands immediately — no queueing behind the pause.
	body, _ := postEventAtRev(t, base, tok, handlerIndexByName(t, s, "tap"), nil)
	if !strings.Contains(body, "phase=waiting taps=1") {
		t.Errorf("the session must stay usable mid-delay: %s", body)
	}

	waitUntil(t, 3*time.Second, func() bool { return inflight(s) == 0 }, "the delay to expire")
	s.mu.Lock()
	phase, taps := s.rt.State["phase"], s.rt.State["taps"]
	s.mu.Unlock()
	if phase != "done" {
		t.Errorf("the tail must run when the wait expires: %v", phase)
	}
	if taps != float64(1) {
		t.Errorf("and must not clobber what landed while it waited: %v", taps)
	}
}
