package server

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/qorm/qorm/internal/keys"
	"github.com/qorm/qorm/internal/model"
	qrt "github.com/qorm/qorm/internal/runtime"
)

// doJSON issues an HTTP request with optional X-Qorm-Token and Origin headers
// and returns the status code and response body.
func doJSON(t *testing.T, method, url, token, origin, body string) (int, string) {
	t.Helper()
	req, err := http.NewRequest(method, url, strings.NewReader(body))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("X-Qorm-Token", token)
	}
	if origin != "" {
		req.Header.Set("Origin", origin)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, url, err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b)
}

// waitUntil polls cond on a ticker, failing the test if it is not satisfied
// before the deadline. Deterministic: no sleeps, a bounded poll.
func waitUntil(t *testing.T, timeout time.Duration, cond func() bool, msg string) {
	t.Helper()
	tick := time.NewTicker(15 * time.Millisecond)
	defer tick.Stop()
	deadline := time.After(timeout)
	for {
		if cond() {
			return
		}
		select {
		case <-tick.C:
		case <-deadline:
			t.Fatalf("timed out waiting for: %s", msg)
		}
	}
}

// TestPresenceEndpointHumanGating: /presence is a human-side surface — POST
// requires the page-embedded token, records focus / typing / filled (password
// label only), truncates over-long elements, and GET reports what is shared.
func TestPresenceEndpointHumanGating(t *testing.T) {
	s := counterServer(t)
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()
	tok := pageEventToken(t, ts.URL)

	// Without the page-embedded token, presence reports are refused.
	if code, _ := doJSON(t, http.MethodPost, ts.URL+"/presence", "", "", `{"element":"#email"}`); code != http.StatusForbidden {
		t.Fatalf("tokenless POST /presence: want 403, got %d", code)
	}
	if code, _ := doJSON(t, http.MethodPost, ts.URL+"/presence", "wrong", "", `{"element":"#email"}`); code != http.StatusForbidden {
		t.Fatalf("bad-token POST /presence: want 403, got %d", code)
	}
	s.actMu.Lock()
	focus := s.humanFocus
	s.actMu.Unlock()
	if focus != "" {
		t.Fatalf("rejected presence reports must not set focus, got %q", focus)
	}

	// A plain focus report with the real token.
	if code, _ := doJSON(t, http.MethodPost, ts.URL+"/presence", tok, "", `{"element":"#email"}`); code != http.StatusNoContent {
		t.Fatalf("POST /presence: want 204, got %d", code)
	}
	s.actMu.Lock()
	focus = s.humanFocus
	typing := s.humanTyping
	s.actMu.Unlock()
	if focus != "#email" {
		t.Fatalf("humanFocus = %q, want #email", focus)
	}
	if typing != "" {
		t.Fatalf("a plain focus report must not set typing, got %q", typing)
	}

	// A typed entry ("<field> = <value>") is retained separately, and a hidden
	// (password) field by label only — the value is never part of the report.
	doJSON(t, http.MethodPost, ts.URL+"/presence", tok, "", `{"element":"Email = ada@example.com"}`)
	doJSON(t, http.MethodPost, ts.URL+"/presence", tok, "", `{"element":"Password = (hidden)"}`)
	s.actMu.Lock()
	typing = s.humanTyping
	filled := s.humanFilled
	s.actMu.Unlock()
	if typing != "Email = ada@example.com" {
		t.Fatalf("humanTyping = %q, want the retained entry", typing)
	}
	if filled != "Password" {
		t.Fatalf("humanFilled = %q, want the label Password", filled)
	}

	// Over-long elements are truncated to 120 chars before storage.
	long := strings.Repeat("a", 200)
	doJSON(t, http.MethodPost, ts.URL+"/presence", tok, "", `{"element":"`+long+`"}`)
	s.actMu.Lock()
	focus = s.humanFocus
	s.actMu.Unlock()
	if len(focus) != 120 {
		t.Fatalf("element should be truncated to 120 chars, got %d", len(focus))
	}

	// The human's own panel reads back exactly what is shared with the agent.
	code, body := doJSON(t, http.MethodGet, ts.URL+"/presence", "", "", "")
	if code != http.StatusOK {
		t.Fatalf("GET /presence: want 200, got %d", code)
	}
	var got map[string]string
	if err := json.Unmarshal([]byte(body), &got); err != nil {
		t.Fatalf("GET /presence not JSON: %v (%s)", err, body)
	}
	if got["focus"] != strings.Repeat("a", 120) || got["typing"] != "Email = ada@example.com" || got["filled"] != "Password" {
		t.Fatalf("GET /presence = %#v", got)
	}
}

