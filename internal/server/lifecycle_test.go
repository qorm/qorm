package server

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/qorm/platform/internal/loader"
	"github.com/qorm/platform/internal/runtime"
)

// Server-level tests for the execution-model additions: the scene onEnter
// lifecycle hook and the declarative timer's dispatch chain.

// lifecycleDocs is an app with an entry scene carrying onEnter + a polling
// timer, and a second scene with its own onEnter (for deep-link tests).
func lifecycleDocs() []map[string]any {
	raw := []string{
		`{"type":"app","id":"lc","entry":"main","globalState":{
			"schema":{"mainEnters":"number","detailEnters":"number","ticks":"number"},
			"initial":{"mainEnters":0,"detailEnters":0,"ticks":0}}}`,
		`{"type":"scene","id":"main","onEnter":"enterMain","root":{"type":"column","id":"root","children":[
			{"type":"text","id":"m","text":"main entered {{ state.mainEnters }}"},
			{"type":"timer","id":"poll","every":1000,"onTick":"tick"},
			{"type":"button","id":"go","label":"Go","onPress":{"name":"goDetail"}}]}}`,
		`{"type":"scene","id":"detail","onEnter":"enterDetail","root":{"type":"column","id":"droot","children":[
			{"type":"text","id":"d","text":"detail entered {{ state.detailEnters }}"}]}}`,
		`{"type":"action","id":"enterMain","steps":[{"type":"state.increment","path":"mainEnters"}]}`,
		`{"type":"action","id":"enterDetail","steps":[{"type":"state.increment","path":"detailEnters"}]}`,
		`{"type":"action","id":"tick","steps":[{"type":"state.increment","path":"ticks"}]}`,
		`{"type":"action","id":"goDetail","steps":[{"type":"navigate","to":"detail"}]}`,
	}
	docs := make([]map[string]any, len(raw))
	for i, s := range raw {
		if err := json.Unmarshal([]byte(s), &docs[i]); err != nil {
			panic(err)
		}
	}
	return docs
}

func lifecycleServer(t *testing.T) (*Server, string) {
	t.Helper()
	app := loader.FromDocs(lifecycleDocs())
	if len(app.Diagnostics) != 0 {
		t.Fatalf("lifecycle fixture must load clean: %v", app.Diagnostics)
	}
	s := New(runtime.New(app))
	ts := httptest.NewServer(s.Handler())
	t.Cleanup(ts.Close)
	return s, ts.URL
}

func getPage(t *testing.T, url string) string {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatalf("get %s: %v", url, err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return string(b)
}

func TestOnEnterFiresOnFirstPageLoad(t *testing.T) {
	_, base := lifecycleServer(t)
	page := getPage(t, base+"/")
	if !strings.Contains(page, "main entered 1") {
		t.Fatalf("the entry scene's onEnter must run before the first render: %s", excerpt(page, "main entered"))
	}
	// A refresh of the live session must NOT replay the hook.
	page = getPage(t, base+"/")
	if !strings.Contains(page, "main entered 1") {
		t.Errorf("a refresh must not replay onEnter: %s", excerpt(page, "main entered"))
	}
}

func TestOnEnterFiresOnDeepLink(t *testing.T) {
	_, base := lifecycleServer(t)
	page := getPage(t, base+"/?scene=detail")
	if !strings.Contains(page, "detail entered 1") {
		t.Fatalf("a deep link straight into a scene must fire its onEnter: %s", excerpt(page, "detail entered"))
	}
	// Refresh of the same deep link: still on the scene — no replay.
	page = getPage(t, base+"/?scene=detail")
	if !strings.Contains(page, "detail entered 1") {
		t.Errorf("a deep-link refresh must not replay onEnter: %s", excerpt(page, "detail entered"))
	}
}

func TestOnEnterFiresOnActionNavigation(t *testing.T) {
	s, base := lifecycleServer(t)
	tok := pageEventToken(t, base)
	// Find the Go button's handler and press it (navigates main -> detail).
	h := handlerIndexByName(t, s, "goDetail")
	html, _ := postEventH(t, base, tok, h)
	if !strings.Contains(html, "detail entered 1") {
		t.Fatalf("navigating via an action must fire the target scene's onEnter in the same response: %s", excerpt(html, "detail entered"))
	}
}

func TestOnEnterFiresOnClientNavigateAndBack(t *testing.T) {
	s, base := lifecycleServer(t)
	tok := pageEventToken(t, base)
	postNavigate(t, base, tok, map[string]any{"scene": "detail"})
	postNavigate(t, base, tok, map[string]any{"back": true})
	s.mu.Lock()
	enters := s.rt.State["mainEnters"]
	dEnters := s.rt.State["detailEnters"]
	s.mu.Unlock()
	if dEnters != float64(1) {
		t.Errorf("client navigation must fire the target's onEnter: detailEnters=%v", dEnters)
	}
	if enters != float64(2) {
		t.Errorf("navigating back re-enters the previous scene: mainEnters=%v", enters)
	}
}

func TestOnEnterNotReplayedOnSSEReconnect(t *testing.T) {
	s, base := lifecycleServer(t)
	getPage(t, base+"/") // initial entry fired
	// A reconnecting EventSource replays Last-Event-Id / ?rev — the catch-up
	// render must not re-fire the hook.
	req, _ := http.NewRequest(http.MethodGet, base+"/events?rev=0", nil)
	req.Header.Set("Last-Event-Id", "0")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 1024)
	resp.Body.Read(buf) // read the preamble, then drop the stream
	resp.Body.Close()
	s.mu.Lock()
	enters := s.rt.State["mainEnters"]
	s.mu.Unlock()
	if enters != float64(1) {
		t.Errorf("an SSE reconnect must not replay onEnter: mainEnters=%v", enters)
	}
}

