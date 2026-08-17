package server

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/qorm/qorm/internal/loader"
	qrt "github.com/qorm/qorm/internal/runtime"
)

func TestPageInjectsCapabilityPolicy(t *testing.T) {
	app, err := loader.LoadDir(filepath.Join("..", "..", "examples", "counter"))
	if err != nil {
		t.Fatal(err)
	}
	html := Page(qrt.New(app), "<div>hi</div>", 0)
	if strings.Contains(html, "__QORM_CAP_POLICY__") {
		t.Fatal("cap policy placeholder should be substituted")
	}
	if !strings.Contains(html, "window.__qormCapPolicy=null") {
		t.Fatalf("legacy counter should inject null cap policy, got snippet missing")
	}
}

func TestPageInjectsEnforcedCapabilityPolicy(t *testing.T) {
	app, err := loader.LoadDir(filepath.Join("..", "..", "examples", "hardware"))
	if err != nil {
		t.Fatal(err)
	}
	app.RequiredCapabilities = []string{"location"}
	html := Page(qrt.New(app), "<div>hi</div>", 0)
	if !strings.Contains(html, `"enforce":true`) {
		t.Fatalf("hardware app with requirements should enforce gate, html tail:\n%s", html[len(html)-400:])
	}
	if !strings.Contains(html, "qormCapAllowed") {
		t.Fatal("app.js should define qormCapAllowed")
	}
}
