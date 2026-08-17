package loader

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCapabilitiesPolicyParsed(t *testing.T) {
	dir := t.TempDir()
	write := func(name, body string) {
		t.Helper()
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("qorm.json", `{
  "type":"app","id":"c","entry":"main",
  "capabilities":{"mode":"manifest","allow":["location"],"deny":["nfc"],"customOps":["myOp"]}
}`)
	write("scenes/main.json", `{"type":"scene","id":"main","root":{"type":"text","id":"t","text":"x"}}`)
	app, err := LoadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if app.Capabilities.Mode != "manifest" {
		t.Fatalf("mode: %q", app.Capabilities.Mode)
	}
	if len(app.Capabilities.Allow) != 1 || app.Capabilities.Allow[0] != "location" {
		t.Fatalf("allow: %v", app.Capabilities.Allow)
	}
	if len(app.Capabilities.Deny) != 1 || app.Capabilities.Deny[0] != "nfc" {
		t.Fatalf("deny: %v", app.Capabilities.Deny)
	}
	if len(app.Capabilities.CustomOps) != 1 || app.Capabilities.CustomOps[0] != "myOp" {
		t.Fatalf("customOps: %v", app.Capabilities.CustomOps)
	}
}