// TestPresenceSurfacedThroughAgentActivity: presence reports reach the agent
// through qorm_activity (focus / typing / filled), and the password marker
// never leaks beyond the field label.
func TestPresenceSurfacedThroughAgentActivity(t *testing.T) {
	s := counterServer(t)
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()
	tok := pageEventToken(t, ts.URL)

	doJSON(t, http.MethodPost, ts.URL+"/presence", tok, "", `{"element":"#email"}`)
	doJSON(t, http.MethodPost, ts.URL+"/presence", tok, "", `{"element":"Email = ada@example.com"}`)
	doJSON(t, http.MethodPost, ts.URL+"/presence", tok, "", `{"element":"Password = (hidden)"}`)

	_, body := doJSON(t, http.MethodPost, ts.URL+"/mcp", "", "",
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"qorm_activity","arguments":{}}}`)
	var rpc struct {
		Result struct {
			Content []struct {
				Text string `json:"text"`
			} `json:"content"`
		} `json:"result"`
	}
	if err := json.Unmarshal([]byte(body), &rpc); err != nil || len(rpc.Result.Content) == 0 {
		t.Fatalf("qorm_activity: bad response %q (%v)", body, err)
	}
	text := rpc.Result.Content[0].Text
	for _, want := range []string{
		`"element":"Password = (hidden)"`,   // focus follows the last touch
		`"entry":"Email = ada@example.com"`, // a later tap must not erase the typed entry
		`"field":"Password"`,                // the password field, by label only
		`"secondsAgo":`,
	} {
		if !strings.Contains(text, want) {
			t.Errorf("qorm_activity should surface %s, got: %s", want, text)
		}
	}
	// The password line is retained by label in humanFilled — never as a typed
	// entry, and never with the marker glued to the label.
	if strings.Contains(text, `"field":"Password = (hidden)"`) {
		t.Errorf("humanFilled must carry the label only: %s", text)
	}
	if strings.Contains(text, `"entry":"Password = (hidden)"`) {
		t.Errorf("the password marker must not be retained as typed text: %s", text)
	}

	// With no presence at all the activity payload carries no human blocks.
	fresh := counterServer(t)
	out := string(fresh.agent.HandleHTTP([]byte(
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"qorm_activity","arguments":{}}`)))
	for _, banned := range []string{"humanFocus", "humanTyping", "humanFilled"} {
		if strings.Contains(out, banned) {
			t.Errorf("empty session must not report %s: %s", banned, out)
		}
	}
}

// TestMeasureEndpointRoundTrip: POST /measure stores the app's self-reported
// layout; GET returns it (an empty array before anything was measured).
func TestMeasureEndpointRoundTrip(t *testing.T) {
	ts := httptest.NewServer(counterServer(t).Handler())
	defer ts.Close()

	code, body := doJSON(t, http.MethodGet, ts.URL+"/measure", "", "", "")
	if code != http.StatusOK || body != "[]" {
		t.Fatalf("GET /measure before any report = %d %q, want 200 []", code, body)
	}

	payload := `[{"id":"btn_plus","rect":{"x":10,"y":20,"w":30,"h":40}}]`
	if code, _ := doJSON(t, http.MethodPost, ts.URL+"/measure", "", "", payload); code != http.StatusNoContent {
		t.Fatalf("POST /measure: want 204, got %d", code)
	}
	if code, body := doJSON(t, http.MethodGet, ts.URL+"/measure", "", "", ""); code != http.StatusOK || body != payload {
		t.Fatalf("GET /measure = %d %q, want the stored payload", code, body)
	}
}

