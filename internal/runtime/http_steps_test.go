package runtime

import (
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/qorm/qorm/internal/model"
)

// These tests exercise the http.* dispatch steps against a loopback
// httptest.Server (no external network), plus deterministic transport failures
// via a swapped-in httpClient.

// echoHandler records the request it saw and replies with a canned response.
type echoHandler struct {
	method      string
	path        string
	contentType string
	auth        string
	body        string

	status     int
	respHeader http.Header
	respBody   string
}

func (h *echoHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.method = r.Method
	h.path = r.URL.Path
	h.contentType = r.Header.Get("Content-Type")
	h.auth = r.Header.Get("Authorization")
	b, _ := io.ReadAll(r.Body)
	h.body = string(b)
	for k, vs := range h.respHeader {
		for _, v := range vs {
			w.Header().Set(k, v)
		}
	}
	if h.status != 0 {
		w.WriteHeader(h.status)
	}
	w.Write([]byte(h.respBody))
}

func dispatchHTTP(t *testing.T, state map[string]any, step model.Step, args map[string]any) *Runtime {
	t.Helper()
	app := &model.App{Actions: map[string]*model.Action{"call": {ID: "call", Steps: []model.Step{step}}}}
	rt := &Runtime{App: app, State: state, RouteParams: map[string]any{}}
	rt.Dispatch("call", args)
	return rt
}

func TestHTTPGetJSON(t *testing.T) {
	h := &echoHandler{respBody: `{"ok":true,"count":42}`}
	srv := httptest.NewServer(h)
	defer srv.Close()

	rt := dispatchHTTP(t, map[string]any{}, model.Step{
		Type: "http.get", URL: srv.URL + "/data", Result: "resp", Error: "err",
	}, nil)

	if h.method != "GET" {
		t.Errorf("http.get should send GET, sent %q", h.method)
	}
	resp, ok := rt.State["resp"].(map[string]any)
	if !ok {
		t.Fatalf("JSON response should decode into a map, got %T (%v)", rt.State["resp"], rt.State["resp"])
	}
	if resp["ok"] != true || resp["count"] != float64(42) {
		t.Errorf("decoded response wrong: %#v", resp)
	}
	// A successful call clears any stale error.
	if rt.State["err"] != "" {
		t.Errorf("error path should be cleared on success, got %v", rt.State["err"])
	}
}

func TestHTTPMethodDefaultsAndOverride(t *testing.T) {
	cases := []struct {
		stepType   string
		methodOver string
		wantMethod string
	}{
		{"http.post", "", "POST"},
		{"http.put", "", "PUT"},
		{"http.delete", "", "DELETE"},
		{"http.request", "PATCH", "PATCH"},
		// An explicit Method on a typed step wins over the type default.
		{"http.get", "POST", "POST"},
		{"http.request", "", "GET"}, // http.request with no Method falls back to GET
	}
	for _, c := range cases {
		h := &echoHandler{respBody: `{}`}
		srv := httptest.NewServer(h)
		dispatchHTTP(t, map[string]any{}, model.Step{
			Type: c.stepType, Method: c.methodOver, URL: srv.URL, Result: "r",
		}, nil)
		srv.Close()
		if h.method != c.wantMethod {
			t.Errorf("%s (method %q): server saw %q, want %q", c.stepType, c.methodOver, h.method, c.wantMethod)
		}
	}
}

