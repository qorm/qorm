package server

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/qorm/qorm/internal/bundle"
	"github.com/qorm/qorm/internal/model"
	qrt "github.com/qorm/qorm/internal/runtime"
)

// TestClientHost maps a connection's remote address to the identity shown in
// the activity log: loopback renders as "local", a real device as its IP.
func TestClientHost(t *testing.T) {
	cases := map[string]string{
		"127.0.0.1:49152":  "local",
		"[::1]:49152":      "local",
		"192.168.0.9:1234": "192.168.0.9",
		"10.0.0.1:80":      "10.0.0.1",
		"no-port-here":     "no-port-here",
	}
	for addr, want := range cases {
		if got := clientHost(&http.Request{RemoteAddr: addr}); got != want {
			t.Errorf("clientHost(%q) = %q, want %q", addr, got, want)
		}
	}
}

// TestVersionOr: a bundle without a manifest version reads as "unversioned".
func TestVersionOr(t *testing.T) {
	b, err := bundle.Build(counterDir())
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if b.Version() != "" {
		t.Fatalf("the counter example carries no version, got %q", b.Version())
	}
	if got := versionOr(b); got != "unversioned" {
		t.Errorf("versionOr(unversioned) = %q", got)
	}
	if err := b.SetVersion("3.1.4"); err != nil {
		t.Fatalf("SetVersion: %v", err)
	}
	if got := versionOr(b); got != "3.1.4" {
		t.Errorf("versionOr(versioned) = %q, want 3.1.4", got)
	}
}

// TestReplaceOnce: exactly one replacement, and a loud error when the anchor
// is missing (so Page-template drift fails the build, not the package).
func TestReplaceOnce(t *testing.T) {
	out, err := replaceOnce("aXbXc", "X", "-")
	if err != nil || out != "a-bXc" {
		t.Fatalf("replaceOnce = %q, %v — only the first occurrence should change", out, err)
	}
	if _, err := replaceOnce("abc", "zzz", "-"); err == nil || !strings.Contains(err.Error(), "page anchor not found") {
		t.Fatalf("a missing anchor must error, got %v", err)
	}
}

// TestOfflineHTMLBrandingAndNativeJS: the packaged page carries the generator
// note when branding is on (and omits it when off), and the app's native/web.js
// travels with the package.
func TestOfflineHTMLBrandingAndNativeJS(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "native"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "native", "web.js"), []byte("window.__nativeHook=1;"), 0o644); err != nil {
		t.Fatal(err)
	}

	app := offlineTestApp()
	app.Branding = true
	app.BaseDir = dir
	html, err := OfflineHTML(qrt.New(app), `{"entry":"main"}`, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(html, `<meta name="generator" content="Made with QORM`) {
		t.Error("branding=true should add the generator meta")
	}
	if !strings.Contains(html, "<script>window.__nativeHook=1;</script>") {
		t.Error("native/web.js should travel with the offline package")
	}

	plain, err := OfflineHTML(qrt.New(offlineTestApp()), `{"entry":"main"}`, nil)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(plain, "Made with QORM") {
		t.Error("branding=false must omit the generator meta")
	}
}

// TestAppMetadataAccessors: the desktop-host accessors return JSON (with
// documented empties) for the manifest's window / shortcuts / menu / tray.
func TestAppMetadataAccessors(t *testing.T) {
	s := New(qrt.New(offlineTestApp()))

	if got := s.AppShortcutsJSON(); got != "[]" {
		t.Errorf("empty AppShortcutsJSON = %q, want []", got)
	}
	if got := s.AppMenuJSON(); got != "" {
		t.Errorf("empty AppMenuJSON = %q, want \"\"", got)
	}
	if got := s.AppTrayJSON(); got != "" {
		t.Errorf("empty AppTrayJSON = %q, want \"\"", got)
	}

	s.mu.Lock()
	s.rt.App.Window = model.Window{Width: 320, Height: 240, Title: "HUD"}
	s.rt.App.Shortcuts = []model.Shortcut{{ID: "new", Title: "New"}}
	s.rt.App.DesktopMenu = []model.MenuGroup{{Title: "File", Items: []model.MenuItem{{ID: "quit", Role: "quit"}}}}
	s.rt.App.Tray = model.TrayConfig{Tip: "tip", Items: []model.MenuItem{{ID: "show", Title: "Show"}}}
	s.mu.Unlock()

	if w := s.AppWindow(); w.Width != 320 || w.Height != 240 || w.Title != "HUD" {
		t.Errorf("AppWindow = %+v", w)
	}
	var shorts []model.Shortcut
	if err := json.Unmarshal([]byte(s.AppShortcutsJSON()), &shorts); err != nil || len(shorts) != 1 || shorts[0].ID != "new" {
		t.Errorf("AppShortcutsJSON = %s (%v)", s.AppShortcutsJSON(), err)
	}
	if got := s.AppMenuJSON(); !strings.Contains(got, `"title":"File"`) || !strings.Contains(got, `"role":"quit"`) {
		t.Errorf("AppMenuJSON = %s", got)
	}
	if got := s.AppTrayJSON(); !strings.Contains(got, `"tip":"tip"`) || !strings.Contains(got, `"id":"show"`) {
		t.Errorf("AppTrayJSON = %s", got)
	}

	s.SetAppBaseDir("/tmp/wherever")
	s.mu.Lock()
	bd := s.rt.App.BaseDir
	s.mu.Unlock()
	if bd != "/tmp/wherever" {
		t.Errorf("SetAppBaseDir did not stick: %q", bd)
	}
}

// TestSetWindowControlWiresAgent: registering window control also exposes the
// qorm_window MCP tool, letting the agent drive the same callbacks.
func TestSetWindowControlWiresAgent(t *testing.T) {
	s := counterServer(t)
	rec := &winRecorder{}
	s.SetWindowControl(rec.mover, rec.op, rec.open, rec.eval)

	out := string(s.agent.HandleHTTP([]byte(
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"qorm_window","arguments":{"op":"move","x":5,"y":6,"w":7,"h":8}}}`)))
	if strings.Contains(out, `"error"`) {
		t.Fatalf("qorm_window with control registered: %s", out)
	}
	rec.mu.Lock()
	moves := strings.Join(rec.moves, ";")
	rec.mu.Unlock()
	if moves != "main:5,6,7,8" {
		t.Fatalf("agent window move not applied to the registered callback: %q", moves)
	}

	// Without registration the tool errors instead of panicking.
	bare := counterServer(t)
	out = string(bare.agent.HandleHTTP([]byte(
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"qorm_window","arguments":{"op":"move"}}}`)))
	if !strings.Contains(out, "window control unavailable") {
		t.Fatalf("qorm_window without control should error, got %s", out)
	}
}

// TestRecordAgentCallIgnoresForeignInput: only mutating tools/call requests are
// logged; everything else (other methods, garbage, read-only tools) is silent.
func TestRecordAgentCallIgnoresForeignInput(t *testing.T) {
	s := counterServer(t)
	s.recordAgentCall([]byte(`{"method":"tools/list"}`))
	s.recordAgentCall([]byte(`{not json`))
	s.recordAgentCall([]byte(`{"method":"tools/call","params":{"name":"qorm_inspect"}}`))
	s.actMu.Lock()
	n := len(s.activity)
	s.actMu.Unlock()
	if n != 0 {
		t.Fatalf("non-mutating calls must not log activity, got %d entries", n)
	}
}