// winRecorder captures native window-control callbacks for assertions.
type winRecorder struct {
	mu    sync.Mutex
	moves []string
	ops   []string
	opens []string
	evals []string
}

func (r *winRecorder) mover(id string, x, y, w, h int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.moves = append(r.moves, fmt.Sprintf("%s:%d,%d,%d,%d", id, x, y, w, h))
}

func (r *winRecorder) op(id, op string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.ops = append(r.ops, id+":"+op)
}

func (r *winRecorder) open(id, url string, w, h int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.opens = append(r.opens, fmt.Sprintf("%s:%s %dx%d", id, url, w, h))
}

func (r *winRecorder) eval(id, js string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.evals = append(r.evals, id+":"+js)
}

// TestWindowControlEndpoint: /window is 501 without a native host, and drives
// move/open/eval/emit/op through the registered control callbacks once present.
func TestWindowControlEndpoint(t *testing.T) {
	s := counterServer(t)
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()

	if code, body := doJSON(t, http.MethodPost, ts.URL+"/window", "", "", `{"op":"move"}`); code != http.StatusNotImplemented {
		t.Fatalf("window control should be 501 on a plain server, got %d %q", code, body)
	}

	rec := &winRecorder{}
	s.SetWindowControl(rec.mover, rec.op, rec.open, rec.eval)

	if code, body := doJSON(t, http.MethodPost, ts.URL+"/window", "", "", `{"op":"move","x":10,"y":20,"w":400,"h":820}`); code != http.StatusOK || body != "ok" {
		t.Fatalf("move: %d %q, want 200 ok", code, body)
	}
	// No op and no id: defaults to move on "main".
	doJSON(t, http.MethodPost, ts.URL+"/window", "", "", `{"x":1,"y":2,"w":3,"h":4}`)
	doJSON(t, http.MethodPost, ts.URL+"/window", "", "", `{"op":"open","id":"win2","url":"https://x.example","w":100,"h":200}`)
	doJSON(t, http.MethodPost, ts.URL+"/window", "", "", `{"op":"eval","id":"win2","js":"doIt(1)"}`)
	doJSON(t, http.MethodPost, ts.URL+"/window", "", "", `{"op":"emit","id":"win2","event":"ping","data":{"n":1}}`)
	doJSON(t, http.MethodPost, ts.URL+"/window", "", "", `{"op":"emit","id":"win2","event":"nopayload"}`)
	doJSON(t, http.MethodPost, ts.URL+"/window", "", "", `{"op":"focus"}`)

	rec.mu.Lock()
	moves := strings.Join(rec.moves, ";")
	ops := strings.Join(rec.ops, ";")
	opens := strings.Join(rec.opens, ";")
	evals := strings.Join(rec.evals, ";")
	rec.mu.Unlock()

	if want := "main:10,20,400,820;main:1,2,3,4"; moves != want {
		t.Errorf("moves = %q, want %q", moves, want)
	}
	if want := "win2:https://x.example 100x200"; opens != want {
		t.Errorf("opens = %q, want %q", opens, want)
	}
	if want := `win2:doIt(1);win2:window.qormOnWindowEvent&&qormOnWindowEvent("ping",{"n":1});win2:window.qormOnWindowEvent&&qormOnWindowEvent("nopayload",null)`; evals != want {
		t.Errorf("evals = %q, want %q", evals, want)
	}
	if want := "main:focus"; ops != want {
		t.Errorf("ops = %q, want %q", ops, want)
	}

	if code, _ := doJSON(t, http.MethodPost, ts.URL+"/window", "", "", `{`); code != http.StatusBadRequest {
		t.Fatalf("bad window body: want 400, got %d", code)
	}
}