func TestHTTPBodyAndHeaders(t *testing.T) {
	h := &echoHandler{respBody: `{"saved":true}`}
	srv := httptest.NewServer(h)
	defer srv.Close()

	// Body bindings are evaluated; a JSON Content-Type is added when unset.
	rt := dispatchHTTP(t, map[string]any{"name": "Ada"}, model.Step{
		Type: "http.post", URL: srv.URL + "/save",
		Body:    `{"name":"{{ name }}"}`,
		Headers: map[string]string{"Authorization": "Bearer {{ token }}"},
		Result:  "resp",
	}, map[string]any{"token": "sekrit"})

	if h.body != `{"name":"Ada"}` {
		t.Errorf("body not evaluated/sent: %q", h.body)
	}
	if h.contentType != "application/json" {
		t.Errorf("default Content-Type should be application/json, got %q", h.contentType)
	}
	if h.auth != "Bearer sekrit" {
		t.Errorf("header binding not evaluated: %q", h.auth)
	}
	if rt.State["resp"].(map[string]any)["saved"] != true {
		t.Errorf("response not stored: %v", rt.State["resp"])
	}

	// An explicit Content-Type header is respected (not overwritten).
	h2 := &echoHandler{respBody: `{}`}
	srv2 := httptest.NewServer(h2)
	defer srv2.Close()
	dispatchHTTP(t, map[string]any{}, model.Step{
		Type: "http.post", URL: srv2.URL, Body: "plain text",
		Headers: map[string]string{"Content-Type": "text/plain"},
	}, nil)
	if h2.contentType != "text/plain" {
		t.Errorf("explicit Content-Type should win, got %q", h2.contentType)
	}
	if h2.body != "plain text" {
		t.Errorf("body = %q", h2.body)
	}
}

func TestHTTPURLBindingAndResultFallbackToPath(t *testing.T) {
	h := &echoHandler{respBody: `[1,2,3]`}
	srv := httptest.NewServer(h)
	defer srv.Close()

	// The URL itself may carry bindings; Result defaults to Path when unset.
	rt := dispatchHTTP(t, map[string]any{"id": "77"}, model.Step{
		Type: "http.get", URL: srv.URL + "/items/{{ id }}", Path: "items",
	}, nil)
	if h.path != "/items/77" {
		t.Errorf("URL binding not applied: path = %q", h.path)
	}
	arr, ok := rt.State["items"].([]any)
	if !ok || len(arr) != 3 {
		t.Errorf("Result should fall back to Path (JSON array): %T %v", rt.State["items"], rt.State["items"])
	}
}

func TestHTTPNonJSONBodyStoredRaw(t *testing.T) {
	h := &echoHandler{respBody: "hello world"}
	srv := httptest.NewServer(h)
	defer srv.Close()

	rt := dispatchHTTP(t, map[string]any{}, model.Step{
		Type: "http.get", URL: srv.URL, Result: "text",
	}, nil)
	if rt.State["text"] != "hello world" {
		t.Errorf("non-JSON body should be stored as raw text, got %v", rt.State["text"])
	}
}

func TestHTTPErrorStatus(t *testing.T) {
	h := &echoHandler{status: http.StatusInternalServerError, respBody: `{"error":"boom"}`}
	srv := httptest.NewServer(h)
	defer srv.Close()

	// A pre-existing error value proves a failure overwrites it with the status.
	rt := dispatchHTTP(t, map[string]any{"err": "stale"}, model.Step{
		Type: "http.get", URL: srv.URL, Result: "resp", Error: "err",
	}, nil)
	if rt.State["err"] != "500 Internal Server Error" {
		t.Errorf("error path should hold the status text, got %v", rt.State["err"])
	}
	// On failure the body is NOT stored at the result path: only the Error path
	// is written, honoring the "On success" contract (the body is discarded).
	if _, exists := rt.State["resp"]; exists {
		t.Errorf("error body must not be stored at the result path, got %v", rt.State["resp"])
	}

	// Without an Error path configured, a failure is silent (no panic, no store).
	rt = dispatchHTTP(t, map[string]any{}, model.Step{Type: "http.get", URL: srv.URL}, nil)
	if len(rt.State) != 0 {
		t.Errorf("no Error/Result path: state should stay empty, got %v", rt.State)
	}
}

func TestHTTPNotFoundStatus(t *testing.T) {
	h := &echoHandler{status: http.StatusNotFound, respBody: "missing"}
	srv := httptest.NewServer(h)
	defer srv.Close()

	rt := dispatchHTTP(t, map[string]any{}, model.Step{
		Type: "http.get", URL: srv.URL, Error: "err",
	}, nil)
	if rt.State["err"] != "404 Not Found" {
		t.Errorf("404 should record the status, got %v", rt.State["err"])
	}
}

