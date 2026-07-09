package server

import (
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

func navServer(t *testing.T) *Server {
	t.Helper()
	app, err := loader.LoadDir(filepath.Join("..", "..", "examples", "navigation"))
	if err != nil {
		t.Fatalf("load navigation example: %v", err)
	}
	return New(qrt.New(app))
}

// TestDeepLinkInitialLoad: GET /?scene=profile&<params> navigates the runtime
// before rendering, so the page loads straight into that scene with its route
// params bound to route.* — no click needed.
func TestDeepLinkInitialLoad(t *testing.T) {
	s := navServer(t)
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/?scene=profile&userId=u-101&name=Ada&role=Design")
	if err != nil {
		t.Fatalf("get deep link: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	html := string(body)
	if !strings.Contains(html, "User id: u-101") {
		t.Fatalf("deep link should render profile with route.userId:\n%s", html)
	}
	if !strings.Contains(html, "Ada") {
		t.Fatalf("deep link should render route.name Ada")
	}
	if s.rt.CurrentScene() != "profile" {
		t.Fatalf("runtime scene after deep link = %q, want profile", s.rt.CurrentScene())
	}

	// An unknown scene is ignored — falls back to the entry scene.
	s2 := navServer(t)
	ts2 := httptest.NewServer(s2.Handler())
	defer ts2.Close()
	r2, _ := http.Get(ts2.URL + "/?scene=does-not-exist")
	io.Copy(io.Discard, r2.Body)
	r2.Body.Close()
	if s2.rt.CurrentScene() != "" {
		t.Fatalf("unknown deep-link scene must fall back to entry, got %q", s2.rt.CurrentScene())
	}
}

// TestNavigateEndpoint: /navigate is token-gated, drives the runtime, and
// broadcasts the new route to SSE subscribers.
func TestNavigateEndpoint(t *testing.T) {
	s := navServer(t)
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()
	tok := pageEventToken(t, ts.URL)

	postNav := func(token, body string) int {
		req, _ := http.NewRequest(http.MethodPost, ts.URL+"/navigate", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		if token != "" {
			req.Header.Set("X-Qorm-Token", token)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("post /navigate: %v", err)
		}
		defer resp.Body.Close()
		io.Copy(io.Discard, resp.Body)
		return resp.StatusCode
	}

	// No / wrong token → 403, no navigation.
	if code := postNav("", `{"scene":"profile"}`); code != http.StatusForbidden {
		t.Fatalf("tokenless /navigate: want 403, got %d", code)
	}
	if s.rt.CurrentScene() != "" {
		t.Fatalf("rejected /navigate must not navigate, scene=%q", s.rt.CurrentScene())
	}

	// Subscribe like a browser tab, then navigate with a valid token.
	ch := make(chan string, 8)
	s.subsMu.Lock()
	if s.subs == nil {
		s.subs = map[chan string]struct{}{}
	}
	s.subs[ch] = struct{}{}
	s.subsMu.Unlock()

	if code := postNav(tok, `{"scene":"profile","params":{"userId":"u-7","name":"Zed"}}`); code != http.StatusNoContent {
		t.Fatalf("valid /navigate: want 204, got %d", code)
	}
	if s.rt.CurrentScene() != "profile" || s.rt.RouteParams["userId"] != "u-7" {
		t.Fatalf("after /navigate: scene=%q params=%#v", s.rt.CurrentScene(), s.rt.RouteParams)
	}
	select {
	case msg := <-ch:
		if !strings.Contains(msg, `"route"`) || !strings.Contains(msg, "scene=profile") {
			t.Fatalf("broadcast should carry the new route: %s", msg)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no broadcast after /navigate")
	}

	// The activity log records it as a human navigation.
	s.actMu.Lock()
	var found bool
	for _, e := range s.activity {
		if e.Source == "human" && strings.HasPrefix(e.Detail, "navigate ") {
			found = true
		}
	}
	s.actMu.Unlock()
	if !found {
		t.Fatal("/navigate should log a human navigation")
	}

	// back:true pops to the entry scene.
	if code := postNav(tok, `{"back":true}`); code != http.StatusNoContent {
		t.Fatalf("/navigate back: want 204, got %d", code)
	}
	if s.rt.CurrentScene() != "" {
		t.Fatalf("after back: scene=%q, want entry", s.rt.CurrentScene())
	}
}

// TestEventCarriesRouteHeader: a navigation dispatched through /event returns the
// current deep-link path in the X-Qorm-Route header, so the client can pushState.
func TestEventCarriesRouteHeader(t *testing.T) {
	s := navServer(t)
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()
	tok := pageEventToken(t, ts.URL)

	// The home scene's list row (handler 0) opens a profile with route params.
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/event", strings.NewReader(`{"h":0,"inputs":{}}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Qorm-Token", tok)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("post /event: %v", err)
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)
	route := resp.Header.Get("X-Qorm-Route")
	if !strings.HasPrefix(route, "/?") || !strings.Contains(route, "scene=profile") {
		t.Fatalf("X-Qorm-Route after navigate = %q, want a /?...scene=profile path", route)
	}
	if s.rt.CurrentScene() != "profile" {
		t.Fatalf("event should have navigated to profile, scene=%q", s.rt.CurrentScene())
	}
}
