package render_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/qorm/qorm/internal/loader"
	"github.com/qorm/qorm/internal/render"
	qrt "github.com/qorm/qorm/internal/runtime"
)

// writeApp writes a minimal QORM app (manifest + one scene) into a temp dir.
func writeApp(t *testing.T, manifest, scene string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "scenes"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "qorm.json"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "scenes", "main.json"), []byte(scene), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

// TestComponentPipeline exercises the full load -> render pipeline for an
// app-defined component: its props (incl. the text shorthand) resolve inside the
// template and its children fill the slot.
func TestComponentPipeline(t *testing.T) {
	dir := writeApp(t,
		`{"type":"app","id":"cmp","entry":"main","components":{`+
			`"Card":{"type":"card","children":[`+
			`{"type":"text","id":"title","text":"{{prop.title}}"},`+
			`{"type":"slot"}]}}}`,
		`{"type":"scene","id":"main","root":{"type":"column","children":[`+
			`{"type":"Card","id":"c1","title":"Heading","children":[{"type":"text","id":"body","text":"BODY1"}]},`+
			`{"type":"Card","id":"c2","title":"Other","children":[{"type":"text","id":"body","text":"BODY2"}]}]}}`)
	app, err := loader.LoadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	html := render.Render(qrt.New(app)).HTML

	for _, w := range []string{"Heading", "Other", "BODY1", "BODY2"} {
		if !strings.Contains(html, w) {
			t.Errorf("component pipeline lacks %q:\n%s", w, html)
		}
	}
	// per-instance id suffixing survives the load/render round trip
	for _, w := range []string{`id="title_c1"`, `id="title_c2"`, `id="body_c1"`, `id="body_c2"`} {
		if !strings.Contains(html, w) {
			t.Errorf("component ids should be suffixed per instance, lacks %q:\n%s", w, html)
		}
	}
	if strings.Contains(html, "data-qorm-unknown") {
		t.Errorf("component instance rendered as unknown:\n%s", html)
	}
}

// TestUnknownWidgetReportedThroughLoader guards that a typo'd widget type in a
// loaded scene is surfaced via Result.Unknown (the self-verify surface) while
// still rendering a container.
func TestUnknownWidgetReportedThroughLoader(t *testing.T) {
	dir := writeApp(t,
		`{"type":"app","id":"unk","entry":"main"}`,
		`{"type":"scene","id":"main","root":{"type":"colunm","id":"oops","children":[{"type":"text","text":"KEPT"}]}}`)
	app, err := loader.LoadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	res := render.Render(qrt.New(app))
	if !strings.Contains(res.HTML, `data-qorm-unknown="colunm"`) {
		t.Errorf("typo'd widget should be tagged unknown:\n%s", res.HTML)
	}
	if !strings.Contains(res.HTML, "KEPT") {
		t.Errorf("unknown widget children should still render:\n%s", res.HTML)
	}
	found := false
	for _, u := range res.Unknown {
		if u == "colunm" {
			found = true
		}
	}
	if !found {
		t.Errorf("Result.Unknown should report 'colunm', got %v", res.Unknown)
	}
}
