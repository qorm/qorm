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

// TestAnimatedContainerLayout guards that AnimatedContainer honours layout
// align/justify (via containerCSS) so children can be centred.
func TestAnimatedContainerLayout(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "scenes"), 0o755)
	os.WriteFile(filepath.Join(dir, "qorm.json"), []byte(`{"type":"app","id":"a","entry":"main"}`), 0o644)
	os.WriteFile(filepath.Join(dir, "scenes", "main.json"),
		[]byte(`{"type":"scene","id":"main","root":{"type":"animatedcontainer","layout":{"align":"center","justify":"center"},"children":[{"type":"text","text":"x"}]}}`), 0o644)
	app, err := loader.LoadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	html := Render(qrt.New(app)).HTML
	if !strings.Contains(html, "align-items:center") || !strings.Contains(html, "justify-content:center") {
		t.Error("animatedcontainer did not apply layout align/justify")
	}
}

// TestNavigation checks the navigate action step: dispatching it switches the
// runtime's current scene, and navigate-back returns to the previous one.
func TestNavigation(t *testing.T) {
	dir := t.TempDir()
	w := func(p, s string) {
		if err := os.WriteFile(filepath.Join(dir, p), []byte(s), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	os.MkdirAll(filepath.Join(dir, "scenes"), 0o755)
	os.MkdirAll(filepath.Join(dir, "actions"), 0o755)
	w("qorm.json", `{"type":"app","id":"nav","entry":"home"}`)
	w("scenes/home.json", `{"type":"scene","id":"home","root":{"type":"text","text":"HOME"}}`)
	w("scenes/details.json", `{"type":"scene","id":"details","root":{"type":"text","text":"DETAILS"}}`)
	w("actions/go.json", `{"type":"action","id":"go","steps":[{"type":"navigate","to":"details"}]}`)
	w("actions/back.json", `{"type":"action","id":"back","steps":[{"type":"navigate","back":true}]}`)
	app, err := loader.LoadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	rt := qrt.New(app)
	if !strings.Contains(Render(rt).HTML, "HOME") {
		t.Fatal("entry scene should be home")
	}
	rt.Dispatch("go", nil)
	if rt.CurrentScene() != "details" {
		t.Fatalf("navigate: current scene = %q, want details", rt.CurrentScene())
	}
	if !strings.Contains(RenderScene(rt, rt.CurrentScene()).HTML, "DETAILS") {
		t.Error("details scene did not render after navigate")
	}
	rt.Dispatch("back", nil)
	if s := rt.CurrentScene(); s != "" && s != "home" {
		t.Fatalf("navigate back: current scene = %q, want entry", s)
	}
}

// TestReorder checks drag-to-reorder end to end: state.move relocates a list
// element, and a reorderable list wires the qormReorder client helper.
func TestReorder(t *testing.T) {
	dir := t.TempDir()
	w := func(p, s string) {
		if err := os.WriteFile(filepath.Join(dir, p), []byte(s), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	os.MkdirAll(filepath.Join(dir, "scenes"), 0o755)
	os.MkdirAll(filepath.Join(dir, "actions"), 0o755)
	w("qorm.json", `{"type":"app","id":"re","entry":"main","globalState":{"schema":{"items":"array"},"initial":{"items":[{"label":"A"},{"label":"B"},{"label":"C"}]}}}`)
	w("actions/onReorder.json", `{"type":"action","id":"onReorder","steps":[{"type":"state.move","path":"items","from":"{{_reorderFrom}}","to":"{{_reorderTo}}"}]}`)
	w("scenes/main.json", `{"type":"scene","id":"main","root":{"type":"list","id":"L","reorderable":true,"onReorder":{"type":"invoke","name":"onReorder"},"data":"{{state.items}}","renderItem":{"type":"text","text":"{{item.label}}"}}}`)
	app, err := loader.LoadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	rt := qrt.New(app)
	if !strings.Contains(Render(rt).HTML, "qormReorder(") {
		t.Error("reorderable list did not wire the qormReorder client helper")
	}
	rt.Dispatch("onReorder", map[string]any{"_reorderFrom": 0.0, "_reorderTo": 2.0})
	items, _ := rt.State["items"].([]any)
	if len(items) != 3 || items[2].(map[string]any)["label"] != "A" {
		t.Errorf("state.move did not relocate element: %v", items)
	}
}
