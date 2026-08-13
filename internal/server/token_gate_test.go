package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/qorm/qorm/internal/keys"
)

// TestRequireTokenGateLoopbackUnchanged pins the loopback default: the page
// token gate (set from `qorm run --lan`) is OFF, so the five LAN-facing
// endpoints behave byte-for-byte as before — no auth ever required, and /mcp
// still refuses the human token (identity separation intact).
func TestRequireTokenGateLoopbackUnchanged(t *testing.T) {
	s := counterServer(t)
	s.SetWindowControl(func(id string, x, y, w, h int) {}, func(id, op string) {}, nil, nil)
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()

	// No token anywhere: /measure write + read, /window (native host set),
	// /mcp /update /rollback all pass the (inert) gate to their own logic.
	if code, _ := doJSON(t, http.MethodPost, ts.URL+"/measure", "", "", `[{"id":"a"}]`); code != http.StatusNoContent {
		t.Errorf("loopback POST /measure without token = %d, want 204 (gate must be inert)", code)
	}
	if code, _ := doJSON(t, http.MethodGet, ts.URL+"/measure", "", "", ""); code != http.StatusOK {
		t.Errorf("loopback GET /measure without token = %d, want 200", code)
	}
	if code, body := doJSON(t, http.MethodPost, ts.URL+"/window", "", "", `{"op":"move","x":1,"y":2,"w":3,"h":4}`); code != http.StatusOK || body != "ok" {
		t.Errorf("loopback /window without token = %d %q, want 200 ok", code, body)
	}
	if code, _ := doJSON(t, http.MethodPost, ts.URL+"/update", "", "", `{"source":"x"}`); code != http.StatusBadRequest {
		t.Errorf("loopback /update without token = %d, want 400 (no bundle)", code)
	}
	if code, _ := doJSON(t, http.MethodPost, ts.URL+"/rollback", "", "", ""); code != http.StatusConflict {
		t.Errorf("loopback /rollback without token = %d, want 409 (no prev)", code)
	}
	if code, _ := doJSON(t, http.MethodPost, ts.URL+"/mcp", "", "", `{"jsonrpc":"2.0","method":"notifications/initialized"}`); code != http.StatusNoContent {
		t.Errorf("loopback /mcp without token = %d, want 204", code)
	}
	// The symmetric-isolation refusal is untouched on loopback: the human
	// token still cannot route through the agent channel.
	if code, _ := doJSON(t, http.MethodPost, ts.URL+"/mcp", s.eventToken, "", `{"jsonrpc":"2.0","id":1,"method":"ping"}`); code != http.StatusForbidden {
		t.Errorf("loopback /mcp WITH token = %d, want 403 (human must use /event)", code)
	}
}