// TestHTTPErrorDoesNotStoreBody is a regression test: a non-2xx response must
// not write the body to the result path (any previous value is preserved); the
// status text is recorded on the error path instead.
func TestHTTPErrorDoesNotStoreBody(t *testing.T) {
	for _, status := range []int{
		http.StatusBadRequest, http.StatusNotFound, http.StatusInternalServerError,
	} {
		h := &echoHandler{status: status, respBody: `{"error":"boom"}`}
		srv := httptest.NewServer(h)
		// A pre-existing result value proves the failure leaves it untouched.
		rt := dispatchHTTP(t, map[string]any{"resp": "kept", "err": "stale"}, model.Step{
			Type: "http.get", URL: srv.URL, Result: "resp", Error: "err",
		}, nil)
		srv.Close()
		if rt.State["resp"] != "kept" {
			t.Errorf("status %d: result path should be preserved on error, got %v", status, rt.State["resp"])
		}
		if e, _ := rt.State["err"].(string); e == "" || e == "stale" {
			t.Errorf("status %d: error path should hold the status text, got %v", status, rt.State["err"])
		}
	}
}

func TestHTTPRequestBuildFailure(t *testing.T) {
	// A URL with a space fails http.NewRequest before any network I/O.
	rt := dispatchHTTP(t, map[string]any{}, model.Step{
		Type: "http.get", URL: "http://bad url with spaces", Error: "err",
	}, nil)
	err, _ := rt.State["err"].(string)
	if err == "" {
		t.Errorf("request build failure should populate the error path, got %v", rt.State["err"])
	}
}

func TestHTTPTransportFailure(t *testing.T) {
	// Swap the package client for one whose transport always fails — fully
	// deterministic, no network, no timing.
	orig := httpClient
	httpClient = &http.Client{Transport: failTransport{}}
	defer func() { httpClient = orig }()

	rt := dispatchHTTP(t, map[string]any{}, model.Step{
		Type: "http.get", URL: "http://127.0.0.1/anything", Error: "err",
	}, nil)
	err, _ := rt.State["err"].(string)
	if !strings.Contains(err, "synthetic transport failure") {
		t.Errorf("transport failure should populate the error path, got %v", rt.State["err"])
	}
}

type failTransport struct{}

func (failTransport) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, errors.New("synthetic transport failure")
}

// closedPortServer spins up a listener, closes it, and returns a now-refused
// 127.0.0.1 address — a real loopback dial failure without timing dependence.
func TestHTTPConnectionRefused(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skipf("cannot bind loopback: %v", err)
	}
	addr := ln.Addr().String()
	ln.Close() // the port is now (almost certainly) refused

	orig := httpClient
	// A refused dial fails fast; the 5s cap bounds the pathological case where
	// another process re-binds the freed port with a non-responding listener.
	httpClient = &http.Client{Timeout: 5 * time.Second}
	defer func() { httpClient = orig }()

	rt := dispatchHTTP(t, map[string]any{}, model.Step{
		Type: "http.get", URL: "http://" + addr + "/x", Error: "err",
	}, nil)
	if e, _ := rt.State["err"].(string); e == "" {
		t.Errorf("connection refused should populate the error path, got %v", rt.State["err"])
	}
}

func TestHTTPNoResultPathNoStore(t *testing.T) {
	h := &echoHandler{respBody: `{"a":1}`}
	srv := httptest.NewServer(h)
	defer srv.Close()

	// Success with neither Result nor Path stores nothing...
	rt := dispatchHTTP(t, map[string]any{"err": "keepme"}, model.Step{
		Type: "http.get", URL: srv.URL, Error: "err",
	}, nil)
	if len(rt.State) != 1 {
		t.Errorf("no Result/Path: nothing should be stored, state = %v", rt.State)
	}
	// ...but a configured Error path is still cleared on success.
	if rt.State["err"] != "" {
		t.Errorf("success should clear the error path, got %v", rt.State["err"])
	}
}
