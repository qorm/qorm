package server

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/qorm/qorm/internal/keys"
)

// TestRequireTokenGateLoopbackUnchanged pins the loopback default: the two-
// token gate (set from `qorm run --lan`) is OFF, so every endpoint behaves
// byte-for-byte as before — no auth ever required, including the diagnostics
// reads, and /mcp still refuses the human page token (identity separation
// intact). The admin token exists on loopback too but gates nothing.
func TestRequireTokenGateLoopbackUnchanged(t *testing.T) {
	s := counterServer(t)
	s.SetWindowControl(func(id string, x, y, w, h int) {}, func(id, op string) {}, nil, nil)
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()

	// No token anywhere: browser + admin endpoints all pass the (inert) gate
	// to their own logic.
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
	// Diagnostics reads stay tokenless on loopback.
	if code, _ := doJSON(t, http.MethodGet, ts.URL+"/dev/state", "", "", ""); code != http.StatusOK {
		t.Errorf("loopback GET /dev/state without token = %d, want 200", code)
	}
	if code, _ := doJSON(t, http.MethodGet, ts.URL+"/log?since=0", "", "", ""); code != http.StatusOK {
		t.Errorf("loopback GET /log without token = %d, want 200", code)
	}
	if code, _ := doJSON(t, http.MethodGet, ts.URL+"/presence", "", "", ""); code != http.StatusOK {
		t.Errorf("loopback GET /presence without token = %d, want 200", code)
	}
	// The symmetric-isolation refusal is untouched on loopback: the human
	// page token still cannot route through the agent channel, and the admin
	// token — printed only on --lan — is simply another non-page header.
	if code, _ := doJSON(t, http.MethodPost, ts.URL+"/mcp", s.eventToken, "", `{"jsonrpc":"2.0","id":1,"method":"ping"}`); code != http.StatusForbidden {
		t.Errorf("loopback /mcp WITH page token = %d, want 403 (human must use /event)", code)
	}
	if code, body := doJSON(t, http.MethodPost, ts.URL+"/mcp", s.AdminToken(), "", `{"jsonrpc":"2.0","id":1,"method":"ping"}`); code != http.StatusOK || !strings.Contains(body, `"result"`) {
		t.Errorf("loopback /mcp with admin token = %d %q, want 200 jsonrpc result", code, body)
	}
}

