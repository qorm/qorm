package server

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/qorm/qorm/internal/model"
	qrt "github.com/qorm/qorm/internal/runtime"
)

// sseLines streams the raw lines of an SSE response body on a channel.
func sseLines(t *testing.T, resp *http.Response) <-chan string {
	t.Helper()
	ch := make(chan string, 64)
	go func() {
		defer close(ch)
		sc := bufio.NewScanner(resp.Body)
		for sc.Scan() {
			ch <- sc.Text()
		}
	}()
	return ch
}

// nextDataFrame returns the payload of the next "data: " frame, skipping
// comment / id lines. Fails the test on timeout or stream close.
func nextDataFrame(t *testing.T, lines <-chan string, timeout time.Duration) string {
	t.Helper()
	deadline := time.After(timeout)
	for {
		select {
		case line, ok := <-lines:
			if !ok {
				t.Fatal("SSE stream closed before a data frame arrived")
			}
			if strings.HasPrefix(line, "data: ") {
				return strings.TrimPrefix(line, "data: ")
			}
		case <-deadline:
			t.Fatal("no SSE data frame within timeout")
		}
	}
}

// connectSSE opens /events with extra headers and returns the response plus a
// cancel func that tears the connection down.
func connectSSE(t *testing.T, base string, headers map[string]string) (*http.Response, func(), <-chan string) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/events", nil)
	if err != nil {
		cancel()
		t.Fatalf("new request: %v", err)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		cancel()
		t.Fatalf("connect /events: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		cancel()
		t.Fatalf("/events status = %d", resp.StatusCode)
	}
	return resp, func() { cancel(); resp.Body.Close() }, sseLines(t, resp)
}

func (s *Server) subscriberCount() int {
	s.subsMu.Lock()
	defer s.subsMu.Unlock()
	return len(s.subs)
}

// agentIncrement dispatches the counter's increment action over MCP.
func agentIncrement(t *testing.T, base string) {
	t.Helper()
	post(t, base+"/mcp", `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"qorm_dispatch","arguments":{"action":"increment","args":{"count":0}}}}`)
}

