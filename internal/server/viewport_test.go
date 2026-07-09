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
	qrt "github.com/qorm/qorm/internal/runtime"
)

func responsiveServer(t *testing.T) *Server {
	t.Helper()
	app, err := loader.LoadDir(filepath.Join("..", "..", "examples", "responsive"))
	if err != nil {
		t.Fatalf("load responsive example: %v", err)
	}
	return New(qrt.New(app))
}

// postViewport POSTs {w,h} with the given token and returns the status code.
func postViewport(t *testing.T, base, token string, w, h int) int {
	t.Helper()
	body, _ := json.Marshal(map[string]int{"w": w, "h": h})
	req, _ := http.NewRequest(http.MethodPost, base+"/viewport", strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("X-Qorm-Token", token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("post /viewport: %v", err)
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)
	return resp.StatusCode
}

// TestViewportEndpointTokenAndState: /viewport requires the page-embedded human
// token, updates rt.Viewport, bumps the revision (so live-sync pushes the new
// branch), and is a no-op for an unchanged size.
func TestViewportEndpointTokenAndState(t *testing.T) {
	s := responsiveServer(t)
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()
	tok := pageEventToken(t, ts.URL)

	// No / wrong token → 403 and no state change.
	if code := postViewport(t, ts.URL, "", 1440, 900); code != http.StatusForbidden {
		t.Fatalf("tokenless POST /viewport: want 403, got %d", code)
	}
	if code := postViewport(t, ts.URL, "wrong", 1440, 900); code != http.StatusForbidden {
		t.Fatalf("bad-token POST /viewport: want 403, got %d", code)
	}
	if s.rt.Viewport != (qrt.Viewport{}) {
		t.Fatalf("rejected POST must not change the viewport: %+v", s.rt.Viewport)
	}

	// Valid token updates the viewport and bumps the revision.
	rev0 := s.rev.Load()
	if code := postViewport(t, ts.URL, tok, 1440, 900); code != http.StatusNoContent {
		t.Fatalf("POST /viewport: want 204, got %d", code)
	}
	if s.rt.Viewport != (qrt.Viewport{W: 1440, H: 900}) {
		t.Fatalf("viewport not stored: %+v", s.rt.Viewport)
	}
	if s.rev.Load() != rev0+1 {
		t.Fatalf("viewport change must bump the revision: %d -> %d", rev0, s.rev.Load())
	}

	// Same size again is a no-op (no extra render/broadcast churn on jittery resize).
	if code := postViewport(t, ts.URL, tok, 1440, 900); code != http.StatusNoContent {
		t.Fatalf("repeat POST /viewport: want 204, got %d", code)
	}
	if s.rev.Load() != rev0+1 {
		t.Fatalf("unchanged viewport must not bump the revision")
	}

	// GET returns the current value.
	resp, err := http.Get(ts.URL + "/viewport")
	if err != nil {
		t.Fatalf("get /viewport: %v", err)
	}
	defer resp.Body.Close()
	var got struct{ W, H int }
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.W != 1440 || got.H != 900 {
		t.Fatalf("GET /viewport = %+v, want 1440x900", got)
	}
}

// TestViewportChangeBroadcastsBranchSwap: a viewport report is pushed to SSE
// subscribers with the re-rendered HTML showing the matching `when` branch.
func TestViewportChangeBroadcastsBranchSwap(t *testing.T) {
	s := responsiveServer(t)
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()
	tok := pageEventToken(t, ts.URL)

	// Subscribe like a second browser tab.
	ch := make(chan string, 8)
	s.subsMu.Lock()
	if s.subs == nil {
		s.subs = map[chan string]struct{}{}
	}
	s.subs[ch] = struct{}{}
	s.subsMu.Unlock()

	// First frame (unknown viewport) renders the narrow (else) branch.
	if html := renderCurrent(s); !strings.Contains(html, "cards-narrow") {
		t.Fatalf("unknown viewport should render the else (column) branch:\n%s", html)
	}

	if code := postViewport(t, ts.URL, tok, 1440, 900); code != http.StatusNoContent {
		t.Fatalf("POST /viewport: %d", code)
	}
	select {
	case msg := <-ch:
		if !strings.Contains(msg, "cards-wide") || strings.Contains(msg, "cards-narrow") {
			t.Fatalf("broadcast after 1440x900 should carry the wide (row) branch: %s", msg)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no broadcast after a viewport change")
	}

	if code := postViewport(t, ts.URL, tok, 375, 667); code != http.StatusNoContent {
		t.Fatalf("POST /viewport: %d", code)
	}
	select {
	case msg := <-ch:
		if !strings.Contains(msg, "cards-narrow") {
			t.Fatalf("broadcast after 375x667 should carry the narrow (column) branch: %s", msg)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no broadcast after the second viewport change")
	}
}