func TestOnEnterNotReplayedOnHotReload(t *testing.T) {
	s, base := lifecycleServer(t)
	getPage(t, base+"/") // initial entry fired (mainEnters=1)
	next := runtime.New(loader.FromDocs(lifecycleDocs()))
	s.Reload(next)
	s.mu.Lock()
	enters := s.rt.State["mainEnters"]
	s.mu.Unlock()
	if enters != float64(1) {
		t.Errorf("a hot reload continues the session and must not replay onEnter: mainEnters=%v", enters)
	}
}

func TestTimerMarkerAndDispatchOverHTTP(t *testing.T) {
	s, base := lifecycleServer(t)
	tok := pageEventToken(t, base)
	page := getPage(t, base+"/")
	if !strings.Contains(page, `data-qorm-timer="poll"`) || !strings.Contains(page, `data-every="1000"`) {
		t.Fatalf("the timer marker must render into the served page")
	}
	// Fire the tick exactly as the client scheduler does: POST /event with the
	// marker's handler index (the same chain as a button press).
	h := handlerIndexByName(t, s, "tick")
	html, _ := postEventH(t, base, tok, h)
	if !strings.Contains(html, `data-qorm-timer="poll"`) {
		t.Errorf("the re-rendered body still carries the marker (idempotent morph target)")
	}
	s.mu.Lock()
	ticks := s.rt.State["ticks"]
	s.mu.Unlock()
	if ticks != float64(1) {
		t.Errorf("a timer tick must dispatch onTick: ticks=%v", ticks)
	}
}

func TestTimerRemovedWhenConditionFalsy(t *testing.T) {
	// A timer guarded by `if` disappears from the rendered HTML when the
	// condition turns falsy — the client stops the schedule on the next sync.
	docs := lifecycleDocs()
	var scene map[string]any
	json.Unmarshal([]byte(`{"type":"scene","id":"main","onEnter":"enterMain","root":{"type":"column","id":"root","children":[
		{"type":"timer","id":"cd","every":1000,"onTick":"tick","if":"{{ state.ticks < 2 }}"}]}}`), &scene)
	docs[1] = scene
	app := loader.FromDocs(docs)
	if len(app.Diagnostics) != 0 {
		t.Fatalf("fixture must load clean: %v", app.Diagnostics)
	}
	s := New(runtime.New(app))
	ts := httptest.NewServer(s.Handler())
	t.Cleanup(ts.Close)
	tok := pageEventToken(t, ts.URL)
	if page := getPage(t, ts.URL+"/"); !strings.Contains(page, `data-qorm-timer="cd"`) {
		t.Fatal("timer must render while its condition is truthy")
	}
	h := handlerIndexByName(t, s, "tick")
	postEventH(t, ts.URL, tok, h)
	html, _ := postEventH(t, ts.URL, tok, handlerIndexByName(t, s, "tick"))
	if strings.Contains(html, `data-qorm-timer="cd"`) {
		t.Errorf("timer must leave the rendered tree once its condition is falsy: %s", html)
	}
}

// TestTimerConcurrentTicks exercises timer ticks racing user events and SSE
// broadcasts; run with -race, the server mutex must serialize them all.
func TestTimerConcurrentTicks(t *testing.T) {
	s, base := lifecycleServer(t)
	tok := pageEventToken(t, base)
	tickH := handlerIndexByName(t, s, "tick")
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(2)
		go func() { defer wg.Done(); postEventH(t, base, tok, tickH) }()
		go func() { defer wg.Done(); getPage(t, base+"/") }()
	}
	wg.Wait()
	s.mu.Lock()
	ticks := s.rt.State["ticks"]
	s.mu.Unlock()
	if ticks != float64(8) {
		t.Errorf("all concurrent ticks must dispatch exactly once each: ticks=%v", ticks)
	}
}