// TestSSECatchUpAfterReconnect: docs/collaboration.md promises SSE "to keep
// every viewer in sync". A viewer whose connection drops while an edit lands
// reconnects (EventSource does this automatically, replaying the last event
// id) and must be resynced — not left stale until the next mutation. The
// server must honour Last-Event-Id by pushing the current UI on connect.
func TestSSECatchUpAfterReconnect(t *testing.T) {
	s := counterServer(t)
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()

	// Viewer connects, then the connection drops.
	_, drop, lines1 := connectSSE(t, ts.URL, nil)
	select {
	case line := <-lines1:
		if line != ": connected" {
			t.Fatalf("first SSE line = %q", line)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("no SSE handshake")
	}
	drop()
	waitUntil(t, 2*time.Second, func() bool { return s.subscriberCount() == 0 }, "subscriber removal after drop")

	// An edit lands while the viewer is gone.
	agentIncrement(t, ts.URL)

	// The browser auto-reconnects and replays the last event id it saw (0: it
	// only ever received the handshake). It must immediately receive the
	// current UI — the count=1 it missed.
	_, drop2, lines2 := connectSSE(t, ts.URL, map[string]string{"Last-Event-Id": "0"})
	defer drop2()
	frame := nextDataFrame(t, lines2, 3*time.Second)
	var d struct {
		Rev  int64  `json:"rev"`
		HTML string `json:"html"`
	}
	if err := json.Unmarshal([]byte(frame), &d); err != nil {
		t.Fatalf("catch-up frame not JSON: %v (%s)", err, frame)
	}
	if d.Rev < 1 || !strings.Contains(d.HTML, ">1<") {
		t.Fatalf("catch-up frame must carry the missed UI (count=1), got %+v", d)
	}
}

// TestSSECatchUpNotSentWhenCurrent: a reconnecting viewer whose last event id
// is already the tip receives no redundant snapshot — the next real frame is
// the first data frame.
func TestSSECatchUpNotSentWhenCurrent(t *testing.T) {
	s := counterServer(t)
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()

	agentIncrement(t, ts.URL) // rev -> 1 with no subscribers

	_, drop, lines := connectSSE(t, ts.URL, map[string]string{"Last-Event-Id": "1"})
	defer drop()
	agentIncrement(t, ts.URL) // rev -> 2

	frame := nextDataFrame(t, lines, 3*time.Second)
	var d struct {
		Rev int64 `json:"rev"`
	}
	if err := json.Unmarshal([]byte(frame), &d); err != nil {
		t.Fatalf("frame not JSON: %v (%s)", err, frame)
	}
	if d.Rev != 2 {
		t.Fatalf("an up-to-date reconnect must not get a stale snapshot: first frame rev = %d, want 2", d.Rev)
	}
}

// TestSSEFramesCarryEventID: every data frame ships an id line carrying its
// revision, so the browser tracks Last-Event-Id and can replay it on
// reconnect (the catch-up above).
func TestSSEFramesCarryEventID(t *testing.T) {
	s := counterServer(t)
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()

	_, drop, lines := connectSSE(t, ts.URL, nil)
	defer drop()
	waitUntil(t, 2*time.Second, func() bool { return s.subscriberCount() == 1 }, "subscriber registration")

	agentIncrement(t, ts.URL)

	var sawID string
	deadline := time.After(3 * time.Second)
	for {
		select {
		case line, ok := <-lines:
			if !ok {
				t.Fatal("stream closed before the data frame")
			}
			if strings.HasPrefix(line, "id: ") {
				sawID = strings.TrimPrefix(line, "id: ")
			}
			if strings.HasPrefix(line, "data: ") {
				if sawID != "1" {
					t.Fatalf("data frame must be preceded by \"id: 1\", got id %q", sawID)
				}
				return
			}
		case <-deadline:
			t.Fatal("no id+data frame pair within 3s")
		}
	}
}

// TestSSEReconnectUnsubscribesNoLeak: connect/drop cycles always remove the
// subscriber and their handler goroutines exit — no leak across reconnects.
func TestSSEReconnectUnsubscribesNoLeak(t *testing.T) {
	s := counterServer(t)
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()
	client := &http.Client{Transport: &http.Transport{DisableKeepAlives: true}}

	cycle := func() {
		ctx, cancel := context.WithCancel(context.Background())
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, ts.URL+"/events", nil)
		resp, err := client.Do(req)
		if err != nil {
			cancel()
			return
		}
		// Drain the handshake so the handler is definitely subscribed.
		b := make([]byte, 1)
		resp.Body.Read(b)
		cancel()
		resp.Body.Close()
		waitUntil(t, 2*time.Second, func() bool { return s.subscriberCount() == 0 }, "subscriber removal on disconnect")
	}

	cycle() // warm-up: settle httptest bookkeeping before taking the baseline
	base := runtime.NumGoroutine()
	for i := 0; i < 5; i++ {
		cycle()
	}
	waitUntil(t, 3*time.Second, func() bool { return runtime.NumGoroutine() <= base+2 },
		"SSE handler goroutines to exit after every disconnect")
	if s.subscriberCount() != 0 {
		t.Fatalf("subscribers remain after all disconnects: %d", s.subscriberCount())
	}
}

// TestAuditDetectsDisplayTimeTamper: the chain is documented as tamper-evident
// for "any edited, dropped or reordered" line. The display `time` field is
// part of the persisted line, so silently editing it must break verification
// too — otherwise an attacker shifts the displayed time of an entry without
// detection (TS alone does not cover what a reader sees).
func TestAuditDetectsDisplayTimeTamper(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "audit.jsonl")
	s := counterServer(t)
	if err := s.SetAuditLog(path); err != nil {
		t.Fatal(err)
	}
	s.logEvent("human", "first")
	s.logEvent("agent", "second")
	s.logEvent("human", "third")

	raw, _ := os.ReadFile(path)
	lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
	var e LogEntry
	if err := json.Unmarshal([]byte(lines[1]), &e); err != nil {
		t.Fatal(err)
	}
	e.Time = "23:59:59" // alter ONLY the display time; TS + hash untouched
	forged, _ := json.Marshal(e)
	lines[1] = string(forged)
	tampered := filepath.Join(dir, "tampered.jsonl")
	os.WriteFile(tampered, []byte(strings.Join(lines, "\n")+"\n"), 0o600)

	f, _ := os.Open(tampered)
	n, err := VerifyAuditChain(f)
	f.Close()
	if err == nil {
		t.Fatalf("editing the display time of a line passed verification (n=%d)", n)
	}
	if n != 1 {
		t.Fatalf("chain should break at the tampered line (verified=1), got %d", n)
	}
}