// TestUpdateRollbackOverHTTP: the OTA endpoints fail closed at every gate —
// no bundle, no trust key, bad source — and apply / roll back when trusted.
func TestUpdateRollbackOverHTTP(t *testing.T) {
	// A plain (non-bundle) server has no OTA.
	ts0 := httptest.NewServer(counterServer(t).Handler())
	defer ts0.Close()
	if code, body := doJSON(t, http.MethodPost, ts0.URL+"/update", "", "", `{"source":"x"}`); code != http.StatusBadRequest || !strings.Contains(body, "OTA not enabled") {
		t.Fatalf("update without a bundle: %d %q", code, body)
	}
	if code, _ := doJSON(t, http.MethodPost, ts0.URL+"/rollback", "", "", ""); code != http.StatusConflict {
		t.Fatalf("rollback without a previous bundle: want 409, got %d", code)
	}

	pub, priv, _ := keys.Generate()

	// A bundle without a trusted key cannot verify authenticity: 403.
	noTrust, err := NewBundle(signedBundle(t, "1.0.0", priv, pub), nil, nil)
	if err != nil {
		t.Fatalf("NewBundle: %v", err)
	}
	ts1 := httptest.NewServer(noTrust.Handler())
	defer ts1.Close()
	if code, body := doJSON(t, http.MethodPost, ts1.URL+"/update", "", "", `{"source":"x"}`); code != http.StatusForbidden || !strings.Contains(body, "--trust") {
		t.Fatalf("update without a trust key: %d %q", code, body)
	}

	// A trusted bundle server.
	s, err := NewBundle(signedBundle(t, "1.0.0", priv, pub), pub, nil)
	if err != nil {
		t.Fatalf("NewBundle: %v", err)
	}
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()

	if code, _ := doJSON(t, http.MethodPost, ts.URL+"/update", "", "", `{}`); code != http.StatusBadRequest {
		t.Fatalf("update without a source: want 400, got %d", code)
	}
	if code, _ := doJSON(t, http.MethodPost, ts.URL+"/update", "", "", `{bad json`); code != http.StatusBadRequest {
		t.Fatalf("update with bad JSON: want 400, got %d", code)
	}

	// An unverifiable source: 409 and the live app keeps the current bundle.
	srcMissing, _ := json.Marshal(map[string]string{"source": filepath.Join(t.TempDir(), "missing.bundle")})
	if code, body := doJSON(t, http.MethodPost, ts.URL+"/update", "", "", string(srcMissing)); code != http.StatusConflict || !strings.Contains(body, "kept current") {
		t.Fatalf("update from a missing file: %d %q", code, body)
	}
	if s.current.Version() != "1.0.0" {
		t.Fatalf("rejected update must not change the live app, got %s", s.current.Version())
	}

	// A signed v2 bundle applies; /rollback restores v1; then nothing is left.
	v2 := writeBundle(t, signedBundle(t, "2.0.0", priv, pub))
	srcV2, _ := json.Marshal(map[string]string{"source": v2})
	code, body := doJSON(t, http.MethodPost, ts.URL+"/update", "", "", string(srcV2))
	if code != http.StatusOK || !strings.Contains(body, "updated 1.0.0 -> 2.0.0") {
		t.Fatalf("trusted update: %d %q", code, body)
	}
	if s.current.Version() != "2.0.0" {
		t.Fatalf("active version after update = %s", s.current.Version())
	}
	if code, body := doJSON(t, http.MethodPost, ts.URL+"/rollback", "", "", ""); code != http.StatusOK || !strings.Contains(body, "rolled back 2.0.0 -> 1.0.0") {
		t.Fatalf("rollback: %d %q", code, body)
	}
	if code, _ := doJSON(t, http.MethodPost, ts.URL+"/rollback", "", "", ""); code != http.StatusConflict {
		t.Fatalf("second rollback must fail (nothing previous), got %d", code)
	}

	// CSRF: cross-origin /update and /rollback are rejected at the door.
	if code, body := doJSON(t, http.MethodPost, ts.URL+"/update", "", "https://evil.example.com", string(srcV2)); code != http.StatusForbidden || !strings.Contains(body, "cross-origin") {
		t.Fatalf("cross-origin update: %d %q", code, body)
	}
	if code, _ := doJSON(t, http.MethodPost, ts.URL+"/rollback", "", "https://evil.example.com", ""); code != http.StatusForbidden {
		t.Fatalf("cross-origin rollback: want 403, got %d", code)
	}
}