// ---- helpers ----------------------------------------------------------------

// handlerIndexByName renders the current scene server-side and returns the
// index of the first handler dispatching the named action.
func handlerIndexByName(t *testing.T, s *Server, name string) int {
	t.Helper()
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.handlers == nil {
		t.Fatal("handler table not primed — GET / first")
	}
	for i, h := range s.handlers {
		if h.Name == name {
			return i
		}
	}
	t.Fatalf("no handler dispatches %q: %+v", name, s.handlers)
	return -1
}

func postEventH(t *testing.T, base, tok string, h int) (string, int) {
	t.Helper()
	body, _ := json.Marshal(map[string]any{"h": h})
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

func postNavigate(t *testing.T, base, tok string, body map[string]any) {
	t.Helper()
	b, _ := json.Marshal(body)
	req, _ := http.NewRequest(http.MethodPost, base+"/navigate", strings.NewReader(string(b)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Qorm-Token", tok)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("post /navigate: %v", err)
	}
	resp.Body.Close()
}

// excerpt returns the region of s around the first occurrence of frag's prefix
// (for readable failures on large HTML bodies).
func excerpt(s, frag string) string {
	i := strings.Index(s, frag[:len(frag)/2])
	if i < 0 {
		if len(s) > 400 {
			return s[:400] + "..."
		}
		return s
	}
	end := i + 120
	if end > len(s) {
		end = len(s)
	}
	return s[i:end]
}

// TestSweepLifecycleExample drives the shipped examples/lifecycle app through
// the real HTTP surface: onEnter data load, the polling + one-shot timers'
// markers, the countdown's start -> tick -> stop cycle (the `if`-guarded timer
// leaves the tree when the countdown stops), and the form's if/else branches
// with an invoke step.
func TestSweepLifecycleExample(t *testing.T) {
	s, base, tok := exampleServer(t, "lifecycle")
	page := getPage(t, base+"/")
	// onEnter loaded the data before the first render.
	if !strings.Contains(page, "onEnter: loaded") || !strings.Contains(page, "Alpha (loaded on enter)") {
		t.Fatalf("onEnter must load data before the first page render")
	}
	// Both timers render markers; the countdown timer (running=false) does not.
	for _, want := range []string{`data-qorm-timer="poll"`, `data-qorm-timer="hint_once"`, `data-after="2000"`} {
		if !strings.Contains(page, want) {
			t.Errorf("page must contain %s", want)
		}
	}
	if strings.Contains(page, `data-qorm-timer="countdown"`) {
		t.Error("the countdown timer must not render while running is false")
	}
	// A refresh must not re-run the load (items stay 2, status stays loaded).
	if page2 := getPage(t, base+"/"); strings.Count(page2, "loaded on enter") != strings.Count(page, "loaded on enter") {
		t.Error("a refresh must not replay onEnter's load")
	}
	// Start the countdown: its timer appears; ticks count it down; at zero the
	// timer leaves the tree and the finish message (via invoke) shows.
	html, _ := postEventH(t, base, tok, handlerIndexByName(t, s, "startCountdown"))
	if !strings.Contains(html, "Countdown: 5") || !strings.Contains(html, `data-qorm-timer="countdown"`) {
		t.Fatalf("starting the countdown must arm its timer")
	}
	for i := 0; i < 5; i++ {
		html, _ = postEventH(t, base, tok, handlerIndexByName(t, s, "tick"))
	}
	if !strings.Contains(html, "Countdown: 0") {
		t.Errorf("five ticks must count 5 down to 0")
	}
	if strings.Contains(html, `data-qorm-timer="countdown"`) {
		t.Errorf("the countdown timer must stop (leave the tree) at zero")
	}
	if !strings.Contains(html, "Countdown finished.") {
		t.Errorf("the final tick must invoke finishCountdown")
	}
	// Form: empty name takes the else branch; a name takes then + invoke reset.
	html, _ = sweepEvent(t, base, tok, handlerIndexByName(t, s, "submitForm"), map[string]any{"name": ""})
	if !strings.Contains(html, "Please enter a name first.") {
		t.Errorf("empty submit must take the else branch")
	}
	html, _ = sweepEvent(t, base, tok, handlerIndexByName(t, s, "submitForm"), map[string]any{"name": "Ada"})
	if !strings.Contains(html, "Hello, Ada!") {
		t.Errorf("named submit must take the then branch")
	}
	s.mu.Lock()
	name := s.rt.State["name"]
	s.mu.Unlock()
	if name != "" {
		t.Errorf("the invoke step must have reset the name: %v", name)
	}
}