// TestRequireTokenGateLANTwoToken is the --lan gate matrix under the two-token
// model. The page token (public to anyone who GETs the served page) opens ONLY
// browser endpoints; the admin token (printed at startup, never embedded in
// any served response) is the sole key to /mcp, /update, /rollback, /window
// and the diagnostics reads.
func TestRequireTokenGateLANTwoToken(t *testing.T) {
	s := counterServer(t)
	s.SetRequireToken(true)
	s.SetWindowControl(func(id string, x, y, w, h int) {}, func(id, op string) {}, nil, nil)
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()

	// Tokenless: every gated endpoint and every diagnostics read is refused.
	for _, ep := range []struct{ method, path, body string }{
		{http.MethodPost, "/measure", `[{"id":"a"}]`},
		{http.MethodGet, "/measure", ""},
		{http.MethodPost, "/window", `{"op":"move","x":1,"y":2,"w":3,"h":4}`},
		{http.MethodPost, "/update", `{"source":"x"}`},
		{http.MethodPost, "/rollback", ""},
		{http.MethodPost, "/mcp", `{"jsonrpc":"2.0","method":"notifications/initialized"}`},
		{http.MethodGet, "/dev/state", ""},
		{http.MethodGet, "/log?since=0", ""},
		{http.MethodGet, "/presence", ""},
	} {
		if code, body := doJSON(t, ep.method, ts.URL+ep.path, "", "", ep.body); code != http.StatusUnauthorized {
			t.Errorf("gated %s %s without token = %d %q, want 401", ep.method, ep.path, code, body)
		}
	}

	// Wrong tokens are as bad as none.
	if code, _ := doJSON(t, http.MethodPost, ts.URL+"/measure", "deadbeef", "", `[]`); code != http.StatusUnauthorized {
		t.Errorf("gated /measure with a wrong token = %d, want 401", code)
	}
	if code, _ := doJSON(t, http.MethodPost, ts.URL+"/mcp", "deadbeef", "", `{"jsonrpc":"2.0","id":1,"method":"ping"}`); code != http.StatusUnauthorized {
		t.Errorf("gated /mcp with a wrong token = %d, want 401", code)
	}
	if code, _ := doJSON(t, http.MethodGet, ts.URL+"/dev/state", "deadbeef", "", ""); code != http.StatusUnauthorized {
		t.Errorf("gated /dev/state with a wrong token = %d, want 401", code)
	}

	// KEY separation: the PAGE token (harvestable from GET /) opens NO admin
	// surface. /mcp, /window, the OTA pair and the diagnostics reads all
	// refuse it with 401 — even though it would be the correct token on a
	// loopback bind.
	for _, ep := range []struct{ method, path, body string }{
		{http.MethodPost, "/window", `{"op":"move","x":1,"y":2,"w":3,"h":4}`},
		{http.MethodPost, "/update", `{"source":"x"}`},
		{http.MethodPost, "/rollback", ""},
		{http.MethodPost, "/mcp", `{"jsonrpc":"2.0","method":"notifications/initialized"}`},
		{http.MethodGet, "/dev/state", ""},
		{http.MethodGet, "/log?since=0", ""},
		{http.MethodGet, "/presence", ""},
	} {
		if code, _ := doJSON(t, ep.method, ts.URL+ep.path, s.eventToken, "", ep.body); code != http.StatusUnauthorized {
			t.Errorf("gated %s %s with PAGE token = %d, want 401 (admin-only; page token is public)", ep.method, ep.path, code)
		}
	}

	// The page token still opens the browser endpoints, so the real page
	// keeps working: layout self-report, events, presence, viewport, app log.
	if code, _ := doJSON(t, http.MethodPost, ts.URL+"/measure", s.eventToken, "", `[{"id":"a"}]`); code != http.StatusNoContent {
		t.Errorf("gated POST /measure with page token = %d, want 204", code)
	}
	if code, body := doJSON(t, http.MethodGet, ts.URL+"/measure", s.eventToken, "", ""); code != http.StatusOK || body != `[{"id":"a"}]` {
		t.Errorf("gated GET /measure with page token = %d %q, want 200 with stored payload", code, body)
	}
	if code, _ := doJSON(t, http.MethodPost, ts.URL+"/event", s.eventToken, "", `{"h":1,"inputs":{}}`); code != http.StatusOK {
		t.Errorf("gated /event with page token = %d, want 200", code)
	}
	if code, _ := doJSON(t, http.MethodPost, ts.URL+"/presence", s.eventToken, "", `{"element":"#email"}`); code != http.StatusNoContent {
		t.Errorf("gated POST /presence with page token = %d, want 204", code)
	}
	if code, _ := doJSON(t, http.MethodPost, ts.URL+"/log", s.eventToken, "", `{"source":"app","detail":"rendered"}`); code != http.StatusNoContent {
		t.Errorf("gated POST /log with page token = %d, want 204", code)
	}
	if code, _ := doJSON(t, http.MethodPost, ts.URL+"/dev/state", s.eventToken, "", `{"count":1}`); code != http.StatusOK {
		t.Errorf("gated POST /dev/state with page token = %d, want 200 (DevTool write)", code)
	}
	if code, _ := doJSON(t, http.MethodGet, ts.URL+"/poll?rev=0", "", "", ""); code != http.StatusOK {
		t.Errorf("gated GET /poll without token = %d, want 200 (browser polling must stay open)", code)
	}

	// The ADMIN token opens the admin surfaces, which then run their normal
	// logic (evidencing the gate passed rather than the endpoint breaking).
	if code, body := doJSON(t, http.MethodPost, ts.URL+"/window", s.AdminToken(), "", `{"op":"move","x":1,"y":2,"w":3,"h":4}`); code != http.StatusOK || body != "ok" {
		t.Errorf("gated /window with admin token = %d %q, want 200 ok", code, body)
	}
	if code, _ := doJSON(t, http.MethodPost, ts.URL+"/update", s.AdminToken(), "", `{"source":"x"}`); code != http.StatusBadRequest {
		t.Errorf("gated /update with admin token = %d, want 400 (no bundle on a plain server)", code)
	}
	if code, _ := doJSON(t, http.MethodPost, ts.URL+"/rollback", s.AdminToken(), "", ""); code != http.StatusConflict {
		t.Errorf("gated /rollback with admin token = %d, want 409 (no prev)", code)
	}
	if code, body := doJSON(t, http.MethodPost, ts.URL+"/mcp", s.AdminToken(), "", `{"jsonrpc":"2.0","id":9,"method":"ping"}`); code != http.StatusOK || !strings.Contains(body, `"result"`) {
		t.Errorf("gated /mcp with admin token = %d %q, want 200 jsonrpc result", code, body)
	}
	if code, _ := doJSON(t, http.MethodPost, ts.URL+"/mcp", s.AdminToken(), "", `{"jsonrpc":"2.0","method":"notifications/initialized"}`); code != http.StatusNoContent {
		t.Errorf("gated /mcp notification with admin token = %d, want 204", code)
	}
	// Diagnostics reads open to the admin token.
	for _, ep := range []struct{ method, path, body string }{
		{http.MethodGet, "/dev/state", ""},
		{http.MethodGet, "/log?since=0", ""},
		{http.MethodGet, "/presence", ""},
	} {
		if code, _ := doJSON(t, ep.method, ts.URL+ep.path, s.AdminToken(), "", ep.body); code != http.StatusOK {
			t.Errorf("gated %s %s with admin token = %d, want 200", ep.method, ep.path, code)
		}
	}

	// Method-bypass regression: the read path serves EVERY non-POST request,
	// so the gate must refuse every verb except the browser's POST writes —
	// a GET-only gate leaked the full state and activity log through
	// OPTIONS/PUT/PATCH/DELETE/HEAD with zero credentials (adversarially
	// reproduced with curl against the built binary).
	for _, method := range []string{http.MethodOptions, http.MethodPut, http.MethodPatch, http.MethodDelete, http.MethodHead, http.MethodTrace} {
		for _, path := range []string{"/dev/state", "/log?since=0", "/presence"} {
			if code, _ := doJSON(t, method, ts.URL+path, "", "", ""); code != http.StatusUnauthorized {
				t.Errorf("gated %s %s without token = %d, want 401 (read gate covers every non-POST verb)", method, path, code)
			}
			if code, _ := doJSON(t, method, ts.URL+path, s.eventToken, "", ""); code != http.StatusUnauthorized {
				t.Errorf("gated %s %s with PAGE token = %d, want 401 (admin-only)", method, path, code)
			}
		}
	}
	// And the admin token opens the reads under any of those verbs too.
	if code, _ := doJSON(t, http.MethodOptions, ts.URL+"/dev/state", s.AdminToken(), "", ""); code != http.StatusOK {
		t.Errorf("gated OPTIONS /dev/state with admin token = %d, want 200", code)
	}
}