// TestAuditDetectsReorderedLines: swapping two adjacent entries must break the
// chain (reorder detection, complementing the existing drop + re-attribute
// cases).
func TestAuditDetectsReorderedLines(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "audit.jsonl")
	s := counterServer(t)
	if err := s.SetAuditLog(path); err != nil {
		t.Fatal(err)
	}
	s.logEvent("human", "a")
	s.logEvent("agent", "b")
	s.logEvent("system", "c")

	raw, _ := os.ReadFile(path)
	lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
	lines[1], lines[2] = lines[2], lines[1]
	swapped := filepath.Join(dir, "swapped.jsonl")
	os.WriteFile(swapped, []byte(strings.Join(lines, "\n")+"\n"), 0o600)

	f, _ := os.Open(swapped)
	_, err := VerifyAuditChain(f)
	f.Close()
	if err == nil {
		t.Fatal("reordered entries passed verification")
	}
}

// TestPresenceConcurrentWritersRuneSafe: the round-4 rune-boundary truncation
// must hold under concurrent writers — every stored presence string is valid
// UTF-8 within the rune cap, and (under -race) nothing races.
func TestPresenceConcurrentWritersRuneSafe(t *testing.T) {
	s := counterServer(t)
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()
	tok := pageEventToken(t, ts.URL)

	labels := []string{
		strings.Repeat("世", 150),              // 3-byte runes, over cap
		strings.Repeat("é", 140),              // 2-byte runes, over cap
		"Email = " + strings.Repeat("x", 200), // a typed entry, over cap
		"Password = (hidden)",                 // the filled marker
		"btn: " + strings.Repeat("界x", 90),    // mixed widths
	}
	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			doJSON(t, http.MethodPost, ts.URL+"/presence", tok, "", `{"element":"`+labels[i%len(labels)]+`"}`)
		}(i)
	}
	wg.Wait()

	s.actMu.Lock()
	focus, typing, filled := s.humanFocus, s.humanTyping, s.humanFilled
	s.actMu.Unlock()
	for name, v := range map[string]string{"focus": focus, "typing": typing, "filled": filled} {
		if v == "" {
			continue
		}
		if !utf8.ValidString(v) {
			t.Errorf("%s stored invalid UTF-8 under concurrent writes: %q", name, v)
		}
		if n := utf8.RuneCountInString(v); n > 120 {
			t.Errorf("%s exceeded the 120-rune cap: %d runes", name, n)
		}
	}
}

// TestEventsRejectsCrossOrigin: /events streams the app's full live UI, and
// EventSource is readable cross-origin (unlike fetch, SSE does NOT enforce
// CORS) — so any web page the user visits could snoop the localhost app's
// data by pointing an EventSource at it. The CSRF/DNS-rebind guard must cover
// /events too; loopback / null / absent origins (the app's own page, webviews,
// local tools) still connect.
func TestEventsRejectsCrossOrigin(t *testing.T) {
	ts := httptest.NewServer(counterServer(t).Handler())
	defer ts.Close()

	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/events", nil)
	req.Header.Set("Origin", "https://evil.example.com")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("get /events: %v", err)
	}
	body := make([]byte, 128)
	n, _ := resp.Body.Read(body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden || !strings.Contains(string(body[:n]), "cross-origin") {
		t.Fatalf("cross-origin /events: want 403 cross-origin rejection, got %d %q", resp.StatusCode, body[:n])
	}

	for _, origin := range []string{"", "null", "http://localhost:51234", "http://127.0.0.1:51234", "https://[::1]:8443"} {
		req, _ := http.NewRequest(http.MethodGet, ts.URL+"/events", nil)
		if origin != "" {
			req.Header.Set("Origin", origin)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("get /events (origin %q): %v", origin, err)
		}
		ok := resp.StatusCode == http.StatusOK && resp.Header.Get("Content-Type") == "text/event-stream"
		resp.Body.Close()
		if !ok {
			t.Fatalf("/events with Origin %q must still stream, got %d %q", origin, resp.StatusCode, resp.Header.Get("Content-Type"))
		}
	}
}