// TestBlockCrossOriginGuard: non-loopback Origin headers are rejected on every
// guarded route; loopback / null / absent Origins pass through to the handler.
func TestBlockCrossOriginGuard(t *testing.T) {
	ts := httptest.NewServer(counterServer(t).Handler())
	defer ts.Close()

	for _, route := range []string{"/event", "/navigate", "/presence", "/viewport", "/window", "/mcp", "/update", "/rollback", "/dev/state", "/dev/tree", "/dev/highlight"} {
		code, body := doJSON(t, http.MethodPost, ts.URL+route, "", "https://evil.example.com", `{}`)
		if code != http.StatusForbidden || !strings.Contains(body, "cross-origin request rejected") {
			t.Errorf("%s with a cross-origin Origin: want 403 cross-origin, got %d %q", route, code, body)
		}
	}

	// Loopback origins, "null", and no Origin pass the guard — the token gate
	// behind it still refuses the unauthenticated request, proving the request
	// made it past the CSRF guard.
	for _, origin := range []string{"", "null", "http://localhost:8080", "http://127.0.0.1:8080", "https://[::1]:8443"} {
		code, body := doJSON(t, http.MethodPost, ts.URL+"/event", "", origin, `{"h":0,"inputs":{}}`)
		if code != http.StatusForbidden || !strings.Contains(body, "invalid event token") {
			t.Errorf("/event with Origin %q should pass the guard and hit the token gate, got %d %q", origin, code, body)
		}
	}

	// An unparseable Origin fails closed.
	if code, body := doJSON(t, http.MethodPost, ts.URL+"/event", "", "http://[bad", `{"h":0}`); code != http.StatusForbidden || !strings.Contains(body, "cross-origin") {
		t.Fatalf("unparseable Origin: want 403 cross-origin, got %d %q", code, body)
	}

	// A loopback-origin agent call reaches the MCP session.
	code, body := doJSON(t, http.MethodPost, ts.URL+"/mcp", "", "http://127.0.0.1:8080", `{"jsonrpc":"2.0","id":9,"method":"ping"}`)
	if code != http.StatusOK || !strings.Contains(body, `"result"`) {
		t.Fatalf("loopback-origin /mcp ping: %d %q", code, body)
	}
}

// TestEventsSSEStreamEndToEnd drives /events over a real connection: the
// handshake comment, a live frame after an agent edit, and clean unsubscribe
// + disconnect logging when the client goes away.
func TestEventsSSEStreamEndToEnd(t *testing.T) {
	s := counterServer(t)
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, ts.URL+"/events", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("connect /events: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("/events status = %d", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Fatalf("/events Content-Type = %q", ct)
	}

	lines := make(chan string, 16)
	done := make(chan struct{})
	go func() {
		defer close(done)
		sc := bufio.NewScanner(resp.Body)
		for sc.Scan() {
			lines <- sc.Text()
		}
	}()

	select {
	case line := <-lines:
		if line != ": connected" {
			t.Fatalf("first SSE line = %q, want \": connected\"", line)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("no SSE handshake within 3s")
	}

	waitUntil(t, 2*time.Second, func() bool {
		s.subsMu.Lock()
		defer s.subsMu.Unlock()
		return len(s.subs) == 1
	}, "SSE subscriber registration")

	// An agent edit bumps the revision and pushes a frame to the subscriber.
	post(t, ts.URL+"/mcp", `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"qorm_dispatch","arguments":{"action":"increment","args":{"count":0}}}}`)

	deadline := time.After(3 * time.Second)
	var frame string
waiting:
	for {
		select {
		case line := <-lines:
			if strings.HasPrefix(line, "data: ") {
				frame = strings.TrimPrefix(line, "data: ")
				break waiting
			}
		case <-deadline:
			t.Fatal("no SSE data frame within 3s of the agent edit")
		}
	}
	var d struct {
		Rev  int64  `json:"rev"`
		HTML string `json:"html"`
	}
	if err := json.Unmarshal([]byte(frame), &d); err != nil {
		t.Fatalf("SSE frame not JSON: %v (%s)", err, frame)
	}
	if d.Rev == 0 || !strings.Contains(d.HTML, ">1<") {
		t.Fatalf("SSE frame should carry the count=1 UI, got %+v", d)
	}

	// Disconnecting removes the subscriber and logs both arrival and departure
	// against the loopback client identity.
	cancel()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("stream reader did not finish after cancel")
	}
	waitUntil(t, 2*time.Second, func() bool {
		s.subsMu.Lock()
		defer s.subsMu.Unlock()
		return len(s.subs) == 0
	}, "subscriber removal on disconnect")
	waitUntil(t, 2*time.Second, func() bool {
		s.actMu.Lock()
		defer s.actMu.Unlock()
		var connected, disconnected bool
		for _, e := range s.activity {
			if e.Source == "system" && strings.Contains(e.Detail, "client connected (local)") {
				connected = true
			}
			if e.Source == "system" && strings.Contains(e.Detail, "client disconnected (local)") {
				disconnected = true
			}
		}
		return connected && disconnected
	}, "connect/disconnect activity entries")
}

