package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// postJSON posts a body with optional token and returns the status code.
func postJSON(t *testing.T, url, token, body string) int {
	t.Helper()
	req, _ := http.NewRequest(http.MethodPost, url, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("X-Qorm-Token", token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("post %s: %v", url, err)
	}
	defer resp.Body.Close()
	return resp.StatusCode
}

func sources(s *Server) []string {
	s.actMu.Lock()
	defer s.actMu.Unlock()
	var out []string
	for _, e := range s.activity {
		out = append(out, e.Source)
	}
	return out
}

// TestChannelIsolation locks down the attribution guarantees: no channel can
// mint an identity it does not own.
func TestChannelIsolation(t *testing.T) {
	s := counterServer(t)
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()
	token := pageEventToken(t, ts.URL)

	// 1. /event without the token: rejected AND no "human" entry appears.
	if code := postJSON(t, ts.URL+"/event", "", `{"h":1,"inputs":{}}`); code != http.StatusForbidden {
		t.Fatalf("tokenless /event: status %d, want 403", code)
	}
	for _, src := range sources(s) {
		if src == "human" {
			t.Fatal("a rejected /event still produced a human audit entry")
		}
	}

	// 2. /log can never mint "human" or "agent": with a valid token the entry
	//    is forced to "app"; without one it is rejected outright.
	if code := postJSON(t, ts.URL+"/log", token, `{"source":"human","detail":"forged"}`); code != http.StatusNoContent {
		t.Fatalf("tokened /log: status %d, want 204", code)
	}
	if code := postJSON(t, ts.URL+"/log", "", `{"source":"agent","detail":"forged"}`); code != http.StatusForbidden {
		t.Fatalf("tokenless /log: status %d, want 403", code)
	}
	for _, e := range s.activity {
		if e.Detail == "forged" && e.Source != "app" {
			t.Fatalf("injected /log entry recorded as %q, want app", e.Source)
		}
	}

	// 3. /dev/state without the token: rejected and state untouched.
	before := fmt.Sprintf("%v", s.rt.State["count"])
	if code := postJSON(t, ts.URL+"/dev/state", "", `{"count":999}`); code != http.StatusForbidden {
		t.Fatalf("tokenless /dev/state: status %d, want 403", code)
	}
	if after := fmt.Sprintf("%v", s.rt.State["count"]); after != before {
		t.Fatalf("tokenless /dev/state mutated state: %s -> %s", before, after)
	}
	// With the token it works and is attributed to "devtool" (not "system").
	if code := postJSON(t, ts.URL+"/dev/state", token, `{"count":7,"status":"x"}`); code != http.StatusOK {
		t.Fatalf("tokened /dev/state: status %d, want 200", code)
	}
	found := false
	for _, e := range s.activity {
		if e.Source == "devtool" && strings.Contains(e.Detail, "devtool") {
			found = true
		}
	}
	if !found {
		t.Fatal("devtool state write not attributed as devtool in the audit log")
	}

	// 4. The mirror guard: a human (holding the token) cannot route through
	//    the agent channel. /mcp refuses the token.
	if code := postJSON(t, ts.URL+"/mcp", token,
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"qorm_dispatch","arguments":{"action":"increment","args":{}}}}`); code != http.StatusForbidden {
		t.Fatalf("tokened /mcp: status %d, want 403 (human must not use the agent channel)", code)
	}

	// 5. Legit channels: a tokened human click and a tokenless agent MCP
	//    dispatch land with their own identities.
	postEvent(t, ts.URL, token, `{"h":1,"inputs":{}}`)
	post(t, ts.URL+"/mcp", `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"qorm_dispatch","arguments":{"action":"increment","args":{}}}}`)
	var haveHuman, haveAgent bool
	for _, src := range sources(s) {
		if src == "human" {
			haveHuman = true
		}
		if src == "agent" {
			haveAgent = true
		}
	}
	if !haveHuman || !haveAgent {
		t.Fatalf("expected human+agent entries, got %v", sources(s))
	}
}

// TestMCPSurfaceNeverLeaksEventToken guards the isolation boundary itself: no
// MCP tool response may contain the human event token (an agent that reads it
// could forge /event calls).
func TestMCPSurfaceNeverLeaksEventToken(t *testing.T) {
	s := counterServer(t)
	http.Get(httptest.NewServer(s.Handler()).URL + "/") // prime handlers
	for _, tool := range []string{
		"qorm_inspect", "qorm_render_html", "qorm_list_actions",
		"qorm_capabilities", "qorm_activity", "qorm_get_node",
	} {
		out, err := s.agent.HandleHTTP([]byte(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"` + tool + `","arguments":{"id":"btn_plus"}}}`))
		if err != nil {
			t.Fatalf("%s: %v", tool, err)
		}
		if strings.Contains(string(out), s.eventToken) {
			t.Fatalf("%s leaks the human event token", tool)
		}
	}
}

// TestAuditChain covers the tamper-evident log: chained entries verify, any
// modification is detected, and the file survives a server restart.
func TestAuditChain(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "audit.jsonl")

	s := counterServer(t)
	if err := s.SetAuditLog(path); err != nil {
		t.Fatal(err)
	}
	s.logEvent("human", "dispatch increment")
	s.logEvent("agent", "set_state count = 7")
	s.logEvent("devtool", "state modified via devtool")

	f, _ := os.Open(path)
	n, err := VerifyAuditChain(f)
	f.Close()
	if err != nil || n != 3 {
		t.Fatalf("verify: n=%d err=%v", n, err)
	}

	// Restart: a new server resumes the chain in the same file.
	s2 := counterServer(t)
	if err := s2.SetAuditLog(path); err != nil {
		t.Fatal(err)
	}
	s2.logEvent("human", "after restart")
	f, _ = os.Open(path)
	n, err = VerifyAuditChain(f)
	f.Close()
	if err != nil || n != 4 {
		t.Fatalf("verify after restart: n=%d err=%v", n, err)
	}

	// Tamper: re-attribute the agent entry to "human" — the chain must break.
	raw, _ := os.ReadFile(path)
	lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
	var e LogEntry
	json.Unmarshal([]byte(lines[1]), &e)
	e.Source = "human"
	forged, _ := json.Marshal(e)
	lines[1] = string(forged)
	tampered := filepath.Join(dir, "tampered.jsonl")
	os.WriteFile(tampered, []byte(strings.Join(lines, "\n")+"\n"), 0o600)
	f, _ = os.Open(tampered)
	n, err = VerifyAuditChain(f)
	f.Close()
	if err == nil {
		t.Fatal("re-attributed entry passed verification")
	}
	if n != 1 {
		t.Fatalf("chain should break at entry 2 (verified=1), got %d", n)
	}

	// Tamper: drop an entry — also detected.
	dropped := filepath.Join(dir, "dropped.jsonl")
	os.WriteFile(dropped, []byte(lines[0]+"\n"+strings.Join(lines[2:], "\n")+"\n"), 0o600)
	f, _ = os.Open(dropped)
	_, err = VerifyAuditChain(f)
	f.Close()
	if err == nil {
		t.Fatal("dropped entry passed verification")
	}
}
