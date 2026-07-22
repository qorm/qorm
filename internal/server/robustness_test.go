package server

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestMeasureProviderFeedsAgent: what the app POSTs to /measure reaches the
// agent through qorm_measure (via the provider wired in initAgent) — before
// any measurement the tool says so instead of guessing.
func TestMeasureProviderFeedsAgent(t *testing.T) {
	s := counterServer(t)
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()

	call := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"qorm_measure","arguments":{}}}`
	_, before := doJSON(t, http.MethodPost, ts.URL+"/mcp", "", "", call)
	if !strings.Contains(before, "no measurement yet") {
		t.Fatalf("qorm_measure before any report should say none yet, got %s", before)
	}

	doJSON(t, http.MethodPost, ts.URL+"/measure", "", "", `[{"id":"btn_plus","rect":{"x":1,"y":2,"w":30,"h":40}}]`)
	_, after := doJSON(t, http.MethodPost, ts.URL+"/mcp", "", "", call)
	// The report is the tool's text payload: decode it out of the JSON-RPC body.
	var rpc struct {
		Result struct {
			Content []struct {
				Text string `json:"text"`
			} `json:"content"`
		} `json:"result"`
	}
	if err := json.Unmarshal([]byte(after), &rpc); err != nil || len(rpc.Result.Content) == 0 {
		t.Fatalf("qorm_measure: bad response %q (%v)", after, err)
	}
	report := rpc.Result.Content[0].Text
	if !strings.Contains(report, `"components": 1`) || !strings.Contains(report, "btn_plus") {
		t.Fatalf("qorm_measure should report the stored layout, got %s", report)
	}
}

// TestBroadcastDropsForFullSubscriber: a subscriber that never drains must not
// block the broadcaster — its frame is dropped, healthy subscribers still get it.
func TestBroadcastDropsForFullSubscriber(t *testing.T) {
	s := counterServer(t)
	full := make(chan string) // unbuffered and never read: always "full"
	healthy := make(chan string, 4)
	s.subsMu.Lock()
	s.subs = map[chan string]struct{}{full: {}, healthy: {}}
	s.subsMu.Unlock()

	done := make(chan struct{})
	go func() {
		defer close(done)
		s.agent.HandleHTTP([]byte(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"qorm_dispatch","arguments":{"action":"increment","args":{"count":0}}}}`))
	}()

	select {
	case msg := <-healthy:
		var d struct {
			Rev  int64  `json:"rev"`
			HTML string `json:"html"`
		}
		if err := json.Unmarshal([]byte(msg), &d); err != nil {
			t.Fatalf("broadcast frame not JSON: %v", err)
		}
		if d.Rev == 0 || !strings.Contains(d.HTML, ">1<") {
			t.Fatalf("healthy subscriber should receive the count=1 UI, got %s", msg)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("healthy subscriber starved while another was full")
	}
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("broadcast must not block on a full subscriber")
	}
}

// TestActivityLogTrimsToLast200: the display log keeps the last 200 entries
// and the hash chain stays intact across the trim.
func TestActivityLogTrimsToLast200(t *testing.T) {
	s := counterServer(t)
	for i := 0; i < 205; i++ {
		s.logEvent("system", "tick")
	}
	s.actMu.Lock()
	defer s.actMu.Unlock()
	if len(s.activity) != 200 {
		t.Fatalf("display log retained %d entries, want 200", len(s.activity))
	}
	if s.activity[0].Seq != 6 {
		t.Fatalf("oldest retained seq = %d, want 6 (205-199)", s.activity[0].Seq)
	}
	if s.actSeq != 205 {
		t.Fatalf("actSeq = %d, want 205 (the chain counts every entry)", s.actSeq)
	}
	last := s.activity[len(s.activity)-1]
	if last.Hash != auditHash(s.activity[len(s.activity)-2].Hash, last) {
		t.Fatal("trimming the display log must not break the hash chain")
	}
}