// TestRequireTokenGateLANOTA drives a full update + rollback through the gate
// on an OTA-capable server: without the token the endpoint is refused before
// even parsing the request; with the ADMIN token the normal 200 update / 200
// rollback responses come back. The page token must NOT be able to update or
// roll back the app.
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
	if code, resp := doJSON(t, http.MethodPost, ts.URL+"/update", s.eventToken, "", body); code != http.StatusUnauthorized {
		t.Fatalf("gated /update with PAGE token = %d %q, want 401 (public token must not update)", code, resp)
	}
	if code, resp := doJSON(t, http.MethodPost, ts.URL+"/update", s.AdminToken(), "", body); code != http.StatusOK || !strings.Contains(resp, "updated 1.0.0 -> 2.0.0") {
		t.Fatalf("gated /update with admin token = %d %q, want 200 updated", code, resp)
	}
	if code, _ := doJSON(t, http.MethodPost, ts.URL+"/rollback", s.eventToken, "", ""); code != http.StatusUnauthorized {
		t.Fatalf("gated /rollback with PAGE token = %d, want 401", code)
	}
	if code, resp := doJSON(t, http.MethodPost, ts.URL+"/rollback", s.AdminToken(), "", ""); code != http.StatusOK || !strings.Contains(resp, "rolled back 2.0.0 -> 1.0.0") {
		t.Fatalf("gated /rollback with admin token = %d %q, want 200 rolled back", code, resp)
	}
}

