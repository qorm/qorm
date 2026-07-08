package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/qorm/qorm/internal/model"
	"github.com/qorm/qorm/internal/runtime"
)

func consoleServer() *Server {
	app := &model.App{Entry: "main", Name: "T",
		Scenes: map[string]*model.Node{"main": {Type: "column", ID: "root"}}}
	return New(runtime.New(app))
}

func TestActivityLogAndConsole(t *testing.T) {
	s := consoleServer()
	// record a human + agent event
	s.logEvent("human", "dispatch increment")
	s.recordAgentCall([]byte(`{"method":"tools/call","params":{"name":"qorm_set_state","arguments":{"path":"theme","value":"dark"}}}`))

	// /log returns both, newest-inclusive, filtered by since
	rr := httptest.NewRecorder()
	s.serveLog(rr, httptest.NewRequest(http.MethodGet, "/log?since=0", nil))
	var entries []LogEntry
	if err := json.Unmarshal(rr.Body.Bytes(), &entries); err != nil {
		t.Fatalf("log json: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("want 2 log entries, got %d", len(entries))
	}
	if entries[0].Source != "human" || !strings.Contains(entries[0].Detail, "increment") {
		t.Errorf("human entry wrong: %+v", entries[0])
	}
	if entries[1].Source != "agent" || !strings.Contains(entries[1].Detail, "theme = dark") {
		t.Errorf("agent entry wrong: %+v", entries[1])
	}
	// since filters
	rr2 := httptest.NewRecorder()
	s.serveLog(rr2, httptest.NewRequest(http.MethodGet, "/log?since=1", nil))
	var after []LogEntry
	json.Unmarshal(rr2.Body.Bytes(), &after)
	if len(after) != 1 {
		t.Errorf("since=1 should return 1 entry, got %d", len(after))
	}
	// /console renders the two-pane page
	rr3 := httptest.NewRecorder()
	s.serveConsole(rr3, httptest.NewRequest(http.MethodGet, "/console", nil))
	body := rr3.Body.String()
	for _, m := range []string{`<iframe src="/"`, "Collaboration log", "/log?since="} {
		if !strings.Contains(body, m) {
			t.Errorf("console page missing %q", m)
		}
	}
}

func TestDevtoolEndpoints(t *testing.T) {
	s := consoleServer()

	// Test 1: GET /dev/state
	rr1 := httptest.NewRecorder()
	s.serveDevState(rr1, httptest.NewRequest(http.MethodGet, "/dev/state", nil))
	if rr1.Code != http.StatusOK {
		t.Fatalf("GET /dev/state returned status %d", rr1.Code)
	}
	var state map[string]any
	if err := json.Unmarshal(rr1.Body.Bytes(), &state); err != nil {
		t.Fatalf("JSON decode state: %v", err)
	}

	// Test 2: POST /dev/state
	stateUpdate := map[string]any{"count": 42.0}
	body, _ := json.Marshal(stateUpdate)
	rr2 := httptest.NewRecorder()
	s.serveDevState(rr2, httptest.NewRequest(http.MethodPost, "/dev/state", strings.NewReader(string(body))))
	if rr2.Code != http.StatusOK {
		t.Fatalf("POST /dev/state returned status %d", rr2.Code)
	}
	// Verify state got modified
	s.mu.Lock()
	val := s.rt.State["count"]
	s.mu.Unlock()
	if val != 42.0 {
		t.Errorf("expected state count to be 42, got %v", val)
	}

	// Test 3: GET /dev/tree
	rr3 := httptest.NewRecorder()
	s.serveDevTree(rr3, httptest.NewRequest(http.MethodGet, "/dev/tree", nil))
	if rr3.Code != http.StatusOK {
		t.Fatalf("GET /dev/tree returned status %d", rr3.Code)
	}
	var node model.Node
	if err := json.Unmarshal(rr3.Body.Bytes(), &node); err != nil {
		t.Fatalf("JSON decode tree: %v", err)
	}
	if node.Type != "column" || node.ID != "root" {
		t.Errorf("unexpected root node: %+v", node)
	}

	// Test 4: POST /dev/highlight
	hlBody, _ := json.Marshal(map[string]string{"id": "root"})
	rr4 := httptest.NewRecorder()
	s.serveDevHighlight(rr4, httptest.NewRequest(http.MethodPost, "/dev/highlight", strings.NewReader(string(hlBody))))
	if rr4.Code != http.StatusOK {
		t.Fatalf("POST /dev/highlight returned status %d", rr4.Code)
	}
}