// TestMeasureRejectsCrossOrigin: /measure is an unauthenticated WRITE surface
// (the stored layout feeds the agent's qorm_check_layout decisions); a
// cross-origin page must not be able to poison it. Loopback writes pass.
func TestMeasureRejectsCrossOrigin(t *testing.T) {
	ts := httptest.NewServer(counterServer(t).Handler())
	defer ts.Close()

	code, body := doJSON(t, http.MethodPost, ts.URL+"/measure", "", "https://evil.example.com", `[{"id":"forged"}]`)
	if code != http.StatusForbidden || !strings.Contains(body, "cross-origin") {
		t.Fatalf("cross-origin POST /measure: want 403, got %d %q", code, body)
	}
	// A loopback-origin report still stores.
	if code, _ := doJSON(t, http.MethodPost, ts.URL+"/measure", "", "http://localhost:51234", `[{"id":"real"}]`); code != http.StatusNoContent {
		t.Fatalf("loopback POST /measure: want 204, got %d", code)
	}
	if code, body := doJSON(t, http.MethodGet, ts.URL+"/measure", "", "", ""); code != http.StatusOK || body != `[{"id":"real"}]` {
		t.Fatalf("GET /measure = %d %q", code, body)
	}
}

// TestBlockCrossOriginOriginVariants: loopback allowlisting compares the
// hostname only, so lookalike hosts (localhost.evil.com, credentials before
// evil.com, paths mentioning localhost) stay rejected while every genuine
// loopback spelling passes through to the handler behind the guard.
func TestBlockCrossOriginOriginVariants(t *testing.T) {
	ts := httptest.NewServer(counterServer(t).Handler())
	defer ts.Close()

	for _, evil := range []string{
		"https://evil.example.com",
		"http://localhost.evil.com",
		"http://evil.com/localhost",
		"http://localhost@evil.com",
		"https://evil.com:8443",
		"http://127.0.0.1.evil.com",
	} {
		code, body := doJSON(t, http.MethodPost, ts.URL+"/event", "", evil, `{"h":0,"inputs":{}}`)
		if code != http.StatusForbidden || !strings.Contains(body, "cross-origin request rejected") {
			t.Errorf("Origin %q: want 403 cross-origin, got %d %q", evil, code, body)
		}
	}

	for _, good := range []string{
		"",
		"null",
		"http://localhost",
		"http://localhost:51234",
		"https://localhost:8443",
		"http://127.0.0.1",
		"http://127.0.0.1:51234",
		"https://[::1]:8443",
	} {
		code, body := doJSON(t, http.MethodPost, ts.URL+"/event", "", good, `{"h":0,"inputs":{}}`)
		if code != http.StatusForbidden || !strings.Contains(body, "invalid event token") {
			t.Errorf("Origin %q: must pass the guard and hit the token gate, got %d %q", good, code, body)
		}
	}
}

