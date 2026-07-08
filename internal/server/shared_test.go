package server

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/qorm/qorm/internal/loader"
	"github.com/qorm/qorm/internal/render"
	qrt "github.com/qorm/qorm/internal/runtime"
)

func counterServer(t *testing.T) *Server {
	t.Helper()
	app, err := loader.LoadDir(filepath.Join("..", "..", "examples", "counter"))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	return New(qrt.New(app))
}

func post(t *testing.T, url, body string) string {
	t.Helper()
	resp, err := http.Post(url, "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("post %s: %v", url, err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return string(b)
}

// pageEventToken fetches the served page and extracts the embedded human event
// token, exactly as the browser client does. (It also primes the handler table,
// like a browser rendering the page before clicking.)
func pageEventToken(t *testing.T, base string) string {
	t.Helper()
	resp, err := http.Get(base + "/")
	if err != nil {
		t.Fatalf("get /: %v", err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	const anchor = "var __tok='"
	page := string(b)
	i := strings.Index(page, anchor)
	if i < 0 {
		t.Fatal("page should embed the event token (var __tok=...)")
	}
	rest := page[i+len(anchor):]
	j := strings.Index(rest, "'")
	if j < 0 {
		t.Fatal("unterminated event token in page")
	}
	return rest[:j]
}

// postEvent POSTs a human event with the page-embedded token, as the browser
// client does.
func postEvent(t *testing.T, base, token, body string) string {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, base+"/event", strings.NewReader(body))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Qorm-Token", token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("post /event: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("post /event: status %d", resp.StatusCode)
	}
	b, _ := io.ReadAll(resp.Body)
	return string(b)
}

// TestSharedSessionHumanAndAgent verifies an agent editing over /mcp and a
// human clicking over /event both act on one live app, and each sees the
// other's change.
func TestSharedSessionHumanAndAgent(t *testing.T) {
	ts := httptest.NewServer(counterServer(t).Handler())
	defer ts.Close()

	// Agent operates the live app.
	post(t, ts.URL+"/mcp", `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"qorm_dispatch","arguments":{"action":"increment","args":{"count":0}}}}`)

	// The browser's live-sync poll observes the agent's change.
	pollBody := func() (int64, string) {
		resp, err := http.Get(ts.URL + "/poll?rev=0")
		if err != nil {
			t.Fatalf("poll: %v", err)
		}
		defer resp.Body.Close()
		var d struct {
			Rev  int64  `json:"rev"`
			HTML string `json:"html"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&d)
		return d.Rev, d.HTML
	}
	rev, html := pollBody()
	if rev == 0 {
		t.Fatal("revision should advance after agent edit")
	}
	if !strings.Contains(html, ">1<") {
		t.Errorf("browser poll should see agent's count=1, html:\n%s", html)
	}

	// Human clicks "+" (btn_plus is handler index 1 after a render). /event is
	// human-only: it requires the token embedded in the rendered page, so fetch
	// the page and authenticate like a real browser.
	token := pageEventToken(t, ts.URL)
	postEvent(t, ts.URL, token, `{"h":1,"inputs":{}}`)

	// Agent sees the human's change.
	inspect := post(t, ts.URL+"/mcp", `{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"qorm_inspect","arguments":{}}}`)
	var rpc struct {
		Result struct {
			Content []struct {
				Text string `json:"text"`
			} `json:"content"`
		} `json:"result"`
	}
	_ = json.Unmarshal([]byte(inspect), &rpc)
	if !strings.Contains(rpc.Result.Content[0].Text, `"count": 2`) {
		t.Errorf("agent should see human's count=2, got:\n%s", rpc.Result.Content[0].Text)
	}
}

// TestSSEBroadcastOnAgentEdit verifies a subscriber receives the new UI the
// instant an agent mutates the shared session (no polling).
func TestSSEBroadcastOnAgentEdit(t *testing.T) {
	s := counterServer(t)
	ch := make(chan string, 4)
	s.subsMu.Lock()
	s.subs = map[chan string]struct{}{ch: {}}
	s.subsMu.Unlock()

	// Agent dispatches an action -> bump() -> broadcast to subscribers.
	s.agent.HandleHTTP([]byte(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"qorm_dispatch","arguments":{"action":"increment","args":{"count":0}}}}`))

	select {
	case msg := <-ch:
		var d struct {
			Rev  int64  `json:"rev"`
			HTML string `json:"html"`
		}
		if err := json.Unmarshal([]byte(msg), &d); err != nil {
			t.Fatalf("broadcast payload not JSON: %v", err)
		}
		if d.Rev == 0 || !strings.Contains(d.HTML, ">1<") {
			t.Errorf("broadcast should carry the new UI (count 1), got rev=%d html=%s", d.Rev, d.HTML)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("expected an SSE broadcast after the agent edit")
	}
}

func TestAccessibilityLangDirAndLandmark(t *testing.T) {
	app, err := loader.LoadDir(filepath.Join("..", "..", "examples", "i18n"))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	rt := qrt.New(app)

	// English: lang=en dir=ltr, root is a main landmark.
	page := Page(rt, render.Render(rt).HTML, 0)
	if !strings.Contains(page, `lang="en"`) || !strings.Contains(page, `dir="ltr"`) {
		t.Error("en page should declare lang=en dir=ltr")
	}
	if !strings.Contains(page, `role="main"`) {
		t.Error("scene root should be a main landmark")
	}
	// Arabic: lang=ar dir=rtl on <html>.
	rt.State["locale"] = "ar"
	pageAr := Page(rt, render.Render(rt).HTML, 0)
	if !strings.Contains(pageAr, `lang="ar"`) || !strings.Contains(pageAr, `dir="rtl"`) {
		t.Error("ar page should declare lang=ar dir=rtl")
	}
}

func TestDevtoolIntegrationAndSSE(t *testing.T) {
	s := counterServer(t)
	ch := make(chan string, 4)
	s.subsMu.Lock()
	s.subs = map[chan string]struct{}{ch: {}}
	s.subsMu.Unlock()

	ts := httptest.NewServer(s.Handler())
	defer ts.Close()

	// 1. 测试 /dev/highlight 的 SSE 广播链路
	highlightPayload := `{"id":"btn_plus"}`
	respHighlight := post(t, ts.URL+"/dev/highlight", highlightPayload)
	if respHighlight != "" {
		t.Errorf("expected empty body for /dev/highlight POST, got: %s", respHighlight)
	}

	select {
	case msg := <-ch:
		var d struct {
			InspectNode string `json:"inspectNode"`
		}
		if err := json.Unmarshal([]byte(msg), &d); err != nil {
			t.Fatalf("broadcast payload not JSON: %v", err)
		}
		if d.InspectNode != "btn_plus" {
			t.Errorf("expected inspectNode to be btn_plus, got %q (payload: %s)", d.InspectNode, msg)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("expected an SSE broadcast after the /dev/highlight post")
	}

	// 2. 测试 /dev/state 更新后的状态同步与重绘广播
	statePayload := `{"count":123}`
	post(t, ts.URL+"/dev/state", statePayload)

	s.mu.Lock()
	currentCount := s.rt.State["count"]
	s.mu.Unlock()
	if currentCount != 123.0 {
		t.Errorf("expected state count to be 123, got %v", currentCount)
	}

	select {
	case msg := <-ch:
		var d struct {
			Rev  int64  `json:"rev"`
			HTML string `json:"html"`
		}
		if err := json.Unmarshal([]byte(msg), &d); err != nil {
			t.Fatalf("broadcast payload not JSON: %v", err)
		}
		if !strings.Contains(d.HTML, ">123<") {
			t.Errorf("broadcast should carry the new HTML showing count 123, got HTML:\n%s", d.HTML)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("expected an SSE broadcast after the /dev/state edit")
	}
}

// TestMCPReadOnlyMode verifies --mcp-read-only enforcement at the shared MCP
// session: mutating tools are rejected with a JSON-RPC error while inspection
// tools keep working, and the app state is untouched.
func TestMCPReadOnlyMode(t *testing.T) {
	s := counterServer(t)
	s.SetMCPReadOnly(true)
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()

	// Mutating tools are refused before reaching the tool handler.
	for _, tool := range []string{"qorm_dispatch", "qorm_set_state", "qorm_apply_patch"} {
		resp := post(t, ts.URL+"/mcp",
			`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"`+tool+`","arguments":{"action":"increment"}}}`)
		var r struct {
			Error *struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		if err := json.Unmarshal([]byte(resp), &r); err != nil {
			t.Fatalf("%s: bad response %q: %v", tool, resp, err)
		}
		if r.Error == nil || !strings.Contains(r.Error.Message, "read-only mode") {
			t.Fatalf("%s must return a read-only JSON-RPC error, got %s", tool, resp)
		}
	}

	// Read-only tools still work.
	inspect := post(t, ts.URL+"/mcp",
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"qorm_inspect","arguments":{}}}`)
	if strings.Contains(inspect, `"error"`) || !strings.Contains(inspect, "counter") {
		t.Fatalf("qorm_inspect should succeed in read-only mode, got %s", inspect)
	}

	// Nothing mutated: the counter is still at 0.
	if got := renderCurrent(s); !strings.Contains(got, ">0<") {
		t.Fatalf("state must be unchanged in read-only mode, html: %s", got)
	}

	// Switching read-only off restores normal operation.
	s.SetMCPReadOnly(false)
	resp := post(t, ts.URL+"/mcp",
		`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"qorm_dispatch","arguments":{"action":"increment","args":{"count":0}}}}`)
	if strings.Contains(resp, "read-only mode") {
		t.Fatalf("dispatch should work after disabling read-only, got %s", resp)
	}
}
