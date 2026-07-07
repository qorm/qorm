package render

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/qorm/qorm/internal/loader"
	qrt "github.com/qorm/qorm/internal/runtime"
)

// TestUniversalAnimation checks that the `animation` prop is a cross-cutting
// property: it wraps both a built-in widget and a component instance in the named
// effect, not just the `motion` widget.
func TestUniversalAnimation(t *testing.T) {
	dir := t.TempDir()
	must := func(p, s string) {
		if err := os.WriteFile(filepath.Join(dir, p), []byte(s), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	os.MkdirAll(filepath.Join(dir, "scenes"), 0o755)
	must("qorm.json", `{"type":"app","id":"anim","entry":"main","components":{"Chip":{"type":"card","children":[{"type":"text","text":"{{prop.text}}"}]}}}`)
	must("scenes/main.json", `{"type":"scene","id":"main","root":{"type":"column","children":[
		{"type":"Chip","animation":"pop","text":"CHIPTEXT"},
		{"type":"card","animation":"fadeup","children":[{"type":"text","text":"y"}]}
	]}}`)
	app, err := loader.LoadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if app.Components["Chip"] == nil {
		t.Fatal("Chip component was not loaded from the manifest")
	}
	html := Render(qrt.New(app)).HTML
	if !strings.Contains(html, "animation:qa-pop") {
		t.Error("component instance with animation:pop was not wrapped in qa-pop")
	}
	if !strings.Contains(html, "animation:qa-fadeup") {
		t.Error("widget with animation:fadeup was not wrapped in qa-fadeup")
	}
	// The wrapper must contain the *instantiated* component, not an unknown node.
	if !strings.Contains(html, "CHIPTEXT") {
		t.Error("animated component did not render its content (component not instantiated)")
	}
	if strings.Contains(html, "data-qorm-unknown") {
		t.Error("animated node rendered as an unknown widget")
	}
}

// TestNestedStyleBinding guards that {{ … }} bindings inside a nested style
// object (e.g. margin:{left}) resolve, not only top-level style values.
func TestNestedStyleBinding(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "scenes"), 0o755)
	if err := os.WriteFile(filepath.Join(dir, "qorm.json"),
		[]byte(`{"type":"app","id":"m","entry":"main","globalState":{"schema":{"on":"boolean"},"initial":{"on":true}}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "scenes", "main.json"),
		[]byte(`{"type":"scene","id":"main","root":{"type":"box","style":{"width":40,"margin":{"left":"{{ state.on ? 240 : 0 }}"}}}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	app, err := loader.LoadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	html := Render(qrt.New(app)).HTML
	if !strings.Contains(html, "240px") {
		t.Error("nested style binding margin.left did not resolve (expected 240px)")
	}
}