// TestDeepLinkNavDirectionNotLeaked: a deep-link GET navigates the runtime to
// render the URL's scene, but the page load renders it DIRECTLY — there is no
// page transition to ship. The pending nav direction must not leak into the
// next unrelated broadcast (an agent edit is not a navigation).
func TestDeepLinkNavDirectionNotLeaked(t *testing.T) {
	s := navServer(t)
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()
	ch := make(chan string, 8)
	s.subsMu.Lock()
	s.subs = map[chan string]struct{}{ch: {}}
	s.subsMu.Unlock()

	resp, err := http.Get(ts.URL + "/?scene=profile&userId=u-1")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if s.rt.CurrentScene() != "profile" {
		t.Fatalf("deep link: scene = %q", s.rt.CurrentScene())
	}

	// A NON-navigation mutation: the broadcast must carry no nav direction.
	post(t, ts.URL+"/mcp", `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"qorm_set_state","arguments":{"path":"whatever","value":1}}}`)
	select {
	case msg := <-ch:
		var d struct {
			Nav string `json:"nav"`
			Rev int64  `json:"rev"`
		}
		if err := json.Unmarshal([]byte(msg), &d); err != nil {
			t.Fatalf("broadcast not JSON: %v", err)
		}
		if d.Nav != "" {
			t.Fatalf("a non-navigation edit after a deep link must not ship nav=%q (stale deep-link direction)", d.Nav)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("no broadcast after the agent edit")
	}
}

// TestMeasureRejectsInvalidBody: the self-report sink stores whatever it is
// POSTed and serves it back as application/json (and to the agent via
// qorm_measure). An empty or non-JSON body would make GET /measure return
// invalid JSON — consistent with the /event, /presence and /viewport error
// policy, malformed input is a 400 and never silently stored.
func TestMeasureRejectsInvalidBody(t *testing.T) {
	ts := httptest.NewServer(counterServer(t).Handler())
	defer ts.Close()

	for _, body := range []string{"", "not json", `{"unterminated":`} {
		if code, _ := doJSON(t, http.MethodPost, ts.URL+"/measure", "", "", body); code != http.StatusBadRequest {
			t.Fatalf("POST /measure %q: want 400, got %d", body, code)
		}
	}
	// Nothing was stored: GET still reports the empty array.
	if code, body := doJSON(t, http.MethodGet, ts.URL+"/measure", "", "", ""); code != http.StatusOK || body != "[]" {
		t.Fatalf("GET /measure after rejected posts = %d %q, want 200 []", code, body)
	}
	// A valid report still round-trips.
	if code, _ := doJSON(t, http.MethodPost, ts.URL+"/measure", "", "", `[{"id":"ok"}]`); code != http.StatusNoContent {
		t.Fatalf("valid POST /measure: want 204, got %d", code)
	}
	if code, body := doJSON(t, http.MethodGet, ts.URL+"/measure", "", "", ""); code != http.StatusOK || body != `[{"id":"ok"}]` {
		t.Fatalf("GET /measure = %d %q", code, body)
	}
}

// TestReloadPreservesRouteParamsAndStack: a hot reload carries the deep-link
// params and the back stack across the swap, so Back still works after an edit.
func TestReloadPreservesRouteParamsAndStack(t *testing.T) {
	s := New(qrt.New(twoSceneApp("v1", "other", map[string]any{"count": float64(0)})))
	s.rt.NavigateTo("other", map[string]any{"userId": "u-9"})
	s.rt.Viewport = qrt.Viewport{W: 390, H: 844}

	s.Reload(qrt.New(twoSceneApp("v2", "other", map[string]any{"count": float64(0)})))

	if s.rt.CurrentScene() != "other" {
		t.Errorf("scene after reload = %q, want other", s.rt.CurrentScene())
	}
	if s.rt.RouteParams["userId"] != "u-9" {
		t.Errorf("route params after reload = %#v, want userId=u-9", s.rt.RouteParams)
	}
	if s.rt.RoutePath() != "/?scene=other&userId=u-9" {
		t.Errorf("route path after reload = %q", s.rt.RoutePath())
	}
	if (s.rt.Viewport != qrt.Viewport{W: 390, H: 844}) {
		t.Errorf("viewport after reload = %+v", s.rt.Viewport)
	}
	// Back returns to the entry scene across the reload.
	s.rt.NavigateBack()
	if s.rt.CurrentScene() != "" && s.rt.CurrentScene() != "main" {
		t.Errorf("back after reload: scene = %q, want the entry", s.rt.CurrentScene())
	}
}

// threeSceneApp builds an app with entry "main" plus "mid" (when withID) and
// "leaf" scenes, for back-stack reload probes.
func threeSceneApp(midID string) *model.App {
	scenes := map[string]*model.Node{
		"main": {Type: "scaffold", ID: "r", Children: []*model.Node{{Type: "text", ID: "t", Text: "main-v"}}},
		"leaf": {Type: "scaffold", ID: "l", Children: []*model.Node{{Type: "text", ID: "lt", Text: "leaf-v"}}},
	}
	if midID != "" {
		scenes[midID] = &model.Node{Type: "scaffold", ID: "m", Children: []*model.Node{{Type: "text", ID: "mt", Text: "mid-v"}}}
	}
	return &model.App{Entry: "main", Scenes: scenes}
}

// TestReloadStaleNavStackRendersSafely: when an edit deletes a scene that is
// still on the carried back stack, navigating back onto it must not crash or
// render nothing — the renderer falls back to the entry scene.
func TestReloadStaleNavStackRendersSafely(t *testing.T) {
	three := func(mid string) *qrt.Runtime { return qrt.New(threeSceneApp(mid)) }
	s := New(three("mid"))
	s.rt.Navigate("mid", nil)
	s.rt.Navigate("leaf", nil)

	// The edit deletes "mid"; the current scene ("leaf") survives.
	s.Reload(three("REMOVED"))
	if s.rt.CurrentScene() != "leaf" {
		t.Fatalf("current scene after reload = %q, want leaf", s.rt.CurrentScene())
	}
	// Back pops onto the deleted scene: rendering must fall back to the entry.
	s.rt.NavigateBack()
	if html := renderCurrent(s); !strings.Contains(html, "main-v") {
		t.Fatalf("back onto a deleted scene must render the entry fallback, got:\n%s", html)
	}
}