// TestServeEventInputValidationAndBounds: bad JSON is a 400; an out-of-range
// handler index is dropped but inputs still fold into state and the response
// is a fresh render at an advanced revision.
func TestServeEventInputValidationAndBounds(t *testing.T) {
	s := counterServer(t)
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()
	tok := pageEventToken(t, ts.URL)

	if code, _ := doJSON(t, http.MethodPost, ts.URL+"/event", tok, "", `{bad`); code != http.StatusBadRequest {
		t.Fatalf("bad /event body: want 400, got %d", code)
	}

	rev0 := s.rev.Load()
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/event", strings.NewReader(`{"h":999,"inputs":{"status":"typed"}}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Qorm-Token", tok)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("post /event: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("out-of-range h: want 200, got %d", resp.StatusCode)
	}
	rev, _ := strconv.ParseInt(resp.Header.Get("X-Qorm-Rev"), 10, 64)
	if rev != rev0+1 {
		t.Fatalf("X-Qorm-Rev = %d, want %d (every /event bumps)", rev, rev0+1)
	}
	if resp.Header.Get("X-Qorm-Theme") == "" {
		t.Fatal("X-Qorm-Theme header missing")
	}
	if !strings.Contains(string(body), ">0<") {
		t.Fatalf("out-of-range h must not dispatch (count stays 0):\n%s", body)
	}
	s.mu.Lock()
	status := s.rt.State["status"]
	s.mu.Unlock()
	if status != "typed" {
		t.Fatalf("inputs must fold into state even without a dispatch, got %v", status)
	}

	// A negative index is likewise a no-op dispatch with a fresh render.
	if code, _ := doJSON(t, http.MethodPost, ts.URL+"/event", tok, "", `{"h":-1,"inputs":{}}`); code != http.StatusOK {
		t.Fatalf("negative h: want 200, got %d", code)
	}
}

// TestViewportRejectsMalformedPayload: bad JSON and negative dimensions are a
// 400 and leave the viewport untouched.
func TestViewportRejectsMalformedPayload(t *testing.T) {
	s := responsiveServer(t)
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()
	tok := pageEventToken(t, ts.URL)

	for _, body := range []string{`{bad`, `{"w":-5,"h":100}`, `{"w":100,"h":-1}`} {
		if code, _ := doJSON(t, http.MethodPost, ts.URL+"/viewport", tok, "", body); code != http.StatusBadRequest {
			t.Fatalf("POST /viewport %s: want 400, got %d", body, code)
		}
	}
	if s.rt.Viewport != (qrt.Viewport{}) {
		t.Fatalf("rejected payloads must not change the viewport: %+v", s.rt.Viewport)
	}
}