// TestRequireTokenGateLAN covers the non-loopback bind: with SetRequireToken
// on, every LAN-facing endpoint demands the page token (401 without, its
// normal response with) — and /mcp accepts the token, because it is now the
// session's shared secret.
func TestRequireTokenGateLAN(t *testing.T) {
	s := counterServer(t)
	s.SetRequireToken(true)
	s.SetWindowControl(func(id string, x, y, w, h int) {}, func(id, op string) {}, nil, nil)
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()

	for _, ep := range []struct{ method, path, body string }{
		{http.MethodPost, "/measure", `[{"id":"a"}]`},
		{http.MethodGet, "/measure", ""},
		{http.MethodPost, "/window", `{"op":"move","x":1,"y":2,"w":3,"h":4}`},
		{http.MethodPost, "/update", `{"source":"x"}`},
		{http.MethodPost, "/rollback", ""},
		{http.MethodPost, "/mcp", `{"jsonrpc":"2.0","method":"notifications/initialized"}`},
	} {
		if code, body := doJSON(t, ep.method, ts.URL+ep.path, "", "", ep.body); code != http.StatusUnauthorized {
			t.Errorf("gated %s %s without token = %d %q, want 401", ep.method, ep.path, code, body)
		}
	}

	// Wrong token is as bad as none.
	if code, _ := doJSON(t, http.MethodPost, ts.URL+"/measure", "deadbeef", "", `[]`); code != http.StatusUnauthorized {
		t.Errorf("gated /measure with a wrong token = %d, want 401", code)
	}

	// With the page token each endpoint proceeds to its normal behavior.
	if code, _ := doJSON(t, http.MethodPost, ts.URL+"/measure", s.eventToken, "", `[{"id":"a"}]`); code != http.StatusNoContent {
		t.Errorf("gated POST /measure with token = %d, want 204", code)
	}
	if code, body := doJSON(t, http.MethodGet, ts.URL+"/measure", s.eventToken, "", ""); code != http.StatusOK || body != `[{"id":"a"}]` {
		t.Errorf("gated GET /measure with token = %d %q, want 200 with stored payload", code, body)
	}
	if code, body := doJSON(t, http.MethodPost, ts.URL+"/window", s.eventToken, "", `{"op":"move","x":1,"y":2,"w":3,"h":4}`); code != http.StatusOK || body != "ok" {
		t.Errorf("gated /window with token = %d %q, want 200 ok", code, body)
	}
	if code, _ := doJSON(t, http.MethodPost, ts.URL+"/update", s.eventToken, "", `{"source":"x"}`); code != http.StatusBadRequest {
		t.Errorf("gated /update with token = %d, want 400 (no bundle on a plain server)", code)
	}
	if code, _ := doJSON(t, http.MethodPost, ts.URL+"/rollback", s.eventToken, "", ""); code != http.StatusConflict {
		t.Errorf("gated /rollback with token = %d, want 409 (no prev)", code)
	}
	if code, body := doJSON(t, http.MethodPost, ts.URL+"/mcp", s.eventToken, "", `{"jsonrpc":"2.0","id":9,"method":"ping"}`); code != http.StatusOK || !strings.Contains(body, `"result"`) {
		t.Errorf("gated /mcp with token = %d %q, want 200 jsonrpc result", code, body)
	}
	if code, _ := doJSON(t, http.MethodPost, ts.URL+"/mcp", s.eventToken, "", `{"jsonrpc":"2.0","method":"notifications/initialized"}`); code != http.StatusNoContent {
		t.Errorf("gated /mcp notification with token = %d, want 204", code)
	}
}

// TestRequireTokenGateLANOTA drives a full update + rollback through the gate
// on an OTA-capable server: without the token the endpoint is refused before
// even parsing the request; with it the normal 200 update / 200 rollback
// responses come back.
func TestRequireTokenGateLANOTA(t *testing.T) {
	pub, priv, _ := keys.Generate()
	s, err := NewBundle(signedBundle(t, "1.0.0", priv, pub), pub, nil)
	if err != nil {
		t.Fatalf("NewBundle: %v", err)
	}
	s.SetRequireToken(true)
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()

	if code, _ := doJSON(t, http.MethodPost, ts.URL+"/update", "", "", ``); code != http.StatusUnauthorized {
		t.Fatalf("gated /update without token = %d, want 401", code)
	}
	body := `{"source":"` + writeBundle(t, signedBundle(t, "2.0.0", priv, pub)) + `"}`
	if code, resp := doJSON(t, http.MethodPost, ts.URL+"/update", s.eventToken, "", body); code != http.StatusOK || !strings.Contains(resp, "updated 1.0.0 -> 2.0.0") {
		t.Fatalf("gated /update with token = %d %q, want 200 updated", code, resp)
	}
	if code, resp := doJSON(t, http.MethodPost, ts.URL+"/rollback", s.eventToken, "", ""); code != http.StatusOK || !strings.Contains(resp, "rolled back 2.0.0 -> 1.0.0") {
		t.Fatalf("gated /rollback with token = %d %q, want 200 rolled back", code, resp)
	}
}

// TestRequireTokenGateKeepsCrossOriginGuard: the CSRF layer stays in front of
// the token gate, so a cross-origin browser request is rejected with 403 even
// when it holds the token (no new bypass), and a cross-origin request without
// the token still reports cross-origin, not the token gate.
func TestRequireTokenGateKeepsCrossOriginGuard(t *testing.T) {
	s := counterServer(t)
	s.SetRequireToken(true)
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()

	if code, body := doJSON(t, http.MethodPost, ts.URL+"/window", s.eventToken, "https://evil.example.com", `{}`); code != http.StatusForbidden || !strings.Contains(body, "cross-origin") {
		t.Errorf("cross-origin with token = %d %q, want 403 cross-origin rejection", code, body)
	}
}