// TestServedPagesHideAdminToken verifies the one-way leak in the two-token
// model: the index page embeds the page token (the browser needs it), but the
// admin token never appears in ANY served response — the GET / harvest that
// defeated the round-3 gate produces the page token and nothing else.
func TestServedPagesHideAdminToken(t *testing.T) {
	s := counterServer(t)
	s.SetRequireToken(true) // pages stay public even in the most locked-down mode
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()

	get := func(path string) string {
		t.Helper()
		resp, err := http.Get(ts.URL + path)
		if err != nil {
			t.Fatalf("get %s: %v", path, err)
		}
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return string(b)
	}
	for _, path := range []string{"/", "/console", "/logwindow"} {
		if page := get(path); strings.Contains(page, s.AdminToken()) {
			t.Errorf("%s page must NOT contain the admin token", path)
		}
	}

	// The index page is the one that carries the page token to the browser.
	if page := get("/"); !strings.Contains(page, s.eventToken) {
		t.Error("index page should embed the page token (__tok) for the in-page JS")
	}

	// Sanity: harvesting the page token as a LAN peer would really yields the
	// page token (pageEventToken parses it out of the served HTML) and that
	// token is genuinely different from the admin secret.
	if tok := pageEventToken(t, ts.URL); tok != s.eventToken {
		t.Errorf("harvested page token %q != server page token %q", tok, s.eventToken)
	}
	if s.AdminToken() == s.eventToken {
		t.Fatal("admin token must differ from the page token")
	}
}

// TestRequireTokenGateKeepsCrossOriginGuard: the CSRF layer stays in front of
// the token gates, so a cross-origin browser request is rejected with 403 even
// when it holds the ADMIN token (no new bypass), and a cross-origin request
// without the token still reports cross-origin, not the token gate.
func TestRequireTokenGateKeepsCrossOriginGuard(t *testing.T) {
	s := counterServer(t)
	s.SetRequireToken(true)
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()

	if code, body := doJSON(t, http.MethodPost, ts.URL+"/window", s.eventToken, "https://evil.example.com", `{}`); code != http.StatusForbidden || !strings.Contains(body, "cross-origin") {
		t.Errorf("cross-origin with page token = %d %q, want 403 cross-origin rejection", code, body)
	}
	if code, body := doJSON(t, http.MethodPost, ts.URL+"/window", s.AdminToken(), "https://evil.example.com", `{}`); code != http.StatusForbidden || !strings.Contains(body, "cross-origin") {
		t.Errorf("cross-origin with admin token = %d %q, want 403 cross-origin rejection (blockCrossOrigin must stay outermost)", code, body)
	}
	if code, body := doJSON(t, http.MethodPost, ts.URL+"/window", "", "https://evil.example.com", `{}`); code != http.StatusForbidden || !strings.Contains(body, "cross-origin") {
		t.Errorf("cross-origin without token = %d %q, want 403 cross-origin rejection", code, body)
	}
}