// TestDevtoolWriteValidation: the devtool write endpoints enforce the token
// and reject malformed bodies.
func TestDevtoolWriteValidation(t *testing.T) {
	s := counterServer(t)
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()
	tok := pageEventToken(t, ts.URL)

	if code, _ := doJSON(t, http.MethodPost, ts.URL+"/dev/state", "wrong", "", `{"count":1}`); code != http.StatusForbidden {
		t.Fatalf("wrong-token /dev/state: want 403, got %d", code)
	}
	if code, _ := doJSON(t, http.MethodPost, ts.URL+"/dev/state", tok, "", `{bad`); code != http.StatusBadRequest {
		t.Fatalf("bad /dev/state body: want 400, got %d", code)
	}
	if code, _ := doJSON(t, http.MethodPost, ts.URL+"/dev/highlight", "", "", `{"id":"btn_plus"}`); code != http.StatusForbidden {
		t.Fatalf("tokenless /dev/highlight: want 403, got %d", code)
	}
	if code, _ := doJSON(t, http.MethodPost, ts.URL+"/dev/highlight", tok, "", `{bad`); code != http.StatusBadRequest {
		t.Fatalf("bad /dev/highlight body: want 400, got %d", code)
	}
}

// TestDevTreeFallsBackToAnyScene: when the entry scene is missing from the
// manifest, /dev/tree still serves a tree (any scene) instead of nothing.
func TestDevTreeFallsBackToAnyScene(t *testing.T) {
	app := &model.App{Entry: "missing", Scenes: map[string]*model.Node{
		"only": {Type: "column", ID: "fallback-root"},
	}}
	s := New(qrt.New(app))
	rr := httptest.NewRecorder()
	s.serveDevTree(rr, httptest.NewRequest(http.MethodGet, "/dev/tree", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("GET /dev/tree status = %d", rr.Code)
	}
	var n model.Node
	if err := json.Unmarshal(rr.Body.Bytes(), &n); err != nil {
		t.Fatalf("dev/tree not JSON: %v", err)
	}
	if n.ID != "fallback-root" {
		t.Fatalf("dev/tree fallback = %+v, want the only scene", n)
	}
}

// TestLogWindowAndConsoleFallbackTitle: /logwindow embeds the current activity
// as JSON (null when empty), and both surfaces fall back to "QORM" without an
// app name.
func TestLogWindowAndConsoleFallbackTitle(t *testing.T) {
	app := &model.App{Entry: "main", Scenes: map[string]*model.Node{"main": {Type: "column", ID: "root"}}}
	s := New(qrt.New(app))
	s.logEvent("human", "dispatch inc")

	rr := httptest.NewRecorder()
	s.serveLogWindow(rr, httptest.NewRequest(http.MethodGet, "/logwindow", nil))
	body := rr.Body.String()
	if rr.Code != http.StatusOK {
		t.Fatalf("GET /logwindow status = %d", rr.Code)
	}
	if !strings.Contains(body, "QORM — QORM DevTool") {
		t.Error("logwindow must fall back to the QORM title")
	}
	if !strings.Contains(body, "dispatch inc") {
		t.Error("logwindow must embed the current activity log")
	}
	if !strings.Contains(body, "var initialLogs = [{") {
		t.Error("logwindow must embed the log entries as a JSON array")
	}

	// Empty activity marshals to null; the page's guard must handle it.
	s2 := New(qrt.New(app))
	rr2 := httptest.NewRecorder()
	s2.serveLogWindow(rr2, httptest.NewRequest(http.MethodGet, "/logwindow", nil))
	if !strings.Contains(rr2.Body.String(), "var initialLogs = null;") {
		t.Error("empty activity must embed as null")
	}

	rr3 := httptest.NewRecorder()
	s2.serveConsole(rr3, httptest.NewRequest(http.MethodGet, "/console", nil))
	if !strings.Contains(rr3.Body.String(), "QORM — collaboration console") {
		t.Error("console must fall back to the QORM title")
	}
}

// TestMCPNotificationReturnsNoContent: a JSON-RPC notification (no id) yields
// no response body — 204.
func TestMCPNotificationReturnsNoContent(t *testing.T) {
	ts := httptest.NewServer(counterServer(t).Handler())
	defer ts.Close()
	code, body := doJSON(t, http.MethodPost, ts.URL+"/mcp", "", "", `{"jsonrpc":"2.0","method":"notifications/initialized"}`)
	if code != http.StatusNoContent || body != "" {
		t.Fatalf("MCP notification: want 204 empty, got %d %q", code, body)
	}
}

// TestAgentMutationsAreLogged: every mutating MCP tool call is attributed to
// "agent" in the shared activity log.
func TestAgentMutationsAreLogged(t *testing.T) {
	ts := httptest.NewServer(counterServer(t).Handler())
	defer ts.Close()

	post(t, ts.URL+"/mcp", `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"qorm_set_state","arguments":{"path":"status","value":"done"}}}`)
	post(t, ts.URL+"/mcp", `{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"qorm_apply_patch","arguments":{"patch":"nope"}}}`)
	post(t, ts.URL+"/mcp", `{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"qorm_undo","arguments":{}}}`)

	code, body := doJSON(t, http.MethodGet, ts.URL+"/log?since=0", "", "", "")
	if code != http.StatusOK {
		t.Fatalf("GET /log: %d", code)
	}
	var entries []LogEntry
	if err := json.Unmarshal([]byte(body), &entries); err != nil {
		t.Fatalf("log not JSON: %v", err)
	}
	var details []string
	for _, e := range entries {
		if e.Source == "agent" {
			details = append(details, e.Detail)
		}
	}
	joined := strings.Join(details, "|")
	for _, want := range []string{"set_state status = done", "apply_patch (UI edit)", "undo"} {
		if !strings.Contains(joined, want) {
			t.Errorf("agent activity should log %q, got %v", want, details)
		}
	}
}

// TestIndexInjectsWebJSAndTransparentShell: GET / injects the app's
// native/web.js before </body> when present (nothing when absent), escapes a
// custom window title, and clears the shell background for transparent windows.
func TestIndexInjectsWebJSAndTransparentShell(t *testing.T) {
	s := counterServer(t)
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()

	// A basedir without native/web.js injects nothing.
	s.SetAppBaseDir(t.TempDir())
	code, body := doJSON(t, http.MethodGet, ts.URL+"/", "", "", "")
	if code != http.StatusOK {
		t.Fatalf("GET /: %d", code)
	}
	if strings.Contains(body, "__customWired") {
		t.Fatal("without native/web.js there is nothing to inject")
	}

	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "native"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "native", "web.js"), []byte("window.__customWired=true;"), 0o644); err != nil {
		t.Fatal(err)
	}
	s.SetAppBaseDir(dir)
	s.mu.Lock()
	s.rt.App.Window.Title = "Q&A <beta>"
	s.rt.App.Window.Transparent = true
	s.mu.Unlock()

	_, body = doJSON(t, http.MethodGet, ts.URL+"/", "", "", "")
	if !strings.Contains(body, "<script>window.__customWired=true;</script></body>") {
		t.Fatal("native/web.js should be injected just before </body>")
	}
	if !strings.Contains(body, "<title>Q&amp;A &lt;beta&gt;</title>") {
		t.Fatal("the window title should be HTML-escaped")
	}
	if !strings.Contains(body, "background:transparent!important") {
		t.Fatal("a transparent window should clear the shell background")
	}
}

// TestHotReloadBroadcastsToSubscribers: Reload pushes the re-rendered app to
// live subscribers and records the reload in the activity log.
func TestHotReloadBroadcastsToSubscribers(t *testing.T) {
	s := New(qrt.New(twoSceneApp("v1", "other", map[string]any{"count": float64(0)})))
	ch := make(chan string, 4)
	s.subsMu.Lock()
	s.subs = map[chan string]struct{}{ch: {}}
	s.subsMu.Unlock()

	s.Reload(qrt.New(twoSceneApp("v2", "other", map[string]any{"count": float64(0)})))

	select {
	case msg := <-ch:
		if !strings.Contains(msg, "v2") {
			t.Fatalf("hot-reload broadcast should carry the new UI, got %s", msg)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no broadcast after hot reload")
	}

	s.actMu.Lock()
	var found bool
	for _, e := range s.activity {
		if e.Source == "system" && e.Detail == "hot-reload: app source changed" {
			found = true
		}
	}
	s.actMu.Unlock()
	if !found {
		t.Fatal("hot reload should be logged as a system event")
	}
}
