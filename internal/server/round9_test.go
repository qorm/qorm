package server

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"
)

// renderedRev fetches the served page and extracts the revision the render
// embedded for the client (`var __rev=N`) — the revision the HTML was produced
// at. The first-connect handshake below keys off exactly this value.
func renderedRev(t *testing.T, base string) int64 {
	t.Helper()
	resp, err := http.Get(base + "/")
	if err != nil {
		t.Fatalf("get /: %v", err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	m := regexp.MustCompile(`var __rev=(\d+)`).FindStringSubmatch(string(b))
	if m == nil {
		t.Fatal("page must embed the rendered revision (var __rev=N)")
	}
	v, _ := strconv.ParseInt(m[1], 10, 64)
	return v
}

// connectSSEQuery opens /events with a raw query string plus extra headers and
// returns the response, a teardown func, and the line stream.
func connectSSEQuery(t *testing.T, base, query string, headers map[string]string) (*http.Response, func(), <-chan string) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/events"+query, nil)
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

// TestSSEFirstConnectCatchesUpMissedMutation: a mutation that lands in the
// window between GET / (the page render) and the EventSource opening is missed
// by a plain reconnect-only catch-up — the first connection ships no
// Last-Event-Id, and the broadcast went out before it subscribed. docs/
// collaboration.md promises SSE "to keep every viewer in sync", so the client
// connects carrying the revision its page was rendered at (?rev=N) and the
// server must resync it to the current UI (rev N+1) as the very first frame.
func TestSSEFirstConnectCatchesUpMissedMutation(t *testing.T) {
	s := counterServer(t)
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()

	// The browser renders the page; the render records its revision (rev N).
	n := renderedRev(t, ts.URL)

	// A mutation lands AFTER the render but BEFORE the EventSource opens — its
	// broadcast goes out while this viewer has no subscription yet.
	agentIncrement(t, ts.URL) // rev -> N+1 (count 0 -> 1)

	// The client connects carrying the revision its page was rendered at. The
	// first thing the stream delivers must be the catch-up snapshot at rev N+1
	// carrying the missed UI, preceded by an id: line (so a later reconnect can
	// replay it).
	_, drop, lines := connectSSEQuery(t, ts.URL, "?rev="+strconv.FormatInt(n, 10), nil)
	defer drop()

	var idSeen, frame string
	deadline := time.After(3 * time.Second)
loop:
	for {
		select {
		case line, ok := <-lines:
			if !ok {
				t.Fatal("stream closed before the catch-up snapshot")
			}
			if strings.HasPrefix(line, "id: ") {
				idSeen = strings.TrimPrefix(line, "id: ")
			}
			if strings.HasPrefix(line, "data: ") {
				frame = strings.TrimPrefix(line, "data: ")
				break loop
			}
		case <-deadline:
			t.Fatal("no catch-up snapshot within 3s — the first-connect mutation was lost")
		}
	}
	if idSeen != strconv.FormatInt(n+1, 10) {
		t.Fatalf("catch-up snapshot must ship id: %d, got %q", n+1, idSeen)
	}
	var d struct {
		Rev  int64  `json:"rev"`
		HTML string `json:"html"`
	}
	if err := json.Unmarshal([]byte(frame), &d); err != nil {
		t.Fatalf("catch-up frame not JSON: %v (%s)", err, frame)
	}
	if d.Rev != n+1 || !strings.Contains(d.HTML, ">1<") {
		t.Fatalf("first-connect catch-up must deliver the missed UI (rev %d, count=1), got %+v", n+1, d)
	}
}

// TestSSEFirstConnectCurrentNoRedundantSnapshot: a first connection whose
// rendered revision is already the tip gets NO redundant snapshot — the next
// data frame is the first live mutation.
func TestSSEFirstConnectCurrentNoRedundantSnapshot(t *testing.T) {
	s := counterServer(t)
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()

	agentIncrement(t, ts.URL) // rev -> 1 with no subscribers

	// Render the page at the tip and connect carrying that current revision.
	n := renderedRev(t, ts.URL)
	if n != 1 {
		t.Fatalf("rendered rev = %d, want 1", n)
	}
	_, drop, lines := connectSSEQuery(t, ts.URL, "?rev="+strconv.FormatInt(n, 10), nil)
	defer drop()
	waitUntil(t, 2*time.Second, func() bool { return s.subscriberCount() == 1 }, "subscriber registration")

	agentIncrement(t, ts.URL) // rev -> 2

	// The first data frame must be the live rev-2 broadcast, not a stale
	// snapshot at the already-current rev 1.
	frame := nextDataFrame(t, lines, 3*time.Second)
	var d struct {
		Rev int64 `json:"rev"`
	}
	if err := json.Unmarshal([]byte(frame), &d); err != nil {
		t.Fatalf("frame not JSON: %v (%s)", err, frame)
	}
	if d.Rev != 2 {
		t.Fatalf("an up-to-date first connect must not get a stale snapshot: first frame rev = %d, want 2", d.Rev)
	}
}

// TestSSEFirstConnectMaxOfRevAndLastEventID: on reconnect the EventSource
// re-requests the same URL (a now-stale ?rev=) AND replays the last frame's id.
// The server must treat the larger of the two as the last applied revision, so
// the stale query alone never forces a backwards/redundant snapshot.
func TestSSEFirstConnectMaxOfRevAndLastEventID(t *testing.T) {
	s := counterServer(t)
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()

	agentIncrement(t, ts.URL) // rev -> 1
	agentIncrement(t, ts.URL) // rev -> 2

	// Rendered rev is 2; simulate a reconnect that already applied rev 2 (the
	// Last-Event-Id) while still carrying the rendered ?rev=0 from its URL.
	_, drop, lines := connectSSEQuery(t, ts.URL, "?rev=0", map[string]string{"Last-Event-Id": "2"})
	defer drop()
	waitUntil(t, 2*time.Second, func() bool { return s.subscriberCount() == 1 }, "subscriber registration")

	agentIncrement(t, ts.URL) // rev -> 3

	frame := nextDataFrame(t, lines, 3*time.Second)
	var d struct {
		Rev int64 `json:"rev"`
	}
	if err := json.Unmarshal([]byte(frame), &d); err != nil {
		t.Fatalf("frame not JSON: %v (%s)", err, frame)
	}
	if d.Rev != 3 {
		t.Fatalf("max(Last-Event-Id, ?rev=) must win: first frame rev = %d, want 3 (no snapshot at the stale rev)", d.Rev)
	}
}

// TestFirstConnectClientHandshakeContract: the served page must embed the
// rendered revision where the client can read it, and the embedded client must
// open the EventSource carrying that revision — otherwise the server-side
// ?rev= catch-up never receives the value on first connect.
func TestFirstConnectClientHandshakeContract(t *testing.T) {
	s := counterServer(t)
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()

	agentIncrement(t, ts.URL) // rev -> 1 so the rendered rev is nonzero

	resp, err := http.Get(ts.URL + "/")
	if err != nil {
		t.Fatalf("get /: %v", err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	page := string(b)

	if !strings.Contains(page, "var __rev=1") {
		t.Fatal("page must embed the rendered revision (var __rev=1) for the client to read")
	}
	if !strings.Contains(page, "new EventSource('/events?rev='+__rev)") {
		t.Fatal("embedded client must construct the EventSource with the rendered rev (?rev=)")
	}
}