// TestEventBeforeFirstPageLoad: a reconnecting client that POSTs /event before
// ever GETting / must not have its action silently dropped — the handler table
// is built on demand.
func TestEventBeforeFirstPageLoad(t *testing.T) {
	s := counterServer(t)
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()

	// No page load yet: the handler table is empty. Use the server's token
	// directly, as a reconnecting browser tab (which still holds it) would.
	code, body := doJSON(t, http.MethodPost, ts.URL+"/event", s.eventToken, "", `{"h":1,"inputs":{}}`)
	if code != http.StatusOK {
		t.Fatalf("/event before first page load: want 200, got %d", code)
	}
	if !strings.Contains(body, ">1<") {
		t.Fatalf("the dispatch must run even without a prior page load (count=1):\n%s", body)
	}
}

// TestNavigateNoMoveIsNoOp: navigating where the app already is returns 204
// but adds no activity entry and does not bump the revision.
func TestNavigateNoMoveIsNoOp(t *testing.T) {
	s := navServer(t)
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()
	tok := pageEventToken(t, ts.URL)

	if code, _ := doJSON(t, http.MethodPost, ts.URL+"/navigate", tok, "", `{"scene":"profile"}`); code != http.StatusNoContent {
		t.Fatalf("navigate to profile: want 204, got %d", code)
	}
	s.actMu.Lock()
	entries := len(s.activity)
	s.actMu.Unlock()
	rev := s.rev.Load()

	// Same scene again: the route does not move.
	if code, _ := doJSON(t, http.MethodPost, ts.URL+"/navigate", tok, "", `{"scene":"profile"}`); code != http.StatusNoContent {
		t.Fatalf("repeat navigate: want 204, got %d", code)
	}
	s.actMu.Lock()
	entriesAfter := len(s.activity)
	s.actMu.Unlock()
	if entriesAfter != entries {
		t.Fatalf("a no-move navigate must not log (%d -> %d entries)", entries, entriesAfter)
	}
	if s.rev.Load() != rev {
		t.Fatal("a no-move navigate must not bump the revision")
	}

	// Back once pops profile; back again on an empty stack is a no-op.
	doJSON(t, http.MethodPost, ts.URL+"/navigate", tok, "", `{"back":true}`)
	revAtEntry := s.rev.Load()
	doJSON(t, http.MethodPost, ts.URL+"/navigate", tok, "", `{"back":true}`)
	if s.rev.Load() != revAtEntry {
		t.Fatal("back on an empty nav stack must not bump the revision")
	}
}

// noFlushWriter is a ResponseWriter without http.Flusher, to exercise the
// streaming-unsupported path.
type noFlushWriter struct {
	header http.Header
	body   bytes.Buffer
	code   int
}

func (w *noFlushWriter) Header() http.Header         { return w.header }
func (w *noFlushWriter) Write(b []byte) (int, error) { return w.body.Write(b) }
func (w *noFlushWriter) WriteHeader(code int)        { w.code = code }

// TestEventsSSERequiresFlusher: a client whose transport cannot stream gets a
// clean 500 instead of a hung connection.
func TestEventsSSERequiresFlusher(t *testing.T) {
	s := counterServer(t)
	w := &noFlushWriter{header: http.Header{}}
	s.serveEvents(w, httptest.NewRequest(http.MethodGet, "/events", nil))
	if w.code != http.StatusInternalServerError {
		t.Fatalf("non-streaming /events: want 500, got %d", w.code)
	}
	if !strings.Contains(w.body.String(), "streaming unsupported") {
		t.Fatalf("non-streaming /events body = %q", w.body.String())
	}
}

// errReader makes request-body reads fail, to exercise the transport error path.
type errReader struct{}

func (errReader) Read([]byte) (int, error) { return 0, errors.New("connection reset") }

// TestMCPBodyReadError: an unreadable request body is a 400, not a panic or a
// silent 204.
func TestMCPBodyReadError(t *testing.T) {
	s := counterServer(t)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/mcp", errReader{})
	s.serveMCP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("unreadable /mcp body: want 400, got %d", rr.Code)
	}
}

// TestAuditLogRobustness: the audit sink fails cleanly on bad paths, refuses to
// resume a corrupt chain, and verification skips blank lines while rejecting
// invalid JSON and oversized entries.
func TestAuditLogRobustness(t *testing.T) {
	s := counterServer(t)
	if err := s.SetAuditLog(filepath.Join(t.TempDir(), "no-such-dir", "audit.jsonl")); err == nil {
		t.Fatal("SetAuditLog must fail when the path cannot be created")
	}

	if _, n, err := lastAuditEntry(filepath.Join(t.TempDir(), "missing.jsonl")); err == nil || n != 0 {
		t.Fatalf("missing audit file: n=%d err=%v, want an error", n, err)
	}

	// A corrupt existing log: the chain is NOT resumed (fresh seq + hash), so
	// new entries form a self-consistent chain instead of extending garbage.
	dir := t.TempDir()
	corrupt := filepath.Join(dir, "corrupt.jsonl")
	if err := os.WriteFile(corrupt, []byte("this is not json\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	s2 := counterServer(t)
	if err := s2.SetAuditLog(corrupt); err != nil {
		t.Fatalf("SetAuditLog on a corrupt file: %v", err)
	}
	s2.actMu.Lock()
	seq, hash := s2.actSeq, s2.lastHash
	s2.actMu.Unlock()
	if seq != 0 || hash != "" {
		t.Fatalf("a corrupt log must not resume the chain: seq=%d hash=%q", seq, hash)
	}

	// Blank lines are ignored everywhere; invalid JSON fails with context.
	mixed := filepath.Join(dir, "mixed.jsonl")
	if err := os.WriteFile(mixed, []byte("\n\nnot-json\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(mixed)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := VerifyAuditChain(f); err == nil || !strings.Contains(err.Error(), "not valid JSON") {
		t.Fatalf("invalid JSON must fail verification, got %v", err)
	}
	f.Close()
	if _, n, err := lastAuditEntry(mixed); err == nil {
		t.Fatalf("lastAuditEntry on a corrupt file must error, got n=%d", n)
	}

	// A valid chain interleaved with blank lines verifies end-to-end, and
	// lastAuditEntry reports its tip.
	good := filepath.Join(dir, "good.jsonl")
	s3 := counterServer(t)
	if err := s3.SetAuditLog(good); err != nil {
		t.Fatal(err)
	}
	s3.logEvent("human", "a")
	s3.logEvent("agent", "b")
	raw, err := os.ReadFile(good)
	if err != nil {
		t.Fatal(err)
	}
	padded := filepath.Join(dir, "padded.jsonl")
	if err := os.WriteFile(padded, []byte("\n"+strings.ReplaceAll(string(raw), "\n", "\n\n")), 0o600); err != nil {
		t.Fatal(err)
	}
	f2, err := os.Open(padded)
	if err != nil {
		t.Fatal(err)
	}
	n, err := VerifyAuditChain(f2)
	f2.Close()
	if err != nil || n != 2 {
		t.Fatalf("blank lines must be skipped: n=%d err=%v", n, err)
	}
	if last, count, err := lastAuditEntry(padded); err != nil || count != 2 || last.Detail != "b" {
		t.Fatalf("lastAuditEntry(padded) = %+v, %d, %v", last, count, err)
	}

	// A line beyond the scanner buffer surfaces as a verification error.
	huge := filepath.Join(dir, "huge.jsonl")
	if err := os.WriteFile(huge, []byte(strings.Repeat("x", 2<<20)+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	f3, err := os.Open(huge)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := VerifyAuditChain(f3); err == nil {
		t.Fatal("an oversized line must fail verification")
	}
	f3.Close()
}
